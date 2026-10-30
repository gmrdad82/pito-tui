use std::hint::black_box;
use std::time::{Duration, Instant};

use pito_tui::footer::{
    Confirm, ConfirmBar, Footer, Hint, Input, InputBar, Notice, Segment, Styles, Words,
};
use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
    widgets::Widget,
};

const WIDTH: u16 = 150;
const HEIGHT: u16 = 40;

const HINTS: [Hint; 12] = [
    Hint::new("↑↓", "move"),
    Hint::new("pgup/pgdn", "page").rank(4),
    Hint::new("g/G", "ends").rank(5),
    Hint::new("enter", "open").rank(1),
    Hint::new("s", "stop").rank(2),
    Hint::new("r", "retry").rank(2),
    Hint::new("R", "retry all").rank(3),
    Hint::new("/", "search").rank(3),
    Hint::new("f", "filter").rank(4),
    Hint::new("y", "copy").rank(5),
    Hint::new("?", "help").rank(6),
    Hint::new("ctrl+c", "twice quit").pinned(),
];

const PAGE: [(&str, Style); 2] = [
    ("page 1/2 ", Style::new()),
    ("●○", Style::new().fg(Color::Magenta)),
];

fn main() {
    let frames: u32 = std::env::args()
        .nth(1)
        .and_then(|text| text.parse().ok())
        .unwrap_or(20_000);
    let styles = Styles::new()
        .accent(Style::new().fg(Color::Rgb(0xff, 0xcf, 0x5c)))
        .muted(Style::new().add_modifier(Modifier::DIM))
        .alert(Style::new().fg(Color::Red))
        .ink(Style::new())
        .rule(Style::new().add_modifier(Modifier::DIM));
    let segments = [
        Segment::text(
            " NORMAL · items ",
            Style::new().add_modifier(Modifier::REVERSED),
        ),
        Segment::text(
            "3 selected of 1,204 items in the current view",
            Style::new(),
        )
        .shrink()
        .rank(1),
        Segment::hints(&HINTS).rank(6),
        Segment::text("12:30:45", Style::new()).right().rank(3),
        Segment::spans(&PAGE).right().rank(2),
    ];
    let input = Input::new().with("the quick brown fox jumps over the lazy dog, ș and 日本語");
    let key = Input::new()
        .masked(true)
        .with("sk-0123456789abcdefghijklmnopqrstuvwxyz");
    let words = Words::new("Yes", "No").hint("y/n choose · enter accept · esc cancel");
    let confirm = Confirm::new();
    let area = Rect::new(0, 0, WIDTH, HEIGHT);
    let mut buffer = Buffer::empty(area);
    let mut parts = [Duration::ZERO; 6];
    let mut total = Duration::ZERO;
    let mut worst = Duration::ZERO;
    for _ in 0..frames {
        buffer.reset();
        let started = Instant::now();
        let footer = Footer::new(&HINTS)
            .styles(styles)
            .version("bench", env!("CARGO_PKG_VERSION"))
            .notice(Some(Notice::legend(
                "✓ done · ✗ failed · ■ stopped · ⧗ running",
            )));
        let height = footer.height(WIDTH);
        footer.render(Rect::new(0, HEIGHT - height, WIDTH, height), &mut buffer);
        let hints = Instant::now();
        Footer::status(&segments)
            .styles(styles)
            .version("bench", env!("CARGO_PKG_VERSION"))
            .render(Rect::new(0, 0, WIDTH, 1), &mut buffer);
        let status = Instant::now();
        let wrapped = Footer::new(&HINTS)
            .styles(styles)
            .wrap(true)
            .version("bench", env!("CARGO_PKG_VERSION"));
        let narrow = Rect::new(0, 2, 60, wrapped.height(60));
        wrapped.render(narrow, &mut buffer);
        let wrap = Instant::now();
        ConfirmBar::new("Stop the import of 12 items?", &confirm, words)
            .styles(styles)
            .render(Rect::new(0, 20, WIDTH, 3), &mut buffer);
        let confirmed = Instant::now();
        InputBar::new("Search", &input)
            .hint("enter search · esc cancel")
            .styles(styles)
            .render(Rect::new(0, 26, 48, 3), &mut buffer);
        let typed = Instant::now();
        InputBar::new("Key", &key)
            .styles(styles)
            .render(Rect::new(0, 30, WIDTH, 2), &mut buffer);
        let masked = Instant::now();
        black_box(&buffer);
        let marks = [started, hints, status, wrap, confirmed, typed, masked];
        for (part, pair) in parts.iter_mut().zip(marks.windows(2)) {
            *part += pair[1] - pair[0];
        }
        let took = masked - started;
        total += took;
        worst = worst.max(took);
    }
    let micros = |time: Duration| time.as_secs_f64() * 1e6 / f64::from(frames.max(1));
    println!(
        "pito-footer bench: {frames} frames at {WIDTH}x{HEIGHT}: mean {:.1} µs, worst {:.1} µs",
        micros(total),
        worst.as_secs_f64() * 1e6
    );
    let names = [
        "hints and notice",
        "status row",
        "wrapped hints",
        "confirm bar",
        "input bar",
        "masked input bar",
    ];
    for (name, part) in names.iter().zip(parts) {
        println!("  {name}: {:.2} µs", micros(part));
    }
}
