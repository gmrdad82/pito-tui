use std::time::{Duration, Instant};

use crate::common::{STYLES, draw};
use pito_tui::footer::{
    Answer, Confirm, ConfirmBar, ConfirmKeys, Edit, Footer, Guard, Hint, Input, InputBar, Key,
    Notice, QuitGuard, Styles, Wording, Words,
};
use ratatui::{
    buffer::Buffer,
    layout::{Position, Rect},
    style::{Color, Modifier, Style},
    widgets::Widget,
};

fn typed(text: &str) -> Input {
    let mut input = Input::new();
    for c in text.chars() {
        input.key(Key::Char(c));
    }
    input
}

#[test]
fn a_bare_carriage_return_is_a_line_break_in_a_paste() {
    let mut input = Input::new();
    assert_eq!(input.paste("foo\rbar\nbaz\r\nqux"), Edit::Changed);
    assert_eq!(input.value(), "foo bar baz qux");
    assert_eq!(Input::new().with("a\r\rb\r").value(), "a b");
}

#[test]
fn control_characters_take_no_cell_and_do_not_glue_words() {
    let row = |text: &str, width: u16| {
        let footer = Footer::new(&[])
            .rule(false)
            .notice(Some(Notice::legend(text)));
        draw(footer, width, 1).text.remove(0)
    };
    assert_eq!(
        row("error: build failed\nsee the log", 40),
        "error: build failed see the log"
    );
    assert_eq!(row("a\r\nb\tc", 10), "a b c");
    assert_eq!(row("line one\nline2", 12), "line one…");
    assert_eq!(row("abc\n", 3), "abc");
    assert_eq!(row("\nabc", 3), "abc");
}

#[test]
fn an_over_notice_with_line_breaks_wraps_like_spaces() {
    const HINTS: [Hint; 2] = [Hint::new("↑↓", "move"), Hint::new("enter", "open")];
    let footer = Footer::new(&HINTS).rule(false).notice(Some(
        Notice::alert("error: build failed\nsee the log for the whole story").over(),
    ));
    let drawn = draw(footer, 40, footer.height(40));
    assert_eq!(
        drawn.text,
        ["error: build failed see the log for the", "whole story"]
    );
}

#[test]
fn a_cap_bounds_what_a_paste_and_typing_can_add() {
    let mut input = Input::new().limit(5);
    assert_eq!(input.paste("0123456789"), Edit::Changed);
    assert_eq!(input.value(), "01234");
    assert_eq!(input.key(Key::Char('x')), Edit::Held);
    assert_eq!(input.paste("zz"), Edit::Held);
    input.key(Key::Backspace);
    assert_eq!(input.key(Key::Char('x')), Edit::Changed);
    assert_eq!(input.value(), "0123x");
    input.set("abcdefghij");
    assert_eq!(input.value(), "abcde");
    assert_eq!(input.cursor(), 5);
    let capped = Input::new().with("abcdefghij").limit(3);
    assert_eq!(capped.value(), "abc");
    assert_eq!(capped.cursor(), 3);
    let unbounded = Input::new().with(&"x".repeat(100_000));
    assert_eq!(unbounded.value().len(), 100_000);
}

fn sweep_value() -> Input {
    Input::new().with("ab界c\u{306}d日本語efg hijk lmn\u{301}o pqr stu")
}

fn caret_column(buf: &Buffer, y: u16) -> Option<u16> {
    (0..buf.area.width).find(|x| buf[(*x, y)].modifier.contains(Modifier::REVERSED))
}

#[test]
fn the_caret_stays_in_view_and_cursor_agrees_with_the_drawing() {
    let mut input = sweep_value();
    input.key(Key::Home);
    let total = input.value().chars().count();
    for at in 0..=total {
        for width in 8..30u16 {
            for label in ["", "Key"] {
                let bar = InputBar::new(label, &input).rule(false).styles(STYLES);
                let area = Rect::new(0, 0, width, 1);
                let mut buf = Buffer::empty(area);
                bar.render(area, &mut buf);
                let spot = bar.cursor(area);
                if let Some(Position { x, y }) = spot {
                    assert!(x < width && y == 0, "{at} {width} {label}");
                    assert_eq!(caret_column(&buf, 0), Some(x), "{at} {width} {label}");
                }
            }
        }
        input.key(Key::Right);
    }
}

#[test]
fn a_huge_value_lays_out_without_overflow() {
    let huge = "x".repeat(70_000);
    let mut input = Input::new().with(&huge);
    for masked in [false, true] {
        input = input.masked(masked);
        for place in [Key::End, Key::Home] {
            input.key(place);
            let bar = InputBar::new("Key", &input).styles(STYLES);
            let area = Rect::new(0, 0, 40, 2);
            let mut buf = Buffer::empty(area);
            bar.render(area, &mut buf);
            let at = bar.cursor(area).expect("a caret");
            assert!(at.x < 40 && at.y == 1, "{at:?}");
            assert_eq!(caret_column(&buf, 1), Some(at.x));
        }
    }
    let wide = "日".repeat(40_000);
    let input = Input::new().with(&wide);
    let bar = InputBar::new("", &input).rule(false);
    let area = Rect::new(0, 0, 30, 1);
    let mut buf = Buffer::empty(area);
    bar.render(area, &mut buf);
    assert!(bar.cursor(area).is_some());
}

#[test]
fn a_long_value_renders_in_linear_time() {
    let long: String = (0..4_000)
        .map(|at| char::from(b'a' + (at % 26) as u8))
        .collect();
    for masked in [false, true] {
        let input = Input::new().masked(masked).with(&long);
        let bar = InputBar::new("Search", &input).styles(STYLES);
        let area = Rect::new(0, 0, 80, 3);
        let mut buf = Buffer::empty(area);
        let mut best = Duration::MAX;
        for _ in 0..30 {
            let started = Instant::now();
            bar.render(area, &mut buf);
            std::hint::black_box(bar.cursor(area));
            best = best.min(started.elapsed());
        }
        assert!(best < Duration::from_millis(50), "{masked}: {best:?}");
    }
}

#[test]
fn the_confirm_keys_are_the_apps() {
    const KEYS: ConfirmKeys = ConfirmKeys::HEY
        .toggle(&[Key::Char('j'), Key::Char('k')])
        .accept(&[Key::Char(' ')]);
    let mut confirm = Confirm::new().keys(KEYS);
    assert_eq!(confirm.key(Key::Tab), None);
    assert_eq!(confirm.key(Key::Left), None);
    assert!(!confirm.yes());
    assert_eq!(confirm.key(Key::Char('j')), None);
    assert!(confirm.yes());
    assert_eq!(confirm.key(Key::Enter), None);
    assert_eq!(confirm.key(Key::Char(' ')), Some(Answer::Yes));
    assert_eq!(confirm.key(Key::Char('k')), None);
    assert_eq!(confirm.key(Key::Char(' ')), Some(Answer::No));
    let none = ConfirmKeys::HEY.toggle(&[]).accept(&[]).choose(true);
    let mut still = Confirm::new().keys(none);
    assert_eq!(still.key(Key::Enter), None);
    assert_eq!(still.key(Key::Right), None);
    assert!(!still.yes());
    let mut default = Confirm::new();
    assert_eq!(default.key(Key::Char('l')), None);
    assert!(default.yes());
    assert_eq!(default.key(Key::Enter), Some(Answer::Yes));
}

#[test]
fn the_question_survives_a_narrow_width() {
    let confirm = Confirm::new();
    let words = Words::new("Yes", "No");
    let question = "Stop the import?";
    for width in 1..=60u16 {
        let bar = ConfirmBar::new(question, &confirm, words)
            .rule(false)
            .styles(STYLES);
        let row = draw(bar, width, 1).text.remove(0);
        let first = row.chars().next().expect("something is drawn");
        assert!(first == 'S' || first == '…', "{width}: {row:?}");
        if width >= 5 {
            assert!(row.starts_with('S'), "{width}: {row:?}");
        }
        assert!(
            !row.contains("Yes") || row.contains("▸ No"),
            "{width}: {row:?}"
        );
    }
    let row = |width| {
        let bar = ConfirmBar::new(question, &confirm, words).rule(false);
        draw(bar, width, 1).text.remove(0)
    };
    assert_eq!(row(12), "Stop the…");
    assert_eq!(row(44), "Stop the import?  Yes   ▸ No");
    assert!(row(30).ends_with("Yes   ▸ No"), "{:?}", row(30));
}

#[test]
fn a_footer_keeps_the_styles_of_the_bar_it_carries() {
    const BOLD: Style = Style::new().fg(Color::Green);
    let own = Styles::new().ink(BOLD).accent(BOLD).muted(BOLD).rule(BOLD);
    let confirm = Confirm::new();
    let words = Words::new("Yes", "No");
    let carried = |bar: ConfirmBar<'static>| {
        let footer = Footer::new(&[]).styles(STYLES).confirm(Some(bar));
        draw(footer, 40, footer.height(40))
    };
    let own_bar = ConfirmBar::new("Stop?", &confirm, words).styles(own);
    let drawn = carried(own_bar);
    assert!(drawn.marks[1].starts_with("?????"), "{:?}", drawn.marks);
    let plain = carried(ConfirmBar::new("Stop?", &confirm, words));
    assert!(plain.marks[1].starts_with("IIIII"), "{:?}", plain.marks);
    let input = Input::new().with("abc");
    let bar = InputBar::new("Key", &input).styles(own);
    let footer = Footer::new(&[]).styles(STYLES).input(Some(bar));
    let drawn = draw(footer, 40, footer.height(40));
    assert!(drawn.marks[1].starts_with("??? ggg"), "{:?}", drawn.marks);
    let bar = InputBar::new("Key", &input);
    let footer = Footer::new(&[]).styles(STYLES).input(Some(bar));
    let drawn = draw(footer, 40, footer.height(40));
    assert!(drawn.marks[1].starts_with("III iii"), "{:?}", drawn.marks);
}

#[test]
fn a_huge_word_never_overflows_a_width() {
    let huge = "x".repeat(70_000);
    let confirm = Confirm::new();
    let words = Words::new(&huge, &huge);
    let bar = ConfirmBar::new(&huge, &confirm, words).styles(STYLES);
    for width in [0, 1, 20, 100] {
        let _ = draw(bar, width, 3);
    }
    let hints = [Hint::new(&huge, &huge), Hint::new("a", "b").pinned()];
    let footer = Footer::new(&hints)
        .separator(&huge)
        .notice(Some(Notice::alert(&huge).over()));
    for width in [0, 1, 20, 100] {
        let rows = footer.height(width).max(1);
        let _ = draw(footer, width, rows);
        let _ = footer.shown("a", width);
    }
}

#[test]
fn a_window_that_never_ends_has_no_deadline_and_no_panic() {
    let start = Instant::now();
    let mut guard = QuitGuard::new(Wording::new("again")).window(Duration::MAX);
    assert_eq!(guard.deadline(), None);
    assert_eq!(guard.key(Key::Ctrl('c'), start, false), Guard::Held);
    assert!(guard.armed());
    assert_eq!(guard.deadline(), None);
    assert!(!guard.tick(start + Duration::from_secs(3_600)));
    assert_eq!(guard.notice(), Some("again"));
    assert_eq!(guard.key(Key::Ctrl('c'), start, false), Guard::Quit);
}

#[test]
fn alt_keys_are_not_plain_keys() {
    let mut input = typed("abc");
    assert_eq!(input.key(Key::Alt('b')), Edit::Pass);
    assert_eq!(input.value(), "abc");
    let mut confirm = Confirm::new();
    assert_eq!(confirm.key(Key::Alt('y')), None);
    assert!(!confirm.yes());
}

#[test]
fn crossterm_carries_alt_and_lets_altgr_through() {
    use crossterm::event::{KeyCode, KeyEvent, KeyModifiers as M};
    let key = |code, modifiers| Key::from(KeyEvent::new(code, modifiers));
    assert_eq!(key(KeyCode::Char('y'), M::ALT), Key::Alt('y'));
    assert_eq!(key(KeyCode::Char('B'), M::ALT | M::SHIFT), Key::Alt('B'));
    assert_eq!(key(KeyCode::Char('ț'), M::CONTROL | M::ALT), Key::Char('ț'));
    assert_eq!(key(KeyCode::Char('€'), M::CONTROL | M::ALT), Key::Char('€'));
    assert_eq!(key(KeyCode::Char('c'), M::CONTROL), Key::Ctrl('c'));
    assert_eq!(key(KeyCode::Backspace, M::ALT), Key::Other);
    assert_eq!(key(KeyCode::Left, M::ALT), Key::Other);
    assert_eq!(key(KeyCode::Char('y'), M::SUPER), Key::Other);
    assert_eq!(key(KeyCode::Char('y'), M::META), Key::Other);
    assert_eq!(key(KeyCode::Char('y'), M::HYPER), Key::Other);
    assert_eq!(key(KeyCode::Delete, M::NONE), Key::Delete);
    let mut input = Input::new();
    assert_eq!(
        input.key(key(KeyCode::Char('ț'), M::CONTROL | M::ALT)),
        Edit::Changed
    );
    assert_eq!(input.value(), "ț");
    assert_eq!(input.key(key(KeyCode::Char('b'), M::ALT)), Edit::Pass);
    let mut confirm = Confirm::new();
    assert_eq!(confirm.key(key(KeyCode::Char('y'), M::ALT)), None);
}

#[test]
fn an_over_notice_that_ends_on_a_space_stops_at_the_last_word() {
    let footer = Footer::new(&[])
        .rule(false)
        .notice(Some(Notice::legend("aaaa ").over()));
    assert_eq!(footer.height(4), 1);
    let drawn = draw(footer, 4, 3);
    assert_eq!(drawn.text, ["aaaa", "", ""]);
}

#[test]
fn an_over_notice_whose_first_glyph_is_wider_than_the_row_draws_nothing_until_it_is_the_last_row() {
    let footer = Footer::new(&[])
        .rule(false)
        .notice(Some(Notice::legend("漢字").over()));
    assert_eq!(draw(footer, 1, 3).text, ["", "", ""]);
    assert_eq!(draw(footer, 1, 1).text, ["…"]);
}

#[test]
fn an_area_taller_than_the_buffer_draws_only_the_rows_inside_it() {
    let over = Footer::new(&[])
        .rule(false)
        .notice(Some(Notice::legend("aaaa bbbb cccc").over()));
    let mut buf = Buffer::empty(Rect::new(0, 0, 6, 1));
    over.render(Rect::new(0, 0, 6, 3), &mut buf);
    assert_eq!(crate::common::read(&buf).text, ["aaaa"]);
    const HINTS: [Hint; 2] = [Hint::new("a", "alpha"), Hint::new("b", "bravo")];
    let wrapped = Footer::new(&HINTS).rule(false).wrap(true);
    assert_eq!(wrapped.height(12), 2);
    assert_eq!(draw(wrapped, 12, 2).text, ["a alpha", "b bravo"]);
    let mut buf = Buffer::empty(Rect::new(0, 0, 12, 1));
    wrapped.render(Rect::new(0, 0, 12, 2), &mut buf);
    assert_eq!(crate::common::read(&buf).text, ["a alpha"]);
}

#[test]
fn an_area_outside_the_buffer_leaves_it_untouched() {
    const QUIT: [Hint; 1] = [Hint::new("q", "quit")];
    let footer = Footer::new(&QUIT).rule(false).version("app", "1.0");
    assert_eq!(draw(footer, 20, 1).text, ["q quit      app v1.0"]);
    let empty = Buffer::empty(Rect::new(0, 0, 20, 1));
    let mut buf = empty.clone();
    footer.render(Rect::new(0, 2, 20, 1), &mut buf);
    assert_eq!(buf, empty);
    footer.rule(true).render(Rect::new(0, 2, 20, 2), &mut buf);
    footer
        .notice(Some(Notice::legend("far too long for this row").over()))
        .render(Rect::new(0, 2, 10, 3), &mut buf);
    assert_eq!(buf, empty);
}

#[test]
fn a_line_break_that_lands_on_the_edge_of_a_cut_notice_is_not_drawn() {
    let footer = Footer::new(&[])
        .rule(false)
        .notice(Some(Notice::legend("ab\ncd")));
    let drawn = draw(footer, 3, 1);
    assert_eq!(drawn.text, ["ab…"]);
    assert_eq!(draw(footer, 5, 1).text, ["ab cd"]);
    assert_eq!(draw(footer, 4, 1).text, ["ab…"]);
}
