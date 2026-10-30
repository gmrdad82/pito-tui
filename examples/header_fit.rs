use std::io;

use crossterm::event::{self, Event};
use pito_tui::header::{Fact, Group, Header, Key, Nav, Section, Styles};
use ratatui::{
    DefaultTerminal, Frame,
    layout::Rect,
    style::{Color, Style},
    text::{Line, Span},
    widgets::Paragraph,
};

const ACCENT: Style = Style::new().fg(Color::Yellow);
const MUTED: Style = Style::new().fg(Color::DarkGray);
const STYLES: Styles = Styles::new().accent(ACCENT).muted(MUTED).rule(MUTED);
const NARROWEST: u16 = 8;

fn nav() -> Nav {
    let mut nav = Nav::new(vec![
        Group::new("Mail")
            .section(Section::new("Inbox"))
            .section(Section::new("Drafts"))
            .section(Section::new("Archive")),
        Group::new("Library")
            .short("Lib")
            .section(Section::new("Items & notes").short("Items"))
            .section(Section::new("Collections").short("Sets"))
            .section(Section::new("Ownership").short("Owners")),
        Group::new("Work")
            .section(Section::new("Imports & exports").short("Imports"))
            .section(Section::new("Schedules").short("Plans"))
            .section(Section::new("Checks")),
    ]);
    nav.go(5);
    nav
}

fn draw(frame: &mut Frame, nav: &Nav, width: u16) {
    let full = frame.area();
    let area = Rect {
        y: full.y + 2,
        height: full.height.saturating_sub(2),
        ..full
    };
    let facts = [
        Fact::new("12 items", MUTED),
        Fact::new("3 shared", ACCENT),
        Fact::new("synced just now", MUTED),
    ];
    let header = Header::new(nav)
        .styles(STYLES)
        .title(Some("Desk"))
        .facts(&facts)
        .closing(true);
    let height = header.height().min(area.height);
    let width = width.min(area.width);
    frame.render_widget(
        header,
        Rect {
            width,
            height,
            ..area
        },
    );
    if width < area.width {
        let edge = Rect {
            x: area.x + width,
            width: 1,
            height,
            ..area
        };
        let bars: Vec<Line> = (0..height).map(|_| Line::styled("┃", ACCENT)).collect();
        frame.render_widget(Paragraph::new(bars), edge);
    }
    let caption = Rect {
        y: area.y + height + 1,
        height: 1,
        ..area
    };
    frame.render_widget(
        Paragraph::new(Line::from(vec![
            Span::styled(format!("{width}"), ACCENT),
            Span::styled(" columns   < narrower   > wider", MUTED),
        ]))
        .centered(),
        caption,
    );
}

fn run(terminal: &mut DefaultTerminal) -> io::Result<()> {
    let mut nav = nav();
    let mut width = terminal.size()?.width;
    loop {
        terminal.draw(|frame| draw(frame, &nav, width))?;
        let full = terminal.size()?.width;
        if let Event::Key(event) = event::read()? {
            match Key::from(event) {
                Key::Ctrl('c') => return Ok(()),
                Key::Char('<') | Key::Left => width = width.saturating_sub(2).max(NARROWEST),
                Key::Char('>') | Key::Right => width = (width + 2).min(full),
                key => {
                    nav.key(key);
                }
            }
        }
    }
}

fn main() -> io::Result<()> {
    let mut terminal = ratatui::init();
    let result = run(&mut terminal);
    ratatui::restore();
    result
}
