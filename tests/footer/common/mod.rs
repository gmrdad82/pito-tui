#![allow(dead_code)]

use pito_tui::footer::Styles;
use ratatui::{
    Terminal,
    backend::TestBackend,
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
    widgets::Widget,
};
use unicode_width::UnicodeWidthStr;

pub const STYLES: Styles = Styles::new()
    .accent(Style::new().fg(Color::Magenta))
    .muted(Style::new().fg(Color::DarkGray))
    .alert(Style::new().fg(Color::Red))
    .ink(Style::new().fg(Color::White))
    .rule(Style::new().fg(Color::Gray));

pub struct Drawn {
    pub text: Vec<String>,
    pub marks: Vec<String>,
}

pub fn draw(widget: impl Widget, width: u16, height: u16) -> Drawn {
    let mut terminal = Terminal::new(TestBackend::new(width, height)).unwrap();
    terminal
        .draw(|frame| frame.render_widget(widget, Rect::new(0, 0, width, height)))
        .unwrap();
    read(terminal.backend().buffer())
}

fn mark(style: Style) -> char {
    if style.add_modifier.contains(Modifier::REVERSED) {
        return 'C';
    }
    let bold = style.add_modifier.contains(Modifier::BOLD);
    let under = style.add_modifier.contains(Modifier::UNDERLINED);
    match (style.fg, bold, under) {
        (Some(Color::Magenta), true, true) => 'U',
        (Some(Color::Magenta), true, false) => 'A',
        (Some(Color::Magenta), false, _) => 'a',
        (Some(Color::DarkGray), false, true) => 'u',
        (Some(Color::DarkGray), false, false) => 'm',
        (Some(Color::Gray), false, false) => 'r',
        (Some(Color::Green), false, false) => 'g',
        (Some(Color::Red), false, false) => 'x',
        (Some(Color::Red), true, false) => 'X',
        (Some(Color::White), true, false) => 'I',
        (Some(Color::White), false, false) => 'i',
        (None | Some(Color::Reset), false, false) => ' ',
        _ => '?',
    }
}

pub fn read(buffer: &Buffer) -> Drawn {
    let width = usize::from(buffer.area.width).max(1);
    let mut text = Vec::new();
    let mut marks = Vec::new();
    for row in buffer.content.chunks(width) {
        let mut line = String::new();
        let mut line_marks = String::new();
        let mut hidden = 0;
        for cell in row {
            if hidden > 0 {
                hidden -= 1;
                continue;
            }
            let symbol = cell.symbol();
            let wide = symbol.width().max(1);
            hidden = wide - 1;
            line.push_str(symbol);
            let m = mark(cell.style());
            for _ in 0..wide {
                line_marks.push(m);
            }
        }
        text.push(line.trim_end().to_string());
        marks.push(line_marks.trim_end().to_string());
    }
    Drawn { text, marks }
}

pub fn show(drawn: &Drawn) -> String {
    let mut out = String::new();
    for (text, marks) in drawn.text.iter().zip(&drawn.marks) {
        out.push_str(&format!("|{text}\n|{marks}\n"));
    }
    out
}

pub fn widest(drawn: &Drawn) -> usize {
    drawn
        .text
        .iter()
        .map(|line| line.width())
        .max()
        .unwrap_or(0)
}
