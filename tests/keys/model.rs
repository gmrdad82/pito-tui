use pito_tui::keys::{Chord, Key, Keymap, Mode, Mods, Now, Outcome, Sheet, When, bind};

const ALPHA: Mode = Mode("alpha");

fn chords() -> Vec<Chord> {
    let keys = [
        Key::Char('?'),
        Key::Char('/'),
        Key::Char('a'),
        Key::Char('Z'),
        Key::Space,
        Key::Esc,
        Key::Enter,
        Key::Backspace,
        Key::PageUp,
        Key::PageDown,
        Key::Up,
        Key::Down,
        Key::Tab,
    ];
    let mods = [Mods::NONE, Mods::SHIFT, Mods::CTRL, Mods::ALT];
    keys.iter()
        .flat_map(|key| mods.iter().map(|mods| Chord::new(*key, *mods)))
        .collect()
}

fn states() -> Vec<Sheet> {
    let closed = Sheet::new();
    let mut open = Sheet::new();
    open.open();
    let mut searched = Sheet::new();
    searched.search("undo");
    let mut typing = Sheet::new();
    typing.open();
    typing.key(Chord::char('/'));
    let mut typed = typing.clone();
    typed.key(Chord::char('u'));
    vec![closed, open, searched, typing, typed]
}

#[test]
fn takes_answers_exactly_the_keys_the_sheet_takes() {
    for sheet in states() {
        for chord in chords() {
            let took = sheet.clone().key(chord) == Outcome::Took;
            assert_eq!(sheet.takes(chord), took, "{chord} in {sheet:?}");
        }
    }
}

#[test]
fn page_down_stops_at_the_last_page_the_drawing_fitted() {
    let mut sheet = Sheet::new();
    sheet.open();
    sheet.key(Chord::plain(Key::PageDown));
    assert_eq!(sheet.page(), 0);
    sheet.fit(3);
    for _ in 0..5 {
        sheet.key(Chord::plain(Key::PageDown));
    }
    assert_eq!(sheet.page(), 2);
    sheet.key(Chord::plain(Key::PageUp));
    assert_eq!(sheet.page(), 1);
}

#[test]
fn an_apps_own_empty_context_is_an_ordinary_context() {
    let keymap: Keymap<()> = Keymap::new()
        .modes(&[ALPHA])
        .group("Grp", [bind("x", "zaps", ()).on(ALPHA).when(When(""))]);
    let mut sheet = Sheet::new();
    sheet.open();
    let view = sheet.view(&keymap, Now::new(ALPHA));
    let row = view.here[0].rows[0];
    assert_eq!((row.does, row.when, row.live), ("zaps", Some(""), false));
    let held = sheet.view(&keymap, Now::new(ALPHA).when(&[When("")]));
    assert!(held.here[0].rows[0].live);
}
