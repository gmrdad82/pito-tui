use std::{
    alloc::{GlobalAlloc, Layout, System},
    cell::Cell,
};

use pito_tui::footer::{
    Confirm, ConfirmBar, Footer, Hint, Input, InputBar, Notice, Segment, Styles, Words,
};
use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
    widgets::Widget,
};

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

const ACCENT: Style = Style::new().fg(Color::Magenta);
const DIM: Style = Style::new().add_modifier(Modifier::DIM);

#[test]
fn drawing_allocates_nothing() {
    assert!(allocations(|| drop(std::hint::black_box(Vec::<u8>::with_capacity(8)))) > 0);
    let styles = Styles::new()
        .accent(ACCENT)
        .muted(DIM)
        .rule(DIM)
        .ink(ACCENT);
    let hints = [
        Hint::new("↑↓", "move"),
        Hint::new("enter", "open").rank(1),
        Hint::new("/", "search").rank(3),
        Hint::new("ctrl+c", "twice quit").pinned(),
    ];
    let spans = [("42%", ACCENT), (" done", DIM)];
    let segments = [
        Segment::text("Library", DIM).rank(2),
        Segment::spans(&spans).right(),
        Segment::hints(&hints).rank(1),
    ];
    let confirm = Confirm::new();
    let words = Words::new("Yes", "No").hint("y/n choose · enter accept · esc cancel");
    let long = Input::new().with(&"a long value ".repeat(40));
    let secret = Input::new()
        .masked(true)
        .with("sk-abcdefghijklmnopqrstuvwxyz");
    let short = Input::new();
    let sizes = [(80, 8), (40, 6), (24, 5), (12, 3), (6, 2), (1, 1), (0, 0)];
    let mut buffers: Vec<Buffer> = sizes
        .iter()
        .map(|&(width, height)| Buffer::empty(Rect::new(0, 0, width, height)))
        .collect();
    let counted = allocations(|| {
        let over =
            "The import failed: the file could not be read\nbecause another program holds it";
        for (wrap, open) in [(false, true), (true, true), (false, false), (true, false)] {
            let notices = [
                None,
                Some(Notice::alert("Import failed")),
                Some(Notice::alert(over).over()),
            ];
            for notice in notices {
                let footer = Footer::new(&hints)
                    .styles(styles)
                    .wrap(wrap)
                    .open(open)
                    .indent(2)
                    .notice(notice)
                    .version("app", "0.4.0");
                let status = Footer::status(&segments)
                    .styles(styles)
                    .notice(notice)
                    .version("app", "0.4.0");
                let bars = [
                    Footer::new(&hints)
                        .styles(styles)
                        .confirm(Some(ConfirmBar::new("Stop the import?", &confirm, words))),
                    Footer::new(&hints)
                        .styles(styles)
                        .input(Some(InputBar::new("Search", &long).hint("enter submits"))),
                    Footer::new(&hints).styles(styles).input(Some(
                        InputBar::new("Key", &secret)
                            .reveal_ends(4)
                            .placeholder("key"),
                    )),
                    Footer::new(&hints)
                        .styles(styles)
                        .input(Some(InputBar::new("Key", &short).placeholder("type")))
                        .version("app", "0.4.0"),
                ];
                for buffer in &mut buffers {
                    let area = buffer.area;
                    for footer in [footer, status].into_iter().chain(bars) {
                        footer.render(area, buffer);
                        std::hint::black_box(footer.height(area.width));
                        std::hint::black_box(footer.shown("enter", area.width));
                        std::hint::black_box(footer.shown("ctrl+c", area.width));
                        std::hint::black_box(footer.cursor(area));
                    }
                }
            }
        }
    });
    assert_eq!(counted, 0);
}
