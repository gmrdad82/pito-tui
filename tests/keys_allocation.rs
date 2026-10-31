use std::alloc::{GlobalAlloc, Layout, System};
use std::cell::Cell;
use std::sync::LazyLock;
use std::time::Duration;

use pito_tui::crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use pito_tui::footer::Hint;
use pito_tui::header::NavKeys;
use pito_tui::keys::{Keymap, Mode, Now, When, bind};
use pito_tui::ratatui::{
    Frame, Terminal,
    backend::TestBackend,
    layout::Rect,
    style::{Color, Style},
};
use pito_tui::{Palette, Screen, Tui, dump};

struct Counting;

thread_local! {
    static ON: Cell<bool> = const { Cell::new(false) };
    static COUNT: Cell<usize> = const { Cell::new(0) };
}

fn note() {
    if ON.try_with(Cell::get).unwrap_or(false) {
        let _ = COUNT.try_with(|count| count.set(count.get() + 1));
    }
}

unsafe impl GlobalAlloc for Counting {
    unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
        note();
        unsafe { System.alloc(layout) }
    }

    unsafe fn alloc_zeroed(&self, layout: Layout) -> *mut u8 {
        note();
        unsafe { System.alloc_zeroed(layout) }
    }

    unsafe fn realloc(&self, ptr: *mut u8, layout: Layout, new_size: usize) -> *mut u8 {
        note();
        unsafe { System.realloc(ptr, layout, new_size) }
    }

    unsafe fn dealloc(&self, ptr: *mut u8, layout: Layout) {
        unsafe { System.dealloc(ptr, layout) }
    }
}

#[global_allocator]
static GLOBAL: Counting = Counting;

fn counted(work: impl FnOnce()) -> usize {
    COUNT.with(|count| count.set(0));
    ON.with(|on| on.set(true));
    work();
    ON.with(|on| on.set(false));
    COUNT.with(Cell::get)
}

const SIZES: [(u16, u16); 2] = [(80, 24), (200, 60)];
const RAIL: Mode = Mode("clips");
const CUT: Mode = Mode("cut");
const PLAYING: When = When("while the cut plays");

static KEYS: LazyLock<Keymap<u8>> = LazyLock::new(|| {
    Keymap::new()
        .modes(&[RAIL, CUT])
        .group(
            "Rail",
            [
                bind("right", "keeps", 0).on(RAIL),
                bind("left", "drops", 1).on(RAIL).rank(1),
            ],
        )
        .group(
            "Player",
            [
                bind("space esc", "stops it", 2)
                    .modes(&[RAIL, CUT])
                    .when(PLAYING)
                    .shown("space or esc"),
                bind("space", "plays", 3).modes(&[RAIL, CUT]),
            ],
        )
        .group(
            "Edit",
            [bind("ctrl+z", "undoes", 4)
                .rank(2)
                .note("as often as you like")],
        )
});

static LAST: LazyLock<Keymap<u8>> = LazyLock::new(|| {
    Keymap::new()
        .modes(&[RAIL])
        .group(
            "Rail",
            [
                bind("right", "keeps", 0).on(RAIL),
                bind("left", "drops", 1).on(RAIL).rank(1),
                bind("ctrl+z", "undoes", 4).rank(2),
            ],
        )
        .help_rank(u8::MAX)
});

struct Plain;

impl Screen<()> for Plain {
    fn draw(&mut self, frame: &mut Frame, area: Rect, _palette: &Palette) {
        frame
            .buffer_mut()
            .set_string(area.x, area.y, "steady body", Style::new());
    }
}

struct Carrying(bool);

impl Screen<()> for Carrying {
    fn draw(&mut self, frame: &mut Frame, area: Rect, _palette: &Palette) {
        frame
            .buffer_mut()
            .set_string(area.x, area.y, "steady body", Style::new());
    }

    fn typing(&self) -> bool {
        true
    }

    fn lets_help(&self) -> bool {
        self.0
    }
}

fn carrying(lets: bool) -> Tui<()> {
    let mut tui = Tui::new("steady", "1.0.0", Color::Cyan)
        .nav_keys(NavKeys::NONE)
        .screen("Board", Carrying(lets))
        .keymap(&KEYS)
        .keys_now(Now::new(RAIL))
        .still_sheet(true);
    tui.start();
    tui
}

fn tui(still: bool) -> Tui<()> {
    hinted(still, [])
}

fn hinted<const N: usize>(still: bool, hints: [Hint<'static>; N]) -> Tui<()> {
    let mut tui = Tui::new("steady", "1.0.0", Color::Cyan)
        .nav_keys(NavKeys::NONE)
        .screen("Clips", Plain)
        .hints(hints)
        .keymap(&KEYS)
        .keys_now(Now::new(RAIL).when(&[PLAYING]))
        .still_sheet(still);
    tui.start();
    tui
}

fn press(tui: &mut Tui<()>, keys: &str) {
    for c in keys.chars() {
        tui.key(KeyEvent::new(KeyCode::Char(c), KeyModifiers::NONE));
    }
}

fn steady(tui: &mut Tui<()>, width: u16, height: u16) -> (usize, String) {
    let Ok(mut terminal) = Terminal::new(TestBackend::new(width, height));
    for _ in 0..2 {
        let Ok(_) = terminal.draw(|frame| tui.draw(frame));
    }
    let count = {
        let mut frame = terminal.get_frame();
        counted(|| tui.draw(&mut frame))
    };
    (count, dump::text(terminal.current_buffer_mut()))
}

#[test]
fn the_counter_counts_this_thread_only_while_on() {
    assert_eq!(counted(|| {}), 0);
    let made = counted(|| drop(std::hint::black_box(vec![1u8; 64])));
    assert!(made >= 1, "{made}");
}

#[test]
fn a_steady_frame_with_the_sheet_open_allocates_nothing() {
    for (width, height) in SIZES {
        let mut tui = tui(true);
        press(&mut tui, "?");
        let (count, shown) = steady(&mut tui, width, height);
        assert_eq!(count, 0, "{width}x{height}");
        assert!(shown.contains("Keys") && shown.contains("keeps"), "{shown}");
    }
}

#[test]
fn a_steady_frame_with_a_search_typed_allocates_nothing() {
    for (width, height) in SIZES {
        let mut tui = tui(true);
        press(&mut tui, "?/undo");
        let (count, shown) = steady(&mut tui, width, height);
        assert_eq!(count, 0, "{width}x{height}");
        assert!(shown.contains("/ undo"), "{shown}");
    }
}

#[test]
fn a_steady_frame_with_keymap_hints_and_the_sheet_shut_allocates_nothing() {
    for (width, height) in SIZES {
        let mut tui = tui(true);
        let (count, shown) = steady(&mut tui, width, height);
        assert_eq!(count, 0, "{width}x{height}");
        assert!(shown.contains("space or esc stops it"), "{shown}");
    }
}

#[test]
fn a_steady_frame_filtering_app_hints_under_the_open_sheet_allocates_nothing() {
    let hints = [
        Hint::new("x", "marks"),
        Hint::new("↑/↓", "moves"),
        Hint::new("space or tab", "steps"),
        Hint::new("a key with a name far too long to be one", "waits"),
    ];
    for keys in ["?", "?/u"] {
        for (width, height) in SIZES {
            let mut tui = hinted(true, hints);
            press(&mut tui, keys);
            let (count, shown) = steady(&mut tui, width, height);
            assert_eq!(count, 0, "{keys} {width}x{height}");
            assert!(shown.contains("Keys"), "{shown}");
        }
    }
}

#[test]
fn a_steady_frame_over_a_typing_screen_allocates_nothing_with_or_without_help() {
    for (lets, keys) in [(false, ""), (true, ""), (true, "?")] {
        for (width, height) in SIZES {
            let mut tui = carrying(lets);
            press(&mut tui, keys);
            let (count, shown) = steady(&mut tui, width, height);
            assert_eq!(count, 0, "{lets} {keys} {width}x{height}");
            assert_eq!(shown.contains("? help"), lets, "{shown}");
        }
    }
}

#[test]
fn a_steady_frame_with_the_help_hint_ranked_last_allocates_nothing() {
    for keys in ["", "?"] {
        for (width, height) in SIZES {
            let mut tui = Tui::new("steady", "1.0.0", Color::Cyan)
                .nav_keys(NavKeys::NONE)
                .screen("Clips", Plain)
                .keymap(&LAST)
                .keys_now(Now::new(RAIL))
                .still_sheet(true);
            tui.start();
            press(&mut tui, keys);
            let (count, shown) = steady(&mut tui, width, height);
            assert_eq!(count, 0, "{keys} {width}x{height}");
            let row = shown
                .lines()
                .find(|row| row.contains("? help"))
                .unwrap_or_default()
                .trim_end()
                .trim_end_matches("steady v1.0.0")
                .trim_end();
            assert!(
                row.ends_with("right keeps · ? help") || row.ends_with("ctrl+z undoes · ? help"),
                "{shown}"
            );
        }
    }
}

#[test]
fn every_easing_frame_after_the_first_allocates_nothing() {
    for (width, height) in SIZES {
        let mut tui = tui(false);
        press(&mut tui, "?");
        let Ok(mut terminal) = Terminal::new(TestBackend::new(width, height));
        for _ in 0..2 {
            let Ok(_) = terminal.draw(|frame| tui.draw(frame));
        }
        for _ in 0..24 {
            tui.advance(Duration::from_millis(8));
            let mut frame = terminal.get_frame();
            assert_eq!(counted(|| tui.draw(&mut frame)), 0, "{width}x{height}");
        }
    }
}
