use std::io;

use crossterm::event::{self, Event};
use pito_tui::header::{Fact, Group, Header, Key, Nav, Section, Styles};
use ratatui::{
    DefaultTerminal, Frame,
    layout::Rect,
    style::{Color, Modifier, Style},
    text::Line,
    widgets::Paragraph,
};

const ACCENT: Style = Style::new().fg(Color::Yellow);
const MUTED: Style = Style::new().fg(Color::DarkGray);
const STYLES: Styles = Styles::new().accent(ACCENT).muted(MUTED).rule(MUTED);
const UP: Style = Style::new().fg(Color::Green);
const DOWN: Style = Style::new().fg(Color::Red);
const BUSY: Style = Style::new().fg(Color::Yellow);

fn desk() -> Nav {
    Nav::new(vec![
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
    ])
}

fn rows(frame: &mut Frame) {
    let mut nav = desk();
    nav.go(2);
    let trial = [Fact::new("trial ", MUTED), Fact::new("9 days left", ACCENT)];
    let facts = [
        Fact::new("3 drafts", ACCENT),
        Fact::new("1 scheduled", MUTED),
        Fact::new("synced just now", MUTED),
    ];
    let header = Header::new(&nav)
        .styles(STYLES)
        .title(Some("Desk"))
        .left_parts(&trial)
        .right(Some(Fact::new("? help", MUTED)))
        .facts(&facts)
        .facts_rule(true)
        .closing(true)
        .notice(Some(Fact::new(
            "Delete the 3 drafts? y/n",
            Style::new().fg(Color::Magenta).add_modifier(Modifier::BOLD),
        )));
    let area = frame.area();
    frame.render_widget(
        header,
        Rect {
            height: header.height().min(area.height),
            ..area
        },
    );
}

fn spans(frame: &mut Frame) {
    let mut nav = Nav::new(vec![
        Group::new("Hosts")
            .section(Section::spans(&[("web-1 ", Style::new()), ("● up", UP)]).short("web-1"))
            .section(Section::spans(&[("web-2 ", Style::new()), ("● up", UP)]).short("web-2"))
            .section(Section::spans(&[("db ", Style::new()), ("● down", DOWN)]).short("db"))
            .section(Section::spans(&[("cache ", Style::new()), ("● busy", BUSY)]).short("cache")),
        Group::new("Jobs")
            .section(Section::new("Queue"))
            .section(Section::new("History")),
    ]);
    nav.go(3);
    let facts = [
        Fact::new("2 up", UP),
        Fact::new("1 down", DOWN),
        Fact::new("1 busy", BUSY),
        Fact::new("checked 4s ago", MUTED),
    ];
    let header = Header::new(&nav)
        .styles(STYLES)
        .title(Some("Fleet"))
        .facts(&facts)
        .facts_rule(true);
    let area = frame.area();
    frame.render_widget(
        header,
        Rect {
            height: header.height().min(area.height),
            ..area
        },
    );
}

fn drill(frame: &mut Frame) {
    let mut nav = desk();
    nav.go(9);
    nav.open("nightly", 0);
    nav.open("step 3: test", 2);
    let area = frame.area();
    let mut y = area.y;
    for width in [area.width, 30, 20] {
        let header = Header::new(&nav)
            .styles(STYLES)
            .title(Some("Desk"))
            .crumbs(true);
        let height = header.height();
        if y + height > area.bottom() {
            break;
        }
        frame.render_widget(
            header,
            Rect {
                x: area.x,
                y,
                width,
                height,
            },
        );
        frame.render_widget(
            Paragraph::new(Line::styled(format!("{width} columns"), MUTED)),
            Rect {
                x: area.x + width + 2,
                y: y + 1,
                width: area.width.saturating_sub(width + 2),
                height: 1,
            },
        );
        y += height + 1;
    }
}

fn run(terminal: &mut DefaultTerminal, scene: fn(&mut Frame)) -> io::Result<()> {
    loop {
        terminal.draw(scene)?;
        if let Event::Key(event) = event::read()?
            && matches!(Key::from(event), Key::Char('q') | Key::Ctrl('c'))
        {
            return Ok(());
        }
    }
}

fn main() -> io::Result<()> {
    let scene: fn(&mut Frame) = match std::env::args().nth(1).as_deref() {
        Some("spans") => spans,
        Some("drill") => drill,
        _ => rows,
    };
    let mut terminal = ratatui::init();
    let result = run(&mut terminal, scene);
    ratatui::restore();
    result
}
