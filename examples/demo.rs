use std::env;
use std::process::ExitCode;
use std::thread;
use std::time::Duration;

use pito_tui::crossterm::event::{KeyCode, KeyEvent};
use pito_tui::footer::{Hint, InputBar, Tone};
use pito_tui::header::Section;
use pito_tui::list::{Cell, Column, Row};
use pito_tui::ratatui::{Frame, layout::Rect, style::Color, style::Style, text::Line};
use pito_tui::{Cx, Filter, Job, Palette, Phase, Screen, Tui, Turn, Words, dump, matches, message};

const NAME: &str = "demo";
const VERSION: &str = env!("CARGO_PKG_VERSION");
const ACCENTS: [Color; 3] = [
    Color::Rgb(0x5b, 0x8c, 0xff),
    Color::Rgb(0xff, 0xcf, 0x5c),
    Color::Rgb(0xd1, 0x3c, 0x9c),
];
const LOAD: Duration = Duration::from_millis(900);
const SIZE: (u16, u16) = (120, 34);

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
        "--dump, --keys, --ansi, --loading and --bench, as main does here",
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

struct Home;

impl Screen<Msg> for Home {
    fn draw(&mut self, frame: &mut Frame, area: Rect, palette: &Palette) {
        let lines = vec![
            Line::styled(NAME, palette.selected),
            Line::styled(format!("v{VERSION}"), palette.muted),
        ];
        message(frame, area, lines);
    }

    fn hints(&self) -> Vec<Hint<'_>> {
        vec![Hint::new("tab", "next screen")]
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

fn tui() -> Tui<Msg> {
    let mut accent = 0;
    Tui::new(NAME, VERSION, ACCENTS[accent])
        .words(words())
        .hints([
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
        .screen(Section::new("Home"), Home)
        .screen(Section::new("What to do next").short("Next"), Next::new())
}

fn main() -> ExitCode {
    let args: Vec<String> = env::args().skip(1).collect();
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
        let buffer = tui().shot(width, height, &keys, !on("--loading"));
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
