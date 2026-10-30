use std::sync::LazyLock;
use std::time::{Duration, Instant};

use pito_tui::Palette;
use pito_tui::keys::{Chord, Key, Keymap, Mode, Now, Outcome, Panel, Words, bind};
use pito_tui::ratatui::{buffer::Buffer, layout::Rect, style::Color};

use crate::common::{Act, BIG, KEYS, RAIL};

const ALPHA: Mode = Mode("alpha");
const TITLE: &str = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmn";

fn moment() -> Instant {
    Instant::now()
}

fn row(buffer: &Buffer, y: u16) -> String {
    let area = buffer.area;
    (area.x..area.right())
        .map(|x| buffer[(x, y)].symbol())
        .collect()
}

fn shown(buffer: &Buffer) -> String {
    let area = buffer.area;
    (area.y..area.bottom())
        .map(|y| row(buffer, y))
        .collect::<Vec<_>>()
        .join("\n")
}

fn drawn<A>(
    panel: &mut Panel<'static>,
    keymap: &'static Keymap<A>,
    size: (u16, u16),
    at: Instant,
) -> Buffer {
    let mut buffer = Buffer::empty(Rect::new(0, 0, size.0, size.1));
    let area = buffer.area;
    let palette = Palette::new(Color::Cyan);
    panel.draw(&mut buffer, area, &palette, keymap, Now::new(RAIL), at);
    buffer
}

fn keys() -> &'static Keymap<Act> {
    &KEYS
}

fn typed(panel: &mut Panel<'_>, text: &str, at: Instant) {
    for c in text.chars() {
        panel.key(Chord::char(c), at);
    }
}

#[test]
fn a_panel_on_its_own_opens_draws_searches_and_closes() {
    let at = moment();
    let mut panel = Panel::new().still(true);
    assert_eq!(panel.key(Chord::char('x'), at), Outcome::Pass);
    assert!(
        shown(&drawn(&mut panel, keys(), (80, 24), at))
            .trim()
            .is_empty()
    );
    assert_eq!(panel.key(Chord::char('?'), at), Outcome::Took);
    assert!(panel.is_open());
    let open = shown(&drawn(&mut panel, keys(), (80, 24), at));
    for part in ["Keys", "clips", "keeps", "Elsewhere"] {
        assert!(open.contains(part), "{part}:\n{open}");
    }
    typed(&mut panel, "/undo", at);
    let found = shown(&drawn(&mut panel, keys(), (80, 24), at));
    assert!(
        found.contains("/ undo▏") && found.contains("undoes"),
        "{found}"
    );
    assert!(!found.contains("keeps"), "{found}");
    assert_eq!(panel.key(Chord::plain(Key::Esc), at), Outcome::Took);
    assert!(panel.is_open() && panel.sheet().query().is_empty());
    assert_eq!(panel.key(Chord::plain(Key::Esc), at), Outcome::Took);
    assert!(!panel.is_open());
    assert!(
        shown(&drawn(&mut panel, keys(), (80, 24), at))
            .trim()
            .is_empty()
    );
}

#[test]
fn the_sheet_draws_no_words_the_keymap_does_not_supply() {
    static OWN: LazyLock<Keymap<()>> = LazyLock::new(|| {
        Keymap::new()
            .words(Words {
                group: "Hlp".into(),
                open: "toggles".into(),
                close: "shuts".into(),
                search: "finds".into(),
                clear: "wipes".into(),
                keep: "holds".into(),
                hint: "hh".into(),
                button: "bb".into(),
                scroll: "rolls".into(),
                title: "Ttl".into(),
                elsewhere: "Away".into(),
                nothing: "none".into(),
            })
            .modes(&[ALPHA])
            .group("Grp", [bind("x", "zaps", ()).on(ALPHA)])
    });
    let allowed = [
        "alpha", "Grp", "x", "zaps", "Hlp", "?", "toggles", "esc", "shuts", "/", "finds", "pgup",
        "pgdn", "rolls", "Ttl",
    ];
    let at = moment();
    let mut panel = Panel::new().still(true);
    panel.toggle(at);
    let mut buffer = Buffer::empty(Rect::new(0, 0, 200, 60));
    let area = buffer.area;
    let palette = Palette::new(Color::Cyan);
    panel.draw(&mut buffer, area, &palette, &*OWN, Now::new(ALPHA), at);
    let text = shown(&buffer).replace(['╭', '╮', '╰', '╯', '│', '─'], " ");
    for word in text.split_whitespace() {
        assert!(allowed.contains(&word), "{word}:\n{text}");
    }
    for word in ["Ttl", "zaps", "shuts", "finds", "rolls"] {
        assert!(text.contains(word), "{word}:\n{text}");
    }
}

#[test]
fn a_long_query_keeps_its_tail_and_caret_in_view() {
    let at = moment();
    let mut panel = Panel::new().still(true);
    panel.key(Chord::char('?'), at);
    let query = format!("{}tail", "x".repeat(58));
    typed(&mut panel, "/", at);
    typed(&mut panel, &query, at);
    assert_eq!(panel.sheet().query(), query);
    let found = shown(&drawn(&mut panel, keys(), (40, 24), at));
    assert!(found.contains("xtail▏"), "{found}");
    assert!(!found.contains(&query), "{found}");
}

#[test]
fn the_search_line_stays_while_the_sheet_eases_shut() {
    let at = moment();
    let mut panel = Panel::new();
    panel.key(Chord::char('?'), at);
    typed(&mut panel, "/undo", at);
    panel.key(Chord::plain(Key::Enter), at);
    let later = at + Duration::from_millis(400);
    assert!(shown(&drawn(&mut panel, keys(), (80, 24), later)).contains("/ undo"));
    panel.key(Chord::char('?'), later);
    assert!(!panel.is_open() && panel.sheet().query().is_empty());
    let closing = shown(&drawn(
        &mut panel,
        keys(),
        (80, 24),
        later + Duration::from_millis(60),
    ));
    assert!(closing.contains("/ undo"), "{closing}");
    let gone = shown(&drawn(
        &mut panel,
        keys(),
        (80, 24),
        later + Duration::from_millis(400),
    ));
    assert!(gone.trim().is_empty(), "{gone}");
}

#[test]
fn the_up_arrow_never_covers_the_title() {
    static LONG: LazyLock<Keymap<()>> = LazyLock::new(|| {
        let binds = (0..30).map(|at| {
            let does: &'static str = format!("does {at:02}").leak();
            bind("f2", does, ()).quiet()
        });
        Keymap::new()
            .words(Words {
                title: TITLE.into(),
                ..Words::default()
            })
            .group("Many", binds.collect::<Vec<_>>())
    });
    let at = moment();
    let mut panel = Panel::new().still(true);
    panel.key(Chord::char('?'), at);
    drawn(&mut panel, &*LONG, (30, 10), at);
    panel.key(Chord::plain(Key::PageDown), at);
    let top = row(&drawn(&mut panel, &*LONG, (30, 10), at), 0);
    assert_eq!(top, format!("╭─ {}↑─╮", &TITLE[..24]));
}

#[test]
fn the_query_stays_in_view_while_typing_pages_the_rows_below_it() {
    let at = moment();
    let mut panel = Panel::new().still(true);
    panel.key(Chord::char('?'), at);
    typed(&mut panel, "/does", at);
    let first = shown(&drawn(&mut panel, &*BIG, (80, 24), at));
    assert!(
        first.contains("/ does▏") && first.contains("does 00"),
        "{first}"
    );
    for _ in 0..20 {
        panel.key(Chord::plain(Key::PageDown), at);
    }
    assert!(panel.sheet().typing());
    let last = shown(&drawn(&mut panel, &*BIG, (80, 24), at));
    assert!(
        last.contains("/ does▏") && last.contains("does 59"),
        "{last}"
    );
    assert!(!last.contains("does 00"), "{last}");
}

#[test]
fn a_panel_drawn_with_another_keymap_shows_that_keymaps_rows() {
    static OTHER: LazyLock<Keymap<Act>> = LazyLock::new(|| {
        Keymap::new()
            .modes(&[RAIL])
            .group("Other", [bind("x", "zaps", Act::Keep).on(RAIL)])
    });
    let at = moment();
    let mut panel = Panel::new().still(true);
    panel.toggle(at);
    let first = shown(&drawn(&mut panel, keys(), (80, 24), at));
    assert!(
        first.contains("keeps") && !first.contains("zaps"),
        "{first}"
    );
    let other = shown(&drawn(&mut panel, &*OTHER, (80, 24), at));
    assert!(
        other.contains("zaps") && !other.contains("keeps"),
        "{other}"
    );
}

#[test]
fn an_empty_title_leaves_the_top_border_whole() {
    static BARE: LazyLock<Keymap<()>> = LazyLock::new(|| {
        Keymap::new()
            .words(Words {
                title: "".into(),
                ..Words::default()
            })
            .modes(&[ALPHA])
            .group("Grp", [bind("x", "zaps", ()).on(ALPHA)])
    });
    let at = moment();
    let mut panel = Panel::new().still(true);
    panel.toggle(at);
    let mut buffer = Buffer::empty(Rect::new(0, 0, 80, 24));
    let area = buffer.area;
    let palette = Palette::new(Color::Cyan);
    panel.draw(&mut buffer, area, &palette, &*BARE, Now::new(ALPHA), at);
    let text = shown(&buffer);
    let top = text.lines().find(|line| line.contains('╭'));
    let top = top.unwrap_or_default().trim();
    let inner = top.trim_start_matches('╭').trim_end_matches('╮');
    assert!(
        !inner.is_empty() && inner.chars().all(|c| c == '─'),
        "{text}"
    );
}

#[test]
fn keys_wider_than_their_column_end_in_an_ellipsis() {
    static WIDE_KEYS: LazyLock<Keymap<()>> = LazyLock::new(|| {
        Keymap::new().modes(&[ALPHA]).group(
            "Grp",
            [
                bind("x", "zaps", ()).on(ALPHA),
                bind("f2", "jumps far", ())
                    .on(ALPHA)
                    .shown("f2 or the very long name of a key"),
            ],
        )
    });
    let at = moment();
    let mut panel = Panel::new().still(true);
    panel.toggle(at);
    let mut buffer = Buffer::empty(Rect::new(0, 0, 80, 24));
    let area = buffer.area;
    let palette = Palette::new(Color::Cyan);
    panel.draw(
        &mut buffer,
        area,
        &palette,
        &*WIDE_KEYS,
        Now::new(ALPHA),
        at,
    );
    let text = shown(&buffer);
    let far = text.lines().find(|line| line.contains("jumps far"));
    let far = far.unwrap_or_default();
    assert!(far.contains("f2 or the very long nam…"), "{text}");
    assert!(!far.contains("of a key"), "{text}");
    let zaps = text.lines().find(|line| line.contains("zaps"));
    assert!(!zaps.unwrap_or_default().contains('…'), "{text}");
}

#[test]
fn a_huge_paste_fills_the_query_to_its_limit_and_no_further() {
    let at = moment();
    let mut panel = Panel::new().still(true);
    typed(&mut panel, "?/ab", at);
    let huge = "x\ty".repeat(100_000);
    assert!(panel.paste(&huge));
    let query = panel.sheet().query();
    assert_eq!(query.chars().count(), 64, "{query}");
    assert!(query.starts_with("abx y"), "{query}");
    assert!(!query.contains('\t'), "{query}");
}
