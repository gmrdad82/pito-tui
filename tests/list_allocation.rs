use std::{
    alloc::{GlobalAlloc, Layout, System},
    borrow::Cow,
    cell::Cell as Counter,
    fmt::Write,
    sync::Arc,
};

use pito_tui::list::{
    Bar, Cell, Column, Count, Key, Keys, List, ListView, Mark, Paging, Part, Place, Row, Source,
    Step, Styles,
};
use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
    widgets::Widget,
};

struct Counting;

thread_local! {
    static COUNTING: Counter<bool> = const { Counter::new(false) };
    static COUNT: Counter<usize> = const { Counter::new(0) };
}

fn note() {
    if COUNTING.with(Counter::get) {
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
    COUNT.with(Counter::get)
}

const ACCENT: Style = Style::new().fg(Color::Magenta).add_modifier(Modifier::BOLD);
const DIM: Style = Style::new().add_modifier(Modifier::DIM);

#[test]
fn drawing_allocates_nothing() {
    assert!(allocations(|| drop(std::hint::black_box(Vec::<u8>::with_capacity(8)))) > 0);
    let styles = Styles::new().selected(ACCENT).faint(DIM);
    let columns = [
        Column::new("Name", 8, 14).pinned(),
        Column::new("Version", 6, 10).priority(3),
        Column::new("Owner", 5, 8).priority(1),
        Column::new("Message", 8, 0),
    ];
    let rows: Vec<Row> = (0..500)
        .map(|n| {
            Row::new([
                Cell::new(format!("ținută {n} 日本語")),
                Cell::new("v1 → v2").faint(),
                Cell::new("owner").style(Style::new().fg(Color::Green)),
                Cell::new("a long message that never fits\nin one line at all, however wide"),
            ])
            .mark(Mark::new("⧗").style(DIM))
        })
        .collect();
    let mut list = List::new().keys(Keys::VIM).with_rows(rows);
    let sizes = [
        (150, 40),
        (80, 12),
        (40, 6),
        (24, 3),
        (6, 2),
        (1, 1),
        (0, 0),
    ];
    let mut buffers: Vec<Buffer> = sizes
        .iter()
        .map(|&(width, height)| Buffer::empty(Rect::new(0, 0, width, height)))
        .collect();
    let counted = allocations(|| {
        for round in 0..30usize {
            for buffer in &mut buffers {
                let area = buffer.area;
                for header in [false, true] {
                    ListView::new(&mut list, &columns)
                        .styles(styles)
                        .header(header)
                        .end(Some("· end ·"))
                        .empty(Some("Nothing."))
                        .render(area, buffer);
                }
                std::hint::black_box(list.hit(area, true, 3, 3));
                std::hint::black_box(list.columns());
            }
            let key = [
                Key::Down,
                Key::Char('j'),
                Key::PageDown,
                Key::Char('G'),
                Key::Home,
                Key::Tab,
            ][round % 6];
            assert_ne!(list.key(key), Step::Open(0));
        }
    });
    assert_eq!(counted, 0);
    let mut empty = List::new();
    let counted = allocations(|| {
        for buffer in &mut buffers {
            let area = buffer.area;
            ListView::new(&mut empty, &columns)
                .styles(styles)
                .header(true)
                .empty(Some("Nothing."))
                .render(area, buffer);
        }
    });
    assert_eq!(counted, 0);
}

#[test]
fn a_base_and_right_columns_allocate_nothing() {
    let styles = Styles::new()
        .selected(ACCENT)
        .faint(DIM)
        .base(Style::new().fg(Color::White).bg(Color::Blue));
    let columns = [
        Column::new("Name", 8, 14).pinned(),
        Column::new("Size", 4, 10).priority(2).right(),
        Column::new("Owner", 5, 8).priority(1),
        Column::new("Total", 3, 0).right(),
    ];
    let rows: Vec<Row> = (0..500)
        .map(|n| {
            Row::new([
                Cell::new(format!("ținută {n} 日本語")),
                Cell::new(format!("{}", n * 1024)).faint(),
                Cell::new("owner"),
                Cell::new("1234567890 ".repeat(n % 7)),
            ])
            .mark(Mark::new("⧗").style(DIM))
        })
        .collect();
    let mut list = List::new().with_rows(rows);
    let mut buffers: Vec<Buffer> = [(150, 40), (40, 6), (12, 3), (1, 1), (0, 0)]
        .iter()
        .map(|&(width, height)| Buffer::empty(Rect::new(0, 0, width, height)))
        .collect();
    let counted = allocations(|| {
        for round in 0..30usize {
            for buffer in &mut buffers {
                let area = buffer.area;
                ListView::new(&mut list, &columns)
                    .styles(styles)
                    .header(round % 2 == 0)
                    .end(Some("· end ·"))
                    .render(area, buffer);
            }
            list.key(Key::PageDown);
        }
    });
    assert_eq!(counted, 0);
}

fn grouped(n: usize) -> Row {
    match n % 12 {
        0 => Row::heading([
            Part::new(format!("Group {n} ținută")).style(ACCENT),
            Part::new(" · 10").faint(),
        ]),
        11 => Row::blank(),
        5 => Row::new([Cell::new(
            "a detail that spans the rest of the row, wider than any area here 日本語",
        )
        .rest()])
        .mark(Mark::new("·")),
        _ => Row::new([
            Cell::parts([
                Part::new(format!("ținută {n}")),
                Part::new(" 日本語").faint(),
            ]),
            Cell::new(format!("{}", n * 1024)),
            Cell::parts(["1", "2"]).style(Style::new().fg(Color::Green)),
        ])
        .mark(Mark::new("⧗").style(DIM)),
    }
}

const GROUPED: [Column<'static>; 3] = [
    Column::new("Name", 8, 0).flex(),
    Column::new("Size", 4, 8).right().priority(1),
    Column::new("Count", 3, 5).right(),
];

const FITTED: [Column<'static>; 4] = [
    Column::new("Name", 4, 0).fit(30).pinned(),
    Column::new("Size", 4, 0).fit(10).right().priority(1),
    Column::new("Count", 3, 5).right().fit(6),
    Column::new("Note", 4, 0),
];

const KEYS: [Key; 8] = [
    Key::Down,
    Key::Char('k'),
    Key::PageDown,
    Key::PageUp,
    Key::Char('G'),
    Key::Home,
    Key::Ctrl('d'),
    Key::Enter,
];

fn sizes() -> Vec<Buffer> {
    [(150, 40), (40, 6), (20, 2), (12, 1), (1, 1), (0, 0)]
        .iter()
        .map(|&(width, height)| Buffer::empty(Rect::new(0, 0, width, height)))
        .collect()
}

#[test]
fn sections_parts_rest_cells_a_flex_column_and_view_paging_allocate_nothing() {
    let styles = Styles::new()
        .selected(ACCENT)
        .faint(DIM)
        .base(Style::new().bg(Color::Blue));
    let mut list = List::new()
        .keys(Keys::VIM)
        .paging(Paging::View)
        .with_rows((0..600).map(grouped));
    let mut buffers = sizes();
    let counted = allocations(|| {
        for round in 0..60usize {
            for buffer in &mut buffers {
                let area = buffer.area;
                ListView::new(&mut list, &GROUPED)
                    .styles(styles)
                    .header(round % 2 == 0)
                    .end(Some("· end ·"))
                    .render(area, buffer);
                std::hint::black_box(list.hit(area, round % 2 == 0, 3, 1));
            }
            std::hint::black_box(list.key(KEYS[round % KEYS.len()]));
            if round % 5 == 0 {
                list.scroll_to(round * 7);
            }
        }
    });
    assert_eq!(counted, 0);
}

struct Lazy {
    len: usize,
    built: Counter<usize>,
    own: Counter<usize>,
}

impl Source for Lazy {
    fn len(&self) -> usize {
        self.len
    }

    fn row(&self, index: usize) -> Cow<'_, Row> {
        let before = COUNT.with(Counter::get);
        let row = grouped(index);
        self.own
            .set(self.own.get() + COUNT.with(Counter::get) - before);
        self.built.set(self.built.get() + 1);
        Cow::Owned(row)
    }

    fn mark_width(&self) -> u16 {
        1
    }
}

#[test]
fn a_lazy_list_allocates_only_what_its_rows_do() {
    let mut list = List::from_source(Lazy {
        len: 50_000,
        built: Counter::new(0),
        own: Counter::new(0),
    })
    .keys(Keys::VIM);
    let mut buffers = sizes();
    let counted = allocations(|| {
        for round in 0..60usize {
            for buffer in &mut buffers {
                let area = buffer.area;
                ListView::new(&mut list, &GROUPED)
                    .header(true)
                    .end(Some("· end ·"))
                    .render(area, buffer);
            }
            std::hint::black_box(list.key(KEYS[round % KEYS.len()]));
        }
    });
    let own = list.source().own.get();
    assert!(own > 0);
    assert_eq!(counted, own);
    assert!(list.source().built.get() < 60 * 100);
}

#[test]
fn a_lazy_list_with_fitted_columns_allocates_only_what_its_rows_do() {
    let mut list = List::from_source(Lazy {
        len: 50_000,
        built: Counter::new(0),
        own: Counter::new(0),
    })
    .keys(Keys::VIM);
    let mut buffers = sizes();
    let styles = Styles::new()
        .selected(Style::new().add_modifier(Modifier::BOLD))
        .cursor(ACCENT)
        .keep_colours(true);
    let counted = allocations(|| {
        for round in 0..60usize {
            for buffer in &mut buffers {
                let area = buffer.area;
                ListView::new(&mut list, &FITTED)
                    .styles(styles)
                    .header(round % 2 == 0)
                    .end(Some("· end ·"))
                    .render(area, buffer);
                std::hint::black_box(list.columns());
            }
            std::hint::black_box(list.key(KEYS[round % KEYS.len()]));
        }
    });
    let own = list.source().own.get();
    assert!(own > 0);
    assert_eq!(counted, own);
    assert!(list.source().built.get() < 60 * 200);
}

#[test]
fn a_shared_row_vector_handed_over_every_frame_with_a_scrollbar_and_a_count_allocates_nothing() {
    let rows: Arc<Vec<Row>> = Arc::new((0..5_000).map(grouped).collect());
    let mut list = List::from_source(Arc::clone(&rows)).keys(Keys::VIM);
    let mut buffers = sizes();
    for buffer in &mut buffers {
        let area = buffer.area;
        ListView::new(&mut list, &FITTED)
            .header(true)
            .render(area, buffer);
    }
    let bars = [Bar::LINE, Bar::new(" ", "█")];
    let places = [Place::Line, Place::Foot];
    let counted = allocations(|| {
        for round in 0..60usize {
            list.set_source(Arc::clone(&rows));
            let count = Count::new("–", " of ").group(",").place(places[round % 2]);
            for buffer in &mut buffers {
                let area = buffer.area;
                ListView::new(&mut list, &FITTED)
                    .header(round % 2 == 0)
                    .end(Some("· end ·"))
                    .empty(Some("Nothing yet."))
                    .scrollbar(Some(bars[round % 3 % 2]))
                    .count(Some(count))
                    .render(area, buffer);
            }
            std::hint::black_box((list.shown(), list.position()));
            std::hint::black_box(list.key(KEYS[round % KEYS.len()]));
        }
    });
    assert_eq!(counted, 0);
}

const MARKS: [&str; 6] = ["⧗", "✓", "10", "9", "100", ""];
const HEAT: [Color; 5] = [
    Color::Green,
    Color::Yellow,
    Color::LightRed,
    Color::Red,
    Color::Rgb(0xff, 0x5f, 0x00),
];

fn rewrite_and_draw(list: &mut List, buffers: &mut [Buffer], number: &mut String, round: usize) {
    let styles = Styles::new()
        .selected(Style::new().bg(Color::Blue).add_modifier(Modifier::BOLD))
        .faint(DIM)
        .base(Style::new().bg(Color::Black))
        .header(Style::new().fg(Color::Yellow).bg(Color::DarkGray))
        .header_key(ACCENT)
        .cursor(Style::new().fg(Color::Yellow))
        .keep_colours(true);
    for step in 0..3 {
        let index = (round * 7 + step * 211) % list.len();
        let Some(row) = list.row_mut(index) else {
            continue;
        };
        number.clear();
        write!(number, "{} kB · row {index} ținută", round * 1031 + step).unwrap();
        let warm = Style::new().fg(HEAT[(round + step) % HEAT.len()]);
        if let Some(cell) = row.cells_mut().get_mut(1) {
            cell.set_text(number);
            cell.set_style(warm);
        }
        if let Some(mark) = row.leading_mut() {
            mark.set_text(MARKS[(round + step) % MARKS.len()]);
            mark.set_style(warm);
        }
        if let Some(cell) = row.cells_mut().first_mut() {
            cell.set_part(1, number);
            cell.set_part_style(1, warm);
            cell.set_part_style(0, warm.add_modifier(Modifier::BOLD));
            cell.set_part_style(2, warm);
        }
    }
    list.update(|rows| {
        let warm = Style::new().fg(HEAT[round % HEAT.len()]);
        for row in rows.iter_mut().skip(round % 12).step_by(12) {
            if let Some(cell) = row.cells_mut().last_mut() {
                cell.set_style(warm);
            }
            if let Some(mark) = row.leading_mut() {
                mark.set_style(warm.add_modifier(Modifier::DIM));
            }
        }
    });
    let at = list.selected().unwrap_or(0);
    list.select_range((!round.is_multiple_of(4)).then_some((at.saturating_sub(round % 9), at)));
    let columns: &[Column] = if round.is_multiple_of(2) {
        &GROUPED
    } else {
        &FITTED
    };
    for buffer in buffers.iter_mut() {
        let area = buffer.area;
        ListView::new(list, columns)
            .styles(styles)
            .header(!round.is_multiple_of(3))
            .key_column(Some(round % 4))
            .end(Some("· end ·"))
            .render(area, buffer);
        for &(at, x, width) in list.columns() {
            std::hint::black_box((at, x.saturating_add(width)));
        }
    }
    std::hint::black_box(list.key(KEYS[round % KEYS.len()]));
}

#[test]
fn a_frame_that_rewrites_and_restyles_cells_and_marks_and_draws_a_range_header_styles_a_cursor_style_and_fitted_columns_allocates_nothing()
 {
    let mut list = List::new().keys(Keys::VIM).with_rows((0..600).map(grouped));
    let mut buffers = sizes();
    let mut number = String::with_capacity(64);
    let cold = allocations(|| {
        for round in 0..90 {
            rewrite_and_draw(&mut list, &mut buffers, &mut number, round);
        }
    });
    assert!(cold > 0);
    let counted = allocations(|| {
        for round in 0..90 {
            rewrite_and_draw(&mut list, &mut buffers, &mut number, round);
        }
    });
    assert_eq!(counted, 0);
    assert_eq!(list.len(), 600);
}
