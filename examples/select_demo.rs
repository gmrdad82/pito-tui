use std::cell::RefCell;
use std::env;
use std::process::ExitCode;
use std::rc::Rc;
use std::sync::Arc;

use pito_tui::crossterm::event::{KeyEvent, KeyModifiers, MouseButton, MouseEvent, MouseEventKind};
use pito_tui::footer::Hint;
use pito_tui::header::NavKeys;
use pito_tui::list::{Column, Key, List, Row};
use pito_tui::ratatui::{
    Frame,
    buffer::Buffer,
    layout::{Position, Rect},
    style::Color,
    text::Line,
};
use pito_tui::select::Clip;
use pito_tui::{Cx, Log, Modes, Palette, Screen, Tui, Words, dump, text};

const NAME: &str = "select_demo";
const SKY: [&str; 4] = [
    "   ·    ✦        ·         ·      ✧       ·        ✦     ·",
    " ✧         ·  ☾       ✦         ·       ·     ✧        ·",
    "      /\\        ·      /\\/\\         ·        /\\     ✦",
    "   __/  \\___/\\_______/    \\___/\\_____/\\_____/  \\______",
];
const JOBS: [[&str; 3]; 5] = [
    ["deploy api", "web", "uploading the bundle"],
    ["build docs", "docs", "waiting for a runner"],
    ["rotate logs", "ops", "starts at 02:00"],
    ["renew certs", "ops", "日本 mirror answered"],
    ["warm caches", "web", "café menu cached"],
];

struct Desk {
    list: List,
    log: Log,
}

impl Desk {
    fn new() -> Self {
        let list = List::new().with_rows(JOBS.map(Row::new));
        let mut log = Log::new("Find");
        let lines = (1..=40)
            .map(|at| Line::from(format!("{at:02} the night shift checked rack {at}")))
            .collect();
        log.set_lines(Arc::new(lines));
        Desk { list, log }
    }
}

impl Screen<()> for Desk {
    fn draw(&mut self, frame: &mut Frame, area: Rect, palette: &Palette) {
        let sky = u16::try_from(SKY.len()).unwrap_or(0).min(area.height);
        for (y, line) in (area.y..area.y + sky).zip(SKY) {
            frame
                .buffer_mut()
                .set_stringn(area.x, y, line, usize::from(area.width), palette.muted);
        }
        let rest = area.height.saturating_sub(sky + 1);
        let rows = u16::try_from(JOBS.len() + 1).unwrap_or(0).min(rest);
        let list = Rect::new(area.x, area.y + sky + 1, area.width, rows);
        let columns = [
            Column::new("Job", 8, 14).pinned(),
            Column::new("Owner", 5, 7).priority(1),
            Column::new("Message", 8, 0),
        ];
        frame.render_widget(palette.view(&mut self.list, &columns).header(true), list);
        let below = list.bottom().saturating_add(1).min(area.bottom());
        let log = Rect::new(area.x, below, area.width, area.bottom() - below);
        self.log.draw(frame, log, palette);
    }

    fn key(&mut self, key: KeyEvent, _cx: &mut Cx<'_, ()>) {
        self.list.key(Key::from(key));
    }

    fn mouse(&mut self, mouse: MouseEvent, _area: Rect, _cx: &mut Cx<'_, ()>) {
        let key = match mouse.kind {
            MouseEventKind::ScrollDown => Key::Down,
            MouseEventKind::ScrollUp => Key::Up,
            _ => return,
        };
        self.list.key(key);
    }
}

#[derive(Clone, Default)]
struct Kept(Rc<RefCell<Vec<String>>>);

impl Clip for Kept {
    fn osc(&mut self, _sequence: &str) {}

    fn command(&mut self, text: &str) {
        self.0.borrow_mut().push(text.to_string());
    }
}

fn tui() -> Tui<()> {
    Tui::new(
        "select demo",
        env!("CARGO_PKG_VERSION"),
        Color::Rgb(0xc8, 0x8c, 0xff),
    )
    .words(
        Words::new()
            .again("ctrl+c again quits")
            .copied("Copied to the clipboard"),
    )
    .hints([
        Hint::new("drag", "selects and copies"),
        Hint::new("↑↓", "move"),
    ])
    .nav_keys(NavKeys::NONE)
    .modes(Modes::new().mouse(true))
    .screen("Desk", Desk::new())
}

fn point(word: &str) -> Option<(u16, u16)> {
    let (column, row) = word.split_once(',')?;
    Some((column.parse().ok()?, row.parse().ok()?))
}

fn press(tui: &mut Tui<()>, kind: MouseEventKind, (column, row): (u16, u16)) {
    tui.mouse(MouseEvent {
        kind,
        column,
        row,
        modifiers: KeyModifiers::NONE,
    });
}

fn marked(buffer: &Buffer, range: Option<(Position, Position)>) -> String {
    let inside = |x: u16, y: u16| {
        range.is_some_and(|(start, end)| (y, x) >= (start.y, start.x) && (y, x) <= (end.y, end.x))
    };
    let mut out = String::new();
    for y in 0..buffer.area.height {
        let mut line = String::new();
        let mut lit = false;
        let mut x = 0;
        while x < buffer.area.width {
            let symbol = buffer[(x, y)].symbol();
            let now = inside(x, y);
            if now != lit {
                line.push_str(if now { "\x1b[7m" } else { "\x1b[27m" });
                lit = now;
            }
            line.push_str(symbol);
            x += u16::try_from(text::cells(symbol)).unwrap_or(1).max(1);
        }
        if lit {
            line.push_str("\x1b[27m");
        }
        out.push_str(line.trim_end());
        out.push('\n');
    }
    out
}

fn shot(width: u16, height: u16, script: &[String]) -> Result<(), String> {
    let kept = Kept::default();
    let mut tui = tui().clipboard(kept.clone());
    tui.start();
    tui.settle();
    tui.frame(width, height);
    let mut words = script.iter();
    while let Some(word) = words.next() {
        if word == "drag" {
            let mut next = || words.next().and_then(|word| point(word));
            let (Some(from), Some(to)) = (next(), next()) else {
                return Err("drag takes two cells such as 4,6 30,8".into());
            };
            press(&mut tui, MouseEventKind::Down(MouseButton::Left), from);
            press(&mut tui, MouseEventKind::Drag(MouseButton::Left), to);
            press(&mut tui, MouseEventKind::Up(MouseButton::Left), to);
        } else {
            for key in dump::keys(word).map_err(|word| format!("can't read the key {word}"))? {
                tui.key(key);
            }
        }
        tui.settle();
        tui.frame(width, height);
    }
    let buffer = tui.frame(width, height);
    print!("{}", marked(&buffer, tui.selection().range()));
    for text in kept.0.borrow().iter() {
        println!("copied {text:?}");
    }
    Ok(())
}

fn main() -> ExitCode {
    let args: Vec<String> = env::args().skip(1).collect();
    if args.first().map(String::as_str) == Some("--dump") {
        let Some((width, height)) = args.get(1).and_then(|size| dump::size(size)) else {
            eprintln!("{NAME}: --dump takes a size such as 90x24, then keys and drags");
            return ExitCode::from(2);
        };
        return match shot(width, height, &args[2..]) {
            Ok(()) => ExitCode::SUCCESS,
            Err(error) => {
                eprintln!("{NAME}: {error}");
                ExitCode::from(2)
            }
        };
    }
    match tui().run() {
        Ok(()) => ExitCode::SUCCESS,
        Err(error) => {
            eprintln!("{NAME}: {error}");
            ExitCode::FAILURE
        }
    }
}
