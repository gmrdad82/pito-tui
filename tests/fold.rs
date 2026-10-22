use std::borrow::Cow;
use std::time::{Duration, Instant};

use pito_tui::crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use pito_tui::footer::Key;
use pito_tui::ratatui::{
    Terminal, backend::TestBackend, buffer::Buffer, layout::Rect, style::Color, style::Style,
};
use pito_tui::text::cells;
use pito_tui::{Fold, FoldState, Folds, Palette, Placed, Span};

const SIZES: [(u16, u16); 2] = [(120, 40), (200, 60)];
const ONE: &[Key] = &[Key::Char(' ')];
const ALL: &[Key] = &[Key::Char('a')];

fn spans(text: &'static str) -> Vec<Span> {
    vec![(Cow::Borrowed(text), Style::new())]
}

fn press(c: char) -> KeyEvent {
    KeyEvent::new(KeyCode::Char(c), KeyModifiers::NONE)
}

fn ms(start: Instant, millis: u64) -> Instant {
    start + Duration::from_millis(millis)
}

fn shut(folds: &mut Folds, ids: &[&str]) {
    let sections = ids.iter().map(|id| (id.to_string(), true)).collect();
    folds.restore(&FoldState { sections });
}

fn row(buf: &Buffer, y: u16) -> String {
    let mut out = String::new();
    let mut x = buf.area.x;
    while x < buf.area.right() {
        let symbol = buf[(x, y)].symbol();
        out.push_str(symbol);
        x += u16::try_from(cells(symbol)).unwrap_or(1).max(1);
    }
    out.trim_end().to_string()
}

fn lines(buf: &mut Buffer, index: usize, rect: Rect) {
    let name = ["a", "b", "c", "d"][index % 4];
    for k in 0..rect.height {
        let text = format!("{name} line {k}");
        buf.set_stringn(
            rect.x,
            rect.y + k,
            &text,
            usize::from(rect.width),
            Style::new(),
        );
    }
}

struct Column {
    titles: Vec<Vec<Span>>,
    summaries: Vec<Vec<Span>>,
    rows: Vec<u16>,
}

impl Column {
    fn new(parts: &[(&'static str, &'static str, u16)]) -> Self {
        Column {
            titles: parts.iter().map(|(title, _, _)| spans(title)).collect(),
            summaries: parts
                .iter()
                .map(|(_, summary, _)| {
                    if summary.is_empty() {
                        Vec::new()
                    } else {
                        spans(summary)
                    }
                })
                .collect(),
            rows: parts.iter().map(|(_, _, rows)| *rows).collect(),
        }
    }

    fn folds(&self) -> Vec<Fold<'_>> {
        (0..self.rows.len())
            .map(|index| Fold {
                title: &self.titles[index],
                summary: &self.summaries[index],
                rows: self.rows[index],
            })
            .collect()
    }
}

fn draw(folds: &mut Folds, column: &Column, size: (u16, u16)) -> Buffer {
    draw_with(folds, column, size, lines)
}

fn draw_with(
    folds: &mut Folds,
    column: &Column,
    size: (u16, u16),
    body: impl FnMut(&mut Buffer, usize, Rect),
) -> Buffer {
    let mut terminal = Terminal::new(TestBackend::new(size.0, size.1)).unwrap();
    let palette = Palette::new(Color::Cyan);
    let sections = column.folds();
    terminal
        .draw(|frame| {
            let area = frame.area();
            folds.draw(frame, area, &palette, &sections, body);
        })
        .unwrap();
    terminal.backend().buffer().clone()
}

fn body_height(folds: &Folds) -> u16 {
    folds.placed()[0].body.height
}

#[test]
fn a_collapsed_section_is_one_row_with_its_summary_at_the_end() {
    for size in SIZES {
        let column = Column::new(&[("Alpha", "3 items", 5), ("Beta", "", 2)]);
        let mut folds = Folds::new(["a", "b"]);
        shut(&mut folds, &["a"]);
        let buf = draw(&mut folds, &column, size);
        let first = row(&buf, 0);
        assert!(first.starts_with("▸ Alpha ─"), "{first}");
        assert!(first.ends_with("─ 3 items ─"), "{first}");
        assert_eq!(cells(&first), usize::from(size.0));
        assert!(row(&buf, 1).starts_with("▾ Beta ─"));
        assert_eq!(row(&buf, 2), "b line 0");
        assert_eq!(row(&buf, 3), "b line 1");
        assert_eq!(row(&buf, 4), "");
        let placed = folds.placed();
        assert_eq!(placed.len(), 2);
        assert_eq!(placed[0].title, Rect::new(0, 0, size.0, 1));
        assert!(placed[0].body.is_empty());
        assert_eq!(placed[0].rows, 5);
        assert_eq!(placed[1].title.y, 1);
        assert_eq!(placed[1].body, Rect::new(0, 2, size.0, 2));
        assert_eq!(folds.total(), 4);
    }
}

#[test]
fn an_expanded_section_shows_no_summary() {
    let column = Column::new(&[("Alpha", "3 items", 2)]);
    let mut folds = Folds::new(["a"]);
    let buf = draw(&mut folds, &column, (120, 40));
    assert!(!row(&buf, 0).contains("3 items"));
    assert!(row(&buf, 0).starts_with("▾ Alpha ─"));
}

#[test]
fn the_summary_is_left_out_whole_when_the_row_is_too_narrow() {
    let column = Column::new(&[("Alpha", "3 items", 5)]);
    let fits = cells("▸ Alpha ─ 3 items ─") as u16;
    for width in fits - 3..=fits + 3 {
        let mut folds = Folds::new(["a"]);
        shut(&mut folds, &["a"]);
        let first = row(&draw(&mut folds, &column, (width, 3)), 0);
        if width >= fits {
            assert!(first.ends_with(" ─ 3 items ─"), "{width}: {first}");
        } else {
            assert!(!first.contains("items"), "{width}: {first}");
            assert!(!first.contains('3'), "{width}: {first}");
            assert!(first.starts_with("▸ Alpha"), "{width}: {first}");
        }
    }
}

#[test]
fn the_title_clips_by_cells_and_never_splits_a_wide_glyph() {
    let column = Column::new(&[("日本語", "much", 5)]);
    for width in 1..=6u16 {
        let mut folds = Folds::new(["a"]);
        shut(&mut folds, &["a"]);
        let buf = draw(&mut folds, &column, (width, 2));
        let first = row(&buf, 0);
        assert!(cells(&first) <= usize::from(width), "{width}: {first}");
        assert!(first.starts_with('▸'));
        assert!(!first.contains("much"));
        assert_eq!(first.contains('日'), width >= 4, "{width}: {first}");
        assert_eq!(first.contains('本'), width >= 6, "{width}: {first}");
    }
}

#[test]
fn an_expanded_body_gets_its_rows_and_a_tall_one_gets_the_room() {
    for size in SIZES {
        let column = Column::new(&[("Alpha", "", 3), ("Tall", "", 500)]);
        let mut folds = Folds::new(["a", "b"]);
        let mut seen = Vec::new();
        let buf = draw_with(&mut folds, &column, size, |buf, index, rect| {
            seen.push((index, rect));
            lines(buf, index, rect);
        });
        assert_eq!(row(&buf, 1), "a line 0");
        assert_eq!(row(&buf, 3), "a line 2");
        assert!(row(&buf, 4).starts_with("▾ Tall"));
        assert_eq!(row(&buf, 5), "b line 0");
        let room = size.1 - 5;
        assert_eq!(row(&buf, size.1 - 1), format!("b line {}", room - 1));
        assert_eq!(
            seen,
            vec![
                (0, Rect::new(0, 1, size.0, 3)),
                (1, Rect::new(0, 5, size.0, room))
            ]
        );
        assert_eq!(folds.placed()[1].body.height, room);
        assert_eq!(folds.placed()[1].rows, 500);
        assert_eq!(folds.total(), 505);
    }
}

#[test]
fn toggling_one_changes_only_that_section_and_toggle_all_follows_any_open() {
    let now = Instant::now();
    let mut folds = Folds::new(["a", "b", "c"]).still(true);
    folds.toggle(1, now);
    assert_eq!(
        (folds.is_shut(0), folds.is_shut(1), folds.is_shut(2)),
        (false, true, false)
    );
    folds.toggle_all(now);
    assert!((0..3).all(|index| folds.is_shut(index)));
    folds.toggle_all(now);
    assert!((0..3).all(|index| !folds.is_shut(index)));
    folds.set(2, true, now);
    folds.set(2, true, now);
    assert!(folds.is_shut(2));
    folds.set_all(false, now);
    assert!(!folds.is_shut(2));
    assert!(!folds.animating());
}

#[test]
fn the_apps_keys_toggle_the_cursors_section_or_all_of_them() {
    let now = Instant::now();
    let mut folds = Folds::new(["a", "b", "c"]).still(true).keys(ONE, ALL);
    folds.point(2);
    assert!(folds.key(press(' '), now));
    assert!(folds.is_shut(2));
    assert!(!folds.is_shut(0));
    assert!(!folds.key(press('x'), now));
    assert!(folds.key(press('a'), now));
    assert!((0..3).all(|index| folds.is_shut(index)));
    assert!(folds.key(press('a'), now));
    assert!((0..3).all(|index| !folds.is_shut(index)));

    let mut bare = Folds::new(["a"]).still(true);
    assert!(!bare.key(press(' '), now));
    assert!(!bare.key(press('a'), now));
    assert!(!bare.is_shut(0));
}

#[test]
fn point_clamps_and_calls_out_of_range_or_on_an_empty_column_do_nothing() {
    let now = Instant::now();
    let mut folds = Folds::new(["a", "b"]).still(true);
    folds.point(9);
    assert_eq!(folds.cursor(), 1);
    folds.toggle(5, now);
    folds.set(5, true, now);
    assert!(!folds.is_shut(0) && !folds.is_shut(1) && !folds.is_shut(5));
    assert_eq!(folds.id(5), None);
    assert_eq!(folds.id(1), Some("b"));
    assert_eq!(folds.index("b"), Some(1));
    assert_eq!(folds.index("z"), None);

    let mut empty = Folds::new(Vec::<String>::new()).keys(ONE, ALL);
    assert!(empty.is_empty());
    assert_eq!(empty.len(), 0);
    empty.point(3);
    assert_eq!(empty.cursor(), 0);
    empty.toggle(0, now);
    empty.toggle_all(now);
    empty.set_all(true, now);
    empty.moment(now);
    assert!(!empty.key(press(' '), now));
    assert!(!empty.animating());
    assert_eq!(empty.state(), FoldState::default());
    let column = Column::new(&[("Alpha", "", 3)]);
    let buf = draw(&mut empty, &column, (120, 40));
    assert_eq!(row(&buf, 0), "");
    assert!(empty.placed().is_empty());
    assert_eq!(empty.top_of(0), None);
}

#[test]
fn state_round_trips_by_id_and_skips_unknown_ids() {
    let now = Instant::now();
    let mut folds = Folds::new(["a", "b", "c"]);
    folds.set(0, true, now);
    folds.set(2, true, now);
    let state = folds.state();
    assert_eq!(
        state.sections,
        vec![
            ("a".to_string(), true),
            ("b".to_string(), false),
            ("c".to_string(), true)
        ]
    );

    let mut fresh = Folds::new(["a", "b", "c"]);
    fresh.restore(&state);
    assert!((0..3).all(|index| fresh.is_shut(index) == folds.is_shut(index)));
    assert!(!fresh.animating());

    let mut reordered = Folds::new(["c", "z", "a", "b"]);
    reordered.set(1, true, now);
    reordered.moment(ms(now, 1000));
    reordered.restore(&state);
    assert!(reordered.is_shut(0));
    assert!(reordered.is_shut(1));
    assert!(reordered.is_shut(2));
    assert!(!reordered.is_shut(3));

    let mut skipping = Folds::new(["b"]);
    skipping.restore(&FoldState {
        sections: vec![("nope".to_string(), true), ("b".to_string(), true)],
    });
    assert!(skipping.is_shut(0));
}

fn heights(folds: &mut Folds, column: &Column, start: Instant, steps: &[u64]) -> Vec<u16> {
    steps
        .iter()
        .map(|step| {
            folds.moment(ms(start, *step));
            draw(folds, column, (120, 40));
            body_height(folds)
        })
        .collect()
}

#[test]
fn an_expand_eases_up_and_a_collapse_eases_down() {
    let column = Column::new(&[("Alpha", "", 16)]);
    let start = Instant::now();
    let steps = [0, 45, 90, 135, 180];

    let mut folds = Folds::new(["a"]);
    shut(&mut folds, &["a"]);
    folds.toggle(0, start);
    folds.moment(ms(start, 90));
    assert!(folds.animating());
    draw(&mut folds, &column, (120, 40));
    assert!(folds.placed()[0].folding);
    let up = heights(&mut folds, &column, start, &steps);
    assert!(up.windows(2).all(|pair| pair[0] <= pair[1]), "{up:?}");
    assert_eq!(up[0], 0);
    assert_eq!(up[4], 16);
    assert!(up[1] < 4, "{up:?}");
    assert!(up[3] > 12, "{up:?}");
    assert!(!folds.animating());
    assert!(!folds.placed()[0].folding);

    let mut folds = Folds::new(["a"]);
    folds.toggle(0, start);
    let down = heights(&mut folds, &column, start, &steps);
    assert!(down.windows(2).all(|pair| pair[0] >= pair[1]), "{down:?}");
    assert_eq!(down[0], 16);
    assert_eq!(down[4], 0);
    assert!(down[1] > 12, "{down:?}");
    assert!(!folds.animating());
}

#[test]
fn a_moment_before_the_start_holds_the_fold_at_its_beginning() {
    let column = Column::new(&[("Alpha", "", 10)]);
    let start = Instant::now() + Duration::from_secs(1);
    let mut folds = Folds::new(["a"]);
    folds.toggle(0, start);
    folds.moment(start - Duration::from_millis(500));
    draw(&mut folds, &column, (120, 40));
    assert_eq!(body_height(&folds), 10);
    assert!(folds.animating());
}

#[test]
fn reversing_mid_fold_never_jumps() {
    let column = Column::new(&[("Alpha", "", 20)]);
    let start = Instant::now();
    let mut folds = Folds::new(["a"]);
    shut(&mut folds, &["a"]);
    folds.toggle(0, start);
    folds.moment(ms(start, 100));
    draw(&mut folds, &column, (120, 40));
    let before = body_height(&folds);
    assert!(before > 0 && before < 20);
    folds.toggle(0, ms(start, 100));
    assert!(folds.is_shut(0));
    draw(&mut folds, &column, (120, 40));
    let after = body_height(&folds);
    assert!(before.abs_diff(after) <= 1, "{before} {after}");
    folds.moment(ms(start, 101));
    draw(&mut folds, &column, (120, 40));
    assert!(before.abs_diff(body_height(&folds)) <= 1);
    folds.moment(ms(start, 1000));
    draw(&mut folds, &column, (120, 40));
    assert_eq!(body_height(&folds), 0);
    assert!(!folds.animating());
}

#[test]
fn reduced_motion_and_a_zero_fold_are_instant() {
    let column = Column::new(&[("Alpha", "", 8)]);
    let now = Instant::now();
    for mut folds in [
        Folds::new(["a"]).still(true),
        Folds::new(["a"]).lasting(Duration::ZERO),
    ] {
        folds.toggle(0, now);
        assert!(!folds.animating());
        folds.moment(now);
        draw(&mut folds, &column, (120, 40));
        assert_eq!(body_height(&folds), 0);
        folds.toggle(0, now);
        draw(&mut folds, &column, (120, 40));
        assert_eq!(body_height(&folds), 8);
        assert!(!folds.placed()[0].folding);
    }

    let mut folds = Folds::new(["a"]);
    folds.toggle(0, now);
    assert!(folds.animating());
    folds.set_still(true);
    assert!(!folds.animating());
    assert!(folds.is_shut(0));
}

#[test]
fn mid_fold_the_body_is_the_top_of_the_full_body_like_a_curtain() {
    let column = Column::new(&[("Alpha", "", 10)]);
    let start = Instant::now();
    let mut folds = Folds::new(["a"]);
    shut(&mut folds, &["a"]);
    folds.toggle(0, start);
    folds.moment(ms(start, 90));
    let mut seen = Vec::new();
    let buf = draw_with(&mut folds, &column, (120, 40), |buf, index, rect| {
        seen.push(rect.height);
        lines(buf, index, rect);
        buf.set_stringn(
            rect.x,
            rect.bottom() - 1,
            "end",
            usize::from(rect.width),
            Style::new(),
        );
    });
    let shown = body_height(&folds);
    assert!(shown > 0 && shown < 10);
    assert_eq!(seen, vec![10]);
    for k in 0..shown {
        assert_eq!(row(&buf, 1 + k), format!("a line {k}"));
    }
    assert_eq!(row(&buf, 1 + shown), "");
}

fn tall() -> Column {
    Column::new(&[("Alpha", "", 30), ("Beta", "", 30), ("Gamma", "", 30)])
}

#[test]
fn scroll_clamps_to_the_total() {
    for size in SIZES {
        let column = tall();
        let mut folds = Folds::new(["a", "b", "c"]);
        folds.scroll(u16::MAX);
        let buf = draw(&mut folds, &column, size);
        assert_eq!(folds.total(), 93);
        assert_eq!(row(&buf, size.1 - 1), "c line 29");
        assert_eq!(
            folds.placed()[2].body.bottom(),
            size.1,
            "{:?}",
            folds.placed()
        );
    }
    let mut folds = Folds::new(["a"]);
    folds.scroll(40);
    let column = Column::new(&[("Alpha", "", 3)]);
    let buf = draw(&mut folds, &column, (120, 40));
    assert!(row(&buf, 0).starts_with("▾ Alpha"));
}

#[test]
fn a_section_scrolled_past_its_top_shows_its_lower_rows() {
    for size in SIZES {
        let column = tall();
        let mut folds = Folds::new(["a", "b", "c"]);
        folds.scroll(5);
        let mut seen = Vec::new();
        let buf = draw_with(&mut folds, &column, size, |buf, index, rect| {
            seen.push((index, rect.height));
            lines(buf, index, rect);
        });
        assert_eq!(row(&buf, 0), "a line 4");
        assert_eq!(row(&buf, 25), "a line 29");
        assert!(row(&buf, 26).starts_with("▾ Beta"));
        assert_eq!(seen[0], (0, 30));
        let placed: &[Placed] = folds.placed();
        assert_eq!(placed[0].title, Rect::new(0, size.1, 0, 0));
        assert_eq!(placed[0].body, Rect::new(0, 0, size.0, 26));
        assert!(!placed[0].folding);
        assert_eq!(folds.top_of(0), Some(0));
        assert_eq!(folds.top_of(1), Some(31));
        assert_eq!(folds.top_of(2), Some(62));
        assert_eq!(folds.top_of(3), None);
        assert_eq!(folds.title_at(3, 26), Some(1));
        assert_eq!(folds.title_at(3, 27), None);
        assert_eq!(folds.title_at(3, 0), None);
        assert_eq!(folds.title_at(size.0, 26), None);
        let gamma = 62 - 5;
        if gamma < size.1 {
            assert_eq!(folds.title_at(0, gamma), Some(2));
        } else {
            assert!(placed[2].title.is_empty());
        }
    }
}

#[test]
fn a_gap_parts_sections_but_never_trails_the_last() {
    let column = Column::new(&[("Alpha", "", 2), ("Beta", "", 2)]);
    let mut folds = Folds::new(["a", "b"]).gap(1);
    let buf = draw(&mut folds, &column, (120, 40));
    assert_eq!(row(&buf, 3), "");
    assert!(row(&buf, 4).starts_with("▾ Beta"));
    assert_eq!(folds.top_of(1), Some(4));
    assert_eq!(folds.total(), 7);
}

#[test]
fn the_apps_marks_and_lit_cursor_are_drawn() {
    let column = Column::new(&[("Alpha", "", 1), ("Beta", "", 1)]);
    let palette = Palette::new(Color::Cyan);
    let mut folds = Folds::new(["a", "b"])
        .marks("-", "+")
        .lit(true)
        .still(true);
    folds.point(1);
    folds.toggle(1, Instant::now());
    let buf = draw(&mut folds, &column, (120, 40));
    assert!(row(&buf, 0).starts_with("- Alpha ─"));
    assert!(row(&buf, 2).starts_with("+ Beta ─"));
    assert_eq!(buf[(0, 2)].fg, palette.selected.fg.unwrap());
    assert_eq!(buf[(2, 2)].fg, palette.selected.fg.unwrap());
    assert_eq!(buf[(2, 0)].fg, Color::Reset);
    assert_eq!(buf[(8, 0)].fg, palette.rule.fg.unwrap());
    assert!(buf[(2, 0)].modifier.contains(palette.strong.add_modifier));
}

#[test]
fn an_empty_area_draws_nothing_and_never_panics() {
    let column = tall();
    let mut folds = Folds::new(["a", "b", "c"]);
    let mut terminal = Terminal::new(TestBackend::new(20, 10)).unwrap();
    let palette = Palette::new(Color::Cyan);
    let sections = column.folds();
    for area in [Rect::new(0, 0, 0, 10), Rect::new(0, 0, 20, 0), Rect::new(30, 30, 5, 5)] {
        terminal
            .draw(|frame| folds.draw(frame, area, &palette, &sections, lines))
            .unwrap();
        assert!(folds.placed().is_empty());
        assert_eq!(row(terminal.backend().buffer(), 0), "");
    }
}
