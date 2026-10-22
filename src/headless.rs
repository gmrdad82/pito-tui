use std::time::{Duration, Instant};

use crossterm::event::KeyEvent;
use ratatui::{Terminal, backend::TestBackend, buffer::Buffer};

use crate::pace::FRAME;
use crate::shell::{Flow, Tui};

#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub struct Bench {
    pub screen: String,
    pub frames: usize,
    pub average: Duration,
    pub worst: Duration,
}

fn terminal(width: u16, height: u16) -> Terminal<TestBackend> {
    match Terminal::new(TestBackend::new(width, height)) {
        Ok(terminal) => terminal,
        Err(never) => match never {},
    }
}

impl<E: Send + 'static> Tui<E> {
    pub fn frame(&mut self, width: u16, height: u16) -> Buffer {
        let mut terminal = terminal(width, height);
        if let Err(never) = terminal.draw(|frame| self.draw(frame)) {
            match never {}
        }
        terminal.backend().buffer().clone()
    }

    pub fn shot(&mut self, width: u16, height: u16, keys: &[KeyEvent], settle: bool) -> Buffer {
        self.start();
        self.ready(settle);
        for key in keys {
            self.frame(width, height);
            if self.key(*key) == Flow::Quit {
                break;
            }
            self.ready(settle);
        }
        self.frame(width, height)
    }

    fn ready(&mut self, settle: bool) {
        if settle {
            self.settle();
        } else {
            drop(self.reads());
        }
    }

    pub fn bench(&mut self, width: u16, height: u16, frames: usize) -> Vec<Bench> {
        let names: Vec<String> = self.names().into_iter().map(str::to_string).collect();
        let mut terminal = terminal(width, height);
        let start = self.moment;
        let mut out = Vec::with_capacity(names.len());
        for (index, screen) in names.into_iter().enumerate() {
            if !self.go(index) {
                continue;
            }
            self.settle();
            let mut total = Duration::ZERO;
            let mut worst = Duration::ZERO;
            for frame in 0..frames {
                self.moment = start + FRAME * u32::try_from(frame).unwrap_or(u32::MAX);
                let began = Instant::now();
                if let Err(never) = terminal.draw(|frame| self.draw(frame)) {
                    match never {}
                }
                let spent = began.elapsed();
                total += spent;
                worst = worst.max(spent);
            }
            self.moment = start;
            out.push(Bench {
                screen,
                frames,
                average: total / u32::try_from(frames.max(1)).unwrap_or(u32::MAX),
                worst,
            });
        }
        self.open = None;
        out
    }
}

#[cfg(test)]
mod tests {
    use crossterm::event::{KeyCode, KeyModifiers};
    use pito_footer::Key;
    use pito_header::Section;
    use ratatui::{
        Frame,
        layout::Rect,
        style::{Color, Style},
        widgets::Paragraph,
    };

    use super::*;
    use crate::{Cx, Job, Palette, Phase, Screen, Words, dump};

    #[derive(Default)]
    struct Feed {
        typed: String,
        loads: u32,
        value: Option<u32>,
    }

    impl Screen<u32> for Feed {
        fn draw(&mut self, frame: &mut Frame, area: Rect, _palette: &Palette) {
            let text = match self.value {
                Some(value) => format!("feed {value} typed {}", self.typed),
                None => format!("empty typed {}", self.typed),
            };
            frame.render_widget(Paragraph::new(text).style(Style::new().fg(Color::Green)), area);
        }

        fn key(&mut self, key: KeyEvent, _cx: &mut Cx<'_, u32>) {
            if let KeyCode::Char(c) = key.code {
                self.typed.push(c);
            }
        }

        fn phase(&self) -> Phase<'_> {
            if self.value.is_none() {
                Phase::Loading("Reading the feed...".into())
            } else {
                Phase::Ready
            }
        }

        fn load(&mut self) -> Option<Job<u32>> {
            self.loads += 1;
            let answer = 6 + self.loads;
            Some(Box::new(move || answer))
        }

        fn loaded(&mut self, answer: u32, _cx: &mut Cx<'_, u32>) {
            self.value = Some(answer);
        }
    }

    struct Blank;

    impl Screen<u32> for Blank {
        fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}
    }

    fn app() -> Tui<u32> {
        Tui::new("probe", "1.2.3", Color::Blue)
            .words(Words::new().help("? keys").again("again to leave"))
            .screen(Section::new("One"), Feed::default())
            .screen(Section::new("Two"), Feed::default())
            .overlay(Section::new("Drill"), &[Key::Char('D')], Feed::default())
            .app(Blank)
    }

    fn typed(word: &str) -> Vec<KeyEvent> {
        word.chars()
            .map(|c| KeyEvent::new(KeyCode::Char(c), KeyModifiers::NONE))
            .collect()
    }

    #[test]
    fn a_frame_of_the_same_app_is_the_same_buffer_every_time() {
        let mut first = app();
        let mut second = app();
        first.start();
        second.start();
        let one = first.frame(60, 16);
        assert_eq!(one, second.frame(60, 16));
        assert_eq!(one, first.frame(60, 16));
        assert_eq!(one.area, Rect::new(0, 0, 60, 16));
        assert_ne!(one, first.frame(80, 24));
    }

    #[test]
    fn a_settled_shot_runs_the_load_and_the_keys_and_repeats_exactly() {
        let keys = typed("ab");
        let shot = app().shot(60, 16, &keys, true);
        assert_eq!(shot, app().shot(60, 16, &keys, true));
        let text = dump::text(&shot);
        assert!(text.contains("feed 7 typed ab"), "{text}");
        assert!(!text.contains("Reading the feed..."));
    }

    #[test]
    fn an_unsettled_shot_stays_loading_and_repeats_exactly() {
        let keys = typed("ab");
        let shot = app().shot(60, 16, &keys, false);
        assert_eq!(shot, app().shot(60, 16, &keys, false));
        let text = dump::text(&shot);
        assert!(!text.contains("feed 7"), "{text}");
        assert_ne!(shot, app().shot(60, 16, &keys, true));
    }

    #[test]
    fn a_shot_without_keys_is_the_started_screen() {
        let shot = app().shot(60, 16, &[], true);
        assert!(dump::text(&shot).contains("feed 7 typed"));
        assert_eq!(shot, app().shot(60, 16, &[], true));
        assert_ne!(shot, app().shot(60, 16, &typed("z"), true));
    }

    #[test]
    fn a_quit_key_ends_the_shot_and_the_rest_are_never_pressed() {
        let ctrl_c = KeyEvent::new(KeyCode::Char('c'), KeyModifiers::CONTROL);
        let mut quitting = vec![ctrl_c, ctrl_c];
        quitting.extend(typed("zz"));
        let mut tui = app();
        let shot = tui.shot(60, 16, &quitting, true);
        assert!(!dump::text(&shot).contains("typed zz"));
        assert!(tui.find::<Feed>().unwrap().typed.is_empty());
    }

    #[test]
    fn bench_gives_one_entry_per_walkable_screen_each_counting_every_frame() {
        let mut tui = app();
        tui.start();
        let bench = tui.bench(60, 16, 5);
        let screens: Vec<&str> = bench.iter().map(|entry| entry.screen.as_str()).collect();
        assert_eq!(screens, ["One", "Two", "Drill"]);
        for entry in &bench {
            assert_eq!(entry.frames, 5);
            assert!(entry.average <= entry.worst);
        }
    }

    #[test]
    fn bench_of_no_frames_reports_zero_times() {
        let mut tui = app();
        tui.start();
        let bench = tui.bench(60, 16, 0);
        assert_eq!(bench.len(), 3);
        for entry in &bench {
            assert_eq!(entry.frames, 0);
            assert_eq!(entry.average, Duration::ZERO);
            assert_eq!(entry.worst, Duration::ZERO);
        }
    }

    #[test]
    fn bench_leaves_the_same_buffer_behind_for_the_same_app() {
        let mut first = app();
        let mut second = app();
        first.start();
        second.start();
        first.bench(60, 16, 3);
        second.bench(60, 16, 3);
        assert_eq!(first.frame(60, 16), second.frame(60, 16));
        assert_eq!(first.open, None);
        let names = |bench: Vec<Bench>| -> Vec<(String, usize)> {
            bench.into_iter().map(|entry| (entry.screen, entry.frames)).collect()
        };
        assert_eq!(names(first.bench(40, 12, 2)), names(second.bench(40, 12, 2)));
    }
}
