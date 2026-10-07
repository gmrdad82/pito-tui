use std::collections::HashSet;
use std::fmt;
use std::fs;
use std::io::{self, Write};
use std::path::{Path, PathBuf};
use std::time::Duration;

use crossterm::event::KeyEvent;
use ratatui::buffer::{Buffer, Cell};
use ratatui::style::Modifier;

use crate::dump;
use crate::shell::Tui;

const HEAD: &str = "pito-tui frame 1";
const SIZES: [(u16, u16); 4] = [(40, 12), (80, 24), (120, 34), (200, 60)];

#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub struct Script {
    pub name: String,
    pub keys: Vec<KeyEvent>,
    pub settle: bool,
    pub at: Option<Duration>,
}

impl Script {
    pub fn new(name: impl Into<String>, keys: impl IntoIterator<Item = KeyEvent>) -> Self {
        Script {
            name: name.into(),
            keys: keys.into_iter().collect(),
            settle: true,
            at: None,
        }
    }

    pub fn loading(mut self) -> Self {
        self.settle = false;
        self
    }

    pub fn at(mut self, at: Duration) -> Self {
        self.at = Some(at);
        self
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub struct Walk {
    pub sizes: Vec<(u16, u16)>,
    pub screens: bool,
    pub scripts: Vec<Script>,
}

impl Walk {
    pub fn new() -> Self {
        Walk {
            sizes: SIZES.to_vec(),
            screens: true,
            scripts: Vec::new(),
        }
    }

    pub fn sizes(mut self, sizes: impl IntoIterator<Item = (u16, u16)>) -> Self {
        self.sizes = sizes.into_iter().collect();
        self
    }

    pub fn screens(mut self, screens: bool) -> Self {
        self.screens = screens;
        self
    }

    pub fn script(mut self, script: Script) -> Self {
        self.scripts.push(script);
        self
    }
}

impl Default for Walk {
    fn default() -> Self {
        Walk::new()
    }
}

#[derive(Debug, Clone, Default, PartialEq, Eq)]
#[non_exhaustive]
pub struct Look {
    pub symbol: String,
    pub fg: String,
    pub bg: String,
    pub modifier: u16,
}

impl fmt::Display for Look {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let modifier = Modifier::from_bits_truncate(self.modifier);
        write!(f, "{:?} {} {} {modifier:?}", self.symbol, self.fg, self.bg)
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum Difference {
    Cell {
        scenario: String,
        size: (u16, u16),
        x: u16,
        y: u16,
        before: Look,
        after: Look,
    },
    Added {
        scenario: String,
        size: (u16, u16),
    },
    Removed {
        scenario: String,
        size: (u16, u16),
    },
}

impl fmt::Display for Difference {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Difference::Cell {
                scenario,
                size: (width, height),
                x,
                y,
                before,
                after,
            } => write!(f, "{scenario} {width}x{height} {x},{y}: {before} → {after}"),
            Difference::Added {
                scenario,
                size: (width, height),
            } => write!(f, "{scenario} {width}x{height}: +"),
            Difference::Removed {
                scenario,
                size: (width, height),
            } => write!(f, "{scenario} {width}x{height}: -"),
        }
    }
}

enum Run {
    Screen(usize),
    Script(usize),
}

fn slug(name: &str) -> String {
    let mut out = String::new();
    for c in name.chars().flat_map(char::to_lowercase) {
        if c.is_alphanumeric() {
            out.push(c);
        } else if !out.is_empty() && !out.ends_with('-') {
            out.push('-');
        }
    }
    out.trim_end_matches('-').to_string()
}

fn screen(index: usize, name: &str) -> String {
    format!("{index:02}-{}", slug(name))
}

fn file((width, height): (u16, u16)) -> String {
    format!("{width}x{height}")
}

fn runs<E: Send + 'static>(walk: &Walk, app: &dyn Fn() -> Tui<E>) -> Vec<(String, Run)> {
    let mut runs = Vec::new();
    if walk.screens {
        let tui = app();
        for (index, name) in tui.names().into_iter().enumerate() {
            if tui.walkable(index) {
                runs.push((screen(index, name), Run::Screen(index)));
            }
        }
    }
    for (index, script) in walk.scripts.iter().enumerate() {
        runs.push((slug(&script.name), Run::Script(index)));
    }
    runs
}

fn render<E: Send + 'static>(
    app: &dyn Fn() -> Tui<E>,
    walk: &Walk,
    run: &Run,
    (width, height): (u16, u16),
) -> Buffer {
    let mut tui = app();
    match run {
        Run::Screen(index) => {
            tui.reveal();
            tui.shot(width, height, &[], true);
            if tui.go(*index) {
                tui.settle();
            }
            tui.frame(width, height)
        }
        Run::Script(index) => {
            let script = &walk.scripts[*index];
            let buffer = tui.shot(width, height, &script.keys, script.settle);
            let Some(at) = script.at else {
                return buffer;
            };
            tui.advance(at);
            if script.settle {
                tui.settle();
            }
            tui.frame(width, height)
        }
    }
}

fn escape(symbol: &str) -> String {
    let mut out = String::with_capacity(symbol.len());
    for c in symbol.chars() {
        match c {
            '\\' => out.push_str("\\\\"),
            '\t' => out.push_str("\\t"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            c => out.push(c),
        }
    }
    out
}

fn unescape(text: &str) -> String {
    let mut out = String::with_capacity(text.len());
    let mut chars = text.chars();
    while let Some(c) = chars.next() {
        if c != '\\' {
            out.push(c);
            continue;
        }
        match chars.next() {
            Some('t') => out.push('\t'),
            Some('n') => out.push('\n'),
            Some('r') => out.push('\r'),
            Some(other) => out.push(other),
            None => out.push('\\'),
        }
    }
    out
}

fn style(cell: &Cell) -> String {
    format!("{} {} {}", cell.fg, cell.bg, cell.modifier.bits())
}

fn encode(buffer: &Buffer) -> String {
    let area = buffer.area;
    let mut out = format!("{HEAD}\n{} {}\n", area.width, area.height);
    for y in area.top()..area.bottom() {
        let cells: Vec<&Cell> = (area.left()..area.right())
            .map(|x| &buffer[(x, y)])
            .collect();
        let symbols: Vec<String> = cells.iter().map(|cell| escape(cell.symbol())).collect();
        out.push_str(&symbols.join("\t"));
        out.push('\n');
        let mut runs: Vec<(usize, String)> = Vec::new();
        for cell in &cells {
            let look = style(cell);
            match runs.last_mut() {
                Some((count, last)) if *last == look => *count += 1,
                _ => runs.push((1, look)),
            }
        }
        let runs: Vec<String> = runs
            .into_iter()
            .map(|(count, look)| format!("{count} {look}"))
            .collect();
        out.push_str(&runs.join("\t"));
        out.push('\n');
    }
    out
}

fn broken(why: &str) -> io::Error {
    io::Error::new(io::ErrorKind::InvalidData, why.to_string())
}

struct Frame {
    width: u16,
    height: u16,
    cells: Vec<Look>,
}

fn decode(text: &str) -> io::Result<Frame> {
    let mut lines = text.lines();
    if lines.next() != Some(HEAD) {
        return Err(broken(HEAD));
    }
    let size = lines.next().ok_or_else(|| broken(HEAD))?;
    let (width, height) = size
        .split_once(' ')
        .and_then(|(width, height)| Some((width.parse().ok()?, height.parse().ok()?)))
        .ok_or_else(|| broken(size))?;
    let mut cells = Vec::with_capacity(usize::from(width) * usize::from(height));
    for _ in 0..height {
        let symbols = lines.next().ok_or_else(|| broken(size))?;
        let runs = lines.next().ok_or_else(|| broken(size))?;
        let mut row: Vec<Look> = symbols
            .split('\t')
            .map(|symbol| Look {
                symbol: unescape(symbol),
                ..Look::default()
            })
            .collect();
        let mut x = 0;
        for run in runs.split('\t').filter(|run| !run.is_empty()) {
            let mut parts = run.split(' ');
            let (Some(count), Some(fg), Some(bg), Some(modifier)) =
                (parts.next(), parts.next(), parts.next(), parts.next())
            else {
                return Err(broken(run));
            };
            let count: usize = count.parse().map_err(|_| broken(run))?;
            let modifier: u16 = modifier.parse().map_err(|_| broken(run))?;
            for look in row.iter_mut().skip(x).take(count) {
                look.fg = fg.to_string();
                look.bg = bg.to_string();
                look.modifier = modifier;
            }
            x += count;
        }
        row.resize(usize::from(width), Look::default());
        cells.extend(row);
    }
    Ok(Frame {
        width,
        height,
        cells,
    })
}

fn cells(scenario: &str, size: (u16, u16), before: &Frame, after: &Frame) -> Vec<Difference> {
    let width = before.width.max(after.width);
    let height = before.height.max(after.height);
    let look = |frame: &Frame, x: u16, y: u16| {
        if x >= frame.width || y >= frame.height {
            return Look::default();
        }
        frame.cells[usize::from(y) * usize::from(frame.width) + usize::from(x)].clone()
    };
    let mut out = Vec::new();
    for y in 0..height {
        for x in 0..width {
            let (was, now) = (look(before, x, y), look(after, x, y));
            if was != now {
                out.push(Difference::Cell {
                    scenario: scenario.to_string(),
                    size,
                    x,
                    y,
                    before: was,
                    after: now,
                });
            }
        }
    }
    out
}

struct Saved {
    scenario: String,
    size: (u16, u16),
    path: PathBuf,
}

fn frames(dir: &Path) -> io::Result<Vec<Saved>> {
    let mut out = Vec::new();
    for folder in fs::read_dir(dir)? {
        let folder = folder?.path();
        if !folder.is_dir() {
            continue;
        }
        let scenario = folder
            .file_name()
            .map(|name| name.to_string_lossy().into_owned())
            .unwrap_or_default();
        for frame in fs::read_dir(&folder)? {
            let path = frame?.path();
            let size = path
                .file_name()
                .and_then(|name| dump::size(&name.to_string_lossy()));
            if let Some(size) = size {
                let scenario = scenario.clone();
                out.push(Saved {
                    scenario,
                    size,
                    path,
                });
            }
        }
    }
    out.sort_by(|a, b| (&a.scenario, a.size).cmp(&(&b.scenario, b.size)));
    Ok(out)
}

#[derive(Debug)]
struct Against {
    dir: PathBuf,
    seen: HashSet<PathBuf>,
    differences: Vec<Difference>,
}

impl Against {
    fn new(dir: &Path) -> Self {
        Against {
            dir: dir.to_path_buf(),
            seen: HashSet::new(),
            differences: Vec::new(),
        }
    }

    fn path(&self, scenario: &str, size: (u16, u16)) -> PathBuf {
        self.dir.join(scenario).join(file(size))
    }

    fn frame(&mut self, scenario: &str, size: (u16, u16), after: &Frame) -> io::Result<()> {
        let path = self.path(scenario, size);
        match fs::read_to_string(&path) {
            Ok(text) => {
                let before = decode(&text)?;
                self.differences
                    .extend(cells(scenario, size, &before, after));
            }
            Err(error) if error.kind() == io::ErrorKind::NotFound => {
                let scenario = scenario.to_string();
                self.differences.push(Difference::Added { scenario, size });
            }
            Err(error) => return Err(error),
        }
        self.seen.insert(path);
        Ok(())
    }

    fn finish(mut self) -> io::Result<Vec<Difference>> {
        let mut removed: Vec<Difference> = frames(&self.dir)?
            .into_iter()
            .filter(|saved| !self.seen.contains(&saved.path))
            .map(|Saved { scenario, size, .. }| Difference::Removed { scenario, size })
            .collect();
        removed.sort_by_key(ToString::to_string);
        self.differences.extend(removed);
        Ok(self.differences)
    }
}

fn fresh(dir: &Path) -> io::Result<()> {
    if dir.exists() && fs::read_dir(dir)?.next().is_some() {
        let taken = dir.display().to_string();
        return Err(io::Error::new(io::ErrorKind::AlreadyExists, taken));
    }
    Ok(())
}

#[derive(Debug)]
pub struct Recorder {
    dir: PathBuf,
    frames: usize,
    against: Option<Against>,
}

impl Recorder {
    pub fn new(dir: impl Into<PathBuf>) -> io::Result<Self> {
        let dir = dir.into();
        fresh(&dir)?;
        fs::create_dir_all(&dir)?;
        Ok(Recorder {
            dir,
            frames: 0,
            against: None,
        })
    }

    pub fn against(dir: impl Into<PathBuf>) -> io::Result<Self> {
        let dir = dir.into();
        fs::read_dir(&dir)?;
        Ok(Recorder {
            against: Some(Against::new(&dir)),
            dir,
            frames: 0,
        })
    }

    pub fn screen(&mut self, index: usize, name: &str, buffer: &Buffer) -> io::Result<()> {
        self.write(&screen(index, name), buffer)
    }

    pub fn record(&mut self, name: &str, buffer: &Buffer) -> io::Result<()> {
        self.write(&slug(name), buffer)
    }

    pub fn frames(&self) -> usize {
        self.frames
    }

    pub fn finish(self) -> io::Result<Vec<Difference>> {
        self.against.map_or(Ok(Vec::new()), Against::finish)
    }

    fn write(&mut self, scenario: &str, buffer: &Buffer) -> io::Result<()> {
        let size = (buffer.area.width, buffer.area.height);
        if let Some(against) = &mut self.against {
            let path = against.path(scenario, size);
            if against.seen.contains(&path) {
                let taken = path.display().to_string();
                return Err(io::Error::new(io::ErrorKind::AlreadyExists, taken));
            }
            against.frame(scenario, size, &decode(&encode(buffer))?)?;
            self.frames += 1;
            return Ok(());
        }
        let folder = self.dir.join(scenario);
        fs::create_dir_all(&folder)?;
        fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(folder.join(file(size)))?
            .write_all(encode(buffer).as_bytes())?;
        self.frames += 1;
        Ok(())
    }
}

pub fn capture<E: Send + 'static>(
    dir: &Path,
    walk: &Walk,
    app: impl Fn() -> Tui<E>,
) -> io::Result<usize> {
    fresh(dir)?;
    let mut frames = 0;
    for (scenario, run) in runs(walk, &app) {
        let folder = dir.join(&scenario);
        fs::create_dir_all(&folder)?;
        for &size in &walk.sizes {
            let buffer = render(&app, walk, &run, size);
            fs::write(folder.join(file(size)), encode(&buffer))?;
            frames += 1;
        }
    }
    Ok(frames)
}

pub fn compare<E: Send + 'static>(
    dir: &Path,
    walk: &Walk,
    app: impl Fn() -> Tui<E>,
) -> io::Result<Vec<Difference>> {
    let mut against = Against::new(dir);
    for (scenario, run) in runs(walk, &app) {
        for &size in &walk.sizes {
            let after = decode(&encode(&render(&app, walk, &run, size)))?;
            against.frame(&scenario, size, &after)?;
        }
    }
    against.finish()
}

pub fn compare_dirs(before: &Path, after: &Path) -> io::Result<Vec<Difference>> {
    fs::read_dir(before)?;
    let mut against = Against::new(before);
    for saved in frames(after)? {
        let frame = decode(&fs::read_to_string(&saved.path)?)?;
        against.frame(&saved.scenario, saved.size, &frame)?;
    }
    against.finish()
}
