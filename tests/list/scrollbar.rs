use std::{cell::Cell as Counter, sync::Arc};

use crate::common::*;
use pito_tui::list::{Bar, Build, Column, Count, List, ListView, Place, Row, Shared};
use ratatui::style::{Color, Modifier, Style};
use ratatui::{buffer::Buffer, layout::Rect, widgets::Widget};

const TWO: [Column<'static>; 2] = [
    Column::new("Name", 8, 12).pinned(),
    Column::new("Message", 8, 0),
];
const COUNT: Count<'static> = Count::new("–", " of ").group(",");

fn numbered(count: usize) -> List {
    List::new().with_rows((0..count).map(|n| Row::new([format!("row {n}"), "a message".into()])))
}

fn render<'a>(
    list: &'a mut List,
    width: u16,
    height: u16,
    tweak: impl FnOnce(ListView<'a>) -> ListView<'a>,
) -> Buffer {
    let area = Rect::new(0, 0, width, height);
    let mut buf = Buffer::empty(area);
    let view = ListView::new(list, &TWO)
        .styles(STYLES)
        .header(true)
        .end(Some("· end ·"));
    tweak(view).render(area, &mut buf);
    buf
}

fn bar(buf: &Buffer, from: u16, to: u16) -> String {
    let x = buf.area.right() - 1;
    (from..to).map(|y| buf[(x, y)].symbol()).collect()
}

#[test]
fn a_list_that_fits_draws_as_before() {
    for (len, end) in [(0, true), (5, true), (8, false), (7, true)] {
        let mut plain = numbered(len);
        let mut barred = numbered(len);
        let tail = end.then_some("· end ·");
        let before = render(&mut plain, 30, 9, |view| view.end(tail));
        let after = render(&mut barred, 30, 9, |view| {
            view.end(tail).scrollbar(Some(Bar::LINE)).count(Some(COUNT))
        });
        assert_eq!(before, after, "{len} rows");
        let area = Rect::new(0, 0, 30, 9);
        assert_eq!(barred.hit(area, true, 29, 1), (len > 0).then_some(0));
        let shown = (len > 0).then_some((1, len, len));
        assert_eq!(barred.shown(), shown);
    }
}

#[test]
fn an_overflowing_list_gives_its_last_column_to_the_bar() {
    for select in [0, 17, 39] {
        let mut plain = numbered(40);
        let mut barred = numbered(40);
        plain.select(select);
        barred.select(select);
        let narrow = render(&mut plain, 29, 9, |view| view);
        let wide = render(&mut barred, 30, 9, |view| view.scrollbar(Some(Bar::LINE)));
        for y in 1..9 {
            for x in 0..29 {
                assert_eq!(narrow[(x, y)], wide[(x, y)], "{select}: {x},{y}");
            }
        }
        assert_eq!(plain.columns(), barred.columns());
        assert_eq!(plain.top(), barred.top());
        let area = Rect::new(0, 0, 30, 9);
        assert_eq!(barred.hit(area, true, 28, 1), Some(barred.top()));
        assert_eq!(barred.hit(area, true, 29, 1), None);
    }
    let mut list = numbered(40);
    let lines = |list: &mut List| {
        bar(
            &render(list, 30, 9, |view| view.scrollbar(Some(Bar::LINE))),
            1,
            9,
        )
    };
    assert_eq!(lines(&mut list), "┃╿││││││");
    list.select(20);
    assert_eq!(lines(&mut list), "││╽┃││││");
    list.select(39);
    assert_eq!(lines(&mut list), "││││││╽┃");
    list.select(1);
    let blocks = Bar::new(" ", "█");
    let drawn = render(&mut list, 30, 9, |view| view.scrollbar(Some(blocks)));
    assert_eq!(bar(&drawn, 1, 9), " ██     ");
    list.select(38);
    let drawn = render(&mut list, 30, 9, |view| view.scrollbar(Some(blocks)));
    assert_eq!(bar(&drawn, 1, 9), "     ██ ");
}

#[test]
fn the_count_takes_the_bottom_line_or_sits_at_the_foot_when_it_fits() {
    assert_eq!(COUNT.label(12, 12, 340).to_string(), "12 of 340");
    assert_eq!(
        COUNT.label(1_693, 1_701, 1_234_567).to_string(),
        "1,693–1,701 of 1,234,567"
    );
    assert_eq!(
        Count::new("-", "/").label(999, 1000, 1000).to_string(),
        "999-1000/1000"
    );
    let line = |width, place, last| {
        let mut list = numbered(3_400);
        list.select(if last { 3_399 } else { 1_700 });
        let count = COUNT.place(place);
        let buf = render(&mut list, width, 10, |view| {
            view.count(Some(count)).scrollbar(Some(Bar::LINE))
        });
        (list, read(&buf).text)
    };
    let (list, text) = line(40, Place::Line, false);
    assert_eq!(text[9], "                    1,694–1,701 of 3,400");
    assert_eq!(text[8], "▌ row 1700      a message              │");
    assert_eq!(list.shown(), Some((1_694, 1_701, 3_400)));
    assert_eq!(list.position(), Some((1_701, 3_400)));
    assert_eq!(list.page(), 7);
    let area = Rect::new(0, 0, 40, 10);
    assert_eq!(list.hit(area, true, 5, 8), Some(1_700));
    assert_eq!(list.hit(area, true, 5, 9), None);
    let (list, text) = line(50, Place::Foot, false);
    assert_eq!(
        text[9],
        "▌ row 1700      a message    1,693–1,701 of 3,400│"
    );
    assert_eq!(list.hit(Rect::new(0, 0, 50, 10), true, 5, 9), Some(1_700));
    let (_, text) = line(40, Place::Foot, false);
    assert_eq!(text[9], "▌ row 1700      a message              │");
    let (_, text) = line(50, Place::Foot, true);
    assert_eq!(
        text[9],
        "  · end ·                    3,393–3,400 of 3,400┃"
    );
}

struct Counted {
    built: Counter<usize>,
}

impl Build<usize> for Counted {
    fn row(&self, _: usize, item: &usize) -> Row {
        self.built.set(self.built.get() + 1);
        Row::new([format!("job {item}"), "queued".into()])
    }

    fn selectable(&self, _: usize, _: &usize) -> bool {
        true
    }
}

#[test]
fn a_shared_list_draws_its_bar_and_count_building_only_the_rows_on_screen() {
    let items: Arc<Vec<usize>> = Arc::new((0..100_000).collect());
    let builder = Counted {
        built: Counter::new(0),
    };
    let mut list = List::from_source(Shared::new(items, builder));
    list.select(50_000);
    let area = Rect::new(0, 0, 40, 12);
    let mut buf = Buffer::empty(area);
    ListView::new(&mut list, &TWO)
        .header(true)
        .scrollbar(Some(Bar::LINE))
        .count(Some(COUNT))
        .render(area, &mut buf);
    assert!(list.source().builder().built.get() <= 20);
    assert_eq!(list.shown(), Some((49_992, 50_001, 100_000)));
    let text = read(&buf).text;
    assert_eq!(text[11], "                49,992–50,001 of 100,000");
    assert_eq!(bar(&buf, 1, 11), "││││╽╿││││");
}

const BLUE: Style = Style::new().fg(Color::Blue);
const RED: Style = Style::new().fg(Color::Red);
const YELLOW: Style = Style::new().fg(Color::Yellow);

#[test]
fn track_thumb_and_count_styles_reach_their_cells() {
    let mut list = numbered(40);
    let styles = STYLES.track(BLUE).thumb(RED).count(YELLOW);
    let buf = render(&mut list, 30, 9, |view| {
        view.styles(styles)
            .scrollbar(Some(Bar::LINE))
            .count(Some(COUNT))
    });
    assert_eq!(bar(&buf, 1, 8), "┃││││││");
    assert_eq!(buf[(29, 1)].fg, Color::Red);
    for y in 2..8 {
        assert_eq!(buf[(29, y)].fg, Color::Blue, "{y}");
    }
    assert_eq!(read(&buf).text[8], "                     1–7 of 40");
    for x in 21..30 {
        assert_eq!(buf[(x, 8)].fg, Color::Yellow, "{x}");
    }
    assert_eq!(buf[(5, 1)].fg, Color::Magenta);
    let mut plain = numbered(40);
    let buf = render(&mut plain, 30, 9, |view| {
        view.scrollbar(Some(Bar::LINE)).count(Some(COUNT))
    });
    assert_eq!(buf[(29, 1)].fg, Color::Magenta);
    assert_eq!(buf[(29, 2)].fg, Color::Reset);
    assert!(buf[(29, 2)].modifier.contains(Modifier::DIM));
    assert!(buf[(29, 8)].modifier.contains(Modifier::DIM));
    assert_eq!(buf[(29, 8)].fg, Color::Reset);
}

#[test]
fn custom_halves_draw_at_odd_and_even_thumb_edges() {
    let halves = Bar::new(" ", "█").halves("▄", "▀");
    assert_ne!(halves, Bar::new(" ", "█"));
    let mut list = numbered(40);
    let lines = |list: &mut List| {
        let buf = render(list, 30, 9, |view| view.scrollbar(Some(halves)));
        bar(&buf, 1, 9)
    };
    assert_eq!(lines(&mut list), "█▀      ");
    list.select(20);
    assert_eq!(lines(&mut list), "  ▄█    ");
    list.select(39);
    assert_eq!(lines(&mut list), "      ▄█");
    let mut plain = numbered(40);
    plain.select(20);
    let whole = bar(
        &render(&mut plain, 30, 9, |view| {
            view.scrollbar(Some(Bar::new(" ", "█")))
        }),
        1,
        9,
    );
    assert_eq!(whole, "  ██    ");
}

#[test]
fn the_default_bar_is_the_line() {
    assert_eq!(Bar::default(), Bar::LINE);
    assert_eq!(Bar::default(), Bar::new("│", "┃").halves("╽", "╿"));
    let mut a = numbered(40);
    let mut b = numbered(40);
    a.select(20);
    b.select(20);
    let by_default = render(&mut a, 30, 9, |view| view.scrollbar(Some(Bar::default())));
    let by_line = render(&mut b, 30, 9, |view| view.scrollbar(Some(Bar::LINE)));
    assert_eq!(by_default, by_line);
}

#[test]
fn a_count_too_long_to_hold_draws_nothing() {
    let long = "x".repeat(130);
    let edge = "y".repeat(121);
    let over = "y".repeat(122);
    let mut list = numbered(40);
    let buf = render(&mut list, 30, 9, |view| {
        view.count(Some(Count::new("–", &long)))
    });
    assert_eq!(read(&buf).text[8], "");
    assert_eq!(list.shown(), Some((1, 7, 40)));
    assert_eq!(read(&buf).text[7], "  row 6         a message");
    let mut list = numbered(40);
    let buf = render(&mut list, 200, 9, |view| {
        view.count(Some(Count::new("–", &edge)))
    });
    let text = &read(&buf).text[8];
    assert!(text.ends_with(&format!("1–7{edge}40")), "{text}");
    let mut list = numbered(40);
    let buf = render(&mut list, 200, 9, |view| {
        view.count(Some(Count::new("–", &over)))
    });
    assert_eq!(read(&buf).text[8], "");
}

#[test]
fn the_foot_count_takes_the_whole_line_under_a_blank_row() {
    let rows = (0..30).map(|n| match n {
        8 => Row::blank(),
        n => Row::new([format!("row {n}"), "a message".into()]),
    });
    let mut list = List::new().with_rows(rows);
    let buf = render(&mut list, 30, 10, |view| {
        view.end(None).count(Some(COUNT.place(Place::Foot)))
    });
    let text = read(&buf).text;
    assert_eq!(text[8], "  row 7         a message");
    assert_eq!(text[9], "                     1–9 of 30");
    assert_eq!(list.shown(), Some((1, 9, 30)));
    let mut filled = numbered(30);
    let buf = render(&mut filled, 30, 10, |view| {
        view.end(None).count(Some(COUNT.place(Place::Foot)))
    });
    assert_eq!(read(&buf).text[9], "  row 8         a message");
}

#[test]
fn a_half_glyph_carries_the_thumb_style_and_the_track_beside_it_does_not() {
    let halves = Bar::new(" ", "█").halves("▄", "▀");
    let mut list = numbered(40);
    list.select(20);
    let buf = render(&mut list, 30, 9, |view| {
        view.styles(STYLES.track(BLUE).thumb(RED))
            .scrollbar(Some(halves))
    });
    assert_eq!(bar(&buf, 1, 9), "  ▄█    ");
    assert_eq!(buf[(29, 3)].symbol(), "▄");
    assert_eq!(buf[(29, 3)].fg, Color::Red);
    assert_eq!(buf[(29, 4)].fg, Color::Red);
    assert_eq!(buf[(29, 2)].fg, Color::Blue);
    assert_eq!(buf[(29, 5)].fg, Color::Blue);
    let mut list = numbered(40);
    let buf = render(&mut list, 30, 9, |view| {
        view.styles(STYLES.track(BLUE).thumb(RED))
            .scrollbar(Some(halves))
    });
    assert_eq!(buf[(29, 2)].symbol(), "▀");
    assert_eq!(buf[(29, 2)].fg, Color::Red);
    assert_eq!(buf[(29, 3)].fg, Color::Blue);
}

#[test]
fn an_area_taller_or_wider_than_the_buffer_draws_only_inside_it() {
    let mut list = numbered(40);
    let mut buf = Buffer::empty(Rect::new(0, 0, 30, 5));
    ListView::new(&mut list, &TWO)
        .styles(STYLES)
        .header(true)
        .scrollbar(Some(Bar::LINE))
        .render(Rect::new(0, 0, 40, 12), &mut buf);
    assert_eq!(buf.area, Rect::new(0, 0, 30, 5));
    let text = read(&buf).text;
    assert_eq!(text[1], "▌ row 0         a message    ┃");
    assert_eq!(text[4], "  row 3         a message    │");
    assert_eq!(list.shown(), Some((1, 4, 40)));
    let mut list = numbered(3);
    let mut buf = Buffer::empty(Rect::new(0, 5, 30, 4));
    ListView::new(&mut list, &TWO)
        .styles(STYLES)
        .render(Rect::new(0, 0, 30, 20), &mut buf);
    let text = read(&buf).text;
    assert_eq!(text[0], "▌ row 0         a message");
    assert_eq!(text[2], "  row 2         a message");
    assert_eq!(text[3], "");
    assert_eq!(list.shown(), Some((1, 3, 3)));
}

struct Short {
    text: String,
    room: usize,
}

impl std::fmt::Write for Short {
    fn write_str(&mut self, s: &str) -> std::fmt::Result {
        if self.text.len() + s.len() > self.room {
            return Err(std::fmt::Error);
        }
        self.text.push_str(s);
        Ok(())
    }
}

#[test]
fn a_label_into_a_writer_that_fills_up_stops_at_the_first_piece_that_does_not_fit() {
    use std::fmt::Write as _;
    let label = COUNT.label(1_234, 5_678, 12_345);
    let full = "1,234–5,678 of 12,345";
    for room in 0..full.len() {
        let mut short = Short {
            text: String::new(),
            room,
        };
        assert!(write!(short, "{label}").is_err(), "{room}");
        assert!(full.starts_with(&short.text), "{room}: {}", short.text);
        assert!(short.text.len() <= room);
    }
    let mut short = Short {
        text: String::new(),
        room: full.len(),
    };
    assert!(write!(short, "{label}").is_ok());
    assert_eq!(short.text, full);
}
