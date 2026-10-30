use std::sync::LazyLock;

use pito_tui::crossterm::event::KeyCode;
use pito_tui::footer::{Hint, Notice};
use pito_tui::header::NavKeys;
use pito_tui::keys::{self, Keymap, Now, bind};
use pito_tui::ratatui::{
    Frame,
    layout::Rect,
    style::{Color, Style},
};
use pito_tui::{Palette, Screen, Tui};

use crate::common::{Act, CUT, KEYS, PLAYING, RAIL, WIDE, bare, ch, code, keyed, started, text};

const SMALL: (u16, u16) = (80, 24);

const LONG: &str = "the import stopped halfway through the second folder and left every clip after it where it was";

const SCREEN_HINTS: &[Hint<'static>] = &[
    Hint::new("esc", "goes back"),
    Hint::new("x", "marks"),
    Hint::new("space", "selects"),
    Hint::new("↑/↓", "moves"),
    Hint::new("pgup / pgdn", "pages"),
];

static CROWDED: LazyLock<Keymap<Act>> = LazyLock::new(|| {
    let binds = ["a", "b", "c", "d", "e", "f", "g", "h"]
        .map(|key| bind(key, "does a fairly long thing", Act::Keep).on(RAIL));
    Keymap::new().modes(&[RAIL]).group("Many", binds)
});

static FEW: LazyLock<Keymap<Act>> = LazyLock::new(|| {
    Keymap::new()
        .modes(&[RAIL])
        .group("Rail", [bind("right", "keeps", Act::Keep).on(RAIL)])
});

#[derive(Default)]
struct Footed {
    notice: Option<&'static str>,
    hints: &'static [Hint<'static>],
}

impl Screen<()> for Footed {
    fn draw(&mut self, frame: &mut Frame, area: Rect, _palette: &Palette) {
        frame
            .buffer_mut()
            .set_string(area.x, area.y, "body", Style::new());
    }

    fn hint_list(&self) -> &[Hint<'static>] {
        self.hints
    }

    fn notice(&self) -> Option<Notice<'_>> {
        self.notice.map(|text| Notice::alert(text).over())
    }
}

fn footed(screen: Footed, keymap: &'static Keymap<Act>, hints: &[Hint<'static>]) -> Tui<()> {
    let mut tui = Tui::new("keys", "2.1.0", Color::Cyan)
        .nav_keys(NavKeys::NONE)
        .screen("Clips", screen)
        .hints(hints.iter().copied())
        .keymap(keymap)
        .keys_now(Now::new(RAIL))
        .still_sheet(true);
    tui.start();
    tui
}

fn footer_line<'s>(shown: &'s str, with: &str) -> &'s str {
    shown
        .lines()
        .find(|line| line.contains(with))
        .unwrap_or_default()
}

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

#[test]
fn the_open_sheet_puts_its_own_keys_first_in_the_footer() {
    let mut tui = started(keyed(&KEYS));
    tui.key(ch('?'));
    let shown = text(&mut tui, WIDE);
    let open = "esc closes it · / searches it · pgup / pgdn scrolls it · right keeps · space plays · ? help · left drops · tab switches · ctrl+z undoes";
    assert!(shown.contains(open), "{shown}");
}

#[test]
fn while_a_search_is_typed_the_footer_drops_the_keys_the_query_takes() {
    let mut tui = started(keyed(&KEYS));
    tui.key(ch('?'));
    tui.key(ch('/'));
    let shown = text(&mut tui, WIDE);
    let typing = "esc clears the search · enter keeps the search · pgup / pgdn scrolls it · right keeps · left drops · tab switches · ctrl+z undoes";
    assert!(shown.contains(typing), "{shown}");
    for gone in ["space plays", "? help", "/ searches it", "esc closes it"] {
        assert!(!shown.contains(gone), "{gone}:\n{shown}");
    }

    tui.key(ch('u'));
    tui.key(code(KeyCode::Enter));
    let kept = text(&mut tui, WIDE);
    assert!(
        kept.contains("esc clears the search · / searches it · pgup / pgdn scrolls it"),
        "{kept}"
    );
    assert!(kept.contains("space plays · ? help"), "{kept}");
}

#[test]
fn a_hint_on_a_key_the_open_sheet_takes_waits_until_it_closes() {
    let mut tui = started(keyed(&KEYS));
    tui.key(ch('?'));
    tui.key(ch(' '));
    let open = text(&mut tui, WIDE);
    assert!(tui.sheet().is_open());
    assert!(open.contains("stops it"), "{open}");
    assert!(!open.contains("space or esc stops it"), "{open}");
    tui.key(code(KeyCode::Esc));
    assert!(!tui.sheet().is_open());
    let shut = text(&mut tui, WIDE);
    assert!(shut.contains("space or esc stops it"), "{shut}");
}

#[test]
fn the_help_key_stays_on_a_crowded_80_column_footer_and_opens_the_sheet() {
    let mut tui = started(keyed(&CROWDED));
    let shown = text(&mut tui, SMALL);
    assert!(shown.contains("? help"), "{shown}");
    assert!(!shown.contains("h does a fairly long thing"), "{shown}");
    tui.key(ch('?'));
    assert!(tui.sheet().is_open());
}

#[test]
fn the_help_key_stays_beside_a_notice_that_takes_over_the_footer() {
    let screen = Footed {
        notice: Some(LONG),
        ..Footed::default()
    };
    let mut tui = footed(screen, &KEYS, &[]);
    let shown = text(&mut tui, SMALL);
    assert!(shown.contains("the import stopped"), "{shown}");
    assert!(shown.contains("? help"), "{shown}");
    assert!(!shown.contains("right keeps"), "{shown}");
    tui.key(ch('?'));
    assert!(tui.sheet().is_open());
    let open = text(&mut tui, SMALL);
    assert!(open.contains("esc closes it"), "{open}");
}

#[test]
fn the_open_sheet_drops_screen_and_app_hints_on_the_keys_it_takes() {
    let screen = Footed {
        hints: SCREEN_HINTS,
        ..Footed::default()
    };
    let mut tui = footed(screen, &FEW, &[Hint::new("q", "leaves")]);
    let shut = text(&mut tui, WIDE);
    for hint in [
        "esc goes back",
        "x marks",
        "space selects",
        "↑/↓ moves",
        "pgup / pgdn pages",
        "q leaves",
        "right keeps",
    ] {
        assert!(shut.contains(hint), "{hint}:\n{shut}");
    }

    tui.key(ch('?'));
    let open = text(&mut tui, WIDE);
    let line = footer_line(&open, "esc closes it");
    for hint in ["x marks", "space selects", "↑/↓ moves", "q leaves"] {
        assert!(line.contains(hint), "{hint}:\n{open}");
    }
    for gone in ["esc goes back", "pgup / pgdn pages"] {
        assert!(!line.contains(gone), "{gone}:\n{open}");
    }
    let at = |part: &str| line.find(part).unwrap_or(usize::MAX);
    assert!(at("esc closes it") < at("x marks"), "{open}");
    assert!(at("right keeps") < at("x marks"), "{open}");

    tui.key(ch('/'));
    let typing = text(&mut tui, WIDE);
    let line = footer_line(&typing, "enter keeps");
    assert!(line.contains("↑/↓ moves"), "{typing}");
    for gone in ["x marks", "space selects", "q leaves", "esc goes back"] {
        assert!(!line.contains(gone), "{gone}:\n{typing}");
    }

    tui.key(code(KeyCode::Esc));
    tui.key(code(KeyCode::Esc));
    assert!(!tui.sheet().is_open());
    let back = text(&mut tui, WIDE);
    assert!(
        back.contains("esc goes back") && back.contains("x marks"),
        "{back}"
    );
}
