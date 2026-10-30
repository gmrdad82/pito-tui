use crate::common::{STYLES, draw, show, widest};
use pito_tui::footer::{Edit, Footer, Hint, Input, InputBar, Key, Tone};
use ratatui::layout::{Position, Rect};

fn typed(text: &str) -> Input {
    let mut input = Input::new();
    for c in text.chars() {
        assert_eq!(input.key(Key::Char(c)), Edit::Changed);
    }
    input
}

fn keys(input: &mut Input, keys: &[Key]) {
    for key in keys {
        input.key(*key);
    }
}

#[test]
fn typing_inserts_at_the_cursor_and_moves_by_grapheme() {
    let mut input = typed("ața");
    assert_eq!(input.value(), "ața");
    assert_eq!(input.cursor(), 3);
    assert_eq!(input.key(Key::Left), Edit::Held);
    assert_eq!(input.key(Key::Left), Edit::Held);
    assert_eq!(input.cursor(), 1);
    input.key(Key::Char('ș'));
    assert_eq!(input.value(), "așța");
    assert_eq!(input.cursor(), 2);
    keys(&mut input, &[Key::Home, Key::Char('>')]);
    assert_eq!(input.value(), ">așța");
    keys(&mut input, &[Key::End, Key::Char('<')]);
    assert_eq!(input.value(), ">așța<");
    keys(
        &mut input,
        &[Key::Ctrl('a'), Key::Delete, Key::Ctrl('e'), Key::Backspace],
    );
    assert_eq!(input.value(), "așța");
    assert_eq!(input.key(Key::Right), Edit::Held);
    assert_eq!(input.cursor(), 4);
}

#[test]
fn combining_marks_and_wide_glyphs_move_and_delete_whole() {
    let mut input = Input::new().with("a\u{306}界b");
    assert_eq!(input.cursor(), 3);
    keys(&mut input, &[Key::Left, Key::Backspace]);
    assert_eq!(input.value(), "a\u{306}b");
    assert_eq!(input.cursor(), 1);
    keys(&mut input, &[Key::Home, Key::Delete]);
    assert_eq!(input.value(), "b");
    let mut input = Input::new().with("\u{306}");
    keys(&mut input, &[Key::Home, Key::Char('a')]);
    assert_eq!(input.value(), "a\u{306}");
    assert_eq!(input.cursor(), 1);
    assert_eq!(input.key(Key::Backspace), Edit::Changed);
    assert!(input.is_empty());
}

#[test]
fn ctrl_u_and_ctrl_w_cut_back_to_the_start_and_the_word() {
    let mut input = typed("copy the  key");
    assert_eq!(input.key(Key::Ctrl('w')), Edit::Changed);
    assert_eq!(input.value(), "copy the  ");
    input.key(Key::Ctrl('w'));
    assert_eq!(input.value(), "copy ");
    keys(&mut input, &[Key::Char('ș'), Key::Char('i'), Key::Left]);
    input.key(Key::Ctrl('u'));
    assert_eq!(input.value(), "i");
    assert_eq!(input.cursor(), 0);
    assert_eq!(input.key(Key::Ctrl('u')), Edit::Held);
    assert_eq!(input.key(Key::Ctrl('w')), Edit::Held);
    assert_eq!(input.key(Key::Backspace), Edit::Held);
}

#[test]
fn enter_and_esc_come_back_and_nothing_else_is_taken() {
    let mut input = typed("abc");
    assert_eq!(input.key(Key::Enter), Edit::Submit("abc".to_string()));
    assert_eq!(input.key(Key::Esc), Edit::Cancel);
    assert_eq!(input.value(), "abc");
    for key in [
        Key::Up,
        Key::Down,
        Key::Tab,
        Key::BackTab,
        Key::Ctrl('c'),
        Key::Char('\u{7}'),
        Key::Other,
    ] {
        assert_eq!(input.key(key), Edit::Pass, "{key:?}");
    }
    assert_eq!(input.value(), "abc");
    input.clear();
    assert!(input.is_empty());
    assert_eq!(input.cursor(), 0);
}

#[test]
fn a_paste_lands_at_the_cursor_on_one_line() {
    let mut input = typed("ab");
    input.key(Key::Left);
    assert_eq!(input.paste("x\r\ny\tz\u{1b}\n"), Edit::Changed);
    assert_eq!(input.value(), "ax y zb");
    assert_eq!(input.cursor(), 6);
    assert_eq!(input.paste("\n"), Edit::Held);
    assert_eq!(input.paste("日本"), Edit::Changed);
    assert_eq!(input.value(), "ax y z日本b");
    assert_eq!(input.cursor(), 8);
}

#[test]
fn a_masked_value_never_reaches_debug() {
    let mut input = Input::new().masked(true).with("sk-secret-value-1234");
    let debug = format!("{input:?} {:?}", input.key(Key::Enter));
    assert!(!debug.contains("secret"), "{debug}");
    assert!(debug.contains("<masked>"), "{debug}");
    let bar = InputBar::new("Key", &input);
    assert!(!format!("{bar:?}").contains("secret"));
    let plain = Input::new().with("visible");
    assert!(format!("{plain:?}").contains("visible"));
}

#[test]
fn the_bar_draws_the_label_value_and_caret() {
    let input = typed("ana");
    let bar = InputBar::new("Name", &input)
        .hint("enter save · esc cancel")
        .styles(STYLES);
    assert_eq!(bar.height(), 3);
    let drawn = draw(bar, 30, 3);
    assert_eq!(
        drawn.text,
        [
            "─".repeat(30).as_str(),
            "Name ana",
            "  enter save · esc cancel"
        ],
        "\n{}",
        show(&drawn)
    );
    assert_eq!(drawn.marks[1], "IIII iiiC");
    assert_eq!(
        bar.cursor(Rect::new(0, 0, 30, 3)),
        Some(Position::new(8, 1))
    );
    let mut input = input;
    input.key(Key::Home);
    let drawn = draw(
        InputBar::new("Name", &input).styles(STYLES).rule(false),
        30,
        1,
    );
    assert_eq!(drawn.marks[0], "IIII Cii");
    let drawn = draw(
        InputBar::new("Name", &input)
            .styles(STYLES)
            .rule(false)
            .caret(false)
            .tone(Tone::Accent),
        30,
        1,
    );
    assert_eq!(drawn.marks[0], "AAAA iii");
}

#[test]
fn an_empty_input_shows_the_placeholder_under_the_caret() {
    let input = Input::new();
    let bar = InputBar::new("Search", &input)
        .placeholder("type to filter")
        .rule(false)
        .styles(STYLES);
    let drawn = draw(bar, 30, 1);
    assert_eq!(drawn.text[0], "Search type to filter");
    assert_eq!(drawn.marks[0], "IIIIII Cmmmmmmmmmmmmm");
    let drawn = draw(InputBar::new("", &input).rule(false).styles(STYLES), 10, 1);
    assert_eq!(drawn.marks[0], "C");
}

#[test]
fn a_masked_key_shows_nothing_unless_the_app_asks() {
    fn bar(input: &Input) -> InputBar<'_> {
        InputBar::new("Key", input).rule(false).styles(STYLES)
    }
    let stored = Input::new().masked(true).with("sk-abcdefghij-wxyz");
    assert_eq!(draw(bar(&stored), 30, 1).text[0], "Key ••••••••••••••••••");
    let drawn = draw(bar(&stored).reveal_ends(4), 30, 1);
    assert_eq!(drawn.text[0], "Key sk-a••••••••••wxyz");
    assert_eq!(drawn.marks[0], "III iiiimmmmmmmmmmiiiiC");
    assert_eq!(stored.value(), "sk-abcdefghij-wxyz");
    let stars = draw(bar(&stored).reveal_ends(4).mask("*"), 30, 1);
    assert_eq!(stars.text[0], "Key sk-a**********wxyz");
}

#[test]
fn the_ends_show_only_when_the_revealed_share_is_small() {
    fn row(value: &str, reveal: usize) -> String {
        let input = Input::new().masked(true).with(value);
        let bar = InputBar::new("", &input).rule(false).styles(STYLES);
        draw(bar.reveal_ends(reveal), 40, 1).text[0].clone()
    }
    assert_eq!(row("abcdefgh", 4), "••••••••");
    assert_eq!(row("abcdefghijkl", 4), "••••••••••••");
    assert_eq!(row("abcdefghijklmno", 4), "•••••••••••••••");
    assert_eq!(row("abcdefghijklmnop", 4), "abcd••••••••mnop");
    assert_eq!(row("abcdefghij", 2), "ab••••••ij");
    assert_eq!(row("abcdefgh", 2), "ab••••gh");
    assert_eq!(row("abcdefg", 2), "•••••••");
    assert_eq!(row("abcdefghijklmnop", 0), "••••••••••••••••");
}

#[test]
fn typing_into_a_masked_input_never_shows_a_character() {
    let secret = "s3cr3t-p4ssw0rd!-and-more";
    let mut input = Input::new().masked(true);
    for c in secret.chars() {
        input.key(Key::Char(c));
        for reveal in [0, 1, 4] {
            let bar = InputBar::new("Key", &input).rule(false).styles(STYLES);
            let row = draw(bar.reveal_ends(reveal), 60, 1).text[0].clone();
            let shown = row.trim_start_matches("Key ").trim_end();
            assert!(shown.chars().all(|c| c == '•'), "{reveal}: {row:?}");
        }
    }
    assert!(input.edited());
    assert_eq!(input.value(), secret);
}

#[test]
fn a_stored_value_is_at_rest_until_the_user_edits_it() {
    let mut input = Input::new().masked(true).with("abcdefghijklmnopqrstuvwx");
    assert!(!input.edited());
    let row = |input: &Input| {
        let bar = InputBar::new("", input).rule(false).styles(STYLES);
        draw(bar.reveal_ends(4), 40, 1).text[0].clone()
    };
    assert_eq!(row(&input), "abcd••••••••••••••••uvwx");
    input.key(Key::Left);
    input.key(Key::Home);
    assert!(!input.edited());
    assert_eq!(row(&input), "abcd••••••••••••••••uvwx");
    input.key(Key::Char('!'));
    assert!(input.edited());
    assert_eq!(row(&input), "•••••••••••••••••••••••••");
    input.set("abcdefghijklmnopqrstuvwx");
    assert!(!input.edited());
    assert_eq!(row(&input), "abcd••••••••••••••••uvwx");
    input.key(Key::Ctrl('u'));
    input.set("abcdefghijklmnopqrstuvwx");
    input.clear();
    assert!(!input.edited());
}

#[test]
fn a_long_value_scrolls_to_keep_the_caret_in_view() {
    let mut input = typed("the quick brown fox jumps");
    fn bar(input: &Input) -> InputBar<'_> {
        InputBar::new("Q", input).rule(false).styles(STYLES)
    }
    let drawn = draw(bar(&input), 12, 1);
    assert_eq!(drawn.text[0], "Q …ox jumps");
    assert_eq!(drawn.marks[0], "I miiiiiiiiC");
    assert_eq!(
        bar(&input).cursor(Rect::new(0, 0, 12, 1)),
        Some(Position::new(11, 0))
    );
    input.key(Key::Home);
    let drawn = draw(bar(&input), 12, 1);
    assert_eq!(drawn.text[0], "Q the quick…");
    assert_eq!(drawn.marks[0], "I Ciiiiiiiim");
    keys(&mut input, &[Key::Right; 12]);
    let drawn = draw(bar(&input), 12, 1);
    assert_eq!(drawn.text[0], "Q …uick bro…");
    assert_eq!(
        bar(&input).cursor(Rect::new(0, 0, 12, 1)),
        Some(Position::new(10, 0))
    );
}

#[test]
fn diacritics_and_wide_glyphs_measure_by_cell() {
    let input = typed("știință 日本語のテキスト");
    for width in 1..=40 {
        let bar = InputBar::new("Notă", &input).rule(false).styles(STYLES);
        let drawn = draw(bar, width, 1);
        assert!(
            widest(&drawn) <= usize::from(width),
            "{width}\n{}",
            show(&drawn)
        );
        if let Some(cursor) = bar.cursor(Rect::new(0, 0, width, 1)) {
            assert!(cursor.x < width, "{width}");
        }
    }
    let drawn = draw(
        InputBar::new("Notă", &input).rule(false).styles(STYLES),
        40,
        1,
    );
    assert_eq!(drawn.text[0], "Notă știință 日本語のテキスト");
    let mut input = input;
    keys(&mut input, &[Key::Left; 3]);
    let drawn = draw(
        InputBar::new("Notă", &input).rule(false).styles(STYLES),
        16,
        1,
    );
    assert_eq!(drawn.text[0], "Notă …語のテキ…");
}

#[test]
fn the_footer_carries_the_input_in_place_of_the_hints() {
    let hints = [Hint::new("/", "search")];
    let input = typed("abc");
    let bar = InputBar::new("Find", &input).hint("enter search · esc cancel");
    let footer = Footer::new(&hints)
        .styles(STYLES)
        .indent(2)
        .input(Some(bar));
    assert_eq!(footer.height(40), 3);
    assert!(!footer.shown("/", 40));
    let drawn = draw(footer, 40, 3);
    assert_eq!(
        drawn.text,
        [
            "─".repeat(40).as_str(),
            "  Find abc",
            "    enter search · esc cancel"
        ],
        "\n{}",
        show(&drawn)
    );
    assert_eq!(
        footer.cursor(Rect::new(0, 10, 40, 3)),
        Some(Position::new(10, 11))
    );
    assert_eq!(Footer::new(&hints).cursor(Rect::new(0, 0, 40, 2)), None);
}

#[test]
fn crossterm_home_end_and_delete_convert() {
    use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
    let key = |code| Key::from(KeyEvent::new(code, KeyModifiers::NONE));
    assert_eq!(key(KeyCode::Home), Key::Home);
    assert_eq!(key(KeyCode::End), Key::End);
    assert_eq!(key(KeyCode::Delete), Key::Delete);
}

#[test]
fn is_masked_follows_the_builder() {
    assert!(!Input::new().is_masked());
    let masked = Input::new().with("pw").masked(true);
    assert!(masked.is_masked());
    assert_eq!(masked.value(), "pw");
    assert!(!masked.clone().masked(false).is_masked());
    assert!(!typed("abc").is_masked());
}

#[test]
fn edit_debug_names_each_variant_and_never_prints_a_submitted_value() {
    assert_eq!(format!("{:?}", Edit::Pass), "Pass");
    assert_eq!(format!("{:?}", Edit::Held), "Held");
    assert_eq!(format!("{:?}", Edit::Changed), "Changed");
    assert_eq!(format!("{:?}", Edit::Cancel), "Cancel");
    let secret = Edit::Submit("hunter2".to_string());
    assert_eq!(format!("{secret:?}"), "Submit(<7 bytes>)");
    assert_eq!(format!("{secret:#?}"), "Submit(<7 bytes>)");
    assert_eq!(
        format!("{:?}", Edit::Submit("ață".to_string())),
        "Submit(<5 bytes>)"
    );
    assert_eq!(
        format!("{:?}", Edit::Submit(String::new())),
        "Submit(<0 bytes>)"
    );
    let mut input = typed("s3cret-pass").masked(true);
    let edit = input.key(Key::Enter);
    assert_eq!(edit, Edit::Submit("s3cret-pass".to_string()));
    let debug = format!("{edit:?} {edit:#?} {:?}", [&edit]);
    assert!(!debug.contains("s3cret"), "{debug}");
    assert!(!debug.contains("pass"), "{debug}");
}
