#![doc = include_str!("README.md")]

mod bar;
mod column;
mod key;
mod row;
mod shared;
mod state;
mod text;
mod view;

pub use bar::{Bar, Count, Place};
pub use column::Column;
pub use key::{Key, Mods, Named};
pub use row::{Cell, Mark, Part, Row};
pub use shared::{Build, Shared};
pub use state::{Keys, List, Paging, Source, Step};
pub use text::cut;
pub use view::{CURSOR, ListView, MAX_COLUMNS, Styles};

#[cfg(doctest)]
#[doc = include_str!("../../docs/pages/list/reference/api.md")]
struct Guide;

#[cfg(doctest)]
#[doc = include_str!("../../docs/pages/install.md")]
struct Install;

#[cfg(doctest)]
#[doc = include_str!("../../docs/pages/list/guides/first-list.md")]
struct FirstList;

#[cfg(doctest)]
#[doc = include_str!("../../docs/pages/list/guides/keys-and-clicks.md")]
struct KeysAndClicks;

#[cfg(doctest)]
#[doc = include_str!("../../docs/pages/list/guides/columns.md")]
struct Columns;

#[cfg(doctest)]
#[doc = include_str!("../../docs/pages/list/guides/sections.md")]
struct Sections;

#[cfg(doctest)]
#[doc = include_str!("../../docs/pages/list/guides/ranges.md")]
struct Ranges;

#[cfg(doctest)]
#[doc = include_str!("../../docs/pages/list/guides/scrolling.md")]
struct Scrolling;

#[cfg(doctest)]
#[doc = include_str!("../../docs/pages/list/guides/styles.md")]
struct Themes;

#[cfg(doctest)]
#[doc = include_str!("../../docs/pages/list/guides/live-rows.md")]
struct LiveRows;

#[cfg(doctest)]
#[doc = include_str!("../../docs/pages/list/guides/big-lists.md")]
struct BigLists;

#[cfg(doctest)]
#[doc = include_str!("../../docs/pages/list/guides/measure.md")]
struct Measure;
