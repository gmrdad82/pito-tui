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
