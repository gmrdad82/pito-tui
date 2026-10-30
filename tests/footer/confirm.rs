use crate::common::{STYLES, draw, show, widest};
use pito_tui::footer::{Answer, Confirm, ConfirmBar, ConfirmKeys, Footer, Hint, Key, Tone, Words};
use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
    widgets::Widget,
};

const WORDS: Words = Words::new("Yes", "No").hint("y/n choose · enter accept · esc cancel");

#[test]
fn the_default_is_no_and_the_bar_marks_it() {
    let confirm = Confirm::new();
    assert!(!confirm.yes());
    let bar = ConfirmBar::new("Stop the import?", &confirm, WORDS).styles(STYLES);
    assert_eq!(bar.height(), 3);
    let drawn = draw(bar, 50, 3);
    assert_eq!(
        drawn.text,
        [
            "─".repeat(50).as_str(),
            "Stop the import?  Yes   ▸ No",
            "  y/n choose · enter accept · esc cancel",
        ],
        "\n{}",
        show(&drawn)
    );
    assert_eq!(drawn.marks[1], "IIIIIIIIIIIIIIII  mmm   AAAA");
}

#[test]
fn moving_marks_yes() {
    let mut confirm = Confirm::new();
    assert_eq!(confirm.key(Key::Left), None);
    assert!(confirm.yes());
    let drawn = draw(
        ConfirmBar::new("Stop the import?", &confirm, WORDS).styles(STYLES),
        50,
        3,
    );
    assert_eq!(drawn.text[1], "Stop the import?  ▸ Yes   No");
    assert_eq!(drawn.marks[1], "IIIIIIIIIIIIIIII  AAAAA   mm");
}

#[test]
fn hey_keys_answer() {
    let answer = |keys: &[Key]| {
        let mut confirm = Confirm::new();
        keys.iter().find_map(|key| confirm.key(*key))
    };
    assert_eq!(answer(&[Key::Char('y')]), Some(Answer::Yes));
    assert_eq!(answer(&[Key::Char('Y')]), Some(Answer::Yes));
    assert_eq!(answer(&[Key::Char('n')]), Some(Answer::No));
    assert_eq!(answer(&[Key::Esc]), Some(Answer::No));
    assert_eq!(answer(&[Key::Enter]), Some(Answer::No));
    assert_eq!(answer(&[Key::Right, Key::Enter]), Some(Answer::Yes));
    assert_eq!(
        answer(&[Key::Char('l'), Key::Char('h'), Key::Enter]),
        Some(Answer::No)
    );
    assert_eq!(
        answer(&[Key::Tab, Key::BackTab, Key::Tab, Key::Enter]),
        Some(Answer::Yes)
    );
    assert_eq!(answer(&[Key::Char('x'), Key::Up, Key::Ctrl('c')]), None);
}

#[test]
fn accept_and_cancel_keys_are_the_apps() {
    const ROMANIAN: ConfirmKeys = ConfirmKeys::HEY
        .yes(&[Key::Char('d'), Key::Char('D')])
        .no(&[Key::Char('n'), Key::Char('N'), Key::Esc]);
    let mut confirm = Confirm::new().keys(ROMANIAN);
    assert_eq!(confirm.key(Key::Char('y')), None);
    assert_eq!(confirm.key(Key::Char('d')), Some(Answer::Yes));
    let mut enter = Confirm::new().keys(ConfirmKeys::ENTER);
    assert!(!enter.choosing());
    assert_eq!(enter.key(Key::Char('y')), None);
    assert_eq!(enter.key(Key::Right), None);
    assert!(!enter.yes());
    assert_eq!(enter.key(Key::Enter), Some(Answer::Yes));
    assert_eq!(enter.key(Key::Esc), Some(Answer::No));
    let mut start = Confirm::new().start(Answer::Yes);
    assert_eq!(start.key(Key::Enter), Some(Answer::Yes));
}

#[test]
fn every_word_is_the_apps() {
    let confirm = Confirm::new().keys(
        ConfirmKeys::HEY
            .yes(&[Key::Char('d')])
            .no(&[Key::Char('n'), Key::Esc]),
    );
    let words = Words::new("Da", "Nu").hint("d/n alege · enter acceptă · esc renunță");
    let drawn = draw(
        ConfirmBar::new("Ștergi înregistrarea?", &confirm, words),
        50,
        3,
    );
    assert_eq!(drawn.text[1], "Ștergi înregistrarea?  Da   ▸ Nu");
    assert_eq!(drawn.text[2], "  d/n alege · enter acceptă · esc renunță");
}

#[test]
fn the_inline_form_fits_a_notice_row() {
    let confirm = Confirm::new().keys(ConfirmKeys::ENTER);
    let words = Words::new("", "").hint("enter confirms · esc cancels");
    let bar = ConfirmBar::new("Reset the devices of item 7?", &confirm, words)
        .inline(true)
        .rule(false)
        .tone(Tone::Accent)
        .styles(STYLES);
    assert_eq!(bar.height(), 1);
    let drawn = draw(bar, 70, 1);
    assert_eq!(
        drawn.text[0],
        "Reset the devices of item 7?  enter confirms · esc cancels"
    );
    assert!(drawn.marks[0].starts_with(&"A".repeat(28)));
    let danger = ConfirmBar::new("Switch to production?", &Confirm::new(), WORDS)
        .tone(Tone::Alert)
        .styles(STYLES);
    assert!(draw(danger, 50, 3).marks[1].starts_with(&"X".repeat(21)));
}

#[test]
fn narrow_bars_clip_the_question_and_keep_the_choices() {
    let bar =
        ConfirmBar::new("Stop the import of every item?", &Confirm::new(), WORDS).styles(STYLES);
    let drawn = draw(bar, 24, 3);
    assert_eq!(drawn.text[1], "Stop the…     Yes   ▸ No");
    assert_eq!(drawn.text[2], "  y/n choose…");
    for width in 1..=60 {
        assert!(
            widest(&draw(bar, width, 3)) <= usize::from(width),
            "{width}"
        );
    }
}

#[test]
fn the_footer_carries_the_confirm_in_place_of_hints() {
    let hints = [Hint::new("enter", "open")];
    let confirm = Confirm::new();
    let footer = Footer::new(&hints)
        .styles(STYLES)
        .confirm(Some(ConfirmBar::new("Remove item 3?", &confirm, WORDS)));
    assert_eq!(footer.height(60), 3);
    assert!(!footer.shown("enter", 60));
    let drawn = draw(footer, 60, 3);
    assert_eq!(
        drawn.text[1..],
        [
            "Remove item 3?  Yes   ▸ No",
            "  y/n choose · enter accept · esc cancel"
        ]
    );
    assert_eq!(drawn.marks[0], "r".repeat(60));
}

#[test]
fn the_footer_insets_the_confirm_but_not_the_rule() {
    let confirm = Confirm::new();
    let footer = Footer::new(&[])
        .styles(STYLES)
        .indent(2)
        .confirm(Some(ConfirmBar::new("Remove item 3?", &confirm, WORDS)));
    assert_eq!(footer.height(60), 3);
    let drawn = draw(footer, 60, 3);
    assert_eq!(
        drawn.text[1..],
        [
            "  Remove item 3?  Yes   ▸ No",
            "    y/n choose · enter accept · esc cancel"
        ]
    );
    assert_eq!(drawn.marks[0], "r".repeat(60));
}

#[test]
fn a_good_tone_suits_a_confirm_too() {
    let styles = STYLES.good(Style::new().fg(Color::Green));
    let bar = ConfirmBar::new("Publish now?", &Confirm::new(), WORDS)
        .tone(Tone::Good)
        .styles(styles);
    let drawn = draw(bar, 50, 3);
    assert_eq!(drawn.text[1], "Publish now?  Yes   ▸ No");
    let mut buf = Buffer::empty(Rect::new(0, 0, 50, 3));
    bar.render(buf.area, &mut buf);
    assert_eq!(buf[(0, 1)].fg, Color::Green);
    assert_eq!(buf[(0, 1)].modifier, Modifier::BOLD);
}

#[test]
fn the_defaults_are_the_hey_keys_and_a_no_start() {
    assert_eq!(ConfirmKeys::default(), ConfirmKeys::HEY);
    assert_ne!(ConfirmKeys::default(), ConfirmKeys::ENTER);
    assert_eq!(Confirm::default(), Confirm::new());
    assert_ne!(Confirm::default(), Confirm::new().keys(ConfirmKeys::ENTER));
    let mut confirm = Confirm::default();
    assert!(!confirm.yes());
    assert!(confirm.choosing());
    assert_eq!(confirm.key(Key::Enter), Some(Answer::No));
    assert_eq!(confirm.key(Key::Tab), None);
    assert!(confirm.yes());
    assert_eq!(confirm.key(Key::Enter), Some(Answer::Yes));
    assert_eq!(confirm.key(Key::Esc), Some(Answer::No));
}

#[test]
fn a_bar_with_a_rule_in_one_row_draws_the_rule_alone() {
    let confirm = Confirm::new();
    let bar = ConfirmBar::new("Stop the import?", &confirm, WORDS).styles(STYLES);
    let drawn = draw(bar, 20, 1);
    assert_eq!(drawn.text, ["─".repeat(20)], "\n{}", show(&drawn));
    assert_eq!(drawn.marks, ["r".repeat(20)], "\n{}", show(&drawn));
    let mut buf = Buffer::empty(Rect::new(0, 0, 20, 3));
    bar.render(Rect::new(0, 1, 20, 1), &mut buf);
    for x in 0..20 {
        assert_eq!(buf[(x, 0)].symbol(), " ", "({x}, 0)");
        assert_eq!(buf[(x, 1)].symbol(), "─", "({x}, 1)");
        assert_eq!(buf[(x, 2)].symbol(), " ", "({x}, 2)");
    }
}
