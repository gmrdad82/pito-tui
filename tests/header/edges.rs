use crate::common::{STYLES, draw, nav};
use pito_tui::header::{
    Action, Breadcrumb, Fact, Group, Header, Key, Nav, NavBar, NavKeys, Place, Section, Step,
};
use ratatui::{
    layout::Rect,
    style::{Color, Style},
};

const GREEN: Style = Style::new().fg(Color::Green);
const RED: Style = Style::new().fg(Color::Red);

fn lone() -> Nav {
    Nav::new(vec![Group::new("A").section(Section::new("Checks"))])
}

fn facts_row(facts: &[&str], width: u16, rule: bool) -> String {
    let facts: Vec<Fact> = facts.iter().map(|text| Fact::new(text, GREEN)).collect();
    let nav = lone();
    let header = Header::new(&nav)
        .styles(STYLES)
        .tabs(false)
        .facts(&facts)
        .facts_rule(rule);
    draw(header, width, 1).text.remove(0)
}

#[test]
fn a_last_fact_one_cell_wide_is_not_elided() {
    assert_eq!(
        facts_row(&["12 items", "✓"], 40, false).trim(),
        "12 items · ✓"
    );
    assert_eq!(
        facts_row(&["3 running", "7"], 40, false).trim(),
        "3 running · 7"
    );
    assert_eq!(facts_row(&["a", "b", "c"], 40, false).trim(), "a · b · c");
    assert_eq!(facts_row(&["a", "b", "c"], 9, false).trim(), "a · b · c");
    assert!(facts_row(&["12 items", "✓"], 40, true).contains(" 12 items · ✓ "));
    assert!(facts_row(&["a", "b", "c"], 13, true).contains(" a · b · c "));
}

#[test]
fn an_empty_fact_is_skipped() {
    assert_eq!(facts_row(&["12 items", ""], 40, false).trim(), "12 items");
    assert_eq!(
        facts_row(&["", "12 items", "", "ok"], 40, false).trim(),
        "12 items · ok"
    );
    assert!(facts_row(&["12 items", ""], 40, true).contains(" 12 items "));
    assert!(!facts_row(&["12 items", ""], 40, true).contains("…"));
}

#[test]
fn facts_that_do_not_fit_still_elide() {
    assert_eq!(facts_row(&["12 items", "✓"], 10, false).trim(), "12 items…");
    assert_eq!(facts_row(&["a", "b", "c"], 8, false).trim(), "a · b…");
    assert_eq!(
        facts_row(&["12 items", "synced"], 14, false).trim(),
        "12 items · sy…"
    );
}

#[test]
fn a_cut_fact_ends_the_row_with_one_ellipsis() {
    for width in 4..24 {
        let row = facts_row(&["日本語のテキスト", "x", "y"], width, false);
        assert!(!row.contains("……"), "{width}: {row:?}");
        assert!(row.matches('…').count() <= 1, "{width}: {row:?}");
    }
}

#[test]
fn facts_from_owned_strings_draw_like_facts_from_slices() {
    let nav = lone();
    let pairs = vec![
        ("12 items".to_string(), GREEN),
        ("synced just now".to_string(), RED),
    ];
    let facts = [
        Fact::new("12 items", GREEN),
        Fact::new("synced just now", RED),
    ];
    for width in [10, 24, 40] {
        let owned = Header::new(&nav)
            .styles(STYLES)
            .tabs(false)
            .facts_pairs(&pairs);
        let borrowed = Header::new(&nav).styles(STYLES).tabs(false).facts(&facts);
        assert_eq!(owned.height(), borrowed.height());
        let (a, b) = (draw(owned, width, 1), draw(borrowed, width, 1));
        assert_eq!((a.text, a.marks), (b.text, b.marks), "{width}");
    }
    let left = vec![("? ".to_string(), GREEN), ("help".to_string(), RED)];
    let right = vec![("trial".to_string(), GREEN)];
    let owned = Header::new(&nav)
        .styles(STYLES)
        .tabs(false)
        .title(Some("Desk"))
        .left_pairs(&left)
        .right_pairs(&right);
    let parts = [Fact::new("? ", GREEN), Fact::new("help", RED)];
    let one = [Fact::new("trial", GREEN)];
    let borrowed = Header::new(&nav)
        .styles(STYLES)
        .tabs(false)
        .title(Some("Desk"))
        .left_parts(&parts)
        .right_parts(&one);
    let (a, b) = (draw(owned, 50, 1), draw(borrowed, 50, 1));
    assert_eq!((a.text, a.marks), (b.text, b.marks));
}

#[test]
fn an_all_empty_facts_row_takes_no_row() {
    let nav = lone();
    let facts = [Fact::new("", GREEN)];
    let header = Header::new(&nav).tabs(false).facts(&facts);
    assert_eq!(header.height(), 0);
}

#[test]
fn the_ellipsis_takes_the_style_of_the_part_it_cuts() {
    let nav = lone();
    let parts = [Fact::new("abcd", GREEN), Fact::new("XYZ", RED)];
    let mut checked = 0;
    for width in 14..30 {
        let header = Header::new(&nav)
            .styles(STYLES)
            .tabs(false)
            .title(Some("T"))
            .left_parts(&parts);
        let drawn = draw(header, width, 1);
        let at = drawn.text[0].chars().position(|c| c == '…');
        let Some(at) = at else { continue };
        let before = drawn.text[0].chars().nth(at - 1).unwrap();
        let mark = drawn.marks[0].chars().nth(at).unwrap();
        let expect = if before == 'd' || "abc".contains(before) {
            'g'
        } else {
            'x'
        };
        if before == 'd' {
            assert_eq!(mark, 'x', "{width}: {}", drawn.text[0]);
            checked += 1;
        } else {
            assert_eq!(mark, expect, "{width}: {}", drawn.text[0]);
        }
    }
    assert!(checked > 0);
}

fn crumb_row(nav: &Nav, width: u16, centred: bool) -> String {
    let crumb = Breadcrumb::new(nav).styles(STYLES).centred(centred);
    draw(crumb, width, 1).text.remove(0)
}

#[test]
fn a_tiny_breadcrumb_never_doubles_its_ellipsis_or_dangles_a_join() {
    let mut nav = lone();
    nav.open("nightly run", 0);
    for width in 1..40 {
        for centred in [false, true] {
            let row = crumb_row(&nav, width, centred);
            assert!(!row.contains("……"), "{width}: {row:?}");
            assert!(!row.contains("… …"), "{width}: {row:?}");
            assert!(
                !row.trim_end_matches('─').ends_with(" / "),
                "{width}: {row:?}"
            );
        }
    }
    assert_eq!(crumb_row(&nav, 6, false), "─ … ──");
    assert_eq!(crumb_row(&nav, 7, false), "─ … ───");
    assert_eq!(crumb_row(&nav, 8, false), "─ … ────");
}

#[test]
fn an_empty_crumb_is_measured_the_way_it_is_drawn() {
    let mut nav = Nav::new(vec![Group::new("A").section(Section::new(""))]);
    nav.open("nightly run", 0);
    for width in 8..40 {
        for centred in [false, true] {
            let row = crumb_row(&nav, width, centred);
            assert_eq!(row.chars().count(), usize::from(width), "{width}: {row:?}");
            assert!(row.ends_with('─'), "{width}: {row:?}");
            let cut = row.contains('…');
            assert!(cut == (width < 18), "{width}: {row:?}");
        }
    }
}

#[test]
fn an_empty_title_leaves_no_hole_in_the_rule() {
    let nav = lone();
    let none = Header::new(&nav).styles(STYLES).tabs(false).title(None);
    let empty = Header::new(&nav).styles(STYLES).tabs(false).title(Some(""));
    assert_eq!(empty.height(), none.height());
    let row = draw(empty, 20, none.height().max(1)).text;
    assert!(!row.concat().contains("  "), "{row:?}");
}

#[test]
fn a_click_outside_the_area_names_nothing() {
    let wide = Nav::new(vec![
        Group::new("Work")
            .section(Section::new("Alpha"))
            .section(Section::new("Beta"))
            .section(Section::new("Gamma"))
            .section(Section::new("Delta"))
            .section(Section::new("Epsilon"))
            .section(Section::new("Zeta"))
            .section(Section::new("Eta"))
            .section(Section::new("Theta")),
    ]);
    let area = Rect::new(2, 0, 5, 1);
    let bar = NavBar::new(&wide);
    assert!(bar.hit(area, 2, 0).is_some());
    assert!(bar.hit(area, 6, 0).is_some());
    for column in 7..60 {
        assert_eq!(bar.hit(area, column, 0), None, "{column}");
    }
    assert_eq!(bar.hit(area, 0, 0), None);
    assert_eq!(bar.hit(area, 1, 0), None);
    let header = Header::new(&wide).tabs(true);
    assert_eq!(header.hit(area, 40, 0), None);
    let roomy = Rect::new(0, 0, 80, 1);
    assert!(NavBar::new(&wide).hit(roomy, 40, 0).is_some());
    assert_eq!(NavBar::new(&wide).hit(roomy, 80, 0), None);
}

fn groups(count: usize) -> Nav {
    Nav::new(
        (0..count)
            .map(|index| Group::new(format!("G{index}")).section(Section::new(format!("S{index}"))))
            .collect(),
    )
}

fn group_of(place: Option<Place>) -> Option<usize> {
    place.map(|place| place.group)
}

#[test]
fn cycle_by_zero_stays_put() {
    let mut nav = groups(3);
    let before = nav.place();
    assert_eq!(nav.cycle(0), Some(before));
    assert_eq!(nav.place(), before);
}

#[test]
fn cycle_walks_by_its_magnitude_and_wraps() {
    let mut nav = groups(4);
    assert_eq!(group_of(nav.cycle(2)), Some(2));
    assert_eq!(group_of(nav.cycle(-3)), Some(3));
    assert_eq!(group_of(nav.cycle(1)), Some(0));
    assert_eq!(group_of(nav.cycle(-1)), Some(3));
    assert_eq!(group_of(nav.cycle(4)), Some(3));
    assert_eq!(group_of(nav.cycle(isize::MIN)), Some(3));
    assert_eq!(group_of(nav.cycle(isize::MAX)), Some(2));
}

#[test]
fn cycle_skips_empty_groups_and_needs_another_group() {
    let mut nav = Nav::new(vec![
        Group::new("A").section(Section::new("a")),
        Group::new("Empty"),
        Group::new("B").section(Section::new("b")),
        Group::new("C").section(Section::new("c")),
    ]);
    assert_eq!(group_of(nav.cycle(2)), Some(3));
    assert_eq!(group_of(nav.cycle(-2)), Some(0));
    assert_eq!(nav.cycle(3), Some(nav.place()));
    let mut alone = lone();
    assert_eq!(alone.cycle(1), None);
    assert_eq!(alone.cycle(-2), None);
    assert_eq!(group_of(alone.cycle(0)), Some(0));
    let mut none = Nav::new(Vec::new());
    assert_eq!(none.cycle(1), None);
}

#[test]
fn step_by_a_huge_amount_does_not_overflow() {
    let mut nav = nav();
    assert!(nav.step(isize::MAX).is_some());
    assert!(nav.step(isize::MIN).is_some());
}

#[test]
fn a_section_changes_its_label_and_keeps_place_and_drills() {
    let mut nav = nav();
    nav.go(5);
    nav.open("detail", 3);
    nav.go(1);
    nav.open("other", 7);
    nav.go(5);
    let before = nav.place();
    let section = nav.section_mut(1, 1).unwrap();
    section.set_name("Sets");
    section.set_short("S");
    assert_eq!(nav.place(), before);
    assert_eq!(nav.section().unwrap().name(), "Sets");
    assert_eq!(nav.section().unwrap().short_name(), "S");
    assert_eq!(nav.depth(), 1);
    assert_eq!(nav.title(), Some("detail"));
    assert_eq!(nav.breadcrumb(), "Sets / detail");
    nav.section_mut(1, 1)
        .unwrap()
        .set_spans(&[("host ", GREEN), ("up", RED)]);
    assert_eq!(nav.section().unwrap().name(), "host up");
    let drawn = draw(NavBar::new(&nav).styles(STYLES), 80, 2);
    assert!(drawn.text[1].contains("host up"), "{:?}", drawn.text);
    nav.section_mut(1, 1)
        .unwrap()
        .set_short_spans(&[("h", GREEN)]);
    assert_eq!(nav.section().unwrap().short_name(), "h");
    assert_eq!(nav.go(1).map(|place| place.number), Some(1));
    assert_eq!(nav.title(), Some("other"));
    assert_eq!(nav.go(5).map(|place| place.number), Some(5));
    assert_eq!(nav.title(), Some("detail"));
    assert!(nav.section_mut(1, 3).is_none());
    assert!(nav.section_mut(9, 0).is_none());
}

#[test]
fn nav_keys_are_built_with_methods() {
    const KEYS: NavKeys = NavKeys::HEY
        .back(&[Key::Esc])
        .digits(false)
        .next_group(&[Key::Right])
        .prev_group(&[Key::Left])
        .next_section(&[Key::Down])
        .prev_section(&[Key::Up])
        .swallow_digits(true);
    let nav = nav().keys(KEYS);
    assert_eq!(nav.action(Key::Char('q')), None);
    assert_eq!(nav.action(Key::Right), Some(Action::NextGroup));
    assert_eq!(nav.action(Key::Char('2')), None);
}

#[test]
fn alt_keys_are_not_plain_keys() {
    let mut nav = nav();
    nav.open("detail", 0);
    assert_eq!(nav.key(Key::Alt('q')), None);
    assert_eq!(nav.key(Key::Alt('2')), None);
    assert_eq!(nav.depth(), 1);
    assert!(matches!(nav.key(Key::Char('q')), Some(Step::Back { .. })));
}

#[test]
fn crossterm_carries_alt_and_lets_altgr_through() {
    use crossterm::event::{KeyCode, KeyEvent, KeyModifiers as M};
    let key = |code, modifiers| Key::from(KeyEvent::new(code, modifiers));
    assert_eq!(key(KeyCode::Char('q'), M::ALT), Key::Alt('q'));
    assert_eq!(key(KeyCode::Char('Q'), M::ALT | M::SHIFT), Key::Alt('Q'));
    assert_eq!(key(KeyCode::Char('2'), M::ALT), Key::Alt('2'));
    assert_eq!(key(KeyCode::Char('ț'), M::CONTROL | M::ALT), Key::Char('ț'));
    assert_eq!(key(KeyCode::Char('@'), M::CONTROL | M::ALT), Key::Char('@'));
    assert_eq!(key(KeyCode::Char('c'), M::CONTROL), Key::Ctrl('c'));
    assert_eq!(key(KeyCode::Enter, M::ALT), Key::Other);
    assert_eq!(key(KeyCode::Left, M::ALT), Key::Other);
    assert_eq!(key(KeyCode::Char('q'), M::SUPER), Key::Other);
    assert_eq!(key(KeyCode::Char('q'), M::META), Key::Other);
    assert_eq!(key(KeyCode::Char('q'), M::HYPER), Key::Other);
    assert_eq!(key(KeyCode::Enter, M::SHIFT), Key::Enter);
    let mut nav = nav();
    nav.open("detail", 0);
    assert_eq!(nav.key(key(KeyCode::Char('q'), M::ALT)), None);
    assert_eq!(nav.key(key(KeyCode::Char('2'), M::ALT)), None);
}

#[test]
fn control_characters_take_no_cell_and_do_not_glue_words() {
    let nav = lone();
    let notice = |text: &'static str, width: u16| {
        let header = Header::new(&nav)
            .tabs(false)
            .notice(Some(Fact::new(text, Style::new())));
        draw(header, width, 1).text.remove(0)
    };
    assert_eq!(notice("line one\nline2", 20), "line one line2");
    assert_eq!(notice("line one\nline2", 14), "line one line2");
    assert_eq!(notice("line one\nline2", 12), "line one li…");
    assert_eq!(
        notice("error: build failed\r\nsee the log", 40),
        "error: build failed see the log"
    );
    assert_eq!(notice("a\tb\rc", 10), "a b c");
    assert_eq!(notice("abc\n", 3), "abc");
    assert_eq!(notice("\n\nabc", 3), "abc");
}

#[test]
fn huge_text_never_overflows_a_width() {
    let huge = "x".repeat(70_000);
    let wide = "日".repeat(40_000);
    let nav = Nav::new(vec![
        Group::new(huge.clone()).section(Section::new(huge.clone())),
        Group::new(wide.clone()).section(Section::new(wide.clone())),
    ]);
    let mut nav = nav;
    nav.open(huge.clone(), 0);
    let facts = [
        Fact::new(&huge, GREEN),
        Fact::new(&wide, RED),
        Fact::new("tail", GREEN),
    ];
    let header = Header::new(&nav)
        .styles(STYLES)
        .title(Some(&wide))
        .left(Some(Fact::new(&huge, GREEN)))
        .right(Some(Fact::new(&wide, GREEN)))
        .facts(&facts)
        .facts_rule(true)
        .crumbs(true)
        .notice(Some(Fact::new(&huge, RED)))
        .separator(&huge);
    for width in [0, 1, 8, 40, 200] {
        let _ = draw(header, width, header.height().max(1));
        let _ = header.hit(Rect::new(0, 0, width, 6), width / 2, 1);
    }
    let _ = draw(Breadcrumb::new(&nav).centred(true), 60, 1);
    nav.go(2);
    nav.open(wide, 0);
    let _ = draw(Breadcrumb::new(&nav).centred(true), 60, 1);
}

fn hollow() -> Nav {
    Nav::new(vec![Group::new("Empty"), Group::new("Hollow")])
}

fn blank(buf: &ratatui::buffer::Buffer) -> bool {
    *buf == ratatui::buffer::Buffer::empty(buf.area)
}

#[test]
fn a_bar_without_its_groups_row_puts_the_sections_on_top() {
    let nav = nav();
    let both = NavBar::new(&nav).styles(STYLES);
    let alone = both.groups(false);
    assert_eq!(both.height(), 2);
    assert_eq!(alone.height(), 1);
    let full = draw(both, 60, 2);
    for height in [1, 2, 3] {
        let drawn = draw(alone, 60, height);
        assert_eq!(drawn.text[0], full.text[1], "{height}");
        assert_eq!(drawn.marks[0], full.marks[1], "{height}");
        assert!(drawn.text[1..].iter().all(String::is_empty), "{height}");
    }
    assert_eq!(full.text[1].trim(), "1 Inbox  2 Drafts  3 Archive");
    let area = Rect::new(0, 0, 60, 2);
    let x = full.text[1].find("2 Drafts").unwrap() as u16;
    let drafts = Some(Place {
        group: 0,
        section: 1,
        number: 2,
    });
    assert_eq!(alone.hit(area, x, 0), drafts);
    assert_eq!(alone.hit(area, x, 1), None);
    assert_eq!(both.hit(area, x, 1), drafts);
}

#[test]
fn groups_can_be_forced_on_for_a_single_group() {
    let nav = lone();
    let bar = NavBar::new(&nav).styles(STYLES);
    assert_eq!(bar.height(), 1);
    assert_eq!(bar.groups(true).height(), 2);
    let drawn = draw(bar.groups(true), 20, 2);
    assert_eq!(drawn.text, ["         A", "      1 Checks"]);
    assert_eq!(drawn.marks, ["        AAA", "     AAAAAAAAAA"]);
}

#[test]
fn a_bar_of_empty_groups_draws_the_groups_row_alone() {
    let nav = hollow();
    let bar = NavBar::new(&nav).styles(STYLES);
    assert_eq!(bar.height(), 1);
    for height in [1, 2] {
        let drawn = draw(bar, 30, height);
        assert_eq!(drawn.text[0], "       Empty    Hollow", "{height}");
        assert_eq!(drawn.marks[0], "      AAAAAAA  mmmmmmmm", "{height}");
        assert!(drawn.text[1..].iter().all(String::is_empty), "{height}");
    }
    assert_eq!(bar.hit(Rect::new(0, 0, 30, 2), 9, 0), None);
    assert_eq!(bar.hit(Rect::new(0, 0, 30, 2), 9, 1), None);
}

#[test]
fn a_bar_with_neither_row_draws_nothing() {
    let nav = hollow();
    let bar = NavBar::new(&nav).styles(STYLES).groups(false);
    assert_eq!(bar.height(), 0);
    for height in [1, 2, 3] {
        let drawn = draw(bar, 30, height);
        assert!(drawn.text.iter().all(String::is_empty), "{height}");
    }
    assert_eq!(bar.hit(Rect::new(0, 0, 30, 2), 9, 0), None);
    let empty = Nav::new(Vec::new());
    let bar = NavBar::new(&empty).styles(STYLES);
    assert_eq!(bar.height(), 0);
    assert!(draw(bar, 30, 2).text.iter().all(String::is_empty));
}

#[test]
fn a_bar_with_no_height_or_width_draws_nothing() {
    let nav = nav();
    for bar in [
        NavBar::new(&nav).styles(STYLES),
        NavBar::new(&nav).styles(STYLES).groups(false),
    ] {
        for area in [Rect::new(0, 0, 40, 0), Rect::new(0, 0, 0, 2)] {
            let mut buf = ratatui::buffer::Buffer::empty(Rect::new(0, 0, 40, 2));
            ratatui::widgets::Widget::render(bar, area, &mut buf);
            assert!(blank(&buf), "{area:?}");
            assert_eq!(bar.hit(area, 0, 0), None, "{area:?}");
        }
    }
}

const LAM_ALEF: &str = "\u{644}\u{627}\u{644}\u{627}\u{644}\u{627}";
const KANA: &str = "\u{FF8A}\u{FF9E}\u{FF8A}\u{FF9E}";

fn drilled(name: &'static str) -> Nav {
    let mut nav = Nav::new(vec![Group::new("G").section(Section::new(name))]);
    nav.open("abc", 0);
    nav
}

fn drawn_crumb(nav: &Nav, width: u16) -> (String, String) {
    let mut drawn = draw(Breadcrumb::new(nav).styles(STYLES), width, 1);
    (drawn.text.remove(0), drawn.marks.remove(0))
}

#[test]
fn a_crumb_whose_letters_join_is_measured_as_it_is_drawn() {
    let nav = drilled(LAM_ALEF);
    let whole = format!("─ {LAM_ALEF} / abc ─");
    assert_eq!(drawn_crumb(&nav, 16), (whole, "rrAAAAAAAAAAAArr".into()));
    for width in [13, 14, 15] {
        let (text, marks) = drawn_crumb(&nav, width);
        assert!(text.starts_with("─ … / abc ─"), "{width} {text:?}");
        assert!(!text.contains(LAM_ALEF), "{width} {text:?}");
        assert_eq!(
            marks,
            format!("rrAAAAAAAr{}", "r".repeat(usize::from(width) - 10))
        );
    }
}

#[test]
fn halfwidth_sound_marks_take_the_cell_the_terminal_gives_them() {
    let nav = drilled(KANA);
    let (text, marks) = drawn_crumb(&nav, 14);
    assert!(text.contains("abc ─"), "{text:?}");
    assert_eq!(marks.chars().count(), 14);
    assert!(marks.ends_with("AAAAAArr"), "{marks:?}");
    assert_eq!(drawn_crumb(&nav, 13).0, "─ … / abc ───");
}

#[test]
fn a_click_lands_on_the_section_drawn_under_it_when_letters_join() {
    let nav = Nav::new(vec![
        Group::new("G")
            .section(Section::new(LAM_ALEF))
            .section(Section::new("Next")),
    ]);
    let bar = NavBar::new(&nav).styles(STYLES);
    let drawn = draw(bar, 30, 1);
    assert_eq!(drawn.marks[0], "      AAAAAAAAAAmmmmmmmm");
    let area = Rect::new(0, 0, 30, 1);
    let number = |x| bar.hit(area, x, 0).map(|place| place.number);
    assert_eq!(number(5), None);
    assert_eq!(number(6), Some(1));
    assert_eq!(number(15), Some(1));
    assert_eq!(number(16), Some(2));
    assert_eq!(number(23), Some(2));
    assert_eq!(number(24), None);
}

fn printable_ascii() -> String {
    let mut text: String = ('!'..='~').collect();
    text.insert(47, ' ');
    text
}

#[test]
fn every_printable_ascii_char_takes_the_one_cell_it_is_drawn_in() {
    let text = printable_ascii();
    assert_eq!(text.len(), 95);
    let nav = lone();
    let notice = |text: &str, width: u16| {
        let header = Header::new(&nav)
            .tabs(false)
            .notice(Some(Fact::new(text, Style::new())));
        draw(header, width, 1).text.remove(0)
    };
    assert_eq!(notice(&text, 95), text);
    assert_eq!(notice(&text, 94), format!("{}…", &text[..93]));
    let split = format!("{}\t{}", &text[..40], &text[40..]);
    let spaced = format!("{} {}", &text[..40], &text[40..]);
    assert_eq!(notice(&split, 96), spaced);
    assert_eq!(notice(&split, 95), format!("{}…", &spaced[..94]));
}

#[test]
fn a_click_lands_on_the_section_drawn_under_it_for_every_printable_ascii_char() {
    let name: &'static str = printable_ascii().leak();
    let nav = Nav::new(vec![
        Group::new("G")
            .section(Section::new(name))
            .section(Section::new("Next")),
    ]);
    let bar = NavBar::new(&nav).styles(STYLES);
    let drawn = draw(bar, 160, 1);
    assert!(drawn.text[0].contains(name), "{:?}", drawn.text[0]);
    let marks = &drawn.marks[0];
    let start = marks.find('A').unwrap();
    let next = marks.find('m').unwrap();
    let end = marks.len();
    assert_eq!(next - start, 95 + 4);
    let area = Rect::new(0, 0, 160, 1);
    let number = |x: usize| bar.hit(area, x as u16, 0).map(|place| place.number);
    assert_eq!(number(start - 1), None);
    assert_eq!(number(start), Some(1));
    assert_eq!(number(next - 1), Some(1));
    assert_eq!(number(next), Some(2));
    assert_eq!(number(end - 1), Some(2));
    assert_eq!(number(end), None);
}
