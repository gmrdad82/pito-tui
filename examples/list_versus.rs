use std::alloc::{GlobalAlloc, Layout, System};
use std::hint::black_box;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::time::{Duration, Instant};

use pito_tui::list::{Cell, Column, List, ListView, Mark, Row, Shared, Source, Styles};
use ratatui::{
    buffer::Buffer,
    layout::{Constraint, Rect},
    style::{Color, Modifier, Style},
    widgets::{self, StatefulWidget, TableState, Widget},
};

struct Counting;

static COUNT: AtomicUsize = AtomicUsize::new(0);
static BYTES: AtomicUsize = AtomicUsize::new(0);

unsafe impl GlobalAlloc for Counting {
    unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
        COUNT.fetch_add(1, Ordering::Relaxed);
        BYTES.fetch_add(layout.size(), Ordering::Relaxed);
        unsafe { System.alloc(layout) }
    }

    unsafe fn dealloc(&self, ptr: *mut u8, layout: Layout) {
        unsafe { System.dealloc(ptr, layout) }
    }

    unsafe fn realloc(&self, ptr: *mut u8, layout: Layout, new_size: usize) -> *mut u8 {
        COUNT.fetch_add(1, Ordering::Relaxed);
        BYTES.fetch_add(new_size, Ordering::Relaxed);
        unsafe { System.realloc(ptr, layout, new_size) }
    }
}

#[global_allocator]
static ALLOCATOR: Counting = Counting;

const WIDTH: u16 = 150;
const HEIGHT: u16 = 40;
const BODY: usize = HEIGHT as usize - 1;
const ITEMS: usize = 20_000;

struct Item {
    name: String,
    version: String,
    owner: String,
    state: String,
    message: String,
}

fn item(n: usize) -> Item {
    Item {
        name: format!("item {n} ținută 日本語"),
        version: "v1 → v2".into(),
        owner: "owner".into(),
        state: "running".into(),
        message: "a message that is long enough to be clipped at the right edge".into(),
    }
}

fn row(item: &Item) -> Row {
    Row::new([
        Cell::new(item.name.as_str()),
        Cell::new(item.version.as_str()).faint(),
        Cell::new(item.owner.as_str()),
        Cell::new(item.state.as_str()).style(Style::new().fg(Color::Green)),
        Cell::new(item.message.as_str()),
    ])
    .mark(Mark::new("⧗").style(Style::new().fg(Color::Green)))
}

fn table_row(item: &Item) -> widgets::Row<'_> {
    widgets::Row::new([
        widgets::Cell::new("⧗").style(Style::new().fg(Color::Green)),
        widgets::Cell::new(item.name.as_str()),
        widgets::Cell::new(item.version.as_str()).style(Style::new().add_modifier(Modifier::DIM)),
        widgets::Cell::new(item.owner.as_str()),
        widgets::Cell::new(item.state.as_str()).style(Style::new().fg(Color::Green)),
        widgets::Cell::new(item.message.as_str()),
    ])
}

const SELECTED: Style = Style::new()
    .fg(Color::Rgb(0xff, 0xcf, 0x5c))
    .add_modifier(Modifier::BOLD);
const FAINT: Style = Style::new().add_modifier(Modifier::DIM);

const COLUMNS: [Column; 5] = [
    Column::new("Name", 10, 24).pinned(),
    Column::new("Version", 8, 12).priority(4),
    Column::new("Owner", 5, 12).priority(2),
    Column::new("State", 5, 10).priority(3),
    Column::new("Message", 8, 0),
];

const WIDTHS: [Constraint; 6] = [
    Constraint::Length(1),
    Constraint::Length(24),
    Constraint::Length(12),
    Constraint::Length(12),
    Constraint::Length(10),
    Constraint::Fill(1),
];

fn selection(frame: u32, at: usize) -> usize {
    if frame % 64 < 48 {
        (at + 1).min(ITEMS - 1)
    } else {
        at.saturating_sub(BODY - 1)
    }
}

struct Measure {
    mean: Duration,
    allocations: f64,
    bytes: f64,
}

fn measure(frames: u32, mut draw: impl FnMut(usize, &mut Buffer)) -> Measure {
    let area = Rect::new(0, 0, WIDTH, HEIGHT);
    let mut buffer = Buffer::empty(area);
    let mut at = 0;
    for frame in 0..64 {
        at = selection(frame, at);
        buffer.reset();
        draw(at, &mut buffer);
    }
    at = 0;
    let mut total = Duration::ZERO;
    let mut count = 0;
    let mut bytes = 0;
    for frame in 0..frames {
        at = selection(frame, at);
        buffer.reset();
        let from = (COUNT.load(Ordering::Relaxed), BYTES.load(Ordering::Relaxed));
        let started = Instant::now();
        draw(at, &mut buffer);
        total += started.elapsed();
        count += COUNT.load(Ordering::Relaxed) - from.0;
        bytes += BYTES.load(Ordering::Relaxed) - from.1;
        black_box(&buffer);
    }
    let frames = frames.max(1);
    Measure {
        mean: total / frames,
        allocations: count as f64 / f64::from(frames),
        bytes: bytes as f64 / f64::from(frames),
    }
}

fn report(name: &str, measure: &Measure) {
    println!(
        "{name}: mean {:.1} µs, {:.1} allocations and {:.0} bytes a frame",
        measure.mean.as_secs_f64() * 1e6,
        measure.allocations,
        measure.bytes
    );
}

fn pito<S: Source>(list: &mut List<S>, styles: Styles, at: usize, buffer: &mut Buffer) {
    list.select(at);
    ListView::new(list, &COLUMNS)
        .styles(styles)
        .header(true)
        .render(buffer.area, buffer);
}

fn table<'a>(rows: impl IntoIterator<Item = widgets::Row<'a>>) -> widgets::Table<'a> {
    widgets::Table::new(rows, WIDTHS)
        .header(
            widgets::Row::new(["", "Name", "Version", "Owner", "State", "Message"]).style(FAINT),
        )
        .row_highlight_style(SELECTED)
        .highlight_symbol("▌ ")
        .highlight_spacing(widgets::HighlightSpacing::Always)
}

fn main() {
    let frames: u32 = std::env::args()
        .nth(1)
        .and_then(|text| text.parse().ok())
        .unwrap_or(2_000);
    let items: Arc<Vec<Item>> = Arc::new((0..ITEMS).map(item).collect());
    let styles = Styles::new().selected(SELECTED).faint(FAINT);
    println!(
        "{ITEMS} items, 5 columns and a header at {WIDTH}x{HEIGHT}, the selection moving, {frames} frames each"
    );

    let mut owned = List::new().with_rows(items.iter().map(row));
    let measured = measure(frames, |at, buffer| pito(&mut owned, styles, at, buffer));
    report("pito-list, owned rows", &measured);

    let mut shared =
        List::from_source(Shared::new(Arc::clone(&items), |_, item: &Item| row(item)).marks(1));
    let measured = measure(frames, |at, buffer| pito(&mut shared, styles, at, buffer));
    report("pito-list, shared vector built on demand", &measured);

    let mut state = TableState::new();
    let measured = measure(frames, |at, buffer| {
        state.select(Some(at));
        StatefulWidget::render(
            table(items.iter().map(table_row)),
            buffer.area,
            buffer,
            &mut state,
        );
    });
    report("ratatui Table, every row built each frame", &measured);

    let mut state = TableState::new();
    let mut top = 0;
    let measured = measure(frames, |at, buffer| {
        if at < top {
            top = at;
        } else if at >= top + BODY {
            top = at + 1 - BODY;
        }
        let end = (top + BODY).min(ITEMS);
        state.select(Some(at - top));
        StatefulWidget::render(
            table(items[top..end].iter().map(table_row)),
            buffer.area,
            buffer,
            &mut state,
        );
    });
    report(
        "ratatui Table, visible rows only, scrolled by the app",
        &measured,
    );
}
