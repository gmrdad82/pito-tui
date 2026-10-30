use std::hint::black_box;
use std::time::{Duration, Instant};

use pito_tui::header::{Fact, Group, Header, Nav, NavBar, Section, Styles};
use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
    widgets::Widget,
};

const HEIGHT: u16 = 40;

fn nav() -> Nav {
    Nav::new(vec![
        Group::new("Mail")
            .section(Section::new("Inbox"))
            .section(Section::new("Drafts"))
            .section(Section::new("Archive")),
        Group::new("Library")
            .section(Section::new("Items & notes").short("Items"))
            .section(Section::new("Collections").short("Sets"))
            .section(Section::new("Ownership").short("Owners")),
        Group::new("Work")
            .section(Section::new("Imports & exports").short("Imports"))
            .section(Section::new("Schedules"))
            .section(Section::new("Checks")),
    ])
}

fn measure(name: &str, frames: u32, width: u16, mut draw: impl FnMut(u32, &mut Buffer)) {
    let mut buffer = Buffer::empty(Rect::new(0, 0, width, HEIGHT));
    let mut total = Duration::ZERO;
    let mut worst = Duration::ZERO;
    for frame in 0..frames {
        buffer.reset();
        let started = Instant::now();
        draw(frame, &mut buffer);
        let took = started.elapsed();
        black_box(&buffer);
        total += took;
        worst = worst.max(took);
    }
    let mean = total / frames.max(1);
    println!(
        "pito-header bench: {name}: {frames} frames at {width}x{HEIGHT}: mean {:.2} µs, worst {:.1} µs",
        mean.as_secs_f64() * 1e6,
        worst.as_secs_f64() * 1e6
    );
}

fn main() {
    let frames: u32 = std::env::args()
        .nth(1)
        .and_then(|text| text.parse().ok())
        .unwrap_or(20_000);
    let accent = Style::new().fg(Color::Rgb(0xff, 0xcf, 0x5c));
    let dim = Style::new().add_modifier(Modifier::DIM);
    let styles = Styles::new().accent(accent).muted(dim).rule(dim);
    let facts = [
        Fact::new("12 checks", Style::new().fg(Color::Green)),
        Fact::new("1 running", accent),
        Fact::new("2 failed", Style::new().fg(Color::Red)),
        Fact::new("synced just now", dim),
    ];
    let mut top = nav();
    top.go(9);
    let mut drilled = nav();
    drilled.go(9);
    drilled.open("nightly", 4);
    drilled.open("step 3", 2);

    let full = |nav: &Nav, width: u16, frame: u32, buffer: &mut Buffer| {
        let header = Header::new(nav)
            .styles(styles)
            .title(Some("Desk"))
            .left(Some(Fact::new("? help", dim)))
            .right(Some(Fact::new("trial: 9 days left", dim)))
            .facts(&facts)
            .facts_rule(true)
            .crumbs(true)
            .underline(true)
            .notice(
                frame
                    .is_multiple_of(2)
                    .then_some(Fact::new("Saved.", Style::new().fg(Color::Green))),
            );
        header.render(Rect::new(0, 0, width, header.height()), buffer);
    };

    measure("nav rows", frames, 150, |_, buffer| {
        let bar = NavBar::new(&top).styles(styles);
        bar.render(Rect::new(0, 0, 150, bar.height()), buffer);
    });
    measure("full header", frames, 150, |frame, buffer| {
        full(&top, 150, frame, buffer)
    });
    measure("full header, drilled in", frames, 150, |frame, buffer| {
        full(&drilled, 150, frame, buffer)
    });
    measure("full header, abbreviated", frames, 40, |frame, buffer| {
        full(&top, 40, frame, buffer)
    });
    measure("hit test", frames, 150, |frame, _| {
        let bar = NavBar::new(&top).styles(styles);
        black_box(bar.hit(
            Rect::new(0, 0, 150, bar.height()),
            (frame % 150) as u16,
            (frame % 2) as u16,
        ));
    });
}
