use std::{io, time::Duration};

use pito_tui::hourglass::{CYCLE, FRAMES, GLASS_ROWS, Hourglass, WIDTH, frame_at};
use ratatui::{
    DefaultTerminal, Frame,
    crossterm::event::{self, Event, KeyCode, KeyEventKind},
    layout::{Alignment, Rect},
    style::{Color, Style},
    widgets::Paragraph,
};

const PITCH: u16 = WIDTH + 2;

fn main() -> io::Result<()> {
    ratatui::run(run)
}

fn run(terminal: &mut DefaultTerminal) -> io::Result<()> {
    loop {
        terminal.draw(draw)?;
        if let Event::Key(key) = event::read()?
            && key.kind == KeyEventKind::Press
            && matches!(key.code, KeyCode::Esc | KeyCode::Char('q'))
        {
            return Ok(());
        }
    }
}

fn starts() -> Vec<u64> {
    let mut starts = Vec::with_capacity(FRAMES.len());
    for ms in 0..CYCLE.as_millis() as u64 {
        if frame_at(Duration::from_millis(ms)) == starts.len() {
            starts.push(ms);
        }
    }
    starts
}

fn draw(frame: &mut Frame) {
    let screen = frame.area();
    let muted = Style::new().fg(Color::DarkGray);
    let glass = Style::new().fg(Color::Yellow);
    let starts = starts();
    let rows = [
        ("drain: 7 frames over 3 s", 0..7),
        ("flip: 6 frames over 1.2 s", 7..FRAMES.len()),
    ];
    let left = screen.x + screen.width.saturating_sub(PITCH * 7) / 2 + 1;
    let mut y = screen.y + 1;
    for (title, frames) in rows {
        frame.render_widget(
            Paragraph::new(title).style(muted),
            Rect::new(left, y, PITCH * 7, 1).intersection(screen),
        );
        y += 2;
        for (column, index) in frames.enumerate() {
            let x = left + column as u16 * PITCH;
            let at = Duration::from_millis(starts[index]);
            frame.render_widget(
                Hourglass::new(at).label("").glass(glass),
                Rect::new(x, y, WIDTH, GLASS_ROWS + 1).intersection(screen),
            );
            frame.render_widget(
                Paragraph::new(format!("{} ms", starts[index]))
                    .style(muted)
                    .alignment(Alignment::Center),
                Rect::new(x, y + GLASS_ROWS, WIDTH, 1).intersection(screen),
            );
        }
        y += GLASS_ROWS + 3;
    }
}
