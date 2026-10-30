use std::{
    io,
    time::{Duration, Instant},
};

use pito_tui::hourglass::Hourglass;
use ratatui::{
    DefaultTerminal, Frame,
    crossterm::event::{self, Event, KeyCode, KeyEventKind},
    layout::{Alignment, Rect},
    style::{Color, Modifier, Style},
    widgets::{Block, BorderType, Paragraph},
};

const STEP: Duration = Duration::from_millis(900);
const SIZES: [(u16, u16, &str); 7] = [
    (27, 9, "glass, label and hint"),
    (13, 8, "glass, label and hint"),
    (13, 7, "no room for the glass"),
    (21, 2, "label and hint"),
    (21, 1, "the label alone"),
    (11, 1, "the label, cut"),
    (6, 1, "the label, cut"),
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
    let screen = frame.area();
    let step = (elapsed.as_millis() / STEP.as_millis()) as usize % SIZES.len();
    let (width, height, what) = SIZES[step];
    let muted = Style::new().fg(Color::DarkGray);
    frame.render_widget(
        Paragraph::new(format!("{width}×{height}: {what}"))
            .style(muted)
            .alignment(Alignment::Center),
        Rect::new(screen.x, screen.y + 1, screen.width, 1),
    );
    let outer = Rect::new(
        screen.x + screen.width.saturating_sub(width + 2) / 2,
        screen.y + 3 + screen.height.saturating_sub(height + 5) / 2,
        width + 2,
        height + 2,
    )
    .intersection(screen);
    let block = Block::bordered()
        .border_type(BorderType::Rounded)
        .border_style(muted);
    let inner = block.inner(outer);
    frame.render_widget(block, outer);
    let hourglass = Hourglass::new(elapsed)
        .label("Loading the data...")
        .hint(Some("esc stops waiting"))
        .glass(Style::new().fg(Color::Yellow))
        .accent(Style::new().fg(Color::Magenta))
        .muted(muted.add_modifier(Modifier::DIM));
    frame.render_widget(hourglass, inner);
}
