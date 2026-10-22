use pito_tui::crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use pito_tui::dump::{ansi, keys, size, text};
use pito_tui::ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
};

fn plain(code: KeyCode) -> KeyEvent {
    KeyEvent::new(code, KeyModifiers::NONE)
}

fn typed(word: &str) -> Vec<KeyEvent> {
    word.chars().map(|c| plain(KeyCode::Char(c))).collect()
}

fn buffer(width: u16, height: u16) -> Buffer {
    Buffer::empty(Rect::new(0, 0, width, height))
}

#[test]
fn a_size_reads_width_by_height_with_either_x() {
    assert_eq!(size("120x40"), Some((120, 40)));
    assert_eq!(size("120X40"), Some((120, 40)));
    assert_eq!(size("1x1"), Some((1, 1)));
    assert_eq!(size("65535x65535"), Some((u16::MAX, u16::MAX)));
}

#[test]
fn a_size_refuses_zero_missing_halves_letters_and_overflow() {
    for bad in [
        "0x0",
        "0x40",
        "120x0",
        "x40",
        "120x",
        "x",
        "",
        "120",
        "wide",
        "abcxdef",
        "12ax40",
        "120xforty",
        "65536x40",
        "120x65536",
        "99999999999x1",
        "-1x40",
        "120x-1",
        "1.5x40",
        "120x40x2",
        "120 40",
        "120*40",
        "+120x40",
        "120x+40",
        "+1x+1",
    ] {
        assert_eq!(size(bad), None, "{bad:?}");
    }
}

#[test]
fn every_documented_key_name_reads_as_its_key() {
    let names =
        keys("enter esc up down left right pgup pgdn home end backspace delete space tab").unwrap();
    assert_eq!(
        names,
        [
            KeyCode::Enter,
            KeyCode::Esc,
            KeyCode::Up,
            KeyCode::Down,
            KeyCode::Left,
            KeyCode::Right,
            KeyCode::PageUp,
            KeyCode::PageDown,
            KeyCode::Home,
            KeyCode::End,
            KeyCode::Backspace,
            KeyCode::Delete,
            KeyCode::Char(' '),
            KeyCode::Tab,
        ]
        .map(plain)
    );
    assert_eq!(
        keys("backtab").unwrap(),
        [KeyEvent::new(KeyCode::BackTab, KeyModifiers::SHIFT)]
    );
}

#[test]
fn key_names_read_in_any_case() {
    assert_eq!(keys("ENTER").unwrap(), keys("enter").unwrap());
    assert_eq!(keys("PgUp PGDN").unwrap(), keys("pgup pgdn").unwrap());
    assert_eq!(
        keys("Esc BackTab Space").unwrap(),
        keys("esc backtab space").unwrap()
    );
    assert_eq!(keys("Tab").unwrap(), [plain(KeyCode::Tab)]);
}

#[test]
fn ctrl_and_alt_take_one_character() {
    assert_eq!(
        keys("ctrl+k alt+x ctrl+C alt+1 ctrl++ alt+日").unwrap(),
        [
            KeyEvent::new(KeyCode::Char('k'), KeyModifiers::CONTROL),
            KeyEvent::new(KeyCode::Char('x'), KeyModifiers::ALT),
            KeyEvent::new(KeyCode::Char('C'), KeyModifiers::CONTROL),
            KeyEvent::new(KeyCode::Char('1'), KeyModifiers::ALT),
            KeyEvent::new(KeyCode::Char('+'), KeyModifiers::CONTROL),
            KeyEvent::new(KeyCode::Char('日'), KeyModifiers::ALT),
        ]
    );
}

#[test]
fn any_other_word_types_its_characters_one_by_one() {
    assert_eq!(keys("foo").unwrap(), typed("foo"));
    assert_eq!(keys("25").unwrap(), typed("25"));
    assert_eq!(keys("/").unwrap(), typed("/"));
    assert_eq!(keys("entering").unwrap(), typed("entering"));
    assert_eq!(keys("日本").unwrap(), typed("日本"));
    assert_eq!(keys("f1").unwrap(), typed("f1"));
}

#[test]
fn the_documented_example_reads_key_by_key_in_order() {
    let mut expected = vec![plain(KeyCode::Tab)];
    expected.extend(typed("/"));
    expected.extend(typed("foo"));
    expected.push(plain(KeyCode::Enter));
    expected.push(KeyEvent::new(KeyCode::Char('c'), KeyModifiers::CONTROL));
    assert_eq!(keys("tab / foo enter ctrl+c").unwrap(), expected);
}

#[test]
fn any_run_of_whitespace_separates_words_and_nothing_reads_as_no_keys() {
    assert_eq!(keys("").unwrap(), []);
    assert_eq!(keys("   \t\n ").unwrap(), []);
    assert_eq!(
        keys("  up\t\tdown\nenter  ").unwrap(),
        [KeyCode::Up, KeyCode::Down, KeyCode::Enter].map(plain)
    );
}

#[test]
fn a_ctrl_or_alt_word_with_more_than_one_character_is_refused_as_that_word() {
    assert_eq!(keys("ctrl+xy"), Err("ctrl+xy".to_string()));
    assert_eq!(keys("alt+enter"), Err("alt+enter".to_string()));
    assert_eq!(keys("up ctrl+ab down"), Err("ctrl+ab".to_string()));
    assert_eq!(keys("ctrl+k alt+日本"), Err("alt+日本".to_string()));
}

#[test]
fn a_ctrl_or_alt_word_with_nothing_after_it_is_refused_as_that_word() {
    assert_eq!(keys("ctrl+"), Err("ctrl+".to_string()));
    assert_eq!(keys("tab alt+"), Err("alt+".to_string()));
}

#[test]
fn text_prints_each_row_with_trailing_spaces_trimmed_joined_by_newlines() {
    let mut frame = buffer(6, 4);
    frame.set_string(0, 0, "ab", Style::new());
    frame.set_string(2, 2, "x", Style::new().fg(Color::Red));
    frame.set_string(0, 3, "a  b  ", Style::new().add_modifier(Modifier::BOLD));
    assert_eq!(text(&frame), "ab\n\n  x\na  b");
}

#[test]
fn text_of_a_blank_buffer_is_its_newlines() {
    assert_eq!(text(&buffer(5, 3)), "\n\n");
    assert_eq!(text(&buffer(5, 1)), "");
}

#[test]
fn text_prints_a_wide_character_once() {
    let mut frame = buffer(5, 2);
    frame.set_string(0, 0, "日本x", Style::new());
    frame.set_string(0, 1, "a日", Style::new());
    assert_eq!(text(&frame), "日本x\na日");
}

#[test]
fn text_keeps_a_wide_character_that_fills_the_row_end() {
    let mut frame = buffer(4, 1);
    frame.set_string(2, 0, "本", Style::new());
    assert_eq!(text(&frame), "  本");
}

#[test]
fn ansi_hides_the_cursor_clears_then_writes_each_row_at_its_place_and_resets() {
    let mut frame = buffer(3, 2);
    frame.set_string(
        0,
        0,
        "a",
        Style::new().fg(Color::Red).add_modifier(Modifier::BOLD),
    );
    frame.set_string(1, 0, "b", Style::new());
    frame.set_string(
        0,
        1,
        "c",
        Style::new().fg(Color::Indexed(200)).bg(Color::Rgb(1, 2, 3)),
    );
    assert_eq!(
        ansi(&frame),
        concat!(
            "\x1b[?25l\x1b[2J",
            "\x1b[1;1H\x1b[0;1;31ma\x1b[0mb \x1b[0m",
            "\x1b[2;1H\x1b[0;38;5;200;48;2;1;2;3mc\x1b[0m  \x1b[0m",
        )
    );
}

#[test]
fn ansi_keeps_trailing_blanks_and_a_blank_row_still_resets() {
    assert_eq!(
        ansi(&buffer(2, 2)),
        "\x1b[?25l\x1b[2J\x1b[1;1H\x1b[0m  \x1b[0m\x1b[2;1H\x1b[0m  \x1b[0m"
    );
}

#[test]
fn ansi_writes_the_six_styling_modifiers_and_the_bright_colours() {
    let mut frame = buffer(2, 1);
    let every = Modifier::BOLD
        | Modifier::DIM
        | Modifier::ITALIC
        | Modifier::UNDERLINED
        | Modifier::REVERSED
        | Modifier::CROSSED_OUT;
    frame.set_string(
        0,
        0,
        "m",
        Style::new()
            .fg(Color::LightCyan)
            .bg(Color::DarkGray)
            .add_modifier(every),
    );
    frame.set_string(1, 0, "w", Style::new().fg(Color::White).bg(Color::Black));
    assert_eq!(
        ansi(&frame),
        "\x1b[?25l\x1b[2J\x1b[1;1H\x1b[0;1;2;3;4;7;9;96;100mm\x1b[0;97;40mw\x1b[0m"
    );
}

#[test]
fn ansi_names_each_terminal_colour_by_its_sgr_code() {
    let colours = [
        (Color::Black, 30),
        (Color::Red, 31),
        (Color::Green, 32),
        (Color::Yellow, 33),
        (Color::Blue, 34),
        (Color::Magenta, 35),
        (Color::Cyan, 36),
        (Color::Gray, 37),
        (Color::DarkGray, 90),
        (Color::LightRed, 91),
        (Color::LightGreen, 92),
        (Color::LightYellow, 93),
        (Color::LightBlue, 94),
        (Color::LightMagenta, 95),
        (Color::LightCyan, 96),
        (Color::White, 97),
    ];
    for (colour, code) in colours {
        let mut frame = buffer(1, 1);
        frame.set_string(0, 0, "x", Style::new().fg(colour).bg(colour));
        let background = code + 10;
        assert_eq!(
            ansi(&frame),
            format!("\x1b[?25l\x1b[2J\x1b[1;1H\x1b[0;{code};{background}mx\x1b[0m"),
            "{colour:?}"
        );
    }
}

#[test]
fn ansi_writes_a_wide_character_once_and_only_changes_style_when_it_changes() {
    let mut frame = buffer(4, 1);
    frame.set_string(0, 0, "日", Style::new().fg(Color::Green));
    frame.set_string(2, 0, "ab", Style::new().fg(Color::Green));
    assert_eq!(
        ansi(&frame),
        "\x1b[?25l\x1b[2J\x1b[1;1H\x1b[0;32m日ab\x1b[0m"
    );
}

#[test]
fn ansi_drops_blinking_and_hidden_and_keeps_the_symbol() {
    let mut frame = buffer(3, 1);
    for (x, modifier) in [
        Modifier::SLOW_BLINK,
        Modifier::RAPID_BLINK,
        Modifier::HIDDEN,
    ]
    .into_iter()
    .enumerate()
    {
        frame.set_string(
            x as u16,
            0,
            "s",
            Style::new().fg(Color::Red).add_modifier(modifier),
        );
    }
    assert_eq!(
        ansi(&frame),
        "\x1b[?25l\x1b[2J\x1b[1;1H\x1b[0;31msss\x1b[0m"
    );
}
