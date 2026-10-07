use pito_footer::Key;
use pito_header::{Nav, NavKeys};

use crate::label::Label;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum Item {
    Group(usize),
    Tab(usize),
}

pub(crate) struct Heading {
    pub(crate) label: Label,
    pub(crate) keys: &'static [Key],
    shown: bool,
}

pub(crate) struct Tab {
    pub(crate) group: usize,
    pub(crate) label: Label,
    pub(crate) screen: usize,
    shown: bool,
}

pub(crate) struct Model {
    pub(crate) headings: Vec<Heading>,
    pub(crate) tabs: Vec<Tab>,
    pub(crate) nav: Nav,
    pub(crate) keys: NavKeys,
    pub(crate) visible: Vec<usize>,
    pub(crate) groups: Vec<usize>,
    last: Vec<Option<usize>>,
}

impl Model {
    pub(crate) fn new() -> Self {
        Model {
            headings: Vec::new(),
            tabs: Vec::new(),
            nav: Nav::new(Vec::new()),
            keys: NavKeys::HEY,
            visible: Vec::new(),
            groups: Vec::new(),
            last: Vec::new(),
        }
    }

    pub(crate) fn add_group(&mut self, label: Label) {
        self.headings.push(Heading {
            label,
            keys: &[],
            shown: true,
        });
        self.last.push(None);
        self.rebuild();
    }

    pub(crate) fn add_tab(&mut self, label: Label, screen: usize) {
        if self.headings.is_empty() {
            self.headings.push(Heading {
                label: Label::new(""),
                keys: &[],
                shown: true,
            });
            self.last.push(None);
        }
        self.tabs.push(Tab {
            group: self.headings.len() - 1,
            label,
            screen,
            shown: true,
        });
        self.rebuild();
    }

    pub(crate) fn group_keys(&mut self, keys: &'static [Key]) {
        if let Some(heading) = self.headings.last_mut() {
            heading.keys = keys;
        }
    }

    pub(crate) fn set_keys(&mut self, keys: NavKeys) {
        self.keys = keys;
        self.rebuild();
    }

    pub(crate) fn show(&mut self, shown: &dyn Fn(Item) -> bool) -> bool {
        let before = self.current_screen();
        for (index, heading) in self.headings.iter_mut().enumerate() {
            heading.shown = shown(Item::Group(index));
        }
        for tab in &mut self.tabs {
            tab.shown = shown(Item::Tab(tab.screen));
        }
        self.rebuild();
        self.current_screen() != before
    }

    fn harvest(&mut self) {
        for (group, heading) in self.groups.iter().enumerate() {
            if let Some(section) = self.nav.selected(group)
                && let Some(tab) = self.in_group(group).get(section)
            {
                self.last[*heading] = Some(*tab);
            }
        }
    }

    fn in_group(&self, group: usize) -> Vec<usize> {
        let Some(heading) = self.groups.get(group) else {
            return Vec::new();
        };
        self.visible
            .iter()
            .copied()
            .filter(|tab| self.tabs[*tab].group == *heading)
            .collect()
    }

    fn rebuild(&mut self) {
        self.harvest();
        let current = self.current_tab();
        self.visible.clear();
        self.groups.clear();
        let mut groups = Vec::new();
        for (index, heading) in self.headings.iter().enumerate() {
            if !heading.shown {
                continue;
            }
            let tabs: Vec<usize> = (0..self.tabs.len())
                .filter(|tab| self.tabs[*tab].group == index && self.tabs[*tab].shown)
                .collect();
            if tabs.is_empty() && !self.tabs.iter().all(|tab| tab.group != index) {
                continue;
            }
            let mut group = heading.label.group();
            for tab in &tabs {
                group = group.section(self.tabs[*tab].label.section());
            }
            self.visible.extend(tabs);
            self.groups.push(index);
            groups.push(group);
        }
        self.nav = Nav::new(groups).keys(self.keys);
        for group in 0..self.groups.len() {
            let last = self.last[self.groups[group]];
            if let Some(at) =
                last.and_then(|tab| self.in_group(group).iter().position(|t| *t == tab))
            {
                self.nav.go_to(group, at);
            }
        }
        let target = current.and_then(|tab| self.place_of(tab)).or_else(|| {
            let heading = self.tabs.get(current?)?.group;
            let group = self.groups.iter().position(|kept| *kept == heading)?;
            Some((group, self.nav.selected(group).unwrap_or(0)))
        });
        let target = target.or_else(|| {
            let group = (0..self.groups.len()).find(|group| !self.in_group(*group).is_empty())?;
            Some((group, self.nav.selected(group).unwrap_or(0)))
        });
        if let Some((group, section)) = target {
            self.nav.go_to(group, section);
        }
    }

    fn place_of(&self, tab: usize) -> Option<(usize, usize)> {
        let heading = self.tabs.get(tab)?.group;
        let group = self.groups.iter().position(|kept| *kept == heading)?;
        let section = self.in_group(group).iter().position(|t| *t == tab)?;
        Some((group, section))
    }

    pub(crate) fn current_tab(&self) -> Option<usize> {
        let number = self.nav.place().number.checked_sub(1)?;
        self.visible.get(number).copied()
    }

    pub(crate) fn current_screen(&self) -> Option<usize> {
        self.current_tab().map(|tab| self.tabs[tab].screen)
    }

    pub(crate) fn go_screen(&mut self, screen: usize) -> bool {
        let Some(tab) = self.tabs.iter().position(|tab| tab.screen == screen) else {
            return false;
        };
        let Some((group, section)) = self.place_of(tab) else {
            return false;
        };
        self.nav.go_to(group, section).is_some()
    }

    pub(crate) fn go_heading(&mut self, heading: usize) -> bool {
        let Some(group) = self.groups.iter().position(|kept| *kept == heading) else {
            return false;
        };
        let section = self.nav.selected(group).unwrap_or(0);
        self.nav.go_to(group, section).is_some()
    }

    pub(crate) fn heading_for(&self, key: Key) -> Option<usize> {
        self.headings
            .iter()
            .position(|heading| heading.keys.contains(&key))
    }

    pub(crate) fn group_tabs(&self) -> Vec<usize> {
        self.in_group(self.nav.place().group)
    }

    pub(crate) fn lone(&self, group: usize) -> bool {
        self.in_group(group).len() == 1
    }
}
