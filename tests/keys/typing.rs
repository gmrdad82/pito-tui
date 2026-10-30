use pito_tui::crossterm::event::{KeyCode, KeyEvent};
use pito_tui::ratatui::{
    Frame,
    layout::Rect,
    style::{Color, Style},
};
use pito_tui::{Cx, Palette, Screen, Tui, dump};

use crate::common::{KEYS, RAIL, ch, code, keyed};
use pito_tui::header::NavKeys;
use pito_tui::keys::Now;

const SMALL: (u16, u16) = (80, 24);
const HELP: &str = "? help";

#[derive(Default)]
struct Carry {
    carrying: bool,
    lets: bool,
    keys: Vec<KeyCode>,
}

impl Screen<()> for Carry {
    fn draw(&mut self, frame: &mut Frame, area: Rect, _palette: &Palette) {
        frame
            .buffer_mut()
            .set_string(area.x, area.y, "board", Style::new());
    }

    fn key(&mut self, key: KeyEvent, _cx: &mut Cx<'_, ()>) {
        self.keys.push(key.code);
        match key.code {
            KeyCode::Char('c') => self.carrying = true,
            KeyCode::Enter if self.carrying => self.carrying = false,
            _ => {}
        }
    }

    fn typing(&self) -> bool {
        self.carrying
    }

    fn lets_help(&self) -> bool {
        self.lets
    }
}

fn carry(lets: bool) -> Tui<()> {
    let screen = Carry {
        lets,
        ..Carry::default()
    };
    let mut tui = Tui::new("keys", "2.2.0", Color::Cyan)
        .nav_keys(NavKeys::NONE)
        .screen("Board", screen)
        .keymap(&KEYS)
        .keys_now(Now::new(RAIL))
        .still_sheet(true);
    tui.start();
    tui
}

fn footer(tui: &mut Tui<()>) -> String {
    let shown = dump::text(&tui.frame(SMALL.0, SMALL.1));
    shown.lines().last().unwrap_or_default().to_string()
}

fn heard(tui: &Tui<()>) -> Vec<KeyCode> {
    tui.find::<Carry>()
        .map(|screen| screen.keys.clone())
        .unwrap_or_default()
}

#[test]
fn a_typing_screen_hides_the_help_hint_and_takes_the_question_mark() {
    let mut tui = carry(false);
    assert!(footer(&mut tui).contains(HELP), "{}", footer(&mut tui));
    tui.key(ch('c'));
    let shown = footer(&mut tui);
    assert!(!shown.contains(HELP), "{shown}");
    assert!(shown.contains("right keeps"), "{shown}");
    tui.key(ch('?'));
    assert!(!tui.sheet().is_open());
    assert_eq!(heard(&tui), [KeyCode::Char('c'), KeyCode::Char('?')]);
    assert!(!footer(&mut tui).contains(HELP));
}

#[test]
fn the_help_hint_returns_once_the_screen_stops_typing() {
    let mut tui = carry(false);
    tui.key(ch('c'));
    assert!(!footer(&mut tui).contains(HELP));
    tui.key(code(KeyCode::Enter));
    let shown = footer(&mut tui);
    assert!(shown.contains(HELP), "{shown}");
    tui.key(ch('?'));
    assert!(tui.sheet().is_open());
    assert_eq!(heard(&tui), [KeyCode::Char('c'), KeyCode::Enter]);
}

#[test]
fn a_typing_screen_that_lets_help_through_opens_the_sheet_on_question_mark() {
    let mut tui = carry(true);
    tui.key(ch('c'));
    let shown = footer(&mut tui);
    assert!(shown.contains(HELP), "{shown}");
    tui.key(ch('?'));
    assert!(tui.sheet().is_open());
    let open = dump::text(&tui.frame(SMALL.0, SMALL.1));
    assert!(
        open.contains("Elsewhere") || open.contains("keeps"),
        "{open}"
    );
    tui.key(code(KeyCode::Right));
    tui.key(code(KeyCode::Esc));
    assert!(!tui.sheet().is_open());
    tui.key(code(KeyCode::Up));
    tui.key(ch('x'));
    assert_eq!(
        heard(&tui),
        [
            KeyCode::Char('c'),
            KeyCode::Right,
            KeyCode::Up,
            KeyCode::Char('x')
        ]
    );
    assert!(footer(&mut tui).contains(HELP));
    tui.key(ch('?'));
    assert!(tui.sheet().is_open());
    tui.key(ch('?'));
    assert!(!tui.sheet().is_open());
    assert_eq!(heard(&tui).len(), 4);
}

#[test]
fn letting_help_through_changes_nothing_without_a_keymap() {
    let screen = Carry {
        lets: true,
        ..Carry::default()
    };
    let mut tui = Tui::new("keys", "2.2.0", Color::Cyan)
        .nav_keys(NavKeys::NONE)
        .screen("Board", screen);
    tui.start();
    tui.key(ch('c'));
    tui.key(ch('?'));
    assert!(!tui.sheet().is_open());
    assert_eq!(heard(&tui), [KeyCode::Char('c'), KeyCode::Char('?')]);
}

#[test]
fn a_gated_shell_shows_no_help_hint() {
    let mut tui = keyed(&KEYS).gate(
        Carry {
            lets: true,
            ..Carry::default()
        },
        true,
    );
    tui.start();
    let shown = footer(&mut tui);
    assert!(!shown.contains(HELP), "{shown}");
    tui.key(ch('?'));
    assert!(!tui.sheet().is_open());
}
