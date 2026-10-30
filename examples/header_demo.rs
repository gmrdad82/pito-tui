use std::io;
use std::time::{Duration, Instant};

use crossterm::event::{self, Event};
use pito_tui::header::{Fact, Group, Header, Key, Nav, Section, Step, Styles};
use ratatui::{
    DefaultTerminal, Frame,
    layout::Rect,
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::Paragraph,
};

const ACCENT: Style = Style::new().fg(Color::Yellow);
const MUTED: Style = Style::new().fg(Color::DarkGray);
const STYLES: Styles = Styles::new().accent(ACCENT).muted(MUTED).rule(MUTED);
const WINDOW: Duration = Duration::from_secs(2);

const ITEMS: [&[&str]; 9] = [
    &[
        "Welcome aboard",
        "Weekly digest",
        "Invoice 1042",
        "Lunch on Friday",
    ],
    &["Reply: lunch on Friday", "Quarterly summary"],
    &["Old newsletter", "Receipt 0991", "Conference badge"],
    &["Reading list", "Recipes", "Garden plan", "Trip ideas"],
    &["Favourites", "To read", "Shared"],
    &["Studio", "Workshop", "Garage"],
    &["contacts.csv", "calendar.ics"],
    &["Every morning", "Every Monday", "First of the month"],
    &["nightly", "release", "lint", "docs"],
];

const STEPS: &[&str] = &["fetch", "build", "test", "package"];
const PARTS: &[&str] = &["Overview", "History", "Notes"];

struct App {
    nav: Nav,
    selected: usize,
    notice: Option<&'static str>,
    armed: Option<Instant>,
}

impl App {
    fn new() -> Self {
        let nav = Nav::new(vec![
            Group::new("Mail")
                .section(Section::new("Inbox"))
                .section(Section::new("Drafts"))
                .section(Section::new("Archive")),
            Group::new("Library")
                .section(Section::new("Items & notes").short("Items"))
                .section(Section::new("Collections").short("Sets"))
                .section(Section::new("Ownership").short("Owners")),
            Group::new("Work")
                .section(Section::new("Imports & exports").short("Imports"))
                .section(Section::new("Schedules"))
                .section(Section::new("Checks")),
        ]);
        App {
            nav,
            selected: 0,
            notice: None,
            armed: None,
        }
    }

    fn rows(&self) -> &'static [&'static str] {
        let number = self.nav.place().number;
        match self.nav.depth() {
            0 => ITEMS.get(number.wrapping_sub(1)).copied().unwrap_or(&[]),
            1 if number == 9 => STEPS,
            1 => PARTS,
            _ => &[],
        }
    }

    fn key(&mut self, key: Key, now: Instant) -> bool {
        self.notice = None;
        if key == Key::Ctrl('c') {
            let again = self
                .armed
                .take()
                .is_some_and(|at| now.saturating_duration_since(at) < WINDOW);
            if again {
                return false;
            }
            self.armed = Some(now);
            return true;
        }
        self.armed = None;
        match self.nav.key(key) {
            Some(Step::Back { selected }) => self.selected = selected,
            Some(Step::Moved(_)) => self.selected = 0,
            Some(_) => {}
            None => match key {
                Key::Up => self.selected = self.selected.saturating_sub(1),
                Key::Down => {
                    self.selected = (self.selected + 1).min(self.rows().len().saturating_sub(1))
                }
                Key::Enter => match self.rows().get(self.selected) {
                    Some(row) => {
                        self.nav.open(*row, self.selected);
                        self.selected = 0;
                    }
                    None => self.notice = Some("Nothing deeper here."),
                },
                _ => {}
            },
        }
        true
    }

    fn facts(&self) -> Vec<(String, Style)> {
        let count = self.rows().len();
        let mut facts = vec![(format!("{count} items"), MUTED)];
        if self.nav.place().number == 9 {
            facts = vec![
                (format!("{count} checks"), Style::new().fg(Color::Green)),
                ("1 running".into(), ACCENT),
                ("1 failed".into(), Style::new().fg(Color::Red)),
            ];
        }
        facts.push(("synced just now".into(), MUTED));
        facts
    }

    fn draw(&self, frame: &mut Frame) {
        let facts = self.facts();
        let notice = self
            .armed
            .map(|_| "Press ctrl+c again to quit")
            .or(self.notice)
            .map(|text| Fact::new(text, Style::new().fg(Color::Magenta)));
        let header = Header::new(&self.nav)
            .styles(STYLES)
            .title(Some("Desk"))
            .left(Some(Fact::new("header demo", MUTED)))
            .right(Some(Fact::new("ctrl+c twice quit", MUTED)))
            .facts_pairs(&facts)
            .crumbs(true)
            .closing(true)
            .notice(notice);
        let area = frame.area();
        let height = header.height().min(area.height);
        frame.render_widget(header, Rect { height, ..area });
        let body = Rect {
            x: area.x + 2,
            y: area.y + height + 1,
            width: area.width.saturating_sub(4),
            height: area.height.saturating_sub(height + 1),
        };
        frame.render_widget(Paragraph::new(self.lines()), body);
    }

    fn lines(&self) -> Vec<Line<'static>> {
        let rows = self.rows();
        if rows.is_empty() {
            return vec![
                Line::styled(
                    self.nav.breadcrumb(),
                    Style::new().add_modifier(Modifier::BOLD),
                ),
                Line::default(),
                Line::styled("esc goes back, the list keeps its place", MUTED),
            ];
        }
        rows.iter()
            .enumerate()
            .map(|(index, row)| {
                if index == self.selected {
                    Line::from(vec![
                        Span::styled("› ", ACCENT),
                        Span::styled(*row, ACCENT.add_modifier(Modifier::BOLD)),
                    ])
                } else {
                    Line::from(vec![Span::raw("  "), Span::raw(*row)])
                }
            })
            .collect()
    }
}

fn run(terminal: &mut DefaultTerminal) -> io::Result<()> {
    let mut app = App::new();
    loop {
        terminal.draw(|frame| app.draw(frame))?;
        let wait = app.armed.map_or(Duration::from_secs(60), |at| {
            WINDOW.saturating_sub(at.elapsed())
        });
        if !event::poll(wait)? {
            app.armed = None;
            continue;
        }
        if let Event::Key(event) = event::read()?
            && !app.key(Key::from(event), Instant::now())
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
