#![doc = include_str!("README.md")]

mod confirm;
mod cut;
mod input;
mod key;
mod quit;
mod styles;
mod text;
mod widget;

pub use confirm::{Answer, Confirm, ConfirmBar, ConfirmKeys, Words};
pub use input::{Edit, Input, InputBar};
pub use key::Key;
pub use quit::{Guard, Mode, QuitGuard, WINDOW, Wording};
pub use styles::{Styles, Tone};
pub use widget::{Footer, Help, Hint, Notice, Part, Segment};
