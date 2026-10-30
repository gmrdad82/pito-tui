use std::{
    hint::black_box,
    time::{Duration, Instant},
};

use pito_tui::hourglass::Hourglass;
use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
    widgets::Widget,
};

const DRAWS: u64 = 1_000_000;
const CASES: [(u16, u16, &str); 4] = [
    (200, 50, "200×50: GLASS, LABEL, HINT"),
    (80, 24, "80×24: GLASS, LABEL, HINT"),
    (40, 2, "40×2: LABEL, HINT"),
    (12, 1, "12×1: LABEL, CUT"),
];

fn hourglass(ms: u64) -> Hourglass<'static> {
    Hourglass::new(Duration::from_millis(ms))
        .label("Loading the data...")
        .hint(Some("esc stops waiting"))
        .glass(Style::new().fg(Color::Yellow))
        .accent(Style::new().fg(Color::Magenta))
        .muted(Style::new().fg(Color::DarkGray).add_modifier(Modifier::DIM))
}

fn nanos_per_draw(width: u16, height: u16) -> f64 {
    let area = Rect::new(0, 0, width, height);
    let mut buf = Buffer::empty(area);
    for ms in 0..10_000 {
        hourglass(ms).render(area, &mut buf);
    }
    let started = Instant::now();
    for ms in 0..DRAWS {
        black_box(hourglass(black_box(ms))).render(area, &mut buf);
        black_box(&buf);
    }
    started.elapsed().as_nanos() as f64 / DRAWS as f64
}

fn main() {
    println!("title = \"HOURGLASS DRAW TIME\"");
    println!("unit = \"NANOSECONDS PER DRAW\"");
    println!("better = \"lower\"");
    println!("suffix = \" NS\"");
    println!("parties = [{{ name = \"PITO-HOURGLASS\" }}]");
    println!("rows = [");
    for (width, height, label) in CASES {
        let ns = nanos_per_draw(width, height);
        println!("  {{ label = \"{label}\", values = [{ns:.1}] }},");
    }
    println!("]");
    eprintln!(
        "{DRAWS} draws per row, one 1 ms step apart; Hourglass is {} bytes",
        size_of::<Hourglass>()
    );
}
