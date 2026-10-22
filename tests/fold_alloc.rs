use std::alloc::{GlobalAlloc, Layout, System};
use std::borrow::Cow;
use std::cell::Cell;
use std::sync::atomic::{AtomicUsize, Ordering};

use pito_tui::ratatui::{
    Terminal, backend::TestBackend, buffer::Buffer, layout::Rect, style::Color, style::Style,
};
use pito_tui::{Fold, FoldState, Folds, Palette, Span};

struct Counting;

static COUNT: AtomicUsize = AtomicUsize::new(0);

thread_local! {
    static ON: Cell<bool> = const { Cell::new(false) };
}

fn counting() -> bool {
    ON.try_with(Cell::get).unwrap_or(false)
}

unsafe impl GlobalAlloc for Counting {
    unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
        if counting() {
            COUNT.fetch_add(1, Ordering::Relaxed);
        }
        unsafe { System.alloc(layout) }
    }

    unsafe fn alloc_zeroed(&self, layout: Layout) -> *mut u8 {
        if counting() {
            COUNT.fetch_add(1, Ordering::Relaxed);
        }
        unsafe { System.alloc_zeroed(layout) }
    }

    unsafe fn realloc(&self, ptr: *mut u8, layout: Layout, new_size: usize) -> *mut u8 {
        if counting() {
            COUNT.fetch_add(1, Ordering::Relaxed);
        }
        unsafe { System.realloc(ptr, layout, new_size) }
    }

    unsafe fn dealloc(&self, ptr: *mut u8, layout: Layout) {
        unsafe { System.dealloc(ptr, layout) }
    }
}

#[global_allocator]
static ALLOCATOR: Counting = Counting;

const LINES: [&str; 4] = [
    "a steady line of text",
    "another steady line",
    "日本語 wide glyphs too",
    "the last line it gets",
];

fn body(buf: &mut Buffer, index: usize, rect: Rect) {
    for k in 0..rect.height {
        let text = LINES[(index + usize::from(k)) % LINES.len()];
        buf.set_stringn(
            rect.x,
            rect.y + k,
            text,
            usize::from(rect.width),
            Style::new(),
        );
    }
}

#[test]
fn a_steady_column_allocates_nothing_per_frame() {
    for (width, height) in [(120u16, 40u16), (200, 60)] {
        let titles: [[Span; 1]; 4] = [
            [(Cow::Borrowed("Alpha"), Style::new())],
            [(Cow::Borrowed("Beta"), Style::new().fg(Color::Yellow))],
            [(Cow::Borrowed("Gamma"), Style::new())],
            [(Cow::Borrowed("Delta"), Style::new())],
        ];
        let summaries: [[Span; 1]; 4] = [
            [(Cow::Borrowed("3 jobs"), Style::new())],
            [(Cow::Borrowed(""), Style::new())],
            [(Cow::Borrowed("12 done"), Style::new())],
            [(Cow::Borrowed(""), Style::new())],
        ];
        let rows = [6u16, 40, 8, 30];
        let sections: [Fold<'_>; 4] = [
            Fold {
                title: &titles[0],
                summary: &summaries[0],
                rows: rows[0],
            },
            Fold {
                title: &titles[1],
                summary: &summaries[1],
                rows: rows[1],
            },
            Fold {
                title: &titles[2],
                summary: &summaries[2],
                rows: rows[2],
            },
            Fold {
                title: &titles[3],
                summary: &summaries[3],
                rows: rows[3],
            },
        ];
        let mut folds = Folds::new(["a", "b", "c", "d"]);
        folds.restore(&FoldState {
            sections: vec![("a".to_string(), true), ("c".to_string(), true)],
        });
        folds.scroll(4);
        let palette = Palette::new(Color::Cyan);
        let mut terminal = Terminal::new(TestBackend::new(width, height)).unwrap();
        let mut counted = usize::MAX;
        for pass in 0..3 {
            terminal
                .draw(|frame| {
                    let area = frame.area();
                    if pass == 2 {
                        COUNT.store(0, Ordering::Relaxed);
                        ON.with(|on| on.set(true));
                        folds.draw(frame, area, &palette, &sections, body);
                        ON.with(|on| on.set(false));
                        counted = COUNT.load(Ordering::Relaxed);
                    } else {
                        folds.draw(frame, area, &palette, &sections, body);
                    }
                })
                .unwrap();
        }
        assert!(folds.placed()[1].title.is_empty());
        assert!(!folds.placed()[1].body.is_empty());
        assert!(!folds.animating());
        assert_eq!(counted, 0, "{width}x{height}");
    }
}
