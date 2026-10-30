use pito_tui::crossterm::event::KeyCode;
use pito_tui::footer::Hint;
use pito_tui::keys::{self, Now};

use crate::common::{CUT, KEYS, PLAYING, RAIL, WIDE, bare, ch, code, keyed, started, text};

fn line(now: Now) -> String {
    KEYS.ticker(now)[0].to_string()
}

#[test]
fn the_keymap_declares_no_clashes() {
    assert!(KEYS.clashes().is_empty());
}

#[test]
fn the_footer_carries_the_keymaps_hints_in_its_order_and_words() {
    let mut tui = started(keyed(&KEYS));
    let shown = text(&mut tui, WIDE);
    let rail = line(Now::new(RAIL));
    assert_eq!(
        rail,
        "right keeps · space plays · ? help · left drops · tab switches · ctrl+z undoes"
    );
    assert!(shown.contains(&rail), "{shown}");
}

#[test]
fn the_footer_hints_follow_the_mode_and_the_context_the_app_sets() {
    let mut tui = started(keyed(&KEYS));
    tui.key(code(KeyCode::Tab));
    let cut = text(&mut tui, WIDE);
    assert!(cut.contains(&line(Now::new(CUT))), "{cut}");
    assert!(!cut.contains("right keeps"), "{cut}");
    assert!(cut.contains("left and right choose"), "{cut}");

    tui.key(ch(' '));
    let playing = text(&mut tui, WIDE);
    let now = Now::new(CUT).when(&[PLAYING]);
    assert!(playing.contains(&line(now)), "{playing}");
    assert!(playing.contains("space or esc stops it"), "{playing}");

    tui.key(code(KeyCode::Esc));
    let stopped = text(&mut tui, WIDE);
    assert!(!stopped.contains("stops it"), "{stopped}");
}

#[test]
fn the_shell_reads_the_now_it_is_given_before_a_screen_sets_one() {
    let mut tui = started(keyed(&KEYS).keys_now(Now::new(CUT)));
    let cut = text(&mut tui, WIDE);
    assert!(cut.contains("left and right choose"), "{cut}");
    tui.set_keys_now(Now::new(RAIL));
    let rail = text(&mut tui, WIDE);
    assert!(rail.contains("right keeps"), "{rail}");
}

#[test]
fn hints_passed_to_the_shell_still_show_with_or_without_a_keymap() {
    let pinned = [Hint::new("ctrl+c", "twice quits").pinned()];
    let mut with = started(keyed(&KEYS).hints(pinned));
    let shown = text(&mut with, WIDE);
    assert!(
        shown.contains("twice quits") && shown.contains("right keeps"),
        "{shown}"
    );

    let mut without = started(bare().hints(pinned));
    let shown = text(&mut without, WIDE);
    assert!(shown.contains("ctrl+c twice quits"), "{shown}");
    assert!(!shown.contains("? help"), "{shown}");
}

#[test]
fn a_keymap_hint_becomes_a_footer_hint_with_its_rank() {
    let hints = KEYS.hints(Now::new(RAIL));
    let first: Hint = hints[0].into();
    assert_eq!((first.key, first.label, first.rank), ("right", "keeps", 0));
    let ranked: Hint = (&hints[3]).into();
    assert_eq!(
        (ranked.key, ranked.label, ranked.rank),
        ("left", "drops", 1)
    );
    let help = keys::Words::default();
    assert!(hints.iter().any(|hint| hint.does == help.hint));
}
