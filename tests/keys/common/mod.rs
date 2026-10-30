use std::sync::LazyLock;

use pito_tui::crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use pito_tui::header::NavKeys;
use pito_tui::keys::{Chord, Keymap, Mode, Now, When, bind};
use pito_tui::ratatui::{
    Frame,
    layout::Rect,
    style::{Color, Style},
};
use pito_tui::{Cx, Modal, Palette, Pick, Screen, Tui, dump};

pub const RAIL: Mode = Mode("clips");
pub const CUT: Mode = Mode("cut");
pub const PLAYING: When = When("while the cut plays");
pub const WIDE: (u16, u16) = (200, 40);

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Act {
    Choose,
    Keep,
    Drop,
    Play,
    Stop,
    Switch,
    Undo,
}

pub fn keymap() -> Keymap<Act> {
    Keymap::new()
        .modes(&[RAIL, CUT])
        .group(
            "Rail",
            [
                bind("right", "keeps", Act::Keep).on(RAIL),
                bind("left", "drops", Act::Drop).on(RAIL).rank(1),
            ],
        )
        .group(
            "Cut",
            [bind("left right", "choose", Act::Choose)
                .on(CUT)
                .shown("left and right")
                .rank(1)],
        )
        .group(
            "Player",
            [
                bind("space esc", "stops it", Act::Stop)
                    .modes(&[RAIL, CUT])
                    .when(PLAYING)
                    .shown("space or esc"),
                bind("space", "plays", Act::Play).modes(&[RAIL, CUT]),
            ],
        )
        .group(
            "Edit",
            [
                bind("tab", "switches", Act::Switch).rank(2),
                bind("ctrl+z", "undoes", Act::Undo)
                    .rank(3)
                    .note("as often as you like"),
            ],
        )
}

pub static KEYS: LazyLock<Keymap<Act>> = LazyLock::new(keymap);

pub static BIG: LazyLock<Keymap<Act>> = LazyLock::new(|| {
    let binds = (0..60).map(|at| {
        let does: &'static str = format!("does {at:02}").leak();
        bind("f1", does, Act::Keep).on(RAIL).quiet()
    });
    Keymap::new().group("Many", binds.collect::<Vec<_>>())
});

#[derive(Default)]
pub struct Probe {
    pub keys: Vec<KeyCode>,
    pub cut: bool,
    pub playing: bool,
    pub input: bool,
    pub pastes: Vec<String>,
}

impl Probe {
    pub fn now(&self) -> Now<'static> {
        let now = Now::new(if self.cut { CUT } else { RAIL });
        if self.playing {
            now.when(&[PLAYING])
        } else {
            now
        }
    }
}

impl Screen<()> for Probe {
    fn draw(&mut self, frame: &mut Frame, area: Rect, _palette: &Palette) {
        frame
            .buffer_mut()
            .set_string(area.x, area.y, "body", Style::new());
    }

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, ()>) {
        self.keys.push(key.code);
        match key.code {
            KeyCode::Char('m') => cx.open(Modal::new(4, "Rename")),
            KeyCode::Char('p') => cx.pick(Pick::new("Choose", ["alpha", "beta"])),
            KeyCode::Char('i') => self.input = true,
            KeyCode::Char('h') => cx.toggle_sheet(),
            _ => {}
        }
        let act = Chord::from_crossterm(&key).and_then(|chord| KEYS.action(self.now(), chord));
        match act {
            Some(Act::Switch) => self.cut = !self.cut,
            Some(Act::Play) => self.playing = true,
            Some(Act::Stop) => self.playing = false,
            _ => {}
        }
        cx.keys_now(self.now());
    }

    fn paste(&mut self, text: &str, _cx: &mut Cx<'_, ()>) {
        self.pastes.push(text.to_string());
    }

    fn typing(&self) -> bool {
        self.input
    }
}

pub fn bare() -> Tui<()> {
    Tui::new("keys", "2.1.0", Color::Cyan)
        .nav_keys(NavKeys::NONE)
        .screen("Clips", Probe::default())
}

pub fn keyed(keymap: &'static Keymap<Act>) -> Tui<()> {
    bare().keymap(keymap).keys_now(Now::new(RAIL)).still(true)
}

pub fn started(mut tui: Tui<()>) -> Tui<()> {
    tui.start();
    tui
}

pub fn code(code: KeyCode) -> KeyEvent {
    KeyEvent::new(code, KeyModifiers::NONE)
}

pub fn ch(c: char) -> KeyEvent {
    code(KeyCode::Char(c))
}

pub fn ctrl(c: char) -> KeyEvent {
    KeyEvent::new(KeyCode::Char(c), KeyModifiers::CONTROL)
}

pub fn typed(tui: &mut Tui<()>, text: &str) {
    for c in text.chars() {
        tui.key(ch(c));
    }
}

pub fn text(tui: &mut Tui<()>, (width, height): (u16, u16)) -> String {
    dump::text(&tui.frame(width, height))
}

pub fn heard(tui: &Tui<()>) -> Vec<KeyCode> {
    tui.find::<Probe>()
        .map(|probe| probe.keys.clone())
        .unwrap_or_default()
}
