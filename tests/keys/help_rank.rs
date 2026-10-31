use std::sync::LazyLock;

use pito_tui::keys::{Keymap, Now, Words, bind};

use crate::common::{Act, CUT, KEYS, PLAYING, RAIL, WIDE, ch, keyed, keymap, started, text};

const SMALL: (u16, u16) = (80, 24);

static AFTER_ONE: LazyLock<Keymap<Act>> = LazyLock::new(|| keymap().help_rank(1));

static LAST: LazyLock<Keymap<Act>> = LazyLock::new(|| keymap().help_rank(u8::MAX));

static CROWDED_LAST: LazyLock<Keymap<Act>> = LazyLock::new(|| {
    let binds = ["a", "b", "c", "d", "e", "f", "g", "h"]
        .map(|key| bind(key, "does a fairly long thing", Act::Keep).on(RAIL));
    Keymap::new()
        .modes(&[RAIL])
        .group("Many", binds)
        .help_rank(u8::MAX)
});

fn line(keymap: &Keymap<Act>, now: Now) -> String {
    keymap.ticker(now)[0].to_string()
}

fn order(keymap: &Keymap<Act>, now: Now) -> Vec<String> {
    keymap.hints(now).iter().map(ToString::to_string).collect()
}

#[test]
fn rank_zero_is_the_default_place() {
    let now = Now::new(RAIL);
    assert_eq!(order(&keymap().help_rank(0), now), order(&KEYS, now));
    assert_eq!(
        line(&KEYS, now),
        "right keeps · space plays · ? help · left drops · tab switches · ctrl+z undoes"
    );
}

#[test]
fn rank_one_puts_help_after_the_rank_zero_and_rank_one_binds() {
    assert_eq!(
        line(&AFTER_ONE, Now::new(RAIL)),
        "right keeps · space plays · left drops · ? help · tab switches · ctrl+z undoes"
    );
    assert_eq!(
        line(&AFTER_ONE, Now::new(CUT).when(&[PLAYING])),
        "space or esc stops it · space plays · left and right choose · ? help · tab switches · ctrl+z undoes"
    );
}

#[test]
fn the_highest_rank_puts_help_last() {
    assert_eq!(
        line(&LAST, Now::new(RAIL)),
        "right keeps · space plays · left drops · tab switches · ctrl+z undoes · ? help"
    );
    assert_eq!(
        line(&LAST, Now::new(CUT)),
        "space plays · left and right choose · tab switches · ctrl+z undoes · ? help"
    );
}

#[test]
fn the_help_hint_carries_the_rank_it_was_given() {
    let help = Words::default();
    for (keymap, rank) in [(&*KEYS, 0), (&*AFTER_ONE, 1), (&*LAST, u8::MAX)] {
        let hints = keymap.hints(Now::new(RAIL));
        let found = hints.iter().find(|hint| hint.does == help.hint);
        assert_eq!(found.map(|hint| (hint.keys, hint.rank)), Some(("?", rank)));
    }
}

#[test]
fn the_footer_matches_the_public_walk_at_every_rank() {
    for keymap in [&*KEYS, &*AFTER_ONE, &*LAST] {
        let mut tui = started(keyed(keymap));
        let shown = text(&mut tui, WIDE);
        let walk = line(keymap, Now::new(RAIL));
        assert!(shown.contains(&walk), "{walk}\n{shown}");
    }
}

#[test]
fn the_open_sheet_footer_follows_the_help_rank() {
    let mut tui = started(keyed(&LAST));
    tui.key(ch('?'));
    assert!(tui.sheet().is_open());
    let shown = text(&mut tui, WIDE);
    let open = "esc closes it · / searches it · pgup / pgdn scrolls it · right keeps · space plays · left drops · tab switches · ctrl+z undoes · ? help";
    assert!(shown.contains(open), "{shown}");

    let mut tui = started(keyed(&AFTER_ONE));
    tui.key(ch('?'));
    let shown = text(&mut tui, WIDE);
    let open = "esc closes it · / searches it · pgup / pgdn scrolls it · right keeps · space plays · left drops · ? help · tab switches · ctrl+z undoes";
    assert!(shown.contains(open), "{shown}");
}

#[test]
fn a_search_typed_in_the_sheet_still_hides_help_at_any_rank() {
    let mut tui = started(keyed(&LAST));
    tui.key(ch('?'));
    tui.key(ch('/'));
    let shown = text(&mut tui, WIDE);
    assert!(!shown.contains("? help"), "{shown}");
    assert!(shown.contains("ctrl+z undoes"), "{shown}");
}

#[test]
fn a_last_help_hint_stays_pinned_on_a_narrow_footer_while_others_drop() {
    let mut tui = started(keyed(&CROWDED_LAST));
    let shown = text(&mut tui, SMALL);
    assert!(shown.contains("? help"), "{shown}");
    assert!(shown.contains("a does a fairly long thing"), "{shown}");
    assert!(!shown.contains("h does a fairly long thing"), "{shown}");
    let row = shown
        .lines()
        .find(|row| row.contains("? help"))
        .unwrap_or_default();
    let help = row.find("? help").unwrap_or_default();
    let last = row.rfind("does a fairly long thing").unwrap_or_default();
    assert!(last < help, "{row}");
    tui.key(ch('?'));
    assert!(tui.sheet().is_open());
}
