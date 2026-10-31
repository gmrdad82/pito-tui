use std::env;
use std::io::{self, Write};
use std::process::{Command, Stdio};
use std::thread;

use crossterm::event::{KeyModifiers, MouseButton, MouseEvent, MouseEventKind};
use ratatui::buffer::{Buffer, Cell};
use ratatui::layout::{Position, Rect};
use ratatui::style::{Modifier, Style};
use unicode_width::UnicodeWidthStr;

use crate::copy::osc52;

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
enum State {
    #[default]
    Idle,
    Pressed,
    Dragging,
    Shown,
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct Selection {
    anchor: Position,
    head: Position,
    state: State,
    copy: Option<(Position, Position)>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Fed {
    Pass,
    Press,
    Click,
    Drag,
    Done,
}

impl Selection {
    pub const fn new() -> Self {
        Selection {
            anchor: Position::ORIGIN,
            head: Position::ORIGIN,
            state: State::Idle,
            copy: None,
        }
    }

    pub fn feed(&mut self, mouse: MouseEvent) -> Fed {
        let at = Position::new(mouse.column, mouse.row);
        match mouse.kind {
            MouseEventKind::Down(MouseButton::Left)
                if !mouse.modifiers.contains(KeyModifiers::SHIFT) =>
            {
                self.anchor = at;
                self.head = at;
                self.state = State::Pressed;
                Fed::Press
            }
            MouseEventKind::Down(_) => {
                self.clear();
                Fed::Pass
            }
            MouseEventKind::Drag(MouseButton::Left) => match self.state {
                State::Pressed if at == self.anchor => Fed::Pass,
                State::Pressed | State::Dragging => {
                    self.head = at;
                    self.state = State::Dragging;
                    Fed::Drag
                }
                State::Idle | State::Shown => Fed::Pass,
            },
            MouseEventKind::Up(MouseButton::Left) => match self.state {
                State::Pressed => {
                    self.clear();
                    Fed::Click
                }
                State::Dragging => {
                    self.head = at;
                    self.state = State::Shown;
                    self.copy = self.range();
                    Fed::Done
                }
                State::Idle | State::Shown => Fed::Pass,
            },
            _ => Fed::Pass,
        }
    }

    pub fn clear(&mut self) {
        self.state = State::Idle;
    }

    pub fn shown(&self) -> bool {
        matches!(self.state, State::Dragging | State::Shown)
    }

    pub fn dragging(&self) -> bool {
        self.state == State::Dragging
    }

    pub fn range(&self) -> Option<(Position, Position)> {
        if !self.shown() {
            return None;
        }
        let (anchor, head) = (self.anchor, self.head);
        Some(if (anchor.y, anchor.x) <= (head.y, head.x) {
            (anchor, head)
        } else {
            (head, anchor)
        })
    }

    pub fn take(&mut self, buffer: &Buffer) -> Option<String> {
        let (start, end) = self.copy.take()?;
        Some(read(buffer, start, end))
    }

    pub fn text(&self, buffer: &Buffer) -> String {
        match self.range() {
            Some((start, end)) => read(buffer, start, end),
            None => String::new(),
        }
    }

    pub fn paint(&self, buffer: &mut Buffer, style: Style) {
        let Some((start, end)) = self.range() else {
            return;
        };
        let area = buffer.area;
        for (y, from, to) in rows(start, end, area) {
            let mut x = area.left();
            while x < area.right() {
                let wide = span(&buffer[(x, y)]);
                if x.saturating_add(wide - 1) >= from && x <= to {
                    for cell in x..x.saturating_add(wide).min(area.right()) {
                        mark(&mut buffer[(cell, y)], style);
                    }
                }
                x = x.saturating_add(wide);
            }
        }
    }
}

fn rows(start: Position, end: Position, area: Rect) -> impl Iterator<Item = (u16, u16, u16)> {
    let top = start.y.max(area.top());
    let bottom = end.y.saturating_add(1).min(area.bottom());
    (top..bottom).map(move |y| {
        let from = if y == start.y { start.x } else { 0 };
        let to = if y == end.y { end.x } else { u16::MAX };
        (y, from, to)
    })
}

fn span(cell: &Cell) -> u16 {
    u16::try_from(cell.symbol().width())
        .unwrap_or(u16::MAX)
        .max(1)
}

fn read(buffer: &Buffer, start: Position, end: Position) -> String {
    let mut out = String::new();
    let area = buffer.area;
    for (row, (y, from, to)) in rows(start, end, area).enumerate() {
        if row > 0 {
            out.push('\n');
        }
        let begin = out.len();
        let mut x = area.left();
        while x < area.right() {
            let cell = &buffer[(x, y)];
            let wide = span(cell);
            if x.saturating_add(wide - 1) >= from && x <= to {
                out.push_str(cell.symbol());
            }
            x = x.saturating_add(wide);
        }
        let kept = out[begin..].trim_end_matches(' ').len();
        out.truncate(begin + kept);
    }
    out
}

fn mark(cell: &mut Cell, style: Style) {
    let flip = style.add_modifier.contains(Modifier::REVERSED)
        && cell.modifier.contains(Modifier::REVERSED);
    cell.set_style(style);
    if flip {
        cell.modifier.remove(Modifier::REVERSED);
    }
}

pub trait Clip {
    fn osc(&mut self, sequence: &str);

    fn command(&mut self, text: &str);
}

pub fn send(text: &str, sink: &mut dyn Clip) -> bool {
    if text.is_empty() {
        return false;
    }
    if let Some(sequence) = osc52(text) {
        sink.osc(&sequence);
    }
    sink.command(text);
    true
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct System {
    tool: Option<Option<Tool>>,
}

impl System {
    pub const fn new() -> Self {
        System { tool: None }
    }
}

impl Clip for System {
    fn osc(&mut self, sequence: &str) {
        let mut out = io::stdout();
        let _ = out
            .write_all(sequence.as_bytes())
            .and_then(|()| out.flush());
    }

    fn command(&mut self, text: &str) {
        if let Some(tool) = *self.tool.get_or_insert_with(Tool::find) {
            tool.spawn(text);
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Tool {
    pub program: &'static str,
    pub args: &'static [&'static str],
}

impl Tool {
    pub const PBCOPY: Tool = Tool {
        program: "pbcopy",
        args: &[],
    };
    pub const CLIP: Tool = Tool {
        program: "clip.exe",
        args: &[],
    };
    pub const WL_COPY: Tool = Tool {
        program: "wl-copy",
        args: &[],
    };
    pub const XCLIP: Tool = Tool {
        program: "xclip",
        args: &["-selection", "clipboard"],
    };
    pub const XSEL: Tool = Tool {
        program: "xsel",
        args: &["--clipboard", "--input"],
    };

    pub fn find() -> Option<Tool> {
        let set = |name: &str| env::var_os(name).is_some_and(|value| !value.is_empty());
        let path = env::var_os("PATH").unwrap_or_default();
        Tool::pick(
            env::consts::OS,
            set("WAYLAND_DISPLAY"),
            set("DISPLAY"),
            |program| env::split_paths(&path).any(|dir| dir.join(program).is_file()),
        )
    }

    pub fn pick(os: &str, wayland: bool, x11: bool, found: impl Fn(&str) -> bool) -> Option<Tool> {
        [
            (os == "macos", Tool::PBCOPY),
            (os == "windows", Tool::CLIP),
            (wayland, Tool::WL_COPY),
            (x11, Tool::XCLIP),
            (x11, Tool::XSEL),
        ]
        .into_iter()
        .find(|(fits, tool)| *fits && found(tool.program))
        .map(|(_, tool)| tool)
    }

    pub fn bytes(&self, text: &str) -> Vec<u8> {
        if *self != Tool::CLIP {
            return text.as_bytes().to_vec();
        }
        let mut out = Vec::with_capacity(2 + text.len() * 2);
        for unit in std::iter::once(0xfeff).chain(text.encode_utf16()) {
            out.extend_from_slice(&unit.to_le_bytes());
        }
        out
    }

    pub fn spawn(self, text: &str) {
        let bytes = self.bytes(text);
        let mut command = Command::new(self.program);
        command
            .args(self.args)
            .stdin(Stdio::piped())
            .stdout(Stdio::null())
            .stderr(Stdio::null());
        if self == Tool::PBCOPY {
            command.env("LC_CTYPE", "UTF-8");
        }
        #[cfg(windows)]
        {
            use std::os::windows::process::CommandExt;
            command.creation_flags(0x0800_0000);
        }
        thread::spawn(move || {
            let Ok(mut child) = command.spawn() else {
                return;
            };
            if let Some(mut input) = child.stdin.take() {
                let _ = input.write_all(&bytes);
            }
            let _ = child.wait();
        });
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn all(_: &str) -> bool {
        true
    }

    #[test]
    fn each_platform_picks_its_own_clipboard_command() {
        assert_eq!(Tool::pick("macos", false, false, all), Some(Tool::PBCOPY));
        assert_eq!(Tool::pick("windows", false, false, all), Some(Tool::CLIP));
        assert_eq!(Tool::pick("linux", true, true, all), Some(Tool::WL_COPY));
        assert_eq!(Tool::pick("linux", false, true, all), Some(Tool::XCLIP));
        assert_eq!(Tool::pick("freebsd", false, true, all), Some(Tool::XCLIP));
        assert_eq!(Tool::pick("linux", false, false, all), None);
    }

    #[test]
    fn a_command_missing_from_the_path_falls_to_the_next() {
        let only = |name: &'static str| move |program: &str| program == name;
        assert_eq!(
            Tool::pick("linux", true, true, only("xclip")),
            Some(Tool::XCLIP)
        );
        assert_eq!(
            Tool::pick("linux", true, true, only("xsel")),
            Some(Tool::XSEL)
        );
        assert_eq!(Tool::pick("linux", true, false, only("xsel")), None);
        assert_eq!(Tool::pick("macos", false, false, only("xclip")), None);
        assert_eq!(Tool::pick("linux", true, true, |_| false), None);
    }

    #[test]
    fn the_x11_commands_write_the_clipboard_not_the_primary_selection() {
        assert_eq!(Tool::XCLIP.args, ["-selection", "clipboard"]);
        assert_eq!(Tool::XSEL.args, ["--clipboard", "--input"]);
    }

    #[test]
    fn clip_exe_takes_utf_16_with_a_mark_and_the_rest_take_utf_8() {
        assert_eq!(Tool::CLIP.bytes("aé"), [0xff, 0xfe, b'a', 0, 0xe9, 0]);
        assert_eq!(Tool::WL_COPY.bytes("aé"), "aé".as_bytes());
        assert_eq!(Tool::PBCOPY.bytes(""), b"");
    }

    #[test]
    fn a_new_system_sink_looks_nothing_up_until_it_copies() {
        assert_eq!(System::new(), System::default());
        assert_eq!(System::new().tool, None);
    }
}
