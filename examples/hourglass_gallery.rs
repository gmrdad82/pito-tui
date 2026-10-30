use std::{
    io,
    time::{Duration, Instant},
};

use pito_tui::hourglass::{CYCLE, Hourglass};
use ratatui::{
    DefaultTerminal, Frame,
    crossterm::event::{self, Event, KeyCode, KeyEventKind},
    layout::{Constraint, Layout},
    style::{Color, Modifier, Style},
};

const APPS: [(&str, Option<&str>, Color, Color); 4] = [
    (
        "Loading the data...",
        Some("esc stops waiting"),
        Color::Yellow,
        Color::Magenta,
    ),
    (
        "Se încarcă datele…",
        Some("esc anulează"),
        Color::Cyan,
        Color::LightBlue,
    ),
    ("データを読み込み中…", None, Color::Green, Color::LightGreen),
    (
        "Syncing 3 inboxes",
        Some("q quits"),
        Color::Magenta,
        Color::LightYellow,
    ),
];

fn main() -> io::Result<()> {
    ratatui::run(run)
}

fn run(terminal: &mut DefaultTerminal) -> io::Result<()> {
    let started = Instant::now();
    loop {
        terminal.draw(|frame| draw(frame, started.elapsed()))?;
        if event::poll(Duration::from_millis(16))?
            && let Event::Key(key) = event::read()?
            && key.kind == KeyEventKind::Press
            && matches!(key.code, KeyCode::Esc | KeyCode::Char('q'))
        {
            return Ok(());
        }
    }
}

fn draw(frame: &mut Frame, elapsed: Duration) {
    let [top, bottom] = Layout::vertical([Constraint::Fill(1); 2]).areas(frame.area());
    let quarters = [top, bottom].into_iter().flat_map(|half| {
        Layout::horizontal([Constraint::Fill(1); 2])
            .split(half)
            .to_vec()
    });
    for (index, (area, (label, hint, glass, accent))) in quarters.zip(APPS).enumerate() {
        let lag = CYCLE * index as u32 / 4;
        let hourglass = Hourglass::new(elapsed + lag)
            .label(label)
            .hint(hint)
            .glass(Style::new().fg(glass))
            .accent(Style::new().fg(accent))
            .muted(Style::new().fg(Color::DarkGray).add_modifier(Modifier::DIM));
        frame.render_widget(hourglass, area);
    }
}
