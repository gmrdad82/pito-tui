#![allow(dead_code)]

use pito_tui::list::{Cell, Column, List, ListView, Mark, Row, Styles};
use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
    widgets::Widget,
};
use unicode_width::UnicodeWidthStr;

pub const SELECTED: Style = Style::new().fg(Color::Magenta).add_modifier(Modifier::BOLD);
pub const FAINT: Style = Style::new().add_modifier(Modifier::DIM);
pub const GREEN: Style = Style::new().fg(Color::Green);
pub const STYLES: Styles = Styles::new().selected(SELECTED).faint(FAINT);

pub const COLUMNS: [Column<'static>; 5] = [
    Column::new("Operation", 10, 16).pinned(),
    Column::new("Versions", 8, 12).priority(4),
    Column::new("Owner", 5, 8).priority(2),
    Column::new("State", 5, 8).priority(3),
    Column::new("Message", 8, 0),
];

pub fn operations() -> Vec<Row> {
    let data = [
        (
            "⧗",
            "deploy api",
            "v1 → v2",
            "billing",
            "running",
            "uploading the bundle",
        ),
        ("✓", "update tool", "0.1 → 0.2", "tools", "done", "finished"),
        (
            "✗",
            "install app",
            "v3 → v4",
            "design",
            "failed",
            "exit 1: the build could not start because a lock was held",
        ),
        (
            "■",
            "release",
            "v5 → v6",
            "billing",
            "stopped",
            "stopped on request",
        ),
    ];
    data.iter()
        .map(|(mark, name, versions, owner, state, message)| {
            Row::new([
                Cell::new(*name),
                Cell::new(*versions).faint(),
                Cell::new(*owner),
                Cell::new(*state).style(GREEN),
                Cell::new(*message),
            ])
            .mark(Mark::new(*mark).style(GREEN))
        })
        .collect()
}

pub fn list_of(rows: Vec<Row>) -> List {
    List::new().with_rows(rows)
}

pub struct Drawn {
    pub text: Vec<String>,
    pub marks: Vec<String>,
}

pub fn draw_with<'a>(
    list: &'a mut List,
    columns: &'a [Column<'a>],
    width: u16,
    height: u16,
    tweak: impl FnOnce(ListView<'a>) -> ListView<'a>,
) -> Drawn {
    let area = Rect::new(0, 0, width, height);
    let mut buf = Buffer::empty(area);
    tweak(ListView::new(list, columns)).render(area, &mut buf);
    read(&buf)
}

pub fn draw(list: &mut List, width: u16, height: u16) -> Drawn {
    let area = Rect::new(0, 0, width, height);
    let mut buf = Buffer::empty(area);
    ListView::new(list, &COLUMNS)
        .styles(STYLES)
        .header(true)
        .end(Some("· end ·"))
        .empty(Some("Nothing here."))
        .render(area, &mut buf);
    read(&buf)
}

fn mark(style: Style) -> char {
    if style.add_modifier.contains(Modifier::BOLD) {
        'S'
    } else if style.add_modifier.contains(Modifier::DIM) {
        'f'
    } else if style.fg == Some(Color::Green) {
        'g'
    } else {
        '.'
    }
}

pub fn read(buf: &Buffer) -> Drawn {
    let area = buf.area;
    let mut text = Vec::new();
    let mut marks = Vec::new();
    for y in area.top()..area.bottom() {
        let mut line = String::new();
        let mut paint = String::new();
        let mut skip = 0usize;
        for x in area.left()..area.right() {
            let cell = &buf[(x, y)];
            paint.push(mark(cell.style()));
            if skip > 0 {
                skip -= 1;
                continue;
            }
            let symbol = cell.symbol();
            skip = symbol.width().saturating_sub(1);
            line.push_str(symbol);
        }
        text.push(line.trim_end().to_string());
        marks.push(paint.trim_end_matches('.').to_string());
    }
    Drawn { text, marks }
}

pub fn cells(line: &str) -> usize {
    line.width()
}
