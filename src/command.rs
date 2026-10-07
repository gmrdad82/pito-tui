use std::io::{self, BufRead, BufReader, Read};
use std::mem;
use std::process::{Child, Command, ExitStatus, Stdio};
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::mpsc::Sender;
use std::sync::{Arc, Mutex, MutexGuard};
use std::thread;
use std::time::{Duration, Instant};

use ratatui::text::Line;
use serde_json::Value;

use crate::activity::{Activity, State};
use crate::wake::Wake;

pub const GRACE: Duration = Duration::from_secs(2);
const POST: Duration = Duration::from_millis(100);
const REAP: Duration = Duration::from_millis(1);

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
#[non_exhaustive]
pub enum Stream {
    #[default]
    Stdout,
    Stderr,
    Both,
}

impl Stream {
    fn stdout(self) -> bool {
        matches!(self, Stream::Stdout | Stream::Both)
    }

    fn stderr(self) -> bool {
        matches!(self, Stream::Stderr | Stream::Both)
    }
}

#[derive(Debug, Clone, PartialEq)]
#[non_exhaustive]
pub struct Progress {
    pub state: String,
    pub stage: Option<String>,
    pub fraction: Option<f64>,
    pub message: Option<String>,
    pub ok: Option<bool>,
}

#[derive(Debug, Clone, PartialEq)]
#[non_exhaustive]
pub enum Heard {
    #[non_exhaustive]
    Line {
        stream: Stream,
        text: String,
        progress: Option<Progress>,
    },
    Exit(Exit),
}

#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum Exit {
    Code(i32),
    Signal(i32),
    Stopped,
    Error(String),
}

impl Exit {
    pub fn state(&self) -> State {
        match self {
            Exit::Code(0) => State::Done,
            Exit::Stopped => State::Stopped,
            _ => State::Failed,
        }
    }

    fn of(status: io::Result<ExitStatus>) -> Self {
        match status {
            Ok(status) => status
                .code()
                .map_or_else(|| Exit::Signal(killed(status)), Exit::Code),
            Err(error) => Exit::Error(error.to_string()),
        }
    }
}

#[cfg(unix)]
fn killed(status: ExitStatus) -> i32 {
    std::os::unix::process::ExitStatusExt::signal(&status).unwrap_or_default()
}

#[cfg(not(unix))]
fn killed(_status: ExitStatus) -> i32 {
    0
}

#[derive(Debug, Clone)]
pub struct Report {
    pub(crate) activity: Activity,
    pub(crate) heard: Vec<Heard>,
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
    if activity.state.finished() {
        return;
    }
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

pub(crate) fn halt(activity: &mut Activity, now: Instant) {
    activity.state = State::Stopped;
    activity.progress = None;
    activity.ended = Some(now);
}

pub(crate) struct Run {
    pub(crate) screen: usize,
    pub(crate) activity: Activity,
    pub(crate) command: Command,
    pub(crate) progress: Stream,
    pub(crate) keep: bool,
}

pub(crate) struct Handle {
    pub(crate) id: u64,
    child: Arc<Mutex<Child>>,
    shared: Arc<Mutex<Shared>>,
}

impl Handle {
    pub(crate) fn over(&self) -> bool {
        lock(&self.shared).done
    }

    pub(crate) fn stop(&self, now: Instant) -> Option<Activity> {
        let activity = self.stopping(now)?;
        signal(&self.child, false);
        let child = Arc::clone(&self.child);
        thread::spawn(move || {
            thread::sleep(GRACE);
            signal(&child, true);
        });
        Some(activity)
    }

    fn stopping(&self, now: Instant) -> Option<Activity> {
        let mut state = lock(&self.shared);
        if state.activity.state.finished() {
            return None;
        }
        halt(&mut state.activity, now);
        state.stopped = true;
        state.dirty = true;
        Some(state.snapshot())
    }

    fn alive(&self) -> bool {
        matches!(lock(&self.child).try_wait(), Ok(None))
    }

    pub(crate) fn forget(&self) {
        let mut state = lock(&self.shared);
        state.keep = false;
        state.lines = Vec::new();
    }
}

pub(crate) fn end(handles: &[Handle], now: Instant) -> Vec<Activity> {
    let stopped = handles
        .iter()
        .filter_map(|handle| handle.stopping(now))
        .collect();
    for handle in handles {
        signal(&handle.child, false);
    }
    let limit = Instant::now() + GRACE;
    let mut pause = REAP;
    while handles.iter().any(Handle::alive) {
        let left = limit.saturating_duration_since(Instant::now());
        if left.is_zero() {
            break;
        }
        thread::sleep(pause.min(left));
        pause = (pause * 2).min(POST);
    }
    for handle in handles {
        signal(&handle.child, true);
    }
    stopped
}

fn signal(child: &Mutex<Child>, hard: bool) {
    let mut child = lock(child);
    if !matches!(child.try_wait(), Ok(None)) {
        return;
    }
    #[cfg(unix)]
    {
        use rustix::process::{Pid, Signal, kill_process_group};
        let signal = if hard { Signal::KILL } else { Signal::TERM };
        let _ = kill_process_group(Pid::from_child(&child), signal);
    }
    #[cfg(not(unix))]
    {
        let _ = hard;
        let _ = child.kill();
    }
}

fn reap(child: &Mutex<Child>) -> io::Result<ExitStatus> {
    let mut pause = REAP;
    loop {
        if let Some(status) = lock(child).try_wait()? {
            return Ok(status);
        }
        thread::sleep(pause);
        pause = (pause * 2).min(POST);
    }
}

struct Shared {
    activity: Activity,
    lines: Vec<Line<'static>>,
    heard: Vec<Heard>,
    keep: bool,
    stopped: bool,
    dirty: bool,
    done: bool,
}

impl Shared {
    fn push(&mut self, line: impl FnOnce() -> String) {
        if self.keep {
            self.lines.push(Line::from(line()));
        }
        self.dirty = true;
    }

    fn snapshot(&self) -> Activity {
        let mut activity = self.activity.clone();
        activity.detail = Arc::new(self.lines.clone());
        activity
    }

    fn post(&mut self) -> Report {
        self.dirty = false;
        Report {
            activity: self.snapshot(),
            heard: mem::take(&mut self.heard),
        }
    }

    fn end(&mut self, exit: Exit) {
        self.heard.push(Heard::Exit(exit));
        self.done = true;
        self.dirty = true;
    }
}

fn lock<T>(mutex: &Mutex<T>) -> MutexGuard<'_, T> {
    mutex
        .lock()
        .unwrap_or_else(|poisoned| poisoned.into_inner())
}

fn drain(stream: impl Read, shared: &Mutex<Shared>, from: Stream, progress: bool) {
    let mut reader = BufReader::new(stream);
    let mut bytes = Vec::new();
    loop {
        bytes.clear();
        match reader.read_until(b'\n', &mut bytes) {
            Ok(0) | Err(_) => return,
            Ok(_) => {}
        }
        let line = bytes.strip_suffix(b"\n").unwrap_or(&bytes);
        let line = line.strip_suffix(b"\r").unwrap_or(line);
        let line = String::from_utf8_lossy(line).into_owned();
        let parsed = read(&line).filter(|_| progress);
        let mut state = lock(shared);
        match &parsed {
            Some(parsed) => {
                follow(&mut state.activity, parsed, Instant::now());
                state.push(|| note(parsed));
            }
            None => state.push(|| line.clone()),
        }
        state.heard.push(Heard::Line {
            stream: from,
            text: line,
            progress: parsed,
        });
    }
}

pub(crate) fn spawn<E: Send + 'static>(
    run: Run,
    sender: Sender<Wake<E>>,
    running: Arc<AtomicUsize>,
) -> Option<Handle> {
    let Run {
        screen,
        activity,
        mut command,
        progress,
        keep,
    } = run;
    command
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    #[cfg(unix)]
    std::os::unix::process::CommandExt::process_group(&mut command, 0);
    let id = activity.id;
    let shared = Arc::new(Mutex::new(Shared {
        activity,
        lines: Vec::new(),
        heard: Vec::new(),
        keep,
        stopped: false,
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
            state.end(Exit::Error(error.to_string()));
            let _ = sender.send(Wake::Command(screen, state.post()));
            return None;
        }
    };
    running.fetch_add(1, Ordering::SeqCst);
    let stdout = child.stdout.take();
    let stderr = child.stderr.take();
    let child = Arc::new(Mutex::new(child));
    let errors = Arc::clone(&shared);
    let quiet = thread::spawn(move || {
        if let Some(stderr) = stderr {
            drain(stderr, &errors, Stream::Stderr, progress.stderr());
        }
    });
    let reader = Arc::clone(&shared);
    let reaped = Arc::clone(&child);
    thread::spawn(move || {
        if let Some(stdout) = stdout {
            drain(stdout, &reader, Stream::Stdout, progress.stdout());
        }
        let _ = quiet.join();
        let status = reap(&reaped);
        let mut state = lock(&reader);
        if !state.activity.state.finished() {
            let ok = status.as_ref().is_ok_and(|status| status.success());
            state.activity.state = if ok { State::Done } else { State::Failed };
            state.activity.progress = None;
            state.activity.ended = Some(Instant::now());
        }
        let exit = if state.stopped {
            Exit::Stopped
        } else {
            Exit::of(status)
        };
        state.end(exit);
    });
    let handle = Handle {
        id,
        child,
        shared: Arc::clone(&shared),
    };
    thread::spawn(move || {
        loop {
            thread::sleep(POST);
            let mut state = lock(&shared);
            if state.dirty && sender.send(Wake::Command(screen, state.post())).is_err() {
                break;
            }
            if state.done {
                break;
            }
        }
        running.fetch_sub(1, Ordering::SeqCst);
    });
    Some(handle)
}

#[cfg(test)]
mod tests {
    #[cfg(unix)]
    use std::sync::mpsc;

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

    #[cfg(unix)]
    fn started(
        script: &str,
        progress: Stream,
    ) -> (Handle, mpsc::Receiver<Wake<()>>, Arc<AtomicUsize>) {
        let mut command = Command::new("sh");
        command.args(["-c", script]);
        let run = Run {
            screen: 0,
            activity: Activity::new(1, "sim", Instant::now()),
            command,
            progress,
            keep: true,
        };
        let (sender, inbox) = mpsc::channel();
        let running = Arc::new(AtomicUsize::new(0));
        let handle = spawn(run, sender, Arc::clone(&running)).unwrap();
        (handle, inbox, running)
    }

    #[cfg(unix)]
    fn last(
        inbox: &mpsc::Receiver<Wake<()>>,
        until: impl Fn(&Activity) -> bool,
    ) -> (Activity, Vec<Heard>) {
        let limit = Instant::now() + Duration::from_secs(10);
        let mut seen = None;
        let mut heard = Vec::new();
        while let Ok(wake) = inbox.recv_timeout(limit.saturating_duration_since(Instant::now())) {
            if let Wake::Command(_, report) = wake {
                heard.extend(report.heard);
                let done = until(&report.activity);
                seen = Some(report.activity);
                if done {
                    break;
                }
            }
        }
        (seen.unwrap(), heard)
    }

    #[cfg(unix)]
    #[test]
    fn progress_lines_are_followed_on_the_stream_the_app_picks() {
        let script = format!("echo '{FAILED}' >&2; echo plain");
        let (_, inbox, _) = started(&script, Stream::Stderr);
        let (ended, _) = last(&inbox, |_| false);
        assert_eq!(ended.state, State::Failed);
        assert!(ended.detail.iter().any(|line| line.to_string() == "plain"));
        let (_, inbox, _) = started(&script, Stream::Stdout);
        let (ended, _) = last(&inbox, |_| false);
        assert_eq!(ended.state, State::Done);
        assert!(ended.detail.iter().any(|line| line.to_string() == FAILED));
    }

    #[cfg(unix)]
    #[test]
    fn a_stopped_command_ends_at_once_as_stopped_and_keeps_its_detail() {
        let script = format!("echo '{STEP}'; echo started; sleep 30; echo '{END}'");
        let (handle, inbox, running) = started(&script, Stream::Stdout);
        let said = |activity: &Activity| {
            activity
                .detail
                .iter()
                .any(|line| line.to_string() == "started")
        };
        assert!(said(&last(&inbox, said).0));
        let asked = Instant::now();
        let stopped = handle.stop(asked).unwrap();
        assert_eq!(
            (stopped.state, stopped.ended, stopped.progress),
            (State::Stopped, Some(asked), None)
        );
        assert!(handle.stop(asked).is_none());
        let (ended, heard) = last(&inbox, |_| false);
        assert!(asked.elapsed() < GRACE);
        assert_eq!(ended.state, State::Stopped);
        assert_eq!(heard.last(), Some(&Heard::Exit(Exit::Stopped)));
        assert!(said(&ended));
        assert!(handle.over());
        assert_eq!(running.load(Ordering::SeqCst), 0);
    }

    #[cfg(unix)]
    #[test]
    fn the_end_of_the_loop_stops_every_command_and_kills_one_that_ignores_the_signal() {
        let said = |activity: &Activity| {
            activity
                .detail
                .iter()
                .any(|line| line.to_string() == "started")
        };
        for (script, stubborn) in [
            ("echo started; sleep 30", false),
            ("trap '' TERM; echo started; sleep 30", true),
        ] {
            let (handle, inbox, running) = started(script, Stream::Stdout);
            assert!(said(&last(&inbox, said).0));
            let asked = Instant::now();
            let stopped = end(std::slice::from_ref(&handle), asked);
            assert_eq!(stopped.len(), 1);
            assert_eq!(asked.elapsed() >= GRACE, stubborn);
            let (ended, heard) = last(&inbox, |_| false);
            assert!(asked.elapsed() < GRACE + Duration::from_secs(1));
            assert_eq!(ended.state, State::Stopped);
            assert_eq!(heard.last(), Some(&Heard::Exit(Exit::Stopped)));
            assert!(handle.over());
            assert_eq!(running.load(Ordering::SeqCst), 0);
        }
    }

    #[cfg(unix)]
    #[test]
    fn a_forgotten_command_keeps_no_lines_and_its_screen_still_hears_them() {
        let (handle, inbox, _) = started("echo one; sleep 0.3; echo two", Stream::Stdout);
        let one =
            |activity: &Activity| activity.detail.iter().any(|line| line.to_string() == "one");
        let (_, mut heard) = last(&inbox, one);
        handle.forget();
        let (ended, rest) = last(&inbox, |_| false);
        heard.extend(rest);
        assert!(ended.detail.is_empty());
        let texts: Vec<&str> = heard
            .iter()
            .filter_map(|heard| match heard {
                Heard::Line { text, .. } => Some(text.as_str()),
                _ => None,
            })
            .collect();
        assert_eq!(texts, ["one", "two"]);
    }

    #[cfg(unix)]
    #[test]
    fn the_screen_hears_every_line_with_its_stream_then_the_exit() {
        let script = format!("echo '{STEP}' >&2; printf 'one\\377\\n'; echo two; exit 3");
        let (_, inbox, _) = started(&script, Stream::Stderr);
        let (_, heard) = last(&inbox, |_| false);
        let lines = |from: Stream| -> Vec<(String, bool)> {
            heard
                .iter()
                .filter_map(|heard| match heard {
                    Heard::Line {
                        stream,
                        text,
                        progress,
                    } if *stream == from => Some((text.clone(), progress.is_some())),
                    _ => None,
                })
                .collect()
        };
        assert_eq!(
            lines(Stream::Stdout),
            [
                ("one\u{FFFD}".to_string(), false),
                ("two".to_string(), false)
            ]
        );
        assert_eq!(lines(Stream::Stderr), [(STEP.to_string(), true)]);
        assert_eq!(heard.last(), Some(&Heard::Exit(Exit::Code(3))));
    }
}
