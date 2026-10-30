use std::{
    alloc::{GlobalAlloc, Layout, System},
    cell::Cell,
    time::Duration,
};

use pito_tui::hourglass::Hourglass;
use ratatui::{buffer::Buffer, layout::Rect, style::Style, widgets::Widget};

struct Counting;

thread_local! {
    static COUNTING: Cell<bool> = const { Cell::new(false) };
    static COUNT: Cell<usize> = const { Cell::new(0) };
}

fn note() {
    if COUNTING.with(Cell::get) {
        COUNT.with(|count| count.set(count.get() + 1));
    }
}

unsafe impl GlobalAlloc for Counting {
    unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
        note();
        unsafe { System.alloc(layout) }
    }

    unsafe fn dealloc(&self, ptr: *mut u8, layout: Layout) {
        unsafe { System.dealloc(ptr, layout) }
    }

    unsafe fn realloc(&self, ptr: *mut u8, layout: Layout, size: usize) -> *mut u8 {
        note();
        unsafe { System.realloc(ptr, layout, size) }
    }
}

#[global_allocator]
static ALLOCATOR: Counting = Counting;

fn allocations(work: impl FnOnce()) -> usize {
    COUNT.with(|count| count.set(0));
    COUNTING.with(|counting| counting.set(true));
    work();
    COUNTING.with(|counting| counting.set(false));
    COUNT.with(Cell::get)
}

#[test]
fn drawing_allocates_nothing() {
    assert!(allocations(|| drop(std::hint::black_box(Vec::<u8>::with_capacity(8)))) > 0);
    let sizes = [(40, 12), (24, 8), (12, 3), (11, 1), (5, 2), (0, 0)];
    let mut buffers: Vec<Buffer> = sizes
        .iter()
        .map(|&(width, height)| Buffer::empty(Rect::new(0, 0, width, height)))
        .collect();
    let counted = allocations(|| {
        for frame in 0..200u64 {
            let hourglass = Hourglass::new(Duration::from_millis(frame * 50))
                .label("Loading the data, a little longer than the room")
                .hint(Some("esc stops waiting"))
                .glass(Style::new())
                .accent(Style::new())
                .muted(Style::new());
            for buffer in &mut buffers {
                let area = buffer.area;
                hourglass.render(area, buffer);
            }
        }
    });
    assert_eq!(counted, 0);
}
