#![cfg(target_os = "linux")]

use std::fs;
use std::path::{Path, PathBuf};
use std::process::{Command, ExitStatus, Stdio};
use std::thread;
use std::time::{Duration, Instant};

use pito_tui::ratatui::Frame;
use pito_tui::ratatui::layout::Rect;
use pito_tui::ratatui::style::Color;
use pito_tui::{Cx, Palette, Screen, Tui};
use rustix::process::{Pid, Signal, kill_process, test_kill_process};

const APP: &str = "PITO_TUI_SIGNALS_APP";
const NAME: &str = "a_hangup_or_a_termination_stops_every_command_then_ends_the_app_by_that_signal";

struct Sims(PathBuf);

impl Screen<()> for Sims {
    fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}

    fn start(&mut self, cx: &mut Cx<'_, ()>) {
        for id in [1, 2] {
            let mut command = Command::new("sh");
            command
                .args(["-c", r#"echo $$ >> "$0"; exec sleep 30"#])
                .arg(self.0.join("commands"));
            cx.run(id, "sim", command);
        }
        cx.forget(2);
        fs::write(self.0.join("app"), std::process::id().to_string()).unwrap();
    }
}

fn pids(path: &Path, count: usize) -> Option<Vec<Pid>> {
    let text = fs::read_to_string(path).ok()?;
    let pids: Vec<Pid> = text
        .lines()
        .filter_map(|line| Pid::from_raw(line.trim().parse().ok()?))
        .collect();
    (pids.len() == count).then_some(pids)
}

fn gone(pid: Pid) -> bool {
    let stat =
        fs::read_to_string(format!("/proc/{}/stat", pid.as_raw_nonzero())).unwrap_or_default();
    let state = stat
        .rsplit_once(") ")
        .and_then(|(_, rest)| rest.chars().next());
    test_kill_process(pid).is_err() || matches!(state, None | Some('Z'))
}

fn until<T>(what: &str, mut ready: impl FnMut() -> Option<T>) -> T {
    let limit = Instant::now() + Duration::from_secs(10);
    loop {
        if let Some(value) = ready() {
            return value;
        }
        assert!(Instant::now() < limit, "no {what} within 10 s");
        thread::sleep(Duration::from_millis(10));
    }
}

#[test]
fn a_hangup_or_a_termination_stops_every_command_then_ends_the_app_by_that_signal() {
    if let Some(dir) = std::env::var_os(APP) {
        Tui::new("signals", "0", Color::Blue)
            .screen("Sims", Sims(PathBuf::from(dir)))
            .run()
            .unwrap();
        return;
    }
    let exe = std::env::current_exe().unwrap();
    let line = format!(
        "stty cols 80 rows 24; exec '{}' --exact {NAME} --nocapture",
        exe.display()
    );
    for signal in [
        None,
        Some(Signal::HUP),
        Some(Signal::TERM),
        Some(Signal::INT),
    ] {
        let case = signal.map_or(0, Signal::as_raw);
        let dir = Path::new(env!("CARGO_TARGET_TMPDIR")).join(format!("signal-{case}"));
        let _ = fs::remove_dir_all(&dir);
        fs::create_dir_all(&dir).unwrap();
        let mut script = Command::new("script")
            .args(["-qec", &line, "/dev/null"])
            .env(APP, &dir)
            .env("SHELL", "/bin/sh")
            .stdin(Stdio::piped())
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .spawn()
            .unwrap();
        let app = until("app", || Some(pids(&dir.join("app"), 1)?[0]));
        let commands = until("commands", || pids(&dir.join("commands"), 2));
        match signal {
            Some(signal) => kill_process(app, signal).unwrap(),
            None => script.kill().unwrap(),
        }
        let status: Option<ExitStatus> = {
            let limit = Instant::now() + Duration::from_secs(10);
            loop {
                if let Some(status) = script.try_wait().unwrap() {
                    break Some(status);
                }
                if Instant::now() >= limit {
                    let _ = script.kill();
                    break None;
                }
                thread::sleep(Duration::from_millis(10));
            }
        };
        if signal.is_none() {
            until("end of the app", || gone(app).then_some(()));
        }
        let left: Vec<Pid> = commands.into_iter().filter(|pid| !gone(*pid)).collect();
        for pid in &left {
            let _ = kill_process(*pid, Signal::KILL);
        }
        assert!(left.is_empty(), "{signal:?} left {left:?} running");
        if let Some(signal) = signal {
            let code = status.and_then(|status| status.code());
            assert_eq!(code, Some(128 + signal.as_raw()), "{signal:?}");
        }
    }
}
