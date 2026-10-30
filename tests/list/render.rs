use crate::common::*;
use pito_tui::list::{Cell, Column, List, ListView, Row};

#[test]
fn eighty_columns_pad_align_and_clip() {
    let mut list = list_of(operations());
    let drawn = draw(&mut list, 80, 8);
    assert_eq!(
        drawn.text,
        [
            "    Operation         Versions      Owner     State     Message",
            "▌ ⧗ deploy api        v1 → v2       billing   running   uploading the bundle",
            "  ✓ update tool       0.1 → 0.2     tools     done      finished",
            "  ✗ install app       v3 → v4       design    failed    exit 1: the build could…",
            "  ■ release           v5 → v6       billing   stopped   stopped on request",
            "  · end ·",
            "",
            "",
        ]
    );
}

#[test]
fn selected_row_is_one_style_across_the_width() {
    let mut list = list_of(operations());
    let drawn = draw(&mut list, 80, 8);
    assert_eq!(drawn.marks[1], "S".repeat(80));
    assert!(drawn.marks[2].starts_with("..g"));
    assert!(drawn.marks[2].contains('f'));
    assert!(drawn.marks[0].contains('f'));
    assert_eq!(drawn.marks[5].trim_start_matches('.'), "fffffff");
}

#[test]
fn other_rows_keep_their_own_styles() {
    let mut list = list_of(operations());
    list.select(1);
    let drawn = draw(&mut list, 80, 8);
    assert_eq!(drawn.marks[2], "S".repeat(80));
    assert!(drawn.marks[1].starts_with("..g"));
    assert_eq!(&drawn.marks[1][22..29], "fffffff");
    assert_eq!(&drawn.marks[1][46..53], "ggggggg");
}

#[test]
fn widths_never_exceed_the_area() {
    for width in [0u16, 1, 2, 3, 5, 8, 13, 20, 33, 40, 60, 80, 150, 200] {
        let mut list = list_of(operations());
        let drawn = draw(&mut list, width, 8);
        assert_eq!(drawn.text.len(), 8);
        for line in &drawn.text {
            assert!(cells(line) <= usize::from(width), "{width}: {line:?}");
        }
    }
}

#[test]
fn forty_columns_drop_the_versions_and_shrink_the_rest() {
    let mut list = list_of(operations());
    let drawn = draw(&mut list, 40, 8);
    assert_eq!(
        drawn.text,
        [
            "    Operation     Owner  State  Message",
            "▌ ⧗ deploy api    bill…  runn…  uploadi…",
            "  ✓ update tool   tools  done   finished",
            "  ✗ install app   desi…  fail…  exit 1:…",
            "  ■ release       bill…  stop…  stopped…",
            "  · end ·",
            "",
            "",
        ]
    );
}

#[test]
fn thirty_columns_keep_only_the_pinned_one_and_the_message() {
    let mut list = list_of(operations());
    let drawn = draw(&mut list, 30, 8);
    assert_eq!(
        drawn.text,
        [
            "    Operation         Message",
            "▌ ⧗ deploy api        uploadi…",
            "  ✓ update tool       finished",
            "  ✗ install app       exit 1:…",
            "  ■ release           stopped…",
            "  · end ·",
            "",
            "",
        ]
    );
}

#[test]
fn columns_drop_in_priority_order_as_the_width_shrinks() {
    let titles = ["Versions", "Owner", "State", "Message"];
    let mut gone: Vec<&str> = Vec::new();
    for width in (24..=100u16).rev() {
        let mut list = list_of(operations());
        let drawn = draw(&mut list, width, 3);
        let header = &drawn.text[0];
        assert!(header.contains("Operation"), "{width}");
        for title in titles {
            let shown = header.contains(title) || (title == "Message" && header.contains("Mess"));
            if !shown && !gone.contains(&title) {
                gone.push(title);
            }
            if gone.contains(&title) {
                assert!(!header.contains(title), "{width}: {title} came back");
            }
        }
    }
    assert_eq!(gone, ["Versions", "State", "Owner"]);
}

#[test]
fn one_fifty_shows_everything_and_the_message_takes_the_rest() {
    let mut list = list_of(operations());
    let drawn = draw(&mut list, 150, 8);
    assert_eq!(
        drawn.text[3],
        "  ✗ install app       v3 → v4       design    failed    exit 1: the build could not start because a lock was held"
    );
    assert!(drawn.text[0].contains("Message"));
}

#[test]
fn two_hundred_columns_stop_growing_at_their_preferred_widths() {
    let mut list = list_of(operations());
    let drawn = draw(&mut list, 200, 8);
    let at = |line: &str, word: &str| line.find(word).unwrap();
    for word in ["Versions", "Owner", "State", "Message"] {
        let narrow = {
            let mut other = list_of(operations());
            let d = draw(&mut other, 150, 8);
            at(&d.text[0], word)
        };
        assert_eq!(at(&drawn.text[0], word), narrow, "{word}");
    }
}

#[test]
fn columns_align_across_rows_at_every_width() {
    for width in [60u16, 80, 100, 150, 200] {
        let mut list = list_of(operations());
        let drawn = draw(&mut list, width, 8);
        let header = &drawn.text[0];
        let starts: Vec<usize> = ["Versions", "Owner", "State", "Message"]
            .iter()
            .filter_map(|word| header.find(word).map(|at| header[..at].chars().count()))
            .collect();
        for line in &drawn.text[1..5] {
            let chars: Vec<char> = line.chars().collect();
            for &start in &starts {
                assert_eq!(chars[start - 1], ' ', "{width}: {line:?}");
                assert_ne!(chars[start], ' ', "{width}: {line:?}");
            }
        }
    }
}

#[test]
fn wide_glyphs_and_accents_pad_by_cells() {
    let rows = vec![
        Row::new(["ținută", "ă", "x", "y", "ok"]),
        Row::new(["日本語日本語", "漢字", "x", "y", "ok"]),
        Row::new(["e\u{301}e\u{301}", "ș", "x", "y", "ok"]),
    ];
    let mut list = list_of(rows);
    let drawn = draw(&mut list, 80, 6);
    let offset = |line: &str| {
        let at = line.find("ok").unwrap();
        cells(&line[..at])
    };
    assert_eq!(offset(&drawn.text[1]), offset(&drawn.text[2]));
    assert_eq!(offset(&drawn.text[1]), offset(&drawn.text[3]));
}

#[test]
fn a_wide_glyph_is_never_cut_in_half() {
    let columns = [Column::new("", 4, 4), Column::new("", 1, 0)];
    let mut list = List::new().with_rows([Row::new(["日本語日本語", "m"])]);
    let drawn = draw_with(&mut list, &columns, 12, 1, |view| view);
    assert_eq!(drawn.text[0], "▌ 日…   m");
}

#[test]
fn controls_become_one_space_or_nothing() {
    let rows = vec![Row::new(["a\nb\tc", "x\u{7}y", "o", "p", "q"])];
    let mut list = list_of(rows);
    let drawn = draw(&mut list, 80, 3);
    assert!(drawn.text[1].contains("a b c"), "{:?}", drawn.text[1]);
    assert!(drawn.text[1].contains("xy"), "{:?}", drawn.text[1]);
}

#[test]
fn header_is_optional_and_faint() {
    let mut list = list_of(operations());
    let area = ratatui::layout::Rect::new(0, 0, 80, 4);
    let mut buf = ratatui::buffer::Buffer::empty(area);
    ratatui::widgets::Widget::render(
        ListView::new(&mut list, &COLUMNS).styles(STYLES),
        area,
        &mut buf,
    );
    let drawn = read(&buf);
    assert!(drawn.text[0].starts_with("▌ ⧗ deploy api"));
    assert!(!drawn.text.iter().any(|line| line.contains("Operation")));
}

#[test]
fn the_end_tail_scrolls_in_with_the_last_row() {
    let rows: Vec<Row> = (0..30).map(|n| Row::new([format!("row {n}")])).collect();
    let columns = [Column::new("", 6, 6)];
    let mut list = List::new().with_rows(rows);
    list.last();
    let drawn = draw_with(&mut list, &columns, 20, 5, |view| {
        view.styles(STYLES).end(Some("· end ·"))
    });
    assert_eq!(
        drawn.text,
        ["  row 26", "  row 27", "  row 28", "▌ row 29", "  · end ·"]
    );
}

#[test]
fn no_end_tail_without_the_words() {
    let columns = [Column::new("", 6, 6)];
    let mut list = List::new().with_rows([Row::new(["a"]), Row::new(["b"])]);
    let drawn = draw_with(&mut list, &columns, 20, 4, |view| view);
    assert_eq!(drawn.text, ["▌ a", "  b", "", ""]);
}

#[test]
fn empty_list_shows_the_apps_line_under_the_header() {
    let mut list = List::new();
    let drawn = draw(&mut list, 60, 4);
    assert!(drawn.text[0].contains("Operation"));
    assert_eq!(drawn.text[1], "  Nothing here.");
    assert_eq!(drawn.text[2], "");
    assert_eq!(drawn.marks[1].trim_start_matches('.'), "fffffffffffff");
}

#[test]
fn empty_list_without_a_line_draws_nothing() {
    let mut list = List::new();
    let drawn = draw_with(&mut list, &COLUMNS, 30, 3, |view| view);
    assert_eq!(drawn.text, ["", "", ""]);
}

#[test]
fn drawing_clears_what_was_under_it() {
    let area = ratatui::layout::Rect::new(0, 0, 30, 3);
    let mut buf = ratatui::buffer::Buffer::empty(area);
    for cell in buf.content.iter_mut() {
        cell.set_symbol("#");
    }
    let mut list = list_of(vec![Row::new(["a", "b", "c", "d", "e"])]);
    ratatui::widgets::Widget::render(ListView::new(&mut list, &COLUMNS), area, &mut buf);
    let drawn = read(&buf);
    assert!(drawn.text.iter().all(|line| !line.contains('#')));
}

#[test]
fn the_cursor_and_gap_come_from_the_app() {
    let columns = [Column::new("", 3, 3), Column::new("", 1, 0)];
    let mut list = List::new().with_rows([Row::new(["ab", "cd"]), Row::new(["ef", "gh"])]);
    let drawn = draw_with(&mut list, &columns, 12, 2, |view| view.cursor("> ").gap(1));
    assert_eq!(drawn.text, ["> ab  cd", "  ef  gh"]);
}

#[test]
fn rows_with_missing_cells_leave_them_blank() {
    let mut list = list_of(vec![
        Row::new(["only"]),
        Row::new(["a", "b", "c", "d", "e", "f"]),
    ]);
    let drawn = draw(&mut list, 80, 4);
    assert_eq!(drawn.text[1], "▌ only");
    assert!(drawn.text[2].ends_with(" e"));
}

#[test]
fn more_columns_than_the_limit_are_ignored_past_it() {
    let columns: Vec<Column> = (0..40).map(|_| Column::new("", 1, 1)).collect();
    let cells: Vec<Cell> = (0..40).map(|_| Cell::new("x")).collect();
    let mut list = List::new().with_rows([Row::new(cells)]);
    let drawn = draw_with(&mut list, &columns, 200, 1, |view| view);
    assert_eq!(
        drawn.text[0].matches('x').count(),
        pito_tui::list::MAX_COLUMNS
    );
}
