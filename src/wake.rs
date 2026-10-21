use std::io;
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::mpsc::Sender;
use std::thread::{self, JoinHandle};
use std::time::Instant;

use crossterm::event::{self, Event};

use crate::activity::Activity;
use crate::command::Report;
use crate::term;
use crate::wheel::WheelFilter;

#[derive(Debug)]
#[non_exhaustive]
pub enum Wake<E> {
    Input(Event),
    Lost(io::Error),
    Event(usize, E),
    Loaded(usize, u64, E),
    Activity(Activity),
    Command(usize, Report),
    Stamped(u64, usize, E),
    Signal(i32),
}

pub struct Waker<E> {
    sender: Sender<Wake<E>>,
    screen: usize,
    epoch: Option<Arc<AtomicU64>>,
}

impl<E> Clone for Waker<E> {
    fn clone(&self) -> Self {
        Waker {
            sender: self.sender.clone(),
            screen: self.screen,
            epoch: self.epoch.clone(),
        }
    }
}

impl<E> std::fmt::Debug for Waker<E> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("Waker")
            .field("screen", &self.screen)
            .finish_non_exhaustive()
    }
}

impl<E: Send + 'static> Waker<E> {
    pub fn new(sender: Sender<Wake<E>>, screen: usize) -> Self {
        Waker {
            sender,
            screen,
            epoch: None,
        }
    }

    pub(crate) fn epoch(mut self, epoch: Arc<AtomicU64>) -> Self {
        self.epoch = Some(epoch);
        self
    }

    pub fn screen(&self) -> usize {
        self.screen
    }

    pub fn send(&self, event: E) -> bool {
        let wake = match &self.epoch {
            Some(epoch) => Wake::Stamped(epoch.load(Ordering::SeqCst), self.screen, event),
            None => Wake::Event(self.screen, event),
        };
        self.sender.send(wake).is_ok()
    }
}

pub fn listen<E: Send + 'static>(sender: Sender<Wake<E>>) -> JoinHandle<()> {
    thread::spawn(move || {
        let mut filter = WheelFilter::default();
        let mut out = Vec::new();
        loop {
            let read = match filter.deadline() {
                Some(due) => match event::poll(due.saturating_duration_since(Instant::now())) {
                    Ok(true) => event::read().map(Some),
                    Ok(false) => Ok(None),
                    Err(error) => Err(error),
                },
                None => event::read().map(Some),
            };
            let now = Instant::now();
            filter.expire(now, &mut out);
            let lost = match read {
                Ok(Some(input)) if term::keys() => {
                    filter.feed(input, now, &mut out);
                    None
                }
                Ok(Some(input)) => {
                    filter.pass(input, &mut out);
                    None
                }
                Ok(None) => None,
                Err(error) => Some(error),
            };
            for input in out.drain(..) {
                if sender.send(Wake::Input(input)).is_err() {
                    return;
                }
            }
            if let Some(error) = lost {
                let _ = sender.send(Wake::Lost(error));
                return;
            }
        }
    })
}
