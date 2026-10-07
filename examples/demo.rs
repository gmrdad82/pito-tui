use std::cmp::Ordering;
use std::env;
use std::path::Path;
use std::process::{Command, ExitCode};
use std::sync::Arc;
use std::thread;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use pito_tui::activity::State;
use pito_tui::capture::{self, Script, Walk};
use pito_tui::clock::{self, Units};
use pito_tui::crossterm::event::{KeyCode, KeyEvent};
use pito_tui::footer::{Hint, InputBar, Key, Tone};
use pito_tui::header::Section;
use pito_tui::list::{Cell, Column, Row};
use pito_tui::ratatui::{
    Frame,
    layout::Rect,
    style::{Color, Modifier, Style},
    text::Line,
};
use pito_tui::{
    Activities, Activity, Cx, Filter, Job, Log, Palette, Phase, Pick, Screen, Tui, Turn, Words,
    dump, log, matches, message,
};

const NAME: &str = "demo";
const VERSION: &str = env!("CARGO_PKG_VERSION");
const ACCENTS: [Color; 3] = [
    Color::Rgb(0x5b, 0x8c, 0xff),
    Color::Rgb(0xff, 0xcf, 0x5c),
    Color::Rgb(0xd1, 0x3c, 0x9c),
];
const LOAD: Duration = Duration::from_millis(900);
const SIZE: (u16, u16) = (120, 34);
const JOB: Duration = Duration::from_secs(5);
const TICK: Duration = Duration::from_millis(100);
const STAGES: [&str; 5] = ["fetch", "compile", "link", "test", "package"];
const LINES: usize = 20_000;
const PROGRESS: &str = "--progress-demo";
const STEP: Duration = Duration::from_millis(200);

const STEPS: [Step; 5] = [
    Step::new(
        "Replace this screen",
        "Home and Next are two Screen types; swap them for your own",
    ),
    Step::new(
        "Set your accent",
        "Tui::new takes one colour, or Palette sets every token",
    ),
    Step::new(
        "Add your first command",
        "a key in Screen::key, its work in Cx::detach",
    ),
    Step::new(
        "Load real data",
        "Screen::load returns the read; Screen::loaded gets its answer",
    ),
    Step::new(
        "Wire the headless flags",
        "--dump, --keys, --at, --bench, --capture and --compare, as main does here",
    ),
];

const COLUMNS: [Column; 2] = [
    Column::new("Step", 16, 26).pinned(),
    Column::new("How", 12, 0),
];

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
struct Step {
    title: &'static str,
    how: &'static str,
}

impl Step {
    const fn new(title: &'static str, how: &'static str) -> Self {
        Step { title, how }
    }
}

enum Msg {
    Steps(Vec<Step>),
}

#[derive(Default)]
struct Home {
    runs: Vec<(u64, Instant)>,
    made: u64,
    due: Option<Instant>,
    last: Option<Instant>,
    now: Option<Instant>,
    accent: usize,
}

fn units() -> Units {
    Units::new("s", "m", "h", "d")
}

fn build(id: u64, started: Instant, now: Instant) -> Activity {
    let label = format!("build #{id}");
    let gone = now.saturating_duration_since(started);
    let fraction = (gone.as_secs_f64() / JOB.as_secs_f64()).min(1.0);
    let at = ((fraction * STAGES.len() as f64) as usize).min(STAGES.len());
    let detail = STAGES.iter().enumerate().map(|(index, stage)| {
        let sign = match index.cmp(&at) {
            Ordering::Less => "✓",
            Ordering::Equal => "›",
            Ordering::Greater => "·",
        };
        Line::from(format!("{sign} {stage}"))
    });
    let activity = Activity::new(id, label, started).detail(detail);
    if at == STAGES.len() {
        return activity
            .state(State::Done)
            .ended(started + JOB)
            .status("built");
    }
    let status = format!("{} · {} of {}", STAGES[at], at + 1, STAGES.len());
    activity.progress(fraction).estimate(at == 0).status(status)
}

impl Screen<Msg> for Home {
    fn draw(&mut self, frame: &mut Frame, area: Rect, palette: &Palette) {
        let lines = vec![
            Line::styled(NAME, palette.selected),
            Line::styled(format!("v{VERSION}"), palette.muted),
        ];
        message(frame, area, lines);
    }

    fn moment(&mut self, now: Instant) {
        self.now = Some(now);
    }

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, Msg>) {
        match key.code {
            KeyCode::Char('j') => {
                self.made += 1;
                let now = cx.now();
                self.runs.push((self.made, now));
                self.last = Some(now);
                self.due = Some(now + TICK);
                cx.activity(build(self.made, now, now));
            }
            KeyCode::Char('c') => {
                self.made += 1;
                self.last = Some(cx.now());
                let program = env::current_exe().unwrap_or_else(|_| NAME.into());
                let mut command = Command::new(program);
                command.arg(PROGRESS);
                cx.run(self.made, format!("check #{}", self.made), command);
            }
            KeyCode::Char('p') => {
                let pick = Pick::new("Accent", ["Blue", "Amber", "Pink"])
                    .selected(self.accent)
                    .hints([
                        Hint::new("↑↓", "move"),
                        Hint::new("enter", "choose"),
                        Hint::new("esc", "cancel"),
                    ]);
                cx.pick(pick);
            }
            _ => {}
        }
    }

    fn picked(&mut self, choice: Option<usize>, cx: &mut Cx<'_, Msg>) {
        if let Some(accent) = choice {
            self.accent = accent;
            cx.palette(Palette::new(ACCENTS[accent]));
        }
    }

    fn deadline(&self) -> Option<Instant> {
        let label = self
            .last
            .zip(self.now)
            .map(|(last, now)| clock::next_age(last, now));
        [self.due, label].into_iter().flatten().min()
    }

    fn tick(&mut self, cx: &mut Cx<'_, Msg>) {
        if self.runs.is_empty() {
            return;
        }
        let now = cx.now();
        for (id, started) in &self.runs {
            cx.activity(build(*id, *started, now));
        }
        self.runs
            .retain(|(_, started)| now.saturating_duration_since(*started) < JOB);
        self.due = (!self.runs.is_empty()).then(|| now + TICK);
    }

    fn busy(&self) -> usize {
        self.runs.len()
    }

    fn facts(&self) -> Vec<(String, Style)> {
        let (Some(last), Some(now)) = (self.last, self.now) else {
            return Vec::new();
        };
        let ago = clock::age(now.saturating_duration_since(last), &units());
        vec![(format!("last started {ago} ago"), Style::new())]
    }

    fn hints(&self) -> Vec<Hint<'_>> {
        vec![
            Hint::new("tab", "next screen"),
            Hint::new("j", "start a job").rank(1),
            Hint::new("c", "run a command").rank(2),
            Hint::new("p", "pick an accent").rank(3),
        ]
    }
}

struct Journal {
    log: Log,
}

impl Journal {
    fn new() -> Self {
        let verbs = ["fetched", "compiled", "linked", "tested", "packed"];
        let lines: Vec<Line<'static>> = (1..=LINES)
            .map(|number| {
                let stage = STAGES[number % STAGES.len()];
                let verb = verbs[number % verbs.len()];
                let text = format!("{number:05}  {stage:<8}  {verb} part {number} of {LINES}");
                if number % 1000 == 0 {
                    Line::styled(text, Style::new().add_modifier(Modifier::BOLD))
                } else {
                    Line::from(text)
                }
            })
            .collect();
        let mut log = Log::new("Find")
            .placeholder("type to find in the log")
            .hint("enter keeps it · esc clears it · n and N step through");
        log.set_lines(Arc::new(lines));
        Journal { log }
    }
}

impl Screen<Msg> for Journal {
    fn draw(&mut self, frame: &mut Frame, area: Rect, palette: &Palette) {
        self.log.draw(frame, area, palette);
    }

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, Msg>) {
        if let log::Turn::Copy(text) = self.log.key(key) {
            if cx.copy(text) {
                cx.say("Copied.", Tone::Good);
            } else {
                cx.say("Too long to copy; y copies the page.", Tone::Alert);
            }
        }
    }

    fn paste(&mut self, text: &str, _cx: &mut Cx<'_, Msg>) {
        self.log.paste(text);
    }

    fn typing(&self) -> bool {
        self.log.typing()
    }

    fn input(&self) -> Option<InputBar<'_>> {
        self.log.bar()
    }

    fn facts(&self) -> Vec<(String, Style)> {
        let line = format!("line {} of {}", self.log.top() + 1, self.log.len());
        let mut facts = vec![(line, Style::new())];
        if !self.log.query().is_empty() {
            let at = self.log.hit().map_or(0, |hit| hit + 1);
            facts.push((format!("{at} of {} found", self.log.hits()), Style::new()));
        }
        facts
    }

    fn hints(&self) -> Vec<Hint<'_>> {
        vec![
            Hint::new("↑↓", "scroll"),
            Hint::new("g/G", "ends").rank(2),
            Hint::new("/", "find").rank(1),
            Hint::new("n/N", "next").rank(3),
            Hint::new("y", "copy the page").rank(4),
        ]
    }
}

struct Next {
    steps: Option<Vec<Step>>,
    stopped: bool,
    filter: Filter,
    open: Option<usize>,
}

impl Next {
    fn new() -> Self {
        Next {
            steps: None,
            stopped: false,
            filter: Filter::new("Filter")
                .placeholder("type to filter the steps")
                .hint("enter keeps the filter · esc clears it"),
            open: None,
        }
    }

    fn opened(&self) -> Option<&Step> {
        self.steps.as_ref()?.get(self.open?)
    }

    fn sift(&mut self) {
        let steps = self.steps.as_deref().unwrap_or_default();
        self.filter.sift(
            steps,
            |step, query| matches(&format!("{} {}", step.title, step.how), query),
            |step| Row::new([Cell::new(step.title), Cell::new(step.how).faint()]),
        );
    }
}

impl Screen<Msg> for Next {
    fn phase(&self) -> Phase<'_> {
        match (&self.steps, self.stopped) {
            (Some(_), _) => Phase::Ready,
            (None, false) => Phase::Loading("Finding what to do next...".into()),
            (None, true) => Phase::Trouble(
                vec![
                    Line::from("Stopped waiting for the steps."),
                    Line::from("r reads them again."),
                ]
                .into(),
            ),
        }
    }

    fn load(&mut self) -> Option<Job<Msg>> {
        self.steps = None;
        self.stopped = false;
        self.open = None;
        Some(Box::new(|| {
            thread::sleep(LOAD);
            Msg::Steps(STEPS.to_vec())
        }))
    }

    fn loaded(&mut self, event: Msg, _cx: &mut Cx<'_, Msg>) {
        let Msg::Steps(steps) = event;
        self.steps = Some(steps);
        self.sift();
    }

    fn cancel(&mut self) {
        self.stopped = true;
    }

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, Msg>) {
        if let Some(step) = self.opened() {
            if key.code == KeyCode::Char('y') {
                let text = format!("{}: {}", step.title, step.how);
                if cx.copy(text) {
                    cx.say("Copied the step.", Tone::Good);
                }
            }
            return;
        }
        match self.filter.key(key) {
            Turn::Filtered => self.sift(),
            Turn::Open(index) => self.open = Some(index),
            Turn::Pass if key.code == KeyCode::Char('r') => cx.reload(),
            _ => {}
        }
    }

    fn back(&mut self, _cx: &mut Cx<'_, Msg>) {
        self.open = None;
    }

    fn crumb(&self) -> Option<String> {
        self.opened().map(|step| step.title.to_string())
    }

    fn selected(&self) -> usize {
        self.filter.list().selected().unwrap_or(0)
    }

    fn paste(&mut self, text: &str, _cx: &mut Cx<'_, Msg>) {
        if self.filter.paste(text) == Turn::Filtered {
            self.sift();
        }
    }

    fn draw(&mut self, frame: &mut Frame, area: Rect, palette: &Palette) {
        if let Some(step) = self.opened() {
            let lines = vec![
                Line::styled(step.title, palette.selected),
                Line::default(),
                Line::styled(step.how, palette.ink),
            ];
            message(frame, area, lines);
            return;
        }
        let view = self
            .filter
            .view(&COLUMNS, palette)
            .header(true)
            .empty(Some("Nothing matches the filter."));
        frame.render_widget(view, area);
    }

    fn typing(&self) -> bool {
        self.open.is_none() && self.filter.typing()
    }

    fn input(&self) -> Option<InputBar<'_>> {
        self.open.is_none().then(|| self.filter.bar()).flatten()
    }

    fn hints(&self) -> Vec<Hint<'_>> {
        if self.steps.is_none() {
            return vec![Hint::new("r", "read again")];
        }
        if self.open.is_some() {
            return vec![Hint::new("esc", "back"), Hint::new("y", "copy").rank(1)];
        }
        let mut hints = vec![
            Hint::new("↑↓", "move"),
            Hint::new("enter", "open").rank(1),
            Hint::new("/", "filter").rank(2),
            Hint::new("r", "read again").rank(3),
        ];
        if self.filter.active() {
            hints.push(Hint::new("esc", "clear filter"));
        }
        hints
    }

    fn facts(&self) -> Vec<(String, Style)> {
        let Some(steps) = &self.steps else {
            return Vec::new();
        };
        let shown = self.filter.shown().len();
        let count = if self.filter.active() {
            format!("{shown} of {} steps", steps.len())
        } else {
            format!("{} steps", steps.len())
        };
        vec![(count, Style::new())]
    }
}

fn words() -> Words {
    Words::new()
        .help("? help")
        .again("ctrl+c again to quit")
        .busy(|running| format!("{running} still running. Quit anyway?"))
        .yes("Yes")
        .no("No")
        .choose("y/n choose · enter accept · esc cancel")
        .too_small("Make the window at least 40 × 12")
        .waiting("esc stops waiting")
}

fn elapsed(took: Duration) -> String {
    clock::span(took, &units())
}

fn jobs() -> Activities {
    Activities::new(Section::new("Jobs"))
        .keys(&[Key::Char('o')])
        .hints([
            Hint::new("↑↓", "move"),
            Hint::new("enter", "open").rank(1),
            Hint::new("esc", "back"),
        ])
        .detail_hints([
            Hint::new("↑↓", "scroll"),
            Hint::new("/", "find").rank(1),
            Hint::new("y", "copy").rank(2),
            Hint::new("esc", "back"),
        ])
        .log(
            Log::new("Find")
                .placeholder("type to find in the steps")
                .hint("enter keeps it · esc clears it"),
        )
        .empty("No jobs yet. j on Home starts one.")
        .more(|hidden| format!("+{hidden} more · o shows them all"))
        .elapsed(elapsed)
        .facts(|jobs| {
            let running = jobs.iter().filter(|job| !job.state.finished()).count();
            let mut facts = vec![(format!("{} started", jobs.len()), Style::new())];
            if running > 0 {
                facts.push((format!("{running} running"), Style::new()));
            }
            facts
        })
}

fn tui() -> Tui<Msg> {
    let mut accent = 0;
    Tui::new(NAME, VERSION, ACCENTS[accent])
        .words(words())
        .hints([
            Hint::new("o", "jobs").rank(3),
            Hint::new("a", "accent").rank(5),
            Hint::new("?", "help").rank(4),
            Hint::new("ctrl+c", "twice quit").pinned(),
        ])
        .footer_look(|footer| footer.separator(" · "))
        .on_key(move |key, cx| {
            if key.code != KeyCode::Char('a') {
                return false;
            }
            accent = (accent + 1) % ACCENTS.len();
            cx.palette(Palette::new(ACCENTS[accent]));
            true
        })
        .screen(Section::new("Home"), Home::default())
        .screen(Section::new("What to do next").short("Next"), Next::new())
        .screen(Section::new("Log"), Journal::new())
        .activities(jobs())
}

fn walk() -> Walk {
    let script = |name: &str, keys: &str| Script::new(name, dump::keys(keys).unwrap_or_default());
    Walk::new()
        .script(script("filter", "tab / load"))
        .script(script("drill-in", "tab enter"))
        .script(script("copied", "tab enter y"))
        .script(script("quit guard", "ctrl+c"))
        .script(script("jobs", "j j").at(Duration::from_millis(2500)))
        .script(script("job detail", "j o enter").at(Duration::from_millis(2500)))
        .script(script("picker", "p down"))
        .script(script("command", "c"))
        .script(script("log find", "3 / 1999 enter n"))
        .script(
            script("hourglass", "tab")
                .loading()
                .at(Duration::from_millis(500)),
        )
}

fn progress() -> ExitCode {
    let stages = ["resolve", "fetch", "build", "check"];
    let steps = 4;
    let total = (stages.len() * steps) as f64;
    let at = || {
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map_or(0, |gone| gone.as_millis())
    };
    let say = |stage: &str, state: &str, fraction: Option<f64>, message: Option<String>| {
        let fraction = fraction.map_or("null".to_string(), |fraction| format!("{fraction:.3}"));
        let message = message.map_or("null".to_string(), |message| format!("\"{message}\""));
        let at = at();
        println!(
            "{{\"v\":1,\"op\":\"check\",\"app\":\"{NAME}\",\"ref\":null,\"stage\":\"{stage}\",\"state\":\"{state}\",\"fraction\":{fraction},\"msg\":{message},\"at\":{at}}}"
        );
    };
    for (index, stage) in stages.iter().enumerate() {
        say(stage, "start", None, Some(format!("{stage} starting")));
        let mut done = 0.0;
        for step in 1..=steps {
            thread::sleep(STEP);
            done = (index * steps + step) as f64 / total;
            say(
                stage,
                "progress",
                Some(done),
                Some(format!("{stage} {step} of {steps}")),
            );
        }
        say(stage, "done", Some(done), None);
    }
    let at = at();
    println!(
        "{{\"v\":1,\"op\":\"check\",\"app\":\"{NAME}\",\"ref\":null,\"state\":\"end\",\"ok\":true,\"msg\":\"every check passed\",\"at\":{at}}}"
    );
    ExitCode::SUCCESS
}

fn main() -> ExitCode {
    let args: Vec<String> = env::args().skip(1).collect();
    if args.iter().any(|arg| arg == PROGRESS) {
        return progress();
    }
    let value = |name: &str| {
        args.iter()
            .position(|arg| arg == name)
            .and_then(|at| args.get(at + 1))
            .map(String::as_str)
    };
    let on = |name: &str| args.iter().any(|arg| arg == name);
    let size = match value("--dump").map(dump::size) {
        Some(None) => {
            eprintln!("{NAME}: --dump takes a size such as 120x34");
            return ExitCode::from(2);
        }
        Some(Some(size)) => Some(size),
        None => None,
    };
    if let Some(dir) = value("--capture") {
        return match capture::capture(Path::new(dir), &walk(), tui) {
            Ok(frames) => {
                println!("{NAME}: {frames} frames captured in {dir}");
                ExitCode::SUCCESS
            }
            Err(error) => {
                eprintln!("{NAME}: {dir}: {error}");
                ExitCode::from(2)
            }
        };
    }
    if let Some(dir) = value("--compare") {
        return match capture::compare(Path::new(dir), &walk(), tui) {
            Ok(differences) if differences.is_empty() => {
                println!("{NAME}: every frame matches {dir}");
                ExitCode::SUCCESS
            }
            Ok(differences) => {
                for difference in &differences {
                    println!("{difference}");
                }
                eprintln!("{NAME}: {} differences from {dir}", differences.len());
                ExitCode::FAILURE
            }
            Err(error) => {
                eprintln!("{NAME}: {dir}: {error}");
                ExitCode::from(2)
            }
        };
    }
    if let Some(frames) = value("--bench") {
        let Ok(frames) = frames.parse::<usize>() else {
            eprintln!("{NAME}: --bench takes a number of frames");
            return ExitCode::from(2);
        };
        let (width, height) = size.unwrap_or(SIZE);
        for bench in tui().bench(width, height, frames) {
            println!(
                "{NAME} --bench: {frames} frames of the {} screen at {width}x{height}: average {:.3} ms, worst {:.3} ms",
                bench.screen,
                bench.average.as_secs_f64() * 1000.0,
                bench.worst.as_secs_f64() * 1000.0,
            );
        }
        return ExitCode::SUCCESS;
    }
    if let Some((width, height)) = size {
        let keys = match dump::keys(value("--keys").unwrap_or_default()) {
            Ok(keys) => keys,
            Err(word) => {
                eprintln!("{NAME}: --keys can't read {word}");
                return ExitCode::from(2);
            }
        };
        let mut tui = tui();
        let mut buffer = tui.shot(width, height, &keys, !on("--loading"));
        if let Some(at) = value("--at") {
            let Ok(at) = at.parse::<u64>() else {
                eprintln!("{NAME}: --at takes milliseconds");
                return ExitCode::from(2);
            };
            tui.advance(Duration::from_millis(at));
            buffer = tui.frame(width, height);
        }
        if on("--ansi") {
            print!("{}", dump::ansi(&buffer));
        } else {
            println!("{}", dump::text(&buffer));
        }
        return ExitCode::SUCCESS;
    }
    match tui().run() {
        Ok(()) => ExitCode::SUCCESS,
        Err(error) => {
            eprintln!("{NAME}: {error}");
            ExitCode::FAILURE
        }
    }
}
