use std::io;
use std::time::{Duration, Instant};

use pito_tui::footer::{
    Answer, Confirm, ConfirmBar, Edit, Footer, Guard, Help, Hint, Input, InputBar, Key, Notice,
    QuitGuard, Segment, Styles, Tone, Wording, Words,
};
use ratatui::{
    DefaultTerminal, Frame,
    crossterm::event::{self, Event},
    layout::{Constraint, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
};

const HINTS: [Hint; 6] = [
    Hint::new("↑↓", "move"),
    Hint::new("enter", "done").rank(1),
    Hint::new("/", "search").rank(2),
    Hint::new("d", "delete").rank(2),
    Hint::new("?", "help").rank(3),
    Hint::new("ctrl+c", "twice quit").pinned(),
];

const TASKS: [&str; 7] = [
    "Water the plants",
    "Buy oat milk and bread",
    "Fix the bike light",
    "Return the library books",
    "Plan the weekend hike",
    "Write the release notes",
    "Clean the gutters",
];

const DIM: Style = Style::new().add_modifier(Modifier::DIM);
const ACCENT: Style = Style::new().fg(Color::Magenta);

const STYLES: Styles = Styles::new()
    .accent(ACCENT)
    .muted(DIM)
    .rule(DIM)
    .alert(Style::new().fg(Color::Red))
    .good(Style::new().fg(Color::Green));

struct Task {
    name: &'static str,
    done: bool,
}

struct App {
    tasks: Vec<Task>,
    selected: usize,
    filter: String,
    search: Option<Input>,
    deleting: Option<Confirm>,
    question: String,
    notice: Option<(String, Tone)>,
    help: Help,
    guard: QuitGuard,
}

impl App {
    fn new() -> Self {
        App {
            tasks: TASKS
                .iter()
                .map(|&name| Task { name, done: false })
                .collect(),
            selected: 0,
            filter: String::new(),
            search: None,
            deleting: None,
            question: String::new(),
            notice: None,
            help: Help::new(true),
            guard: QuitGuard::new(Wording::new("Press ctrl+c again to quit")),
        }
    }

    fn shown(&self) -> Vec<usize> {
        let filter = self.filter.to_lowercase();
        (0..self.tasks.len())
            .filter(|&at| self.tasks[at].name.to_lowercase().contains(&filter))
            .collect()
    }

    fn current(&self) -> Option<usize> {
        self.shown().get(self.selected).copied()
    }

    fn say(&mut self, text: String, tone: Tone) {
        self.notice = Some((text, tone));
    }

    fn key(&mut self, key: Key) -> bool {
        match self.guard.key(key, Instant::now(), false) {
            Guard::Quit => return true,
            Guard::Held => return false,
            _ => {}
        }
        if let Some(confirm) = self.deleting.as_mut() {
            match confirm.key(key) {
                Some(Answer::Yes) => {
                    self.deleting = None;
                    if let Some(at) = self.current() {
                        let task = self.tasks.remove(at);
                        self.selected = self.selected.min(self.shown().len().saturating_sub(1));
                        self.say(format!("Deleted “{}”", task.name), Tone::Good);
                    }
                }
                Some(_) => self.deleting = None,
                None => {}
            }
            return false;
        }
        if let Some(input) = self.search.as_mut() {
            match input.key(key) {
                Edit::Changed => {
                    self.filter = input.value().to_owned();
                    self.selected = 0;
                }
                Edit::Submit(text) => {
                    self.search = None;
                    self.filter = text;
                    let count = self.shown().len();
                    self.say(
                        format!("{count} tasks match “{}” · esc clears", self.filter),
                        Tone::Accent,
                    );
                }
                Edit::Cancel => {
                    self.search = None;
                    self.filter.clear();
                }
                _ => {}
            }
            return false;
        }
        if self.help.key(key) {
            return false;
        }
        let count = self.shown().len();
        match key {
            Key::Up | Key::Char('k') => self.selected = self.selected.saturating_sub(1),
            Key::Down | Key::Char('j') => {
                self.selected = (self.selected + 1).min(count.saturating_sub(1))
            }
            Key::Enter => {
                if let Some(at) = self.current() {
                    let task = &mut self.tasks[at];
                    task.done = !task.done;
                    let text = if task.done {
                        format!("Done: {}", task.name)
                    } else {
                        format!("Open again: {}", task.name)
                    };
                    self.say(text, Tone::Good);
                }
            }
            Key::Char('/') => {
                self.notice = None;
                self.search = Some(Input::new().limit(40));
            }
            Key::Char('d') => {
                if let Some(at) = self.current() {
                    self.notice = None;
                    self.question = format!("Delete “{}”?", self.tasks[at].name);
                    self.deleting = Some(Confirm::new());
                }
            }
            Key::Esc if !self.filter.is_empty() => {
                self.filter.clear();
                self.selected = 0;
                self.notice = None;
            }
            _ => {}
        }
        false
    }

    fn draw(&self, frame: &mut Frame) {
        let area = frame.area();
        let words = Words::new("Yes", "No").hint("y/n choose · enter accept · esc cancel");
        let notice = match (self.guard.notice(), &self.notice) {
            (Some(again), _) => Notice::accent(again),
            (None, Some((text, tone))) => Notice::new(text, *tone),
            (None, None) => Notice::legend("○ open · ● done"),
        };
        let confirm = self
            .deleting
            .as_ref()
            .map(|confirm| ConfirmBar::new(&self.question, confirm, words));
        let input = self.search.as_ref().map(|input| {
            InputBar::new("Search", input)
                .placeholder("type to filter")
                .hint("enter keep · esc cancel")
        });
        let footer = Footer::new(&HINTS)
            .styles(STYLES)
            .help(&self.help)
            .indent(1)
            .version("demo", env!("CARGO_PKG_VERSION"))
            .notice(Some(notice))
            .confirm(confirm)
            .input(input);
        let height = footer.height(area.width);
        let [top, body, bottom] = Layout::vertical([
            Constraint::Length(2),
            Constraint::Fill(1),
            Constraint::Length(height),
        ])
        .areas(area);
        self.header(frame, top);
        self.list(frame, body);
        frame.render_widget(footer, bottom);
    }

    fn header(&self, frame: &mut Frame, area: Rect) {
        let open = self.tasks.iter().filter(|task| !task.done).count();
        let counts = format!("{open} open · {} done", self.tasks.len() - open);
        let filter = format!("filter “{}”", self.filter);
        let segments = [
            Segment::text(" Tasks ", ACCENT.add_modifier(Modifier::REVERSED)),
            Segment::text(&counts, DIM).rank(1),
            Segment::text(&filter, ACCENT).right().rank(2),
        ];
        let shown = if self.filter.is_empty() { 2 } else { 3 };
        let row = Rect { height: 1, ..area };
        frame.render_widget(Footer::status(&segments[..shown]).styles(STYLES), row);
    }

    fn list(&self, frame: &mut Frame, area: Rect) {
        let shown = self.shown();
        for (row, &at) in shown.iter().enumerate().take(usize::from(area.height)) {
            let task = &self.tasks[at];
            let chosen = row == self.selected;
            let mark = if task.done { "●" } else { "○" };
            let name = if task.done {
                DIM.add_modifier(Modifier::CROSSED_OUT)
            } else if chosen {
                Style::new().add_modifier(Modifier::BOLD)
            } else {
                Style::new()
            };
            let line = Line::from(vec![
                Span::styled(if chosen { " ▸ " } else { "   " }, ACCENT),
                Span::styled(mark, if task.done { ACCENT } else { DIM }),
                Span::raw(" "),
                Span::styled(task.name, name),
            ]);
            let y = area.y + row as u16;
            frame.render_widget(
                line,
                Rect {
                    y,
                    height: 1,
                    ..area
                },
            );
        }
    }
}

fn run(terminal: &mut DefaultTerminal) -> io::Result<()> {
    let mut app = App::new();
    loop {
        terminal.draw(|frame| app.draw(frame))?;
        let wait = app.guard.deadline().map_or(Duration::from_secs(60), |at| {
            at.saturating_duration_since(Instant::now())
        });
        if !event::poll(wait)? {
            app.guard.tick(Instant::now());
            continue;
        }
        if let Event::Key(event) = event::read()?
            && app.key(Key::from(event))
        {
            return Ok(());
        }
    }
}

fn main() -> io::Result<()> {
    let mut terminal = ratatui::init();
    let result = run(&mut terminal);
    ratatui::restore();
    result
}
