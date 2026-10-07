use std::io::{BufRead, BufReader, Read};
use std::process::{Command, Stdio};
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::mpsc::Sender;
use std::sync::{Arc, Mutex};
use std::thread;
use std::time::{Duration, Instant};

use ratatui::text::Line;
use serde_json::Value;

use crate::activity::{Activity, State};
use crate::wake::Wake;

const POST: Duration = Duration::from_millis(100);

#[derive(Debug, Clone, PartialEq)]
#[non_exhaustive]
pub struct Progress {
    pub state: String,
    pub stage: Option<String>,
    pub fraction: Option<f64>,
    pub message: Option<String>,
    pub ok: Option<bool>,
}

pub fn read(line: &str) -> Option<Progress> {
    let value: Value = serde_json::from_str(line.trim()).ok()?;
    let object = value.as_object()?;
    if object.get("v").is_some_and(|v| v.as_u64() != Some(1)) {
        return None;
    }
    let text = |key: &str| {
        object
            .get(key)
            .and_then(Value::as_str)
            .filter(|text| !text.is_empty())
            .map(str::to_string)
    };
    Some(Progress {
        state: text("state")?,
        stage: text("stage"),
        fraction: object.get("fraction").and_then(Value::as_f64),
        message: text("msg"),
        ok: object.get("ok").and_then(Value::as_bool),
    })
}

fn note(progress: &Progress) -> String {
    let mut parts: Vec<String> = Vec::new();
    parts.extend(progress.stage.clone());
    parts.push(progress.state.clone());
    if let Some(fraction) = progress.fraction {
        parts.push(crate::progress::share(fraction, false));
    }
    parts.extend(progress.message.clone());
    parts.join("  ")
}

pub(crate) fn follow(activity: &mut Activity, progress: &Progress, now: Instant) {
    if let Some(text) = progress.message.clone().or_else(|| progress.stage.clone()) {
        activity.status = text;
    }
    if progress.state == "end" {
        activity.state = if progress.ok == Some(true) {
            State::Done
        } else {
            State::Failed
        };
        activity.progress = None;
        activity.ended = Some(now);
        return;
    }
    activity.state = State::Running;
    activity.progress = progress.fraction;
}

pub(crate) struct Run {
    pub(crate) activity: Activity,
    pub(crate) command: Command,
}

struct Shared {
    activity: Activity,
    lines: Vec<Line<'static>>,
    dirty: bool,
    done: bool,
}

impl Shared {
    fn push(&mut self, line: String) {
        self.lines.push(Line::from(line));
        self.dirty = true;
    }

    fn post(&mut self) -> Activity {
        self.dirty = false;
        let mut activity = self.activity.clone();
        activity.detail = Arc::new(self.lines.clone());
        activity
    }
}

fn lock(shared: &Mutex<Shared>) -> std::sync::MutexGuard<'_, Shared> {
    shared
        .lock()
        .unwrap_or_else(|poisoned| poisoned.into_inner())
}

fn drain(stream: impl Read, shared: &Mutex<Shared>, progress: bool) {
    for line in BufReader::new(stream).lines().map_while(Result::ok) {
        let mut state = lock(shared);
        match read(&line).filter(|_| progress) {
            Some(read) => {
                let text = note(&read);
                follow(&mut state.activity, &read, Instant::now());
                state.push(text);
            }
            None => state.push(line),
        }
    }
}

pub(crate) fn spawn<E: Send + 'static>(
    run: Run,
    sender: Sender<Wake<E>>,
    running: Arc<AtomicUsize>,
) {
    let Run {
        activity,
        mut command,
    } = run;
    command
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    let shared = Arc::new(Mutex::new(Shared {
        activity,
        lines: Vec::new(),
        dirty: true,
        done: false,
    }));
    let mut child = match command.spawn() {
        Ok(child) => child,
        Err(error) => {
            let mut state = lock(&shared);
            state.activity.state = State::Failed;
            state.activity.status = error.to_string();
            state.activity.ended = Some(Instant::now());
            let _ = sender.send(Wake::Activity(state.post()));
            return;
        }
    };
    running.fetch_add(1, Ordering::SeqCst);
    let stdout = child.stdout.take();
    let stderr = child.stderr.take();
    let errors = Arc::clone(&shared);
    let quiet = thread::spawn(move || {
        if let Some(stderr) = stderr {
            drain(stderr, &errors, false);
        }
    });
    let reader = Arc::clone(&shared);
    thread::spawn(move || {
        if let Some(stdout) = stdout {
            drain(stdout, &reader, true);
        }
        let _ = quiet.join();
        let status = child.wait();
        let mut state = lock(&reader);
        if !state.activity.state.finished() {
            let ok = status.as_ref().is_ok_and(|status| status.success());
            state.activity.state = if ok { State::Done } else { State::Failed };
            state.activity.progress = None;
            state.activity.ended = Some(Instant::now());
        }
        state.done = true;
        state.dirty = true;
    });
    thread::spawn(move || {
        loop {
            thread::sleep(POST);
            let mut state = lock(&shared);
            if state.dirty {
                let activity = state.post();
                if sender.send(Wake::Activity(activity)).is_err() {
                    break;
                }
            }
            if state.done {
                break;
            }
        }
        running.fetch_sub(1, Ordering::SeqCst);
    });
}

#[cfg(test)]
mod tests {
    use super::*;

    const START: &str = r#"{"v":1,"op":"update","app":"dispatch","ref":"v0.9.1","stage":"clone","state":"start","fraction":null,"msg":"cloning pito-dispatch at v0.9.1","at":1790950000415}"#;
    const STEP: &str = r#"{"v":1,"op":"update","app":"dispatch","ref":null,"stage":"build","state":"progress","fraction":0.5,"msg":null,"at":1790950000900,"id":"7"}"#;
    const END: &str = r#"{"v":1,"op":"update","app":"dispatch","ref":"v0.9.1","state":"end","ok":true,"msg":"pdispatch 0.9.1 is installed","at":1790950057026,"store":{"kept":3}}"#;
    const FAILED: &str = r#"{"v":1,"op":"update","app":"dispatch","ref":null,"state":"end","ok":false,"msg":"","at":1790950057026}"#;

    #[test]
    fn progress_lines_carry_an_activity_from_its_start_to_its_end() {
        let now = Instant::now();
        let mut activity = Activity::new(1, "update", now);
        follow(&mut activity, &read(START).unwrap(), now);
        assert_eq!(activity.status, "cloning pito-dispatch at v0.9.1");
        assert_eq!(activity.progress, None);
        follow(&mut activity, &read(STEP).unwrap(), now);
        assert_eq!(
            (activity.state, activity.progress),
            (State::Running, Some(0.5))
        );
        assert_eq!(activity.status, "build");
        follow(&mut activity, &read(END).unwrap(), now);
        assert_eq!(activity.state, State::Done);
        assert_eq!(activity.status, "pdispatch 0.9.1 is installed");
        assert_eq!(activity.ended, Some(now));

        let mut failed = Activity::new(2, "update", now);
        follow(&mut failed, &read(FAILED).unwrap(), now);
        assert_eq!(failed.state, State::Failed);
        assert!(read(r#"{"v":2,"state":"start"}"#).is_none());
        assert!(read("cloning…").is_none());
    }
}
