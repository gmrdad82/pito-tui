use std::time::{Duration, Instant};

use crate::common::{STYLES, draw, show};
use pito_tui::footer::{
    Confirm, ConfirmKeys, Footer, Guard, Hint, Key, Mode, Notice, QuitGuard, WINDOW, Words,
};

const CTRL_C: Key = Key::Ctrl('c');

fn guard() -> QuitGuard {
    QuitGuard::new(
        pito_tui::footer::Wording::new("ctrl+c again to quit")
            .ask("2 imports running. Quit and leave them?"),
    )
}

fn at(start: Instant, ms: u64) -> Instant {
    start + Duration::from_millis(ms)
}

#[test]
fn twice_within_the_window_quits() {
    let start = Instant::now();
    let mut guard = guard();
    assert_eq!(guard.notice(), None);
    assert_eq!(guard.key(CTRL_C, start, false), Guard::Held);
    assert!(guard.armed());
    assert_eq!(guard.notice(), Some("ctrl+c again to quit"));
    assert_eq!(guard.deadline(), Some(start + WINDOW));
    assert_eq!(guard.key(CTRL_C, at(start, 1999), false), Guard::Quit);
}

#[test]
fn the_window_closes_and_rearms() {
    let start = Instant::now();
    let mut guard = guard();
    guard.key(CTRL_C, start, false);
    assert!(!guard.tick(at(start, 1999)));
    assert!(guard.armed());
    assert!(guard.tick(at(start, 2000)));
    assert!(!guard.armed());
    assert_eq!(guard.deadline(), None);
    assert_eq!(guard.key(CTRL_C, at(start, 2100), false), Guard::Held);
    let mut late = self::guard();
    late.key(CTRL_C, start, false);
    assert_eq!(late.key(CTRL_C, at(start, 2000), false), Guard::Held);
    assert!(late.armed());
}

#[test]
fn any_other_key_disarms_and_passes() {
    let start = Instant::now();
    let mut guard = guard();
    guard.key(CTRL_C, start, false);
    assert_eq!(guard.key(Key::Char('j'), at(start, 10), false), Guard::Pass);
    assert!(!guard.armed());
    assert_eq!(guard.key(CTRL_C, at(start, 20), false), Guard::Held);
}

#[test]
fn twice_mode_quits_even_while_busy() {
    let start = Instant::now();
    let mut guard = guard();
    guard.key(CTRL_C, start, true);
    assert_eq!(guard.key(CTRL_C, at(start, 5), true), Guard::Quit);
}

#[test]
fn ask_mode_asks_while_busy() {
    let start = Instant::now();
    let mut guard = guard().mode(Mode::Ask);
    guard.key(CTRL_C, start, false);
    assert_eq!(guard.key(CTRL_C, at(start, 5), false), Guard::Quit);
    let mut guard = self::guard().mode(Mode::Ask);
    assert_eq!(guard.key(CTRL_C, start, true), Guard::Held);
    assert_eq!(guard.key(CTRL_C, at(start, 5), true), Guard::Held);
    assert!(guard.asking().is_some());
    assert_eq!(guard.notice(), None);
    assert_eq!(guard.key(Key::Char('j'), at(start, 6), true), Guard::Held);
    assert_eq!(guard.key(Key::Right, at(start, 7), true), Guard::Held);
    assert_eq!(guard.key(Key::Enter, at(start, 8), true), Guard::Quit);
    assert!(guard.asking().is_none());
}

#[test]
fn the_question_cancels_on_no_esc_or_the_trigger() {
    let start = Instant::now();
    for cancel in [Key::Char('n'), Key::Esc, CTRL_C, Key::Enter] {
        let mut guard = guard().mode(Mode::Ask);
        guard.key(CTRL_C, start, true);
        guard.key(CTRL_C, start, true);
        assert_eq!(
            guard.key(cancel, at(start, 1), true),
            Guard::Held,
            "{cancel:?}"
        );
        assert!(guard.asking().is_none(), "{cancel:?}");
        assert!(!guard.armed());
    }
    let mut guard = guard().mode(Mode::Ask);
    guard.key(CTRL_C, start, true);
    guard.key(CTRL_C, start, true);
    assert_eq!(guard.key(Key::Char('y'), at(start, 1), true), Guard::Quit);
}

#[test]
fn once_mode_quits_at_once_on_any_trigger() {
    let start = Instant::now();
    let mut guard = guard()
        .mode(Mode::Once)
        .triggers(&[Key::Ctrl('c'), Key::Char('q')]);
    assert_eq!(guard.key(Key::Char('q'), start, true), Guard::Quit);
    assert_eq!(guard.key(CTRL_C, start, true), Guard::Quit);
    assert_eq!(guard.key(Key::Char('x'), start, true), Guard::Pass);
}

#[test]
fn window_triggers_and_confirm_keys_are_configurable() {
    let start = Instant::now();
    let mut guard = guard()
        .window(Duration::from_millis(500))
        .triggers(&[Key::Ctrl('q')])
        .mode(Mode::Ask)
        .confirm(Confirm::new().keys(ConfirmKeys::ENTER));
    assert_eq!(guard.key(CTRL_C, start, true), Guard::Pass);
    guard.key(Key::Ctrl('q'), start, true);
    assert_eq!(guard.deadline(), Some(at(start, 500)));
    guard.key(Key::Ctrl('q'), at(start, 499), true);
    assert_eq!(guard.key(Key::Char('y'), at(start, 500), true), Guard::Held);
    assert_eq!(guard.key(Key::Enter, at(start, 501), true), Guard::Quit);
}

#[test]
fn a_screen_can_pass_every_key_through() {
    let start = Instant::now();
    let mut guard = guard();
    let embedded = true;
    let mut sent = Vec::new();
    for key in [CTRL_C, CTRL_C, Key::Char('q')] {
        if embedded && key != Key::Ctrl(']') {
            sent.push(key);
            continue;
        }
        guard.key(key, start, false);
    }
    assert_eq!(sent.len(), 3);
    assert!(!guard.armed());
    guard.cancel();
}

#[test]
fn the_notice_and_the_question_draw_in_the_footer() {
    let start = Instant::now();
    let hints = [
        Hint::new("enter", "open"),
        Hint::new("ctrl+c", "twice quit").pinned(),
    ];
    let mut guard = guard().mode(Mode::Ask);
    guard.key(CTRL_C, start, true);
    let footer = Footer::new(&hints)
        .styles(STYLES)
        .notice(guard.notice().map(Notice::accent));
    let drawn = draw(footer, 40, 3);
    assert_eq!(
        drawn.text[1..],
        ["enter open · ctrl+c twice quit", "ctrl+c again to quit"],
        "\n{}",
        show(&drawn)
    );
    assert_eq!(drawn.marks[2], "A".repeat(20));
    guard.key(CTRL_C, at(start, 1), true);
    let words = Words::new("Quit", "Stay").hint("y/n choose · enter accept · esc cancel");
    let footer = Footer::new(&hints).styles(STYLES).confirm(guard.bar(words));
    let drawn = draw(footer, 60, 3);
    assert_eq!(
        drawn.text[1..],
        [
            "2 imports running. Quit and leave them?  Quit   ▸ Stay",
            "  y/n choose · enter accept · esc cancel",
        ],
        "\n{}",
        show(&drawn)
    );
    guard.wording_mut().ask = "1 import running. Quit and leave it?".into();
    let footer = Footer::new(&hints).styles(STYLES).confirm(guard.bar(words));
    assert_eq!(
        draw(footer, 60, 3).text[1],
        "1 import running. Quit and leave it?  Quit   ▸ Stay"
    );
}

#[test]
fn crossterm_keys_convert() {
    use crossterm::event::{KeyCode, KeyEvent, KeyEventKind, KeyModifiers};
    let key = |code, modifiers| Key::from(KeyEvent::new(code, modifiers));
    assert_eq!(key(KeyCode::Char('c'), KeyModifiers::CONTROL), CTRL_C);
    assert_eq!(key(KeyCode::Char('y'), KeyModifiers::NONE), Key::Char('y'));
    assert_eq!(key(KeyCode::Enter, KeyModifiers::NONE), Key::Enter);
    assert_eq!(key(KeyCode::Esc, KeyModifiers::NONE), Key::Esc);
    assert_eq!(key(KeyCode::Left, KeyModifiers::NONE), Key::Left);
    let mut release = KeyEvent::new(KeyCode::Char('c'), KeyModifiers::CONTROL);
    release.kind = KeyEventKind::Release;
    assert_eq!(Key::from(release), Key::Other);
}
