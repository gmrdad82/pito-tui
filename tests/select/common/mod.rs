use std::cell::RefCell;
use std::rc::Rc;
use std::sync::LazyLock;

use pito_tui::crossterm::event::{
    KeyCode, KeyEvent, KeyModifiers, MouseButton, MouseEvent, MouseEventKind,
};
use pito_tui::header::NavKeys;
use pito_tui::keys::{Keymap, Mode, Now, bind};
use pito_tui::ratatui::{
    Frame,
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
    text::Line,
};
use pito_tui::select::Clip;
use pito_tui::{Cx, Palette, Screen, Tui, Words, dump};

pub const SIZE: (u16, u16) = (80, 24);
pub const DESK: Mode = Mode("desk");

pub static KEYS: LazyLock<Keymap<u8>> = LazyLock::new(|| {
    Keymap::new()
        .modes(&[DESK])
        .group("Desk", [bind("x", "sweeps the floor", 0).on(DESK)])
});

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Sent {
    Osc(String),
    Command(String),
}

#[derive(Clone, Default)]
pub struct Sink(pub Rc<RefCell<Vec<Sent>>>);

impl Sink {
    pub fn sent(&self) -> Vec<Sent> {
        self.0.borrow().clone()
    }
}

impl Clip for Sink {
    fn osc(&mut self, sequence: &str) {
        self.0.borrow_mut().push(Sent::Osc(sequence.to_string()));
    }

    fn command(&mut self, text: &str) {
        self.0.borrow_mut().push(Sent::Command(text.to_string()));
    }
}

#[derive(Default)]
pub struct Probe {
    pub drags: bool,
    pub fill: bool,
    pub mice: Vec<MouseEventKind>,
    pub keys: Vec<KeyCode>,
}

pub const LINES: [&str; 3] = ["alpha beta", "gamma delta", "epsilon"];

impl Screen<()> for Probe {
    fn draw(&mut self, frame: &mut Frame, area: Rect, _palette: &Palette) {
        let buffer = frame.buffer_mut();
        if self.fill {
            let row = "x".repeat(usize::from(area.width));
            for y in area.top()..area.bottom() {
                buffer.set_string(area.x, y, &row, Style::new());
            }
            return;
        }
        for (at, line) in (area.y..area.bottom()).zip(LINES) {
            buffer.set_string(area.x, at, line, Style::new());
        }
    }

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, ()>) {
        self.keys.push(key.code);
        if key.code == KeyCode::Char('m') {
            cx.alert(1, "Note", [Line::from("modal body text")]);
        }
    }

    fn mouse(&mut self, mouse: MouseEvent, _area: Rect, _cx: &mut Cx<'_, ()>) {
        self.mice.push(mouse.kind);
    }

    fn drags(&self) -> bool {
        self.drags
    }
}

pub fn app(probe: Probe, sink: &Sink, words: Words) -> Tui<()> {
    let mut tui = Tui::new("probe", "1.0.0", Color::Blue)
        .nav_keys(NavKeys::NONE)
        .words(words)
        .clipboard(sink.clone())
        .screen("Main", probe);
    tui.start();
    tui
}

pub fn plain(sink: &Sink) -> Tui<()> {
    app(Probe::default(), sink, Words::new())
}

pub fn keyed(sink: &Sink) -> Tui<()> {
    let mut tui = Tui::new("probe", "1.0.0", Color::Blue)
        .nav_keys(NavKeys::NONE)
        .clipboard(sink.clone())
        .screen("Main", Probe::default())
        .keymap(&KEYS)
        .keys_now(Now::new(DESK))
        .still_sheet(true);
    tui.start();
    tui
}

pub fn mouse(kind: MouseEventKind, (column, row): (u16, u16)) -> MouseEvent {
    MouseEvent {
        kind,
        column,
        row,
        modifiers: KeyModifiers::NONE,
    }
}

pub fn shifted(kind: MouseEventKind, at: (u16, u16)) -> MouseEvent {
    MouseEvent {
        modifiers: KeyModifiers::SHIFT,
        ..mouse(kind, at)
    }
}

pub fn down(at: (u16, u16)) -> MouseEvent {
    mouse(MouseEventKind::Down(MouseButton::Left), at)
}

pub fn drag(at: (u16, u16)) -> MouseEvent {
    mouse(MouseEventKind::Drag(MouseButton::Left), at)
}

pub fn up(at: (u16, u16)) -> MouseEvent {
    mouse(MouseEventKind::Up(MouseButton::Left), at)
}

pub fn ch(c: char) -> KeyEvent {
    KeyEvent::new(KeyCode::Char(c), KeyModifiers::NONE)
}

pub fn sweep(tui: &mut Tui<()>, from: (u16, u16), to: (u16, u16)) {
    tui.mouse(down(from));
    tui.mouse(drag(to));
    tui.mouse(up(to));
}

pub fn spot(buffer: &Buffer, needle: &str) -> (u16, u16) {
    for y in 0..buffer.area.height {
        let line: String = (0..buffer.area.width)
            .map(|x| buffer[(x, y)].symbol())
            .collect();
        if let Some(at) = line.find(needle) {
            let column = u16::try_from(line[..at].chars().count()).unwrap_or(u16::MAX);
            return (column, y);
        }
    }
    panic!("{needle:?} is not on screen:\n{}", dump::text(buffer));
}

pub fn ends(buffer: &Buffer, needle: &str) -> ((u16, u16), (u16, u16)) {
    let (column, row) = spot(buffer, needle);
    let wide = u16::try_from(needle.chars().count()).unwrap_or(u16::MAX);
    ((column, row), (column + wide - 1, row))
}

pub fn reversed(buffer: &Buffer, (column, row): (u16, u16)) -> bool {
    buffer[(column, row)].modifier.contains(Modifier::REVERSED)
}

pub fn heard(tui: &Tui<()>) -> Vec<MouseEventKind> {
    tui.find::<Probe>()
        .map(|probe| probe.mice.clone())
        .unwrap_or_default()
}
