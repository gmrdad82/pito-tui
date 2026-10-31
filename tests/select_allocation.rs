use std::alloc::{GlobalAlloc, Layout, System};
use std::cell::Cell;

use pito_tui::crossterm::event::{KeyModifiers, MouseButton, MouseEvent, MouseEventKind};
use pito_tui::header::NavKeys;
use pito_tui::ratatui::{
    Frame, Terminal,
    backend::TestBackend,
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
};
use pito_tui::select::{Clip, Fed, Selection};
use pito_tui::{Palette, Screen, Tui};

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

const SIZES: [(u16, u16); 3] = [(80, 24), (200, 60), (480, 135)];

struct Quiet;

impl Clip for Quiet {
    fn osc(&mut self, _sequence: &str) {}

    fn command(&mut self, _text: &str) {}
}

struct Page;

impl Screen<()> for Page {
    fn draw(&mut self, frame: &mut Frame, area: Rect, _palette: &Palette) {
        let buffer = frame.buffer_mut();
        for y in area.top()..area.bottom() {
            buffer.set_string(area.x, y, "steady 日本 body é", Style::new());
        }
    }
}

fn tui() -> Tui<()> {
    let mut tui = Tui::new("steady", "1.0.0", Color::Cyan)
        .nav_keys(NavKeys::NONE)
        .clipboard(Quiet)
        .screen("Page", Page);
    tui.start();
    tui
}

fn mouse(kind: MouseEventKind, column: u16, row: u16) -> MouseEvent {
    MouseEvent {
        kind,
        column,
        row,
        modifiers: KeyModifiers::NONE,
    }
}

fn press(tui: &mut Tui<()>, width: u16, height: u16, release: bool) {
    let (right, bottom) = (width - 3, height - 4);
    tui.mouse(mouse(MouseEventKind::Down(MouseButton::Left), 3, 2));
    tui.mouse(mouse(
        MouseEventKind::Drag(MouseButton::Left),
        right,
        bottom,
    ));
    if release {
        tui.mouse(mouse(MouseEventKind::Up(MouseButton::Left), right, bottom));
    }
}

fn steady(tui: &mut Tui<()>, width: u16, height: u16) -> (usize, usize) {
    let Ok(mut terminal) = Terminal::new(TestBackend::new(width, height));
    for _ in 0..2 {
        let Ok(_) = terminal.draw(|frame| tui.draw(frame));
    }
    let mut frame = terminal.get_frame();
    let count = counted(|| tui.draw(&mut frame));
    let lit = frame
        .buffer_mut()
        .content
        .iter()
        .filter(|cell| cell.modifier.contains(Modifier::REVERSED))
        .count();
    (count, lit)
}

#[test]
fn the_counter_counts_this_thread_only_while_on() {
    assert_eq!(counted(|| {}), 0);
    let made = counted(|| drop(std::hint::black_box(vec![1u8; 64])));
    assert!(made >= 1, "{made}");
}

#[test]
fn a_steady_frame_with_a_released_selection_shown_allocates_nothing() {
    for (width, height) in SIZES {
        let mut tui = tui();
        tui.frame(width, height);
        press(&mut tui, width, height, true);
        tui.frame(width, height);
        tui.frame(width, height);
        let (count, lit) = steady(&mut tui, width, height);
        assert_eq!(count, 0, "{width}x{height}");
        assert!(lit > usize::from(width), "{width}x{height}: {lit}");
    }
}

#[test]
fn a_steady_frame_while_dragging_allocates_nothing() {
    for (width, height) in SIZES {
        let mut tui = tui();
        tui.frame(width, height);
        press(&mut tui, width, height, false);
        assert!(tui.selection().dragging());
        let (count, lit) = steady(&mut tui, width, height);
        assert_eq!(count, 0, "{width}x{height}");
        assert!(lit > usize::from(width), "{width}x{height}: {lit}");
    }
}

#[test]
fn painting_a_whole_screen_selection_on_its_own_allocates_nothing() {
    for (width, height) in SIZES {
        let mut selection = Selection::new();
        selection.feed(mouse(MouseEventKind::Down(MouseButton::Left), 0, 0));
        let fed = selection.feed(mouse(
            MouseEventKind::Drag(MouseButton::Left),
            width - 1,
            height - 1,
        ));
        assert_eq!(fed, Fed::Drag);
        let mut buffer = Buffer::empty(Rect::new(0, 0, width, height));
        buffer.set_string(0, 0, "日本 é", Style::new());
        let style = Style::new().add_modifier(Modifier::REVERSED);
        let count = counted(|| selection.paint(&mut buffer, style));
        assert_eq!(count, 0, "{width}x{height}");
        assert!(
            buffer
                .content
                .iter()
                .all(|cell| cell.modifier.contains(Modifier::REVERSED))
        );
    }
}

#[test]
fn only_the_frame_after_a_release_takes_the_text() {
    let (width, height) = SIZES[0];
    let mut tui = tui();
    tui.frame(width, height);
    press(&mut tui, width, height, true);
    let Ok(mut terminal) = Terminal::new(TestBackend::new(width, height));
    let mut frame = terminal.get_frame();
    let taking = counted(|| tui.draw(&mut frame));
    assert!(taking > 0);
    let mut frame = terminal.get_frame();
    assert_eq!(counted(|| tui.draw(&mut frame)), 0);
}
