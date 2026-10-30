use std::{
    alloc::{GlobalAlloc, Layout, System},
    cell::Cell,
};

use pito_tui::header::{Breadcrumb, Drill, Fact, Group, Header, Nav, NavBar, Section, Styles};
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

fn nav() -> Nav {
    let mut nav = Nav::new(vec![
        Group::new("Mail")
            .section(Section::new("Inbox"))
            .section(Section::spans(&[("host ", DIM), ("up", ACCENT)]).short("h"))
            .section(Section::new("Archive")),
        Group::new("Library")
            .short("Lib")
            .section(Section::new("Items & notes").short("Items"))
            .section(Section::new("Ownership").short("Owners")),
    ]);
    nav.go(2);
    nav.open("nightly run", 0);
    nav.open("a long second level title", 1);
    nav
}

#[test]
fn drawing_allocates_nothing() {
    assert!(allocations(|| drop(std::hint::black_box(Vec::<u8>::with_capacity(8)))) > 0);
    let nav = nav();
    let styles = Styles::new().accent(ACCENT).muted(DIM).rule(DIM);
    let facts = [
        Fact::new("12 items", DIM),
        Fact::new("日本語のテキスト", ACCENT),
    ];
    let pairs = vec![
        ("synced".to_string(), DIM),
        ("just now".to_string(), ACCENT),
    ];
    let left = [Fact::new("? ", DIM), Fact::new("help", ACCENT)];
    let sizes = [(80, 8), (40, 8), (24, 8), (12, 4), (6, 2), (1, 1), (0, 0)];
    let mut buffers: Vec<Buffer> = sizes
        .iter()
        .map(|&(width, height)| Buffer::empty(Rect::new(0, 0, width, height)))
        .collect();
    let counted = allocations(|| {
        for drill in [Drill::Replace, Drill::Rows] {
            let header = Header::new(&nav)
                .styles(styles)
                .title(Some("Desk"))
                .left_parts(&left)
                .right(Some(Fact::new("trial: 9 days left", DIM)))
                .facts(&facts)
                .facts_rule(true)
                .crumbs(true)
                .closing(true)
                .notice(Some(Fact::new("line one\nline two", ACCENT)))
                .underline(true)
                .drill(drill);
            let owned = Header::new(&nav).styles(styles).facts_pairs(&pairs);
            for buffer in &mut buffers {
                let area = buffer.area;
                header.render(area, buffer);
                owned.render(area, buffer);
                NavBar::new(&nav).styles(styles).render(area, buffer);
                Breadcrumb::new(&nav)
                    .styles(styles)
                    .centred(true)
                    .render(area, buffer);
                for column in 0..area.width {
                    std::hint::black_box(header.hit(area, column, 1));
                    std::hint::black_box(NavBar::new(&nav).hit(area, column, 0));
                }
            }
        }
    });
    assert_eq!(counted, 0);
}
