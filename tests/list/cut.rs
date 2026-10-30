use crate::common::*;
use pito_tui::list::{Cell, Column, List, Part, Row, cut};
use unicode_width::UnicodeWidthStr;

const TWO: [Column<'static>; 2] = [Column::new("Name", 8, 8), Column::new("Note", 4, 0)];

const SAMPLES: [&str; 15] = [
    "",
    "deploy the api",
    "src/view.rs",
    "billing-team-lead",
    "snake_case_name",
    "uploaded 12 files",
    "2026-10-26 release",
    "日本語のテキスト",
    "a日本",
    "😀😀 smile",
    "cafe\u{301} au lait",
    "build · deploy api · tests",
    "  leading and trailing  ",
    "build 12 · deploy api · x",
    "• item one two",
];

#[test]
fn text_that_fits_is_left_whole() {
    assert_eq!(cut("", 0), None);
    assert_eq!(cut("", 4), None);
    assert_eq!(cut("abc", 3), None);
    assert_eq!(cut("deploy api", 10), None);
    assert_eq!(cut("deploy api", 40), None);
    assert_eq!(cut("日本", 4), None);
    assert_eq!(cut("cafe\u{301}", 4), None);
}

#[test]
fn a_cut_keeps_whole_words_and_drops_the_spaces_before_the_ellipsis() {
    assert_eq!(cut("deploy the api", 12), Some("deploy the"));
    assert_eq!(cut("deploy the api", 11), Some("deploy the"));
    assert_eq!(cut("deploy the api", 10), Some("deploy"));
    assert_eq!(cut("deploy the api", 7), Some("deploy"));
    assert_eq!(cut("deploy   api", 9), Some("deploy"));
    assert_eq!(cut("exit 1: the build", 8), Some("exit 1:"));
}

#[test]
fn a_value_where_no_whole_word_fits_keeps_nothing() {
    assert_eq!(cut("deployment", 5), Some(""));
    assert_eq!(cut("deploy the api", 6), Some(""));
    assert_eq!(cut("ab", 1), Some(""));
    assert_eq!(cut("abc", 0), Some(""));
    assert_eq!(cut("1234567", 6), Some(""));
}

#[test]
fn names_cut_after_a_slash_a_dash_or_an_underscore() {
    assert_eq!(cut("src/view.rs", 8), Some("src/"));
    assert_eq!(cut("billing-team-lead", 14), Some("billing-team-"));
    assert_eq!(cut("billing-team-lead", 13), Some("billing-"));
    assert_eq!(cut("billing-team", 6), Some(""));
    assert_eq!(cut("snake_case_name", 12), Some("snake_case_"));
    assert_eq!(cut("/usr/local/bin", 9), Some("/usr/"));
    assert_eq!(cut("-flag", 3), Some(""));
}

#[test]
fn a_cut_never_lands_right_after_a_bare_number() {
    assert_eq!(cut("uploaded 12 files", 15), Some("uploaded"));
    assert_eq!(cut("12 files", 5), Some(""));
    assert_eq!(cut("2026-10-26 release", 15), Some(""));
    assert_eq!(cut("12.5 kB left", 10), Some("12.5 kB"));
    assert_eq!(cut("v1.2 build", 8), Some("v1.2"));
    assert_eq!(cut("12日本", 4), Some(""));
}

#[test]
fn wide_glyphs_cut_between_glyphs_and_never_in_half() {
    assert_eq!(cut("日本語のテキスト", 5), Some("日本"));
    assert_eq!(cut("日本語", 4), Some("日"));
    assert_eq!(cut("日本語", 1), Some(""));
    assert_eq!(cut("a日本", 3), Some("a"));
    assert_eq!(cut("deploy 日本語", 10), Some("deploy 日"));
    assert_eq!(cut("😀😀 smile", 3), Some("😀"));
}

#[test]
fn graphemes_stay_whole_and_tabs_count_as_spaces() {
    assert_eq!(cut("cafe\u{301} au lait", 8), Some("cafe\u{301} au"));
    assert_eq!(cut("cafe\u{301}s", 4), Some(""));
    assert_eq!(cut("tab\there", 5), Some("tab"));
}

#[test]
fn spaced_separators_drop_whole_parts() {
    let parts = "build · deploy api · tests";
    assert_eq!(cut(parts, 24), Some("build · deploy api"));
    assert_eq!(cut(parts, 18), Some("build"));
    assert_eq!(cut(parts, 6), Some("build"));
    assert_eq!(cut(parts, 5), Some(""));
    assert_eq!(cut("a · b", 2), Some("a"));
    assert_eq!(cut("build 12 · deploy", 12), Some("build"));
    assert_eq!(cut("first part is long · b", 8), Some("first"));
    assert_eq!(cut("build 12 · deploy api · x", 20), Some("build"));
    assert_eq!(
        cut("build 12 · deploy api · x", 23),
        Some("build 12 · deploy api")
    );
    assert_eq!(cut("• item one two", 12), Some("• item one"));
    assert_eq!(cut("v1 → v2 → v3", 9), Some("v1 → v2"));
}

#[test]
fn every_cut_is_a_prefix_that_fits_with_its_ellipsis() {
    for text in SAMPLES {
        for room in 0..=30 {
            match cut(text, room) {
                None => assert!(text.width() <= room, "{text:?} at {room}"),
                Some(kept) => {
                    assert!(text.width() > room, "{text:?} at {room}");
                    assert!(text.starts_with(kept), "{text:?} at {room}");
                    assert!(kept.width() < room.max(1), "{text:?} at {room}");
                    assert_eq!(kept.trim_end(), kept, "{text:?} at {room}");
                }
            }
        }
    }
}

#[test]
fn a_longer_room_never_keeps_less() {
    for text in SAMPLES {
        let mut kept = 0;
        for room in 1..=30 {
            let now = cut(text, room).map_or(text.len(), str::len);
            assert!(now >= kept, "{text:?} at {room}");
            kept = now;
        }
    }
}

#[test]
fn a_cell_of_parts_cuts_at_a_word_and_ends_in_the_style_it_cuts() {
    let mut list = List::new().with_rows([
        Row::new(["zz", "first"]),
        Row::new([
            Cell::parts([
                Part::new("deploy").style(GREEN),
                Part::new(" the api").faint(),
            ]),
            Cell::new("x"),
        ]),
    ]);
    let drawn = draw_with(&mut list, &TWO, 20, 2, |view| view.styles(STYLES));
    assert_eq!(drawn.text[1], "  deploy…   x");
    assert_eq!(drawn.marks[1], "..ggggggf");
}
