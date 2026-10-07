use std::io;
use std::sync::mpsc::Sender;
use std::thread::{self, JoinHandle};

use crossterm::event::{self, Event};

use crate::activity::Activity;
use crate::command::Report;

#[derive(Debug)]
#[non_exhaustive]
pub enum Wake<E> {
    Input(Event),
    Lost(io::Error),
    Event(usize, E),
    Loaded(usize, u64, E),
    Activity(Activity),
    Command(usize, Report),
}

pub struct Waker<E> {
    sender: Sender<Wake<E>>,
    screen: usize,
}

impl<E> Clone for Waker<E> {
    fn clone(&self) -> Self {
        Waker {
            sender: self.sender.clone(),
            screen: self.screen,
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
        Waker { sender, screen }
    }

    pub fn screen(&self) -> usize {
        self.screen
    }

    pub fn send(&self, event: E) -> bool {
        self.sender.send(Wake::Event(self.screen, event)).is_ok()
    }
}

pub fn listen<E: Send + 'static>(sender: Sender<Wake<E>>) -> JoinHandle<()> {
    thread::spawn(move || {
        loop {
            let read = event::read();
            let failed = read.is_err();
            let wake = match read {
                Ok(input) => Wake::Input(input),
                Err(error) => Wake::Lost(error),
            };
            if sender.send(wake).is_err() || failed {
                return;
            }
        }
    })
}
