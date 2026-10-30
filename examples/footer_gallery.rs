use std::io;
use std::time::{Duration, Instant};

use pito_tui::footer::{
    Answer, Confirm, ConfirmBar, Edit, Footer, Guard, Hint, Input, InputBar, Key, Mode, Notice,
    QuitGuard, Segment, Styles, Tone, Wording, Words,
};
use ratatui::{
    DefaultTerminal, Frame,
    crossterm::event::{self, Event},
    layout::Rect,
    style::{Color, Modifier, Style},
    text::{Line, Span},
};

const DIM: Style = Style::new().add_modifier(Modifier::DIM);
const ACCENT: Style = Style::new().fg(Color::Magenta);
const TITLE: Style = Style::new().add_modifier(Modifier::BOLD);

const STYLES: Styles = Styles::new()
    .accent(ACCENT)
    .muted(DIM)
    .rule(DIM)
    .alert(Style::new().fg(Color::Red))
    .good(Style::new().fg(Color::Green));

const HINTS: [Hint; 8] = [
    Hint::new("↑↓", "move"),
    Hint::new("enter", "open").rank(1),
    Hint::new("/", "search").rank(2),
    Hint::new("f", "filter").rank(3),
    Hint::new("s", "sort").rank(4),
    Hint::new("y", "copy").rank(5),
    Hint::new("?", "help").rank(6),
    Hint::new("ctrl+c", "twice quit").pinned(),
];

const SHORT: [Hint; 4] = [
    Hint::new("↑↓", "move"),
    Hint::new("enter", "open").rank(1),
    Hint::new("r", "retry").rank(2),
    Hint::new("ctrl+c", "twice quit").pinned(),
];

const PAGE: [(&str, Style); 2] = [("page 1/3 ", DIM), ("●○○", ACCENT)];

const INPUT_HINTS: [Hint; 2] = [
    Hint::new("tab", "next field"),
    Hint::new("ctrl+c", "quit").pinned(),
];

const JOB_HINTS: [Hint; 3] = [
    Hint::new("d", "delete"),
    Hint::new("p", "pause").rank(1),
    Hint::new("ctrl+c", "twice quit").pinned(),
];

const KEY: &str = "sk-live-4f9a0c2e7b13d58e6a90f4c1b27d3e85";

fn text(frame: &mut Frame, x: u16, y: u16, line: Line) {
    let area = frame.area();
    if y >= area.height {
        return;
    }
    let row = Rect::new(x, y, area.width.saturating_sub(x), 1);
    frame.render_widget(line, row);
}

fn title(frame: &mut Frame, y: u16, words: &str) {
    text(frame, 1, y, Line::styled(words, TITLE));
}

fn label(frame: &mut Frame, y: u16, words: String) {
    text(frame, 1, y, Line::styled(words, DIM));
}

fn place(frame: &mut Frame, footer: Footer, y: u16, width: u16) -> u16 {
    let height = footer.height(width);
    frame.render_widget(footer, Rect::new(1, y, width, height));
    y + height
}

fn fit(frame: &mut Frame) {
    title(
        frame,
        0,
        "One set of hints at four widths: the highest rank drops first, the pinned one stays",
    );
    let mut y = 2;
    for width in [88, 66, 48, 30] {
        label(frame, y, format!("{width} columns"));
        let footer = Footer::new(&HINTS).styles(STYLES).version("app", "1.4.2");
        y = place(frame, footer, y + 1, width) + 1;
    }
    label(frame, y, "48 columns, wrap(true)".into());
    let footer = Footer::new(&HINTS)
        .styles(STYLES)
        .wrap(true)
        .version("app", "1.4.2");
    place(frame, footer, y + 1, 48);
}

fn notices(frame: &mut Frame) {
    title(
        frame,
        0,
        "A notice line under the hints, in the app's tones; over() wraps a long one",
    );
    let rows = [
        ("legend", Notice::legend("○ open · ● done · ✗ failed")),
        (
            "accent",
            Notice::accent("3 tasks match “bike” · esc clears"),
        ),
        ("good", Notice::good("Saved 12 photos to the album")),
        (
            "alert",
            Notice::alert("Couldn't reach the server · r retries"),
        ),
    ];
    let mut y = 2;
    for (tone, notice) in rows {
        label(frame, y, tone.into());
        let footer = Footer::new(&SHORT)
            .styles(STYLES)
            .version("app", "1.4.2")
            .notice(Some(notice));
        y = place(frame, footer, y + 1, 72) + 1;
    }
    label(frame, y, "alert, over()".into());
    let over = Notice::new(
        "The import stopped at photo 7 of 12: the disk is full. Free some space, then press r to retry from where it stopped.",
        Tone::Alert,
    )
    .over();
    let footer = Footer::new(&SHORT)
        .styles(STYLES)
        .version("app", "1.4.2")
        .notice(Some(over));
    place(frame, footer, y + 1, 72);
}

fn status(frame: &mut Frame) {
    title(
        frame,
        0,
        "Footer::status: text, spans and hints on one row, dropped and clipped by rank",
    );
    let segments = [
        Segment::text(" NORMAL ", ACCENT.add_modifier(Modifier::REVERSED)),
        Segment::text("3 of 1,204 selected in this view", Style::new())
            .shrink()
            .rank(1),
        Segment::hints(&SHORT).rank(4),
        Segment::text("12:30", DIM).right().rank(3),
        Segment::spans(&PAGE).right().rank(2),
    ];
    let mut y = 2;
    for width in [88, 72, 54, 36] {
        label(frame, y, format!("{width} columns"));
        let footer = Footer::status(&segments)
            .styles(STYLES)
            .version("app", "1.4.2");
        y = place(frame, footer.rule(true), y + 1, width) + 1;
    }
}

struct Fields {
    search: Input,
    key: Input,
    on_key: bool,
    said: Option<String>,
}

impl Fields {
    fn key(&mut self, key: Key) -> bool {
        if key == Key::Ctrl('c') {
            return true;
        }
        if key == Key::Tab {
            self.on_key = !self.on_key;
            return false;
        }
        let field = if self.on_key {
            &mut self.key
        } else {
            &mut self.search
        };
        match field.key(key) {
            Edit::Submit(value) => {
                let what = if self.on_key { "key" } else { "search" };
                self.said = Some(format!(
                    "Submitted the {what}: {} characters",
                    value.chars().count()
                ));
            }
            Edit::Cancel => field.clear(),
            Edit::Changed => self.said = None,
            _ => {}
        }
        false
    }

    fn draw(&self, frame: &mut Frame) {
        title(
            frame,
            0,
            "A one-line input by grapheme and cell, and a masked one for keys",
        );
        let width = 60;
        label(frame, 2, "Input".into());
        let search = InputBar::new("Search", &self.search)
            .placeholder("type to filter")
            .hint("enter search · esc clear")
            .caret(!self.on_key)
            .styles(STYLES);
        frame.render_widget(search, Rect::new(1, 3, width, search.height()));
        label(
            frame,
            7,
            "Input::masked(true), reveal_ends(4) at rest".into(),
        );
        let key = InputBar::new("API key", &self.key)
            .placeholder("paste a key")
            .hint("enter save · esc clear")
            .reveal_ends(4)
            .caret(self.on_key)
            .styles(STYLES);
        frame.render_widget(key, Rect::new(1, 8, width, key.height()));
        let notice = self.said.as_deref().map(Notice::good);
        let footer = Footer::new(&INPUT_HINTS)
            .styles(STYLES)
            .version("app", "1.4.2")
            .notice(notice);
        let area = frame.area();
        let height = footer.height(area.width);
        frame.render_widget(
            footer,
            Rect::new(0, area.height - height, area.width, height),
        );
    }
}

struct Job {
    deleting: Option<Confirm>,
    guard: QuitGuard,
    said: Option<(&'static str, Tone)>,
}

impl Job {
    fn key(&mut self, key: Key) -> bool {
        match self.guard.key(key, Instant::now(), true) {
            Guard::Quit => return true,
            Guard::Held => return false,
            _ => {}
        }
        if let Some(confirm) = self.deleting.as_mut() {
            match confirm.key(key) {
                Some(Answer::Yes) => {
                    self.deleting = None;
                    self.said = Some(("Deleted 12 photos", Tone::Good));
                }
                Some(_) => {
                    self.deleting = None;
                    self.said = Some(("Kept the photos", Tone::Muted));
                }
                None => {}
            }
            return false;
        }
        if key == Key::Char('d') {
            self.said = None;
            self.deleting = Some(Confirm::new());
        }
        false
    }

    fn draw(&self, frame: &mut Frame) {
        title(
            frame,
            0,
            "A y/n confirm in the app's words, and a quit guard that asks while work runs",
        );
        text(
            frame,
            1,
            2,
            Line::from(vec![
                Span::styled("⧗ ", ACCENT),
                Span::raw("Importing photos  "),
                Span::styled("███████", ACCENT),
                Span::styled("░░░░░", DIM),
                Span::styled("  7 of 12", DIM),
            ]),
        );
        let words = Words::new("Yes", "No").hint("y/n choose · enter accept · esc cancel");
        let notice = match (self.guard.notice(), self.said) {
            (Some(again), _) => Some(Notice::accent(again)),
            (None, Some((said, tone))) => Some(Notice::new(said, tone)),
            (None, None) => None,
        };
        let confirm = self.guard.bar(words).or_else(|| {
            self.deleting
                .as_ref()
                .map(|confirm| ConfirmBar::new("Delete the 12 photos?", confirm, words))
        });
        let footer = Footer::new(&JOB_HINTS)
            .styles(STYLES)
            .version("app", "1.4.2")
            .notice(notice)
            .confirm(confirm);
        let area = frame.area();
        let height = footer.height(area.width);
        frame.render_widget(
            footer,
            Rect::new(0, area.height - height, area.width, height),
        );
    }
}

enum Scene {
    Still(fn(&mut Frame)),
    Fields(Fields),
    Job(Box<Job>),
}

impl Scene {
    fn draw(&self, frame: &mut Frame) {
        match self {
            Scene::Still(draw) => draw(frame),
            Scene::Fields(fields) => fields.draw(frame),
            Scene::Job(job) => job.draw(frame),
        }
    }

    fn key(&mut self, key: Key) -> bool {
        match self {
            Scene::Still(_) => matches!(key, Key::Ctrl('c') | Key::Esc | Key::Char('q')),
            Scene::Fields(fields) => fields.key(key),
            Scene::Job(job) => job.key(key),
        }
    }

    fn deadline(&self) -> Option<Instant> {
        match self {
            Scene::Job(job) => job.guard.deadline(),
            _ => None,
        }
    }

    fn tick(&mut self) {
        if let Scene::Job(job) = self {
            job.guard.tick(Instant::now());
        }
    }
}

fn scene(name: &str) -> Option<Scene> {
    Some(match name {
        "fit" => Scene::Still(fit),
        "notices" => Scene::Still(notices),
        "status" => Scene::Still(status),
        "input" => Scene::Fields(Fields {
            search: Input::new(),
            key: Input::new().masked(true).with(KEY),
            on_key: false,
            said: None,
        }),
        "confirm" => Scene::Job(Box::new(Job {
            deleting: None,
            guard: QuitGuard::new(
                Wording::new("Press ctrl+c again to quit")
                    .ask("The import is still running. Quit anyway?"),
            )
            .mode(Mode::Ask),
            said: None,
        })),
        _ => return None,
    })
}

fn run(terminal: &mut DefaultTerminal, mut scene: Scene) -> io::Result<()> {
    loop {
        terminal.draw(|frame| scene.draw(frame))?;
        let wait = scene.deadline().map_or(Duration::from_secs(60), |at| {
            at.saturating_duration_since(Instant::now())
        });
        if !event::poll(wait)? {
            scene.tick();
            continue;
        }
        if let Event::Key(event) = event::read()?
            && scene.key(Key::from(event))
        {
            return Ok(());
        }
    }
}

fn main() -> io::Result<()> {
    let name = std::env::args().nth(1).unwrap_or_default();
    let Some(scene) = scene(&name) else {
        eprintln!("gallery: unknown scene '{name}', try fit, notices, status, input or confirm");
        std::process::exit(2);
    };
    let mut terminal = ratatui::init();
    let result = run(&mut terminal, scene);
    ratatui::restore();
    result
}
