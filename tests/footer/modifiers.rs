use pito_tui::footer::{Confirm, ConfirmBar, Footer, Hint, Notice, Segment, Styles, Words};
use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
    widgets::Widget,
};

const DIM: Style = Style::new().fg(Color::Gray).add_modifier(Modifier::DIM);
const STYLES: Styles = Styles::new()
    .accent(Style::new().fg(Color::Magenta))
    .muted(Style::new().fg(Color::DarkGray))
    .ink(Style::new().fg(Color::White))
    .rule(DIM);
const HINTS: [Hint; 3] = [
    Hint::new("↑↓", "move"),
    Hint::new("enter", "open").rank(1),
    Hint::new("ctrl+c", "twice quit").pinned(),
];
const WORDS: Words = Words::new("Yes", "No").hint("y/n choose · enter accept");

fn render(widget: impl Widget, width: u16, height: u16) -> Buffer {
    let mut buf = Buffer::empty(Rect::new(0, 0, width, height));
    widget.render(buf.area, &mut buf);
    buf
}

fn only_rules_dim(buf: &Buffer, rules: &[u16]) {
    for y in 0..buf.area.height {
        let rule = rules.contains(&y);
        for x in 0..buf.area.width {
            let cell = &buf[(x, y)];
            assert_eq!(
                cell.modifier.contains(Modifier::DIM),
                rule,
                "({x}, {y}) {:?}",
                cell.symbol()
            );
            assert_eq!(cell.symbol() == "─", rule, "({x}, {y})");
        }
    }
}

#[test]
fn hints_and_a_notice_under_a_dim_rule_keep_their_own_style() {
    for indent in [0, 2] {
        let footer = Footer::new(&HINTS)
            .styles(STYLES)
            .indent(indent)
            .notice(Some(Notice::new("Saved", pito_tui::footer::Tone::Ink)));
        only_rules_dim(&render(footer, 50, 3), &[0]);
        only_rules_dim(&render(footer.wrap(true), 20, 4), &[0]);
        let over = footer.notice(Some(
            Notice::legend("a notice far too long for one row").over(),
        ));
        only_rules_dim(&render(over, 20, 3), &[0]);
    }
}

#[test]
fn a_confirm_under_a_dim_rule_keeps_its_own_style() {
    let confirm = Confirm::new();
    let bar = ConfirmBar::new("Remove item 3?", &confirm, WORDS).styles(STYLES);
    only_rules_dim(&render(bar, 50, 3), &[0]);
    only_rules_dim(&render(bar.rule(false).inline(true), 60, 1), &[]);
    let footer = Footer::new(&HINTS)
        .styles(STYLES)
        .indent(3)
        .confirm(Some(bar));
    only_rules_dim(&render(footer, 50, 3), &[0]);
}

#[test]
fn status_segments_keep_their_own_style() {
    let segments = [
        Segment::text("main", Style::new().fg(Color::Green)),
        Segment::hints(&HINTS),
        Segment::text("12 items", Style::new()).right(),
    ];
    let footer = Footer::status(&segments).styles(STYLES).rule(true);
    for width in [20, 60] {
        only_rules_dim(&render(footer, width, 2), &[0]);
    }
}

#[test]
fn crossterm_named_keys_map_to_their_key_and_alt_or_other_modifiers_win() {
    use crossterm::event::{KeyCode, KeyEvent, KeyEventKind, KeyModifiers as M};
    use pito_tui::footer::Key;
    let key = |code, modifiers| Key::from(KeyEvent::new(code, modifiers));
    let table = [
        (KeyCode::Tab, M::NONE, Key::Tab),
        (KeyCode::Tab, M::SHIFT, Key::BackTab),
        (KeyCode::BackTab, M::NONE, Key::BackTab),
        (KeyCode::BackTab, M::SHIFT, Key::BackTab),
        (KeyCode::Backspace, M::NONE, Key::Backspace),
        (KeyCode::Backspace, M::SHIFT, Key::Backspace),
        (KeyCode::Tab, M::CONTROL, Key::Tab),
        (KeyCode::Tab, M::CONTROL | M::SHIFT, Key::BackTab),
        (KeyCode::Backspace, M::CONTROL, Key::Backspace),
        (KeyCode::Right, M::NONE, Key::Right),
        (KeyCode::Up, M::NONE, Key::Up),
        (KeyCode::Down, M::NONE, Key::Down),
        (KeyCode::End, M::NONE, Key::End),
        (KeyCode::F(5), M::NONE, Key::Other),
        (KeyCode::F(1), M::SHIFT, Key::Other),
        (KeyCode::Insert, M::NONE, Key::Other),
        (KeyCode::Tab, M::ALT, Key::Other),
        (KeyCode::Tab, M::ALT | M::SHIFT, Key::Other),
        (KeyCode::BackTab, M::ALT, Key::Other),
        (KeyCode::Backspace, M::ALT, Key::Other),
        (KeyCode::Right, M::ALT, Key::Other),
        (KeyCode::Up, M::SUPER, Key::Other),
        (KeyCode::Down, M::HYPER, Key::Other),
        (KeyCode::End, M::META, Key::Other),
        (KeyCode::Enter, M::SUPER, Key::Other),
        (KeyCode::Tab, M::CONTROL | M::ALT, Key::Other),
    ];
    for (code, modifiers, want) in table {
        assert_eq!(key(code, modifiers), want, "{code:?} with {modifiers:?}");
    }
    let mut release = KeyEvent::new(KeyCode::Tab, M::NONE);
    release.kind = KeyEventKind::Release;
    assert_eq!(Key::from(release), Key::Other);
    let mut repeat = KeyEvent::new(KeyCode::Down, M::NONE);
    repeat.kind = KeyEventKind::Repeat;
    assert_eq!(Key::from(repeat), Key::Down);
}
