use std::fmt::Write;
use std::io;
use std::time::{Duration, Instant};

use crossterm::event::{self, Event, KeyCode, KeyEvent, KeyEventKind};
use pito_tui::footer::{Guard, QuitGuard, Wording};
use pito_tui::list::{
    Bar, Cell, Column, Count, Key, Keys, List, ListView, Mark, Part, Place, Row, Step, Styles, cut,
};
use ratatui::{
    DefaultTerminal, Frame,
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
};

const BOLD: Style = Style::new().add_modifier(Modifier::BOLD);
const FAINT: Style = Style::new().add_modifier(Modifier::DIM);
const TICK: Duration = Duration::from_millis(120);
const NOTE: Style = Style::new().fg(Color::Cyan);
const COUNT: Count = Count::new("–", " of ").place(Place::Foot);
const HINTS: &str =
    "↑↓ j k move · ctrl+c twice quit · ^d ^u half · pgdn pgup page · v range · enter open";

const COLUMNS: [Column; 5] = [
    Column::new("Job", 10, 16).pinned(),
    Column::new("Version", 7, 14).priority(2),
    Column::new("Owner", 5, 8).priority(1),
    Column::new("Size", 6, 0).priority(3).right().fit(9),
    Column::new("Message", 8, 0),
];

struct Job {
    index: usize,
    done: u32,
    total: u32,
    rate: u32,
    verb: &'static str,
}

struct App {
    list: List,
    jobs: [Job; 2],
    anchor: Option<usize>,
    opened: Option<usize>,
    guard: QuitGuard,
    text: String,
}

fn running(name: &str, from: &str, to: &str, owner: &str) -> Row {
    Row::new([
        Cell::new(name),
        Cell::parts([Part::new(from), Part::new(format!(" → {to}")).faint()]),
        Cell::new(owner),
        Cell::new(""),
        Cell::new(""),
    ])
    .mark(Mark::new("⧗").style(Style::new().fg(Color::Yellow)))
}

fn queued(name: &str, version: &str, owner: &str, message: &str) -> Row {
    Row::new([
        Cell::new(name),
        Cell::new(version),
        Cell::new(owner),
        Cell::new("—").faint(),
        Cell::new(message).faint(),
    ])
    .mark(Mark::new("·").faint())
}

fn done(name: &str, version: &str, owner: &str, size: &str, message: &str) -> Row {
    Row::new([name, version, owner, size, message])
        .mark(Mark::new("✓").style(Style::new().fg(Color::Green)))
}

fn heading(title: &str, count: usize) -> Row {
    Row::heading([
        Part::new(title).style(BOLD),
        Part::new(format!(" · {count}")).faint(),
    ])
}

fn rows() -> Vec<Row> {
    vec![
        heading("Running", 2),
        running("deploy api", "v1.4", "v1.5", "web"),
        running("migrate store", "v12", "v13", "data"),
        Row::blank(),
        heading("Queued", 3),
        queued("build docs", "v0.9", "docs", "waiting for a runner"),
        queued("rotate logs", "daily", "ops", "starts at 02:00"),
        queued("resize images", "v2.1", "media", "after build docs"),
        Row::blank(),
        heading("Done", 15),
        done(
            "update tool",
            "0.1 → 0.2",
            "tools",
            "640 kB",
            "finished in 42 s",
        ),
        done("bench parser", "v3.0", "core", "1.2 MB", "0.2 ms a frame"),
        Row::new([
            Cell::new("release notes"),
            Cell::new("a note that runs across the columns after the name")
                .faint()
                .rest(),
        ])
        .mark(Mark::new("✓").style(Style::new().fg(Color::Green))),
        done("backup config", "nightly", "ops", "88 kB", "finished"),
        done("sync mirrors", "v5", "infra", "9.4 MB", "finished in 3 min"),
        done("prune cache", "weekly", "ops", "2.1 GB", "freed 2.1 GB"),
        done("renew certs", "v2", "web", "4 kB", "valid for 90 days"),
        done("index search", "v8", "data", "310 MB", "finished in 6 min"),
        done("compact logs", "daily", "ops", "740 MB", "finished"),
        done("warm cdn", "v1.5", "web", "56 MB", "412 files"),
        done("vacuum store", "v13", "data", "1.8 GB", "finished in 9 min"),
        done("rotate keys", "monthly", "infra", "2 kB", "three keys"),
        done("export stats", "v4", "core", "12 MB", "finished"),
        done("trim images", "v2.0", "media", "96 MB", "1,204 images"),
        done("check links", "v1", "docs", "18 kB", "no broken links"),
    ]
}

fn styles() -> Styles {
    Styles::new()
        .selected(
            Style::new()
                .bg(Color::Indexed(237))
                .add_modifier(Modifier::BOLD),
        )
        .faint(FAINT)
        .header(Style::new().fg(Color::DarkGray))
        .header_key(Style::new().fg(Color::Yellow).add_modifier(Modifier::BOLD))
        .cursor(Style::new().fg(Color::Cyan))
        .keep_colours(true)
}

impl App {
    fn new() -> Self {
        let mut app = App {
            list: List::new().keys(Keys::VIM).with_rows(rows()),
            jobs: [
                Job {
                    index: 1,
                    done: 30,
                    total: 240,
                    rate: 3,
                    verb: "uploading the bundle",
                },
                Job {
                    index: 2,
                    done: 180,
                    total: 640,
                    rate: 7,
                    verb: "copying tables",
                },
            ],
            anchor: None,
            opened: None,
            guard: QuitGuard::new(Wording::new("Press ctrl+c again to quit")),
            text: String::with_capacity(32),
        };
        app.tick();
        app
    }

    fn tick(&mut self) {
        self.guard.tick(Instant::now());
        for job in &mut self.jobs {
            job.done = (job.done + job.rate) % job.total;
            let share = job.done * 100 / job.total;
            let warm = match share {
                0..40 => Color::Green,
                40..75 => Color::Yellow,
                _ => Color::Red,
            };
            let Some(row) = self.list.row_mut(job.index) else {
                continue;
            };
            let cells = row.cells_mut();
            self.text.clear();
            let _ = write!(self.text, "{}.{} MB", job.done / 10, job.done % 10);
            cells[3].set_text(&self.text);
            cells[3].set_style(Style::new().fg(warm));
            self.text.clear();
            let _ = write!(self.text, "{} · {share}%", job.verb);
            cells[4].set_text(&self.text);
        }
    }

    fn key(&mut self, press: KeyEvent) -> bool {
        match self
            .guard
            .key(pito_tui::footer::Key::from(press), Instant::now(), false)
        {
            Guard::Quit => return true,
            Guard::Held => return false,
            _ => {}
        }
        match press.code {
            KeyCode::Char('v') => {
                self.anchor = match self.anchor {
                    Some(_) => None,
                    None => self.list.selected(),
                }
            }
            KeyCode::Esc => {
                self.anchor = None;
                self.opened = None;
            }
            _ => {
                if let Step::Open(index) = self.list.key(Key::from(press)) {
                    self.opened = Some(index);
                }
            }
        }
        self.list
            .select_range(self.anchor.zip(self.list.selected()));
        false
    }

    fn draw(&mut self, frame: &mut Frame) {
        let area = frame.area();
        let rows = Rect {
            height: area.height.saturating_sub(2),
            ..area
        };
        let view = ListView::new(&mut self.list, &COLUMNS)
            .styles(styles())
            .header(true)
            .key_column(Some(3))
            .end(Some("· end ·"))
            .scrollbar(Some(Bar::LINE))
            .count(Some(COUNT));
        frame.render_widget(view, rows);
        if area.height < 2 {
            return;
        }
        let (note, style) = match (self.guard.notice(), self.list.range(), self.opened) {
            (Some(again), _, _) => (again.to_owned(), NOTE.add_modifier(Modifier::BOLD)),
            (None, Some((from, to)), _) => (
                format!("range {} … {}", self.name(from), self.name(to)),
                NOTE,
            ),
            (None, None, Some(index)) => (format!("opened {}", self.name(index)), NOTE),
            (None, None, None) => (String::new(), NOTE),
        };
        foot(frame.buffer_mut(), area, &note, style);
    }

    fn name(&self, index: usize) -> &str {
        self.list
            .rows()
            .get(index)
            .and_then(|row| row.cells().first())
            .map_or("", Cell::text)
    }
}

fn legend(room: usize) -> String {
    match cut(HINTS, room) {
        None => HINTS.to_owned(),
        Some(kept) => format!("{kept}…"),
    }
}

fn foot(buffer: &mut Buffer, area: Rect, note: &str, style: Style) {
    let line = area.bottom() - 1;
    let width = u16::try_from(note.chars().count()).unwrap_or(0);
    let at = area.right().saturating_sub(width + 2);
    let room = usize::from(at.saturating_sub(area.x + 3));
    buffer.set_stringn(area.x + 2, line, legend(room), room, FAINT);
    buffer.set_string(at, line, note, style);
}

fn run(terminal: &mut DefaultTerminal) -> io::Result<()> {
    let mut app = App::new();
    let mut last = Instant::now();
    loop {
        terminal.draw(|frame| app.draw(frame))?;
        if event::poll(TICK.saturating_sub(last.elapsed()))?
            && let Event::Key(press) = event::read()?
            && press.kind == KeyEventKind::Press
            && app.key(press)
        {
            return Ok(());
        }
        if last.elapsed() >= TICK {
            app.tick();
            last = Instant::now();
        }
    }
}

fn main() -> io::Result<()> {
    let mut terminal = ratatui::init();
    let result = run(&mut terminal);
    ratatui::restore();
    result
}

#[cfg(test)]
mod tests {
    use super::*;

    fn line(buffer: &Buffer, y: u16) -> String {
        (0..buffer.area.width)
            .map(|x| buffer[(x, y)].symbol().to_owned())
            .collect()
    }

    fn footer(width: u16, note: &str) -> String {
        let area = Rect::new(0, 0, width, 3);
        let mut buffer = Buffer::empty(area);
        foot(&mut buffer, area, note, NOTE);
        assert_eq!(line(&buffer, 0), " ".repeat(usize::from(width)));
        assert_eq!(line(&buffer, 1), " ".repeat(usize::from(width)));
        line(&buffer, 2)
    }

    #[test]
    fn the_legend_fits_whole_when_there_is_room() {
        assert_eq!(legend(HINTS.chars().count()), HINTS);
        assert_eq!(legend(200), HINTS);
    }

    #[test]
    fn the_legend_ends_in_a_whole_word_and_an_ellipsis_when_short_of_room() {
        for room in 1..HINTS.chars().count() {
            let shown = legend(room);
            assert!(shown.chars().count() <= room, "room {room}: {shown}");
            let body = shown.strip_suffix('…').expect("an ellipsis");
            assert!(HINTS.starts_with(body), "room {room}: {shown}");
            assert!(
                room < 5 || HINTS[body.len()..].starts_with(' '),
                "room {room}: cut mid-word: {shown}"
            );
        }
    }

    #[test]
    fn the_legend_keeps_its_start_when_no_whole_word_fits() {
        assert_eq!(legend(1), "…");
        assert_eq!(legend(2), "↑…");
        assert_eq!(legend(3), "↑↓…");
        assert_eq!(legend(4), "↑↓…");
        assert_eq!(legend(5), "↑↓ j…");
    }

    #[test]
    fn an_eighty_column_terminal_keeps_the_quit_hint() {
        let text = footer(80, "");
        assert!(text.contains("ctrl+c twice quit"), "{text}");
        assert!(text.trim_end().ends_with('…'), "{text}");
    }

    #[test]
    fn an_eighty_column_terminal_keeps_the_quit_hint_beside_a_note() {
        let text = footer(80, "range Alpha … Bravo");
        assert!(text.contains("ctrl+c twice quit"), "{text}");
        assert!(text.ends_with("range Alpha … Bravo  "), "{text}");
    }

    #[test]
    fn a_wide_terminal_shows_the_whole_legend_and_the_note_at_the_right_edge() {
        let text = footer(120, "opened Alpha");
        assert!(text.starts_with(&format!("  {HINTS}")));
        assert!(text.ends_with("opened Alpha  "));
    }

    #[test]
    fn a_narrow_terminal_cuts_the_legend_before_the_note_never_mid_word() {
        let note = "range Alpha … Bravo";
        let text = footer(60, note);
        assert!(text.ends_with(&format!("{note}  ")));
        let left: String = text.chars().take(39).collect();
        let shown = left.trim();
        assert!(shown.chars().count() <= 36);
        let body = shown.strip_suffix('…').expect("an ellipsis");
        assert!(HINTS.starts_with(body));
        assert!(HINTS[body.len()..].starts_with(' '));
    }

    #[test]
    fn a_note_as_wide_as_the_line_leaves_no_room_for_the_legend() {
        let note = "x".repeat(20);
        assert_eq!(footer(24, &note), format!("  {note}  "));
    }

    #[test]
    fn a_note_wider_than_the_line_draws_from_the_left_without_a_legend() {
        assert_eq!(footer(10, &"y".repeat(30)), "y".repeat(10));
    }

    #[test]
    fn no_note_leaves_the_legend_alone() {
        assert_eq!(footer(120, "").trim_end(), format!("  {HINTS}"));
    }
}
