use std::borrow::Cow;
use std::fmt::Write;
use std::hint::black_box;
use std::sync::Arc;
use std::time::{Duration, Instant};

use pito_tui::list::{
    Bar, Cell, Column, Count, Key, List, ListView, Mark, Part, Row, Shared, Source, Styles,
};
use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
    widgets::Widget,
};

const WIDTH: u16 = 150;
const HEIGHT: u16 = 40;
const ROWS: usize = 20_000;
const LAZY: usize = 100_000;
const GROUP: usize = 25;

fn row(n: usize) -> Row {
    Row::new([
        Cell::new(format!("item {n} ținută 日本語")),
        Cell::new("v1 → v2").faint(),
        Cell::new("owner"),
        Cell::new("running").style(Style::new().fg(Color::Green)),
        Cell::new("a message that is long enough to be clipped at the right edge"),
    ])
    .mark(Mark::new("⧗").style(Style::new().fg(Color::Green)))
}

struct Grouped;

impl Source for Grouped {
    fn len(&self) -> usize {
        LAZY
    }

    fn row(&self, index: usize) -> Cow<'_, Row> {
        Cow::Owned(if index.is_multiple_of(GROUP) {
            Row::heading([
                Part::new(format!("Group {}", index / GROUP))
                    .style(Style::new().add_modifier(Modifier::BOLD)),
                Part::new(format!(" · {}", GROUP - 1)).faint(),
            ])
        } else {
            row(index)
        })
    }

    fn selectable(&self, index: usize) -> bool {
        !index.is_multiple_of(GROUP)
    }

    fn mark_width(&self) -> u16 {
        1
    }
}

fn run<S: Source>(
    list: &mut List<S>,
    frames: u32,
    styles: Styles,
    columns: &[Column],
    key: Option<usize>,
    scroll: bool,
    mut change: impl FnMut(&mut List<S>, u32),
) -> (Duration, Duration) {
    let area = Rect::new(0, 0, WIDTH, HEIGHT);
    let mut buffer = Buffer::empty(area);
    let mut total = Duration::ZERO;
    let mut worst = Duration::ZERO;
    for frame in 0..frames {
        buffer.reset();
        let step = if frame % 64 < 48 {
            Key::Down
        } else {
            Key::PageUp
        };
        list.key(step);
        let started = Instant::now();
        change(list, frame);
        ListView::new(list, columns)
            .styles(styles)
            .header(true)
            .key_column(key)
            .end(Some("· end ·"))
            .scrollbar(scroll.then_some(Bar::LINE))
            .count(scroll.then(|| Count::new("–", " of ").group(",")))
            .render(area, &mut buffer);
        let took = started.elapsed();
        black_box(&buffer);
        total += took;
        worst = worst.max(took);
    }
    (total / frames.max(1), worst)
}

fn main() {
    let frames: u32 = std::env::args()
        .nth(1)
        .and_then(|text| text.parse().ok())
        .unwrap_or(20_000);
    let styles = Styles::new()
        .selected(
            Style::new()
                .fg(Color::Rgb(0xff, 0xcf, 0x5c))
                .add_modifier(Modifier::BOLD),
        )
        .faint(Style::new().add_modifier(Modifier::DIM));
    let columns = [
        Column::new("Name", 10, 24).pinned(),
        Column::new("Version", 8, 12).priority(4),
        Column::new("Owner", 5, 12).priority(2),
        Column::new("State", 5, 10).priority(3),
        Column::new("Message", 8, 0),
    ];
    let mut owned = List::new().with_rows((0..ROWS).map(row));
    let (mean, worst) = run(&mut owned, frames, styles, &columns, None, false, |_, _| {});
    println!(
        "list bench: {frames} frames of a {ROWS}-row list at {WIDTH}x{HEIGHT}: mean {:.1} µs, worst {:.1} µs",
        mean.as_secs_f64() * 1e6,
        worst.as_secs_f64() * 1e6
    );
    let mut lazy = List::from_source(Grouped);
    let (mean, worst) = run(&mut lazy, frames, styles, &columns, None, false, |_, _| {});
    println!(
        "list bench: {frames} frames of a lazy {LAZY}-row list in groups of {GROUP} at {WIDTH}x{HEIGHT}: mean {:.1} µs, worst {:.1} µs",
        mean.as_secs_f64() * 1e6,
        worst.as_secs_f64() * 1e6
    );
    let themed = styles
        .selected(
            Style::new()
                .bg(Color::DarkGray)
                .add_modifier(Modifier::BOLD),
        )
        .header(Style::new().fg(Color::Gray).bg(Color::Indexed(236)))
        .header_key(Style::new().fg(Color::Yellow).add_modifier(Modifier::BOLD))
        .keep_colours(true);
    let mut text = String::with_capacity(32);
    let marks = ["⧗", "✓", "✗"];
    let mut rewritten = List::new().with_rows((0..ROWS).map(row));
    let (mean, worst) = run(
        &mut rewritten,
        frames,
        themed,
        &columns,
        Some(1),
        false,
        |list, frame| {
            let at = list.selected().unwrap_or(0);
            for line in 0..4 {
                let Some(row) = list.row_mut(at.saturating_sub(line)) else {
                    continue;
                };
                text.clear();
                let _ = write!(text, "{frame} → v{line}");
                if let Some(cell) = row.cells_mut().get_mut(1) {
                    cell.set_text(&text);
                }
                if let Some(mark) = row.leading_mut() {
                    mark.set_text(marks[(frame as usize + line) % marks.len()]);
                }
            }
            list.select_range(Some((at.saturating_sub(8), at)));
        },
    );
    println!(
        "list bench: {frames} frames of a {ROWS}-row list with a range, kept colours, a styled header and four rows rewritten a frame at {WIDTH}x{HEIGHT}: mean {:.1} µs, worst {:.1} µs",
        mean.as_secs_f64() * 1e6,
        worst.as_secs_f64() * 1e6
    );
    let fitted = [
        Column::new("Name", 10, 0).fit(32).pinned(),
        Column::new("Version", 8, 0).fit(12).priority(4),
        Column::new("Owner", 5, 0).fit(12).priority(2),
        Column::new("State", 5, 0).fit(10).priority(3),
        Column::new("Message", 8, 0),
    ];
    let cursor = themed.cursor(Style::new().fg(Color::Rgb(0xff, 0xcf, 0x5c)));
    let mut sized = List::new().with_rows((0..ROWS).map(row));
    let (mean, worst) = run(
        &mut sized,
        frames,
        cursor,
        &fitted,
        Some(0),
        false,
        |list, _| {
            black_box(list.columns());
        },
    );
    println!(
        "list bench: {frames} frames of a {ROWS}-row list with columns that fit their cells, a cursor style and the drawn columns read at {WIDTH}x{HEIGHT}: mean {:.1} µs, worst {:.1} µs",
        mean.as_secs_f64() * 1e6,
        worst.as_secs_f64() * 1e6
    );
    let items: Arc<Vec<usize>> = Arc::new((0..LAZY).collect());
    let mut shared =
        List::from_source(Shared::new(Arc::clone(&items), |_, &n: &usize| row(n)).marks(1));
    let (mean, worst) = run(
        &mut shared,
        frames,
        styles,
        &columns,
        None,
        true,
        |list, _| {
            list.update(|shared| shared.set_items(Arc::clone(&items)));
        },
    );
    println!(
        "list bench: {frames} frames of a shared {LAZY}-item vector built on demand and handed over every frame, with a scrollbar and a count, at {WIDTH}x{HEIGHT}: mean {:.1} µs, worst {:.1} µs",
        mean.as_secs_f64() * 1e6,
        worst.as_secs_f64() * 1e6
    );
}
