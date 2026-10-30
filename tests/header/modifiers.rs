use crate::common::nav;
use pito_tui::header::{Breadcrumb, Drill, Fact, Header, Nav, Styles};
use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
    widgets::Widget,
};

const DIM: Style = Style::new().fg(Color::Gray).add_modifier(Modifier::DIM);
const STYLES: Styles = Styles::new()
    .accent(Style::new().fg(Color::Magenta))
    .muted(Style::new().fg(Color::DarkGray))
    .rule(DIM);
const PLAIN: Style = Style::new();
const FACTS: [Fact; 2] = [Fact::new("12 owners", PLAIN), Fact::new("synced", PLAIN)];

fn header<'a>(nav: &'a Nav) -> Header<'a> {
    Header::new(nav)
        .styles(STYLES)
        .title(Some("Desk"))
        .left(Some(Fact::new("? help", PLAIN)))
        .right(Some(Fact::new("trial", PLAIN)))
        .facts(&FACTS)
        .facts_rule(true)
        .crumbs(true)
        .closing(true)
}

fn render(widget: impl Widget, width: u16, height: u16) -> Buffer {
    let mut buf = Buffer::empty(Rect::new(0, 0, width, height));
    widget.render(buf.area, &mut buf);
    buf
}

fn row(buf: &Buffer, y: u16) -> String {
    (0..buf.area.width)
        .map(|x| buf[(x, y)].symbol())
        .collect::<String>()
}

fn only_rules_dim(buf: &Buffer) {
    for y in 0..buf.area.height {
        for x in 0..buf.area.width {
            let cell = &buf[(x, y)];
            let rule = cell.symbol() == "─";
            if cell.symbol() == " " {
                continue;
            }
            assert_eq!(
                cell.modifier.contains(Modifier::DIM),
                rule,
                "({x}, {y}) {:?} in {:?}",
                cell.symbol(),
                row(buf, y)
            );
        }
    }
}

fn styled(buf: &Buffer, y: u16, text: &str, style: Style) {
    let line = row(buf, y);
    let at = line
        .find(text)
        .unwrap_or_else(|| panic!("{text:?} in {line:?}"));
    let start = line[..at].chars().count() as u16;
    for x in start..start + text.chars().count() as u16 {
        let cell = &buf[(x, y)];
        assert_eq!(
            cell.fg,
            style.fg.unwrap_or(Color::Reset),
            "({x}, {y}) in {line:?}"
        );
        assert_eq!(cell.modifier, style.add_modifier, "({x}, {y}) in {line:?}");
    }
}

const LIT: Style = Style::new().fg(Color::Magenta).add_modifier(Modifier::BOLD);

#[test]
fn text_over_a_dim_rule_keeps_its_own_style_at_the_top_level() {
    let nav = nav();
    let header = header(&nav);
    let buf = render(header, 80, header.height());
    assert_eq!(header.height(), 6);
    only_rules_dim(&buf);
    styled(&buf, 0, " Desk ", LIT);
    styled(&buf, 0, "? help", PLAIN);
    styled(&buf, 0, "trial", PLAIN);
    styled(&buf, 3, "12 owners", PLAIN);
    styled(&buf, 3, "synced", PLAIN);
    styled(&buf, 3, " · ", STYLES.muted);
    styled(&buf, 4, "Inbox", LIT);
    assert_eq!(buf[(0, 3)].modifier, Modifier::DIM);
    assert_eq!(buf[(79, 0)].modifier, Modifier::DIM);
}

#[test]
fn text_over_a_dim_rule_keeps_its_own_style_drilled_in() {
    let mut nav = nav();
    nav.go(6);
    nav.open("nightly", 2);
    let header = header(&nav);
    assert_eq!(header.height(), 4);
    let buf = render(header, 80, header.height());
    only_rules_dim(&buf);
    styled(&buf, 0, " Desk ", LIT);
    styled(&buf, 2, "Ownership / nightly", LIT);
    assert_eq!(buf[(0, 2)].modifier, Modifier::DIM);
    assert_eq!(buf[(79, 2)].modifier, Modifier::DIM);

    let rows = header.drill(Drill::Rows);
    let buf = render(rows, 80, rows.height());
    only_rules_dim(&buf);
    styled(&buf, 3, "12 owners", PLAIN);
    styled(&buf, 4, "Ownership / nightly", LIT);
}

#[test]
fn a_clipped_title_and_facts_keep_their_own_style() {
    let mut nav = nav();
    nav.open("a long nightly title", 2);
    for width in 1..40 {
        let header = header(&nav).drill(Drill::Rows);
        let buf = render(header, width, header.height());
        only_rules_dim(&buf);
    }
}

#[test]
fn the_breadcrumb_widget_keeps_its_own_style_either_way() {
    let mut nav = nav();
    nav.open("nightly", 2);
    for centred in [false, true] {
        let crumb = Breadcrumb::new(&nav).styles(STYLES).centred(centred);
        let buf = render(crumb, 40, 1);
        only_rules_dim(&buf);
        styled(&buf, 0, "Inbox / nightly", LIT);
    }
}
