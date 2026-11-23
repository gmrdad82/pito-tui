use pito_tui::crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use pito_tui::header::Section;
use pito_tui::ratatui::{Frame, layout::Rect, style::Color};
use pito_tui::{Head, Palette, Row, Screen, Tui, dump};

struct Blank;

impl Screen<()> for Blank {
    fn draw(&mut self, _frame: &mut Frame, _area: Rect, _palette: &Palette) {}
}

fn tab() -> KeyEvent {
    KeyEvent::new(KeyCode::Tab, KeyModifiers::NONE)
}

#[derive(Clone, Copy, PartialEq, Eq)]
enum Look {
    Header,
    Stack,
}

#[derive(Clone, Copy)]
enum Flag {
    Unset,
    Off,
    On,
}

fn app(look: Look, option: Flag, groups: &[(&'static str, &[&'static str])]) -> Tui<()> {
    let tui = Tui::<()>::new("probe", "1.0.0", Color::Cyan);
    let tui = match look {
        Look::Header => tui,
        Look::Stack => tui.head(Head::new([Row::Title, Row::Groups, Row::Sections])),
    };
    let tui = match option {
        Flag::Unset => tui,
        Flag::Off => tui.groups_always(false),
        Flag::On => tui.groups_always(true),
    };
    let mut tui = groups.iter().fold(tui, |tui, (name, sections)| {
        sections.iter().fold(tui.group(*name), |tui, section| {
            tui.screen(Section::new(*section), Blank)
        })
    });
    tui.start();
    tui
}

const ONE: &[(&str, &[&str])] = &[("Backup", &["Plan", "Run", "Restore"])];
const TWO: &[(&str, &[&str])] = &[
    ("Backup", &["Plan", "Run"]),
    ("Archive", &["Browse", "Prune"]),
];

fn lines(tui: &mut Tui<()>, keys: &[KeyEvent]) -> Vec<String> {
    dump::text(&tui.shot(60, 14, keys, true))
        .lines()
        .map(str::to_string)
        .collect()
}

fn row_of(lines: &[String], word: &str) -> Option<usize> {
    lines.iter().position(|line| line.contains(word))
}

#[test]
fn a_lone_group_shows_its_name_row_above_the_tabs_when_asked() {
    for look in [Look::Header, Look::Stack] {
        let mut tui = app(look, Flag::On, ONE);
        let drawn = lines(&mut tui, &[]);
        let group = row_of(&drawn, "Backup").expect("the group's name is drawn");
        let tabs = row_of(&drawn, "Plan").expect("the tabs are drawn");
        assert_eq!(tabs, group + 1, "{drawn:#?}");
        assert!(drawn[tabs].contains("Run") && drawn[tabs].contains("Restore"));
        assert!(!drawn[group].contains("Plan"), "{drawn:#?}");
        assert_eq!(
            drawn.iter().filter(|line| line.contains("Backup")).count(),
            1,
            "{drawn:#?}"
        );
    }
}

#[test]
fn a_lone_group_draws_no_groups_row_by_default_and_when_turned_off() {
    for look in [Look::Header, Look::Stack] {
        let mut unset = app(look, Flag::Unset, ONE);
        let mut off = app(look, Flag::Off, ONE);
        let drawn = lines(&mut unset, &[]);
        assert_eq!(row_of(&drawn, "Backup"), None, "{drawn:#?}");
        assert!(row_of(&drawn, "Plan").is_some());
        assert_eq!(lines(&mut off, &[]), drawn);
        let mut on = app(look, Flag::On, ONE);
        assert_ne!(lines(&mut on, &[]), drawn);
    }
}

#[test]
fn the_option_adds_one_row_to_the_header_and_moves_nothing_else() {
    for look in [Look::Header, Look::Stack] {
        let plain = lines(&mut app(look, Flag::Unset, ONE), &[]);
        let shown = lines(&mut app(look, Flag::On, ONE), &[]);
        let group = row_of(&shown, "Backup").unwrap();
        assert_eq!(shown.len(), plain.len());
        assert_eq!(shown[..group], plain[..group], "above the group's row");
        assert_eq!(shown[group + 1], plain[group], "the tabs, one row lower");
        assert_eq!(shown.last(), plain.last(), "the footer");
    }
}

#[test]
fn two_groups_look_the_same_with_the_option_unset_off_or_on() {
    for look in [Look::Header, Look::Stack] {
        let plain = lines(&mut app(look, Flag::Unset, TWO), &[]);
        assert!(row_of(&plain, "Backup").is_some() && row_of(&plain, "Archive").is_some());
        assert_eq!(lines(&mut app(look, Flag::Off, TWO), &[]), plain);
        assert_eq!(lines(&mut app(look, Flag::On, TWO), &[]), plain);
        let stepped = [tab()];
        let after = lines(&mut app(look, Flag::Unset, TWO), &stepped);
        assert_eq!(lines(&mut app(look, Flag::On, TWO), &stepped), after);
    }
}

#[test]
fn tab_still_steps_sections_with_a_lone_groups_row_shown() {
    for look in [Look::Header, Look::Stack] {
        for option in [Flag::Unset, Flag::On] {
            let mut tui = app(look, option, ONE);
            assert_eq!(tui.current(), Some(0));
            tui.key(tab());
            assert_eq!(tui.current(), Some(1));
            tui.key(tab());
            tui.key(tab());
            assert_eq!(tui.current(), Some(0), "wraps over the group's sections");
        }
    }
}

#[test]
fn an_app_without_screens_draws_no_groups_row_with_the_option_on() {
    for look in [Look::Header, Look::Stack] {
        let mut on = app(look, Flag::On, &[]);
        let mut unset = app(look, Flag::Unset, &[]);
        assert_eq!(lines(&mut on, &[]), lines(&mut unset, &[]));
    }
}
