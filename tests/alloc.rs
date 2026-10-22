use std::alloc::{GlobalAlloc, Layout, System};
use std::cell::Cell;
use std::time::Instant;

use pito_tui::footer::{Help, Hint};
use pito_tui::ratatui::{
    Frame, Terminal,
    backend::TestBackend,
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
};
use pito_tui::{Band, Head, Palette, Row, Screen, Span, Tui, Words, dump};

struct Counting;

thread_local! {
    static ON: Cell<bool> = const { Cell::new(false) };
    static COUNT: Cell<usize> = const { Cell::new(0) };
}

fn note() {
    if ON.try_with(Cell::get).unwrap_or(false) {
        let _ = COUNT.try_with(|count| count.set(count.get() + 1));
    }
}

unsafe impl GlobalAlloc for Counting {
    unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
        note();
        unsafe { System.alloc(layout) }
    }

    unsafe fn alloc_zeroed(&self, layout: Layout) -> *mut u8 {
        note();
        unsafe { System.alloc_zeroed(layout) }
    }

    unsafe fn realloc(&self, ptr: *mut u8, layout: Layout, new_size: usize) -> *mut u8 {
        note();
        unsafe { System.realloc(ptr, layout, new_size) }
    }

    unsafe fn dealloc(&self, ptr: *mut u8, layout: Layout) {
        unsafe { System.dealloc(ptr, layout) }
    }
}

#[global_allocator]
static GLOBAL: Counting = Counting;

fn counted(work: impl FnOnce()) -> usize {
    COUNT.with(|count| count.set(0));
    ON.with(|on| on.set(true));
    work();
    ON.with(|on| on.set(false));
    COUNT.with(Cell::get)
}

const SIZES: [(u16, u16); 2] = [(120, 40), (200, 60)];

fn facts() -> Vec<Span> {
    vec![
        ("12 unread".into(), Style::new().fg(Color::Yellow)),
        (" · ".into(), Style::new()),
        ("3 drafts".into(), Style::new()),
    ]
}

fn badge() -> Vec<Span> {
    vec![("● live".into(), Style::new().fg(Color::Green))]
}

const HINTS: [Hint<'static>; 3] = [
    Hint::new("↑↓", "scroll"),
    Hint::new("enter", "open"),
    Hint::new("/", "find"),
];

struct Borrowed {
    facts: Vec<Span>,
    hints: Vec<Hint<'static>>,
    badge: Vec<Span>,
}

impl Borrowed {
    fn new() -> Self {
        Borrowed {
            facts: facts(),
            hints: HINTS.to_vec(),
            badge: badge(),
        }
    }
}

fn paint(frame: &mut Frame, area: Rect) {
    frame
        .buffer_mut()
        .set_string(area.x, area.y, "steady body", Style::new());
}

impl Screen<()> for Borrowed {
    fn draw(&mut self, frame: &mut Frame, area: Rect, _palette: &Palette) {
        paint(frame, area);
    }

    fn fact_spans(&self) -> &[Span] {
        &self.facts
    }

    fn hint_list(&self) -> &[Hint<'static>] {
        &self.hints
    }

    fn badge(&self) -> &[Span] {
        &self.badge
    }
}

struct Owned {
    badge: Vec<Span>,
}

impl Owned {
    fn new() -> Self {
        Owned { badge: badge() }
    }
}

impl Screen<()> for Owned {
    fn draw(&mut self, frame: &mut Frame, area: Rect, _palette: &Palette) {
        paint(frame, area);
    }

    fn facts(&self) -> Vec<(String, Style)> {
        facts()
            .into_iter()
            .map(|(text, style)| (text.into_owned(), style))
            .collect()
    }

    fn hints(&self) -> Vec<Hint<'_>> {
        HINTS.to_vec()
    }

    fn badge(&self) -> &[Span] {
        &self.badge
    }
}

struct Plain;

impl Screen<()> for Plain {
    fn draw(&mut self, frame: &mut Frame, area: Rect, _palette: &Palette) {
        paint(frame, area);
    }
}

struct Both {
    spans: Vec<Span>,
    list: Vec<Hint<'static>>,
}

impl Screen<()> for Both {
    fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}

    fn facts(&self) -> Vec<(String, Style)> {
        vec![("owned fact".to_string(), Style::new())]
    }

    fn fact_spans(&self) -> &[Span] {
        &self.spans
    }

    fn hints(&self) -> Vec<Hint<'_>> {
        vec![Hint::new("o", "owned hint")]
    }

    fn hint_list(&self) -> &[Hint<'static>] {
        &self.list
    }
}

struct Strip;

impl Band for Strip {
    fn height(&self, _room: u16, _now: Instant) -> u16 {
        1
    }

    fn draw(&mut self, frame: &mut Frame, area: Rect, _palette: &Palette, _now: Instant) {
        frame
            .buffer_mut()
            .set_string(area.x, area.y, "band row", Style::new());
    }
}

fn typical<S: Screen<()>>(make: impl Fn() -> S, stack: bool) -> Tui<()> {
    let mut tui = built(make, stack);
    tui.start();
    tui
}

fn built<S: Screen<()>>(make: impl Fn() -> S, stack: bool) -> Tui<()> {
    let tui = Tui::<()>::new("steady", "1.0.0", Color::Cyan)
        .title_spans([
            ("Stea", Style::new().add_modifier(Modifier::BOLD)),
            ("dy", Style::new()),
        ])
        .words(Words::new().help("? keys"))
        .hints([Hint::new("ctrl+c", "twice quit").pinned()])
        .group("Mail")
        .screen("Inbox", make())
        .screen("Sent", make())
        .group("Files")
        .screen("Docs", make())
        .band(Strip);
    if stack {
        tui.head(Head::new([
            Row::Title,
            Row::Caption,
            Row::Groups,
            Row::Sections,
            Row::Facts,
            Row::Crumbs,
        ]))
    } else {
        tui.keep_facts(true)
    }
}

fn steady(tui: &mut Tui<()>, width: u16, height: u16) -> (usize, Buffer) {
    let Ok(mut terminal) = Terminal::new(TestBackend::new(width, height));
    for _ in 0..2 {
        let Ok(_) = terminal.draw(|frame| tui.draw(frame));
    }
    let count = {
        let mut frame = terminal.get_frame();
        counted(|| tui.draw(&mut frame))
    };
    (count, terminal.current_buffer_mut().clone())
}

#[test]
fn the_counter_counts_this_thread_only_while_on() {
    assert_eq!(counted(|| {}), 0);
    let made = counted(|| drop(std::hint::black_box(vec![1u8; 64])));
    assert!(made >= 1, "{made}");
    drop(vec![1u8; 64]);
    assert_eq!(counted(|| {}), 0);
}

#[test]
fn a_steady_stacked_frame_with_borrowed_hooks_allocates_nothing() {
    for (width, height) in SIZES {
        let mut tui = typical(Borrowed::new, true);
        let (count, _) = steady(&mut tui, width, height);
        assert_eq!(count, 0, "{width}x{height}");
    }
}

#[test]
fn a_steady_header_frame_with_borrowed_hooks_allocates_nothing() {
    for (width, height) in SIZES {
        let mut tui = typical(Borrowed::new, false);
        let (count, _) = steady(&mut tui, width, height);
        assert_eq!(count, 0, "{width}x{height}");
    }
}

#[test]
fn a_steady_frame_of_a_screen_on_the_defaults_allocates_nothing() {
    for stack in [true, false] {
        for (width, height) in SIZES {
            let mut tui = typical(|| Plain, stack);
            let (count, _) = steady(&mut tui, width, height);
            assert_eq!(count, 0, "{width}x{height} stack {stack}");
        }
    }
}

#[test]
fn a_steady_gated_frame_allocates_nothing() {
    for stack in [true, false] {
        for (width, height) in SIZES {
            let mut tui = built(Borrowed::new, stack).gate(Plain, true);
            tui.start();
            let (count, _) = steady(&mut tui, width, height);
            assert_eq!(count, 0, "{width}x{height} stack {stack}");
        }
    }
}

#[test]
fn owned_hooks_still_allocate_so_the_count_sees_them() {
    for stack in [true, false] {
        let mut tui = typical(Owned::new, stack);
        let (count, _) = steady(&mut tui, 120, 40);
        assert!(count > 0, "stack {stack}");
    }
}

#[test]
fn the_steady_frame_shows_every_part_of_the_chrome() {
    for stack in [true, false] {
        for (width, height) in SIZES {
            let mut tui = typical(Borrowed::new, stack);
            let (_, buffer) = steady(&mut tui, width, height);
            let text = dump::text(&buffer);
            for part in [
                "Steady",
                "Inbox",
                "Sent",
                "12 unread",
                "3 drafts",
                "● live",
                "scroll",
                "twice quit",
                "band row",
                "steady body",
            ] {
                assert!(
                    text.contains(part),
                    "{part} at {width}x{height} stack {stack}:\n{text}"
                );
            }
            if stack {
                assert!(text.contains("Mail") && text.contains("Files"), "{text}");
                assert!(caption_row(&text, "Mail"), "{width}x{height}:\n{text}");
            }
        }
    }
}

#[test]
fn borrowed_hooks_draw_the_same_cells_as_owned_hooks() {
    for stack in [true, false] {
        for (width, height) in SIZES {
            let borrowed = typical(Borrowed::new, stack).frame(width, height);
            let owned = typical(Owned::new, stack).frame(width, height);
            assert_eq!(borrowed, owned, "{width}x{height} stack {stack}");
        }
    }
}

#[test]
fn the_counted_frame_matches_a_headless_frame() {
    for stack in [true, false] {
        let mut tui = typical(Borrowed::new, stack);
        let (_, drawn) = steady(&mut tui, 120, 40);
        assert_eq!(drawn, tui.frame(120, 40), "stack {stack}");
    }
}

#[test]
fn borrowed_hooks_win_over_owned_ones_when_they_hold_anything() {
    for stack in [true, false] {
        let mut tui = typical(
            || Both {
                spans: vec![("borrowed fact".into(), Style::new())],
                list: vec![Hint::new("b", "borrowed hint")],
            },
            stack,
        );
        let text = dump::text(&tui.frame(120, 40));
        assert!(text.contains("borrowed fact"), "{text}");
        assert!(text.contains("borrowed hint"), "{text}");
        assert!(!text.contains("owned fact"), "{text}");
        assert!(!text.contains("owned hint"), "{text}");
    }
}

#[test]
fn empty_borrowed_hooks_fall_back_to_the_owned_ones() {
    for stack in [true, false] {
        let mut tui = typical(
            || Both {
                spans: Vec::new(),
                list: Vec::new(),
            },
            stack,
        );
        let text = dump::text(&tui.frame(120, 40));
        assert!(text.contains("owned fact"), "{text}");
        assert!(text.contains("owned hint"), "{text}");
    }
}

#[test]
fn facts_that_shrink_draw_only_the_parts_still_there() {
    let mut tui = typical(Borrowed::new, true);
    let (_, before) = steady(&mut tui, 120, 40);
    assert!(dump::text(&before).contains("3 drafts"));
    if let Some(screen) = tui.find_mut::<Borrowed>() {
        screen.facts.truncate(1);
        screen.hints.truncate(1);
    }
    let text = dump::text(&tui.frame(120, 40));
    assert!(text.contains("12 unread"), "{text}");
    assert!(!text.contains("3 drafts"), "{text}");
    assert!(text.contains("scroll"), "{text}");
    assert!(!text.contains("open"), "{text}");
    assert!(text.contains("twice quit"), "{text}");
}

fn caption_row(text: &str, group: &str) -> bool {
    text.lines()
        .any(|line| line.contains(group) && !line.contains("Files") && line.contains('─'))
}

struct Captioned {
    caption: Option<Vec<(String, Style)>>,
}

impl Screen<()> for Captioned {
    fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}

    fn caption(&self) -> Option<Vec<(String, Style)>> {
        self.caption.clone()
    }
}

#[test]
fn the_caption_row_falls_back_to_the_group_name() {
    let mut tui = typical(|| Captioned { caption: None }, true);
    let text = dump::text(&tui.frame(120, 40));
    assert!(caption_row(&text, "Mail"), "{text}");
}

#[test]
fn a_screen_caption_beats_the_group_name() {
    let mut tui = typical(
        || Captioned {
            caption: Some(vec![("Captioned".to_string(), Style::new())]),
        },
        true,
    );
    let text = dump::text(&tui.frame(120, 40));
    assert!(text.contains("Captioned"), "{text}");
    assert!(!caption_row(&text, "Mail"), "{text}");
}

#[test]
fn an_empty_screen_caption_still_beats_the_group_name() {
    let mut tui = typical(
        || Captioned {
            caption: Some(Vec::new()),
        },
        true,
    );
    let text = dump::text(&tui.frame(120, 40));
    assert!(text.contains("Mail") && text.contains("Files"), "{text}");
    assert!(!caption_row(&text, "Mail"), "{text}");
}

struct Leading;

impl Screen<()> for Leading {
    fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}

    fn lead(&self, _room: u16) -> Vec<(String, Style)> {
        vec![("led here".to_string(), Style::new())]
    }
}

#[test]
fn the_help_word_and_a_lead_are_joined_by_the_lead_mark() {
    for stack in [true, false] {
        let mut tui = closed(typical(|| Leading, stack));
        let text = dump::text(&tui.frame(120, 40));
        assert!(text.contains("? keys · led here"), "stack {stack}:\n{text}");
    }
}

fn closed(tui: Tui<()>) -> Tui<()> {
    tui.help(Some(Help::new(false)))
}

#[test]
fn a_steady_frame_with_the_help_word_on_the_left_allocates_nothing() {
    for stack in [true, false] {
        for (width, height) in SIZES {
            let mut tui = closed(typical(Borrowed::new, stack));
            let (count, buffer) = steady(&mut tui, width, height);
            assert_eq!(count, 0, "{width}x{height} stack {stack}");
            let text = dump::text(&buffer);
            for part in ["? keys", "Steady", "12 unread", "● live", "twice quit"] {
                assert!(
                    text.contains(part),
                    "{part} at {width}x{height} stack {stack}:\n{text}"
                );
            }
        }
    }
}

#[test]
fn an_open_help_draws_no_help_word_on_the_left() {
    for stack in [true, false] {
        let mut tui = typical(Borrowed::new, stack);
        let text = dump::text(&tui.frame(120, 40));
        assert!(!text.contains("? keys"), "stack {stack}:\n{text}");
        assert!(text.contains("scroll"), "stack {stack}:\n{text}");
    }
}
