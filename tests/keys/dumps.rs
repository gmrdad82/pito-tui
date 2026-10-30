use pito_tui::dump;

use crate::common::{KEYS, ch, keyed};

const SIZES: [(u16, u16); 2] = [(80, 24), (200, 60)];

#[test]
fn the_sheet_dumps_whole_and_centred_from_80x24_to_200x60() {
    for (width, height) in SIZES {
        let mut tui = keyed(&KEYS);
        let buffer = tui.shot(width, height, &[ch('?')], true);
        let shown = dump::text(&buffer);
        let top = shown.lines().find(|line| line.contains('╭'));
        let Some(top) = top else {
            panic!("no sheet at {width}x{height}:\n{shown}");
        };
        let left = top.chars().position(|c| c == '╭').unwrap_or(0);
        let right = top.chars().position(|c| c == '╮').unwrap_or(0);
        let after = usize::from(width) - 1 - right;
        assert!(left.abs_diff(after) <= 1, "{width}x{height}:\n{shown}");
        assert!(shown.contains('╰'), "{width}x{height}:\n{shown}");
        for part in ["Keys", "clips", "keeps"] {
            assert!(shown.contains(part), "{part} at {width}x{height}:\n{shown}");
        }
    }
    let mut tall = keyed(&KEYS);
    let shown = dump::text(&tall.shot(200, 60, &[ch('?')], true));
    for part in ["Elsewhere", "left and right", "pgup / pgdn", "scrolls it"] {
        assert!(shown.contains(part), "{part}:\n{shown}");
    }
    assert!(!shown.contains('↓'), "{shown}");
}

#[test]
fn the_bench_draws_the_open_sheet_every_frame() {
    for (width, height) in SIZES {
        let mut tui = keyed(&KEYS);
        tui.shot(width, height, &[ch('?')], true);
        let benches = tui.bench(width, height, 8);
        assert_eq!(benches.len(), 1);
        assert_eq!(benches[0].frames, 8);
        assert!(tui.sheet().is_open());
    }
}
