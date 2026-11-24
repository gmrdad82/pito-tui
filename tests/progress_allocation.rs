use std::alloc::{GlobalAlloc, Layout, System};
use std::cell::Cell;
use std::time::Duration;

use pito_tui::progress::{Ramp, meter_into};
use pito_tui::ratatui::style::{Color, Style};
use pito_tui::text::shimmer_into;

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

#[test]
fn a_steady_meter_frame_allocates_nothing() {
    let ramp = Ramp::load();
    let track = Style::new().fg(Color::DarkGray);
    let mut spans = Vec::new();
    meter_into(&mut spans, 0.5, 40, &ramp, track);
    let allocations = counted(|| {
        for step in 0..=100 {
            meter_into(&mut spans, f64::from(step) / 100.0, 40, &ramp, track);
        }
    });
    assert_eq!(allocations, 0);
}

#[test]
fn a_steady_shimmer_frame_allocates_nothing() {
    let text = "Building the thing for you now";
    let base = Style::new().fg(Color::Rgb(100, 100, 100));
    let mut spans = Vec::new();
    shimmer_into(&mut spans, text, Duration::from_millis(550), base, Color::White);
    spans.reserve(64);
    let allocations = counted(|| {
        for ms in (0..3200).step_by(16) {
            shimmer_into(&mut spans, text, Duration::from_millis(ms), base, Color::White);
        }
    });
    assert_eq!(allocations, 0);
}
