#![doc = include_str!("../README.md")]

pub mod activity;
mod chrome;
mod copy;
pub mod dump;
mod filter;
mod headless;
mod pace;
mod palette;
mod screen;
mod shell;
mod term;
pub mod text;
mod wake;
mod words;

pub use activity::{Activities, Activity};
pub use chrome::{GAP, PAD, SIDE, TOP, message};
pub use copy::{COPY_MAX, base64, copy};
pub use filter::{Filter, Turn, matches};
pub use headless::Bench;
pub use pace::{FRAME, Pace, wait_until};
pub use palette::Palette;
pub use screen::{Cx, Job, Phase, Screen};
pub use shell::{Flow, FooterLook, HeaderLook, Tui};
pub use term::{Modes, Term, restore};
pub use wake::{Wake, Waker, listen};
pub use words::Words;

pub use crossterm;
pub use pito_footer as footer;
pub use pito_header as header;
pub use pito_hourglass as hourglass;
pub use pito_list as list;
pub use ratatui;
