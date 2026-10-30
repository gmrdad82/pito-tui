use std::env;
use std::process::ExitCode;
use std::sync::LazyLock;

use pito_tui::crossterm::event::KeyEvent;
use pito_tui::header::NavKeys;
use pito_tui::keys::{Chord, Key, Keymap, Mode, Now, When, bind};
use pito_tui::ratatui::{Frame, layout::Rect, style::Color};
use pito_tui::{Cx, Palette, Screen, Tui, Words, dump};

const NAME: &str = "keys_demo";
const RAIL: Mode = Mode("clips");
const CUT: Mode = Mode("cut");
const PLAYING: When = When("while the cut plays");

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Act {
    Choose,
    Keep,
    Drop,
    Remove,
    Play,
    Stop,
    Switch,
}

fn keymap() -> Keymap<Act> {
    Keymap::new()
        .modes(&[RAIL, CUT])
        .group(
            "Rail",
            [
                bind("up down", "choose", Act::Choose)
                    .on(RAIL)
                    .shown("up and down")
                    .rank(1),
                bind("right", "keeps", Act::Keep).on(RAIL),
                bind("left", "drops", Act::Drop).on(RAIL),
            ],
        )
        .group(
            "Cut",
            [
                bind("left right", "choose", Act::Choose)
                    .on(CUT)
                    .shown("left and right")
                    .rank(1),
                bind("delete", "removes", Act::Remove)
                    .on(CUT)
                    .note("the clip goes back to the rail"),
            ],
        )
        .group(
            "Player",
            [
                bind("space esc", "stops it", Act::Stop)
                    .modes(&[RAIL, CUT])
                    .when(PLAYING)
                    .shown("space or esc"),
                bind("space", "plays the cut", Act::Play).modes(&[RAIL, CUT]),
            ],
        )
        .group(
            "Desk",
            [bind("tab", "switches clips and cut", Act::Switch).rank(2)],
        )
}

static KEYS: LazyLock<Keymap<Act>> = LazyLock::new(keymap);

struct Demo {
    clips: Vec<&'static str>,
    cut: Vec<&'static str>,
    mode: Mode,
    at: usize,
    playing: bool,
    said: String,
}

impl Default for Demo {
    fn default() -> Self {
        Demo {
            clips: vec!["intro", "the slip", "the good take", "outro"],
            cut: Vec::new(),
            mode: RAIL,
            at: 0,
            playing: false,
            said: String::new(),
        }
    }
}

impl Demo {
    fn now(&self) -> Now<'static> {
        let now = Now::new(self.mode);
        if self.playing {
            now.when(&[PLAYING])
        } else {
            now
        }
    }

    fn act(&mut self, act: Act, back: bool) {
        let len = self.list().len();
        match act {
            Act::Choose if back => self.at = self.at.saturating_sub(1),
            Act::Choose => self.at = (self.at + 1).min(len.saturating_sub(1)),
            Act::Keep if len > 0 => {
                let clip = self.clips.remove(self.at);
                self.cut.push(clip);
                self.said = format!("kept {clip}");
            }
            Act::Drop if len > 0 => {
                let clip = self.clips.remove(self.at);
                self.said = format!("dropped {clip}");
            }
            Act::Remove if len > 0 => {
                let clip = self.cut.remove(self.at);
                self.clips.push(clip);
                self.said = format!("{clip} went back to the rail");
            }
            Act::Play => self.playing = true,
            Act::Stop => self.playing = false,
            Act::Switch => {
                self.mode = if self.mode == RAIL { CUT } else { RAIL };
                self.at = 0;
            }
            _ => {}
        }
        self.at = self.at.min(self.list().len().saturating_sub(1));
    }

    fn list(&self) -> &[&'static str] {
        if self.mode == RAIL {
            &self.clips
        } else {
            &self.cut
        }
    }
}

impl Screen<()> for Demo {
    fn start(&mut self, cx: &mut Cx<'_, ()>) {
        cx.keys_now(self.now());
    }

    fn draw(&mut self, frame: &mut Frame, area: Rect, palette: &Palette) {
        let buf = frame.buffer_mut();
        let mut y = area.y;
        if y < area.bottom() {
            let (end, _) = buf.set_stringn(
                area.x,
                y,
                self.mode.0,
                usize::from(area.width),
                palette.selected,
            );
            let mut x = end;
            if self.playing {
                x = buf
                    .set_stringn(
                        x,
                        y,
                        " · the cut plays",
                        usize::from(area.right().saturating_sub(x)),
                        palette.accent,
                    )
                    .0;
            }
            if !self.said.is_empty() {
                x = buf
                    .set_stringn(
                        x,
                        y,
                        " · ",
                        usize::from(area.right().saturating_sub(x)),
                        palette.muted,
                    )
                    .0;
                buf.set_stringn(
                    x,
                    y,
                    &self.said,
                    usize::from(area.right().saturating_sub(x)),
                    palette.muted,
                );
            }
            y += 2;
        }
        for (at, clip) in self.list().iter().enumerate() {
            if y >= area.bottom() {
                break;
            }
            let style = if at == self.at {
                palette.selected
            } else {
                palette.ink
            };
            buf.set_stringn(
                area.x + 2,
                y,
                clip,
                usize::from(area.width.saturating_sub(2)),
                style,
            );
            y += 1;
        }
    }

    fn key(&mut self, key: KeyEvent, cx: &mut Cx<'_, ()>) {
        let Some(chord) = Chord::from_crossterm(&key) else {
            return;
        };
        if let Some(act) = KEYS.action(self.now(), chord).copied() {
            self.act(act, matches!(chord.key, Key::Up | Key::Left));
            cx.keys_now(self.now());
        }
    }
}

fn tui() -> Tui<()> {
    Tui::new(
        "keys demo",
        env!("CARGO_PKG_VERSION"),
        Color::Rgb(0xe8, 0xa0, 0x20),
    )
    .words(Words::new().again("ctrl+c again quits"))
    .nav_keys(NavKeys::NONE)
    .screen("Desk", Demo::default())
    .keymap(&KEYS)
}

fn main() -> ExitCode {
    let args: Vec<String> = env::args().skip(1).collect();
    if args.first().map(String::as_str) == Some("--dump") {
        let Some((width, height)) = args.get(1).and_then(|size| dump::size(size)) else {
            eprintln!("{NAME}: --dump takes a size such as 90x24, then keys");
            return ExitCode::from(2);
        };
        let keys = match dump::keys(&args[2..].join(" ")) {
            Ok(keys) => keys,
            Err(word) => {
                eprintln!("{NAME}: can't read the key {word}");
                return ExitCode::from(2);
            }
        };
        let buffer = tui().still_sheet(true).shot(width, height, &keys, true);
        println!("{}", dump::text(&buffer));
        return ExitCode::SUCCESS;
    }
    match tui().run() {
        Ok(()) => ExitCode::SUCCESS,
        Err(error) => {
            eprintln!("{NAME}: {error}");
            ExitCode::FAILURE
        }
    }
}
