use std::time::Duration;

use crate::footer::Styles as FooterStyles;
use crate::header::Styles as HeaderStyles;
use crate::hourglass::Hourglass;
use crate::list::{Bar, Column, Count, List, ListView, Source, Styles as ListStyles};
use ratatui::style::{Color, Modifier, Style};

const GOOD: Color = Color::Rgb(0x8c, 0xe6, 0xb4);
const BAD: Color = Color::Rgb(0xff, 0x6e, 0xc7);
const WARN: Color = Color::Rgb(0xff, 0xc8, 0x57);

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
#[non_exhaustive]
pub enum Tone {
    #[default]
    Ink,
    Strong,
    Muted,
    Accent,
    Good,
    Warn,
    Bad,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub struct Palette {
    pub base: Style,
    pub ink: Style,
    pub strong: Style,
    pub muted: Style,
    pub accent: Style,
    pub selected: Style,
    pub good: Style,
    pub warn: Style,
    pub bad: Style,
    pub rule: Style,
    pub line: Style,
    pub glass: Style,
    pub shimmer: Style,
    pub title: Style,
    pub inactive: Style,
    pub focus: Style,
    pub small: Style,
    pub bar: Option<Bar<'static>>,
    pub count: Option<Count<'static>>,
    pub mono: bool,
}

const fn bare(style: Style) -> Style {
    let mut style = style;
    style.fg = None;
    style.bg = None;
    style
}

impl Palette {
    pub const fn new(accent: Color) -> Self {
        let lit = Style::new().fg(accent);
        let muted = Style::new().add_modifier(Modifier::DIM);
        Palette {
            base: Style::new(),
            ink: Style::new(),
            strong: Style::new().add_modifier(Modifier::BOLD),
            muted,
            accent: lit,
            selected: lit.add_modifier(Modifier::BOLD),
            good: Style::new().fg(GOOD),
            warn: Style::new().fg(WARN),
            bad: Style::new().fg(BAD),
            rule: lit.add_modifier(Modifier::BOLD),
            line: muted,
            glass: lit,
            shimmer: lit,
            title: lit.add_modifier(Modifier::BOLD),
            inactive: muted,
            focus: lit
                .add_modifier(Modifier::BOLD)
                .add_modifier(Modifier::REVERSED),
            small: Style::new(),
            bar: None,
            count: None,
            mono: false,
        }
    }

    pub const fn mono(self) -> Self {
        Palette {
            base: bare(self.base),
            ink: bare(self.ink),
            strong: bare(self.strong).add_modifier(Modifier::BOLD),
            muted: bare(self.muted).add_modifier(Modifier::DIM),
            accent: bare(self.accent).add_modifier(Modifier::BOLD),
            selected: bare(self.selected).add_modifier(Modifier::REVERSED),
            good: bare(self.good).add_modifier(Modifier::BOLD),
            warn: bare(self.warn)
                .add_modifier(Modifier::BOLD)
                .add_modifier(Modifier::UNDERLINED),
            bad: bare(self.bad).add_modifier(Modifier::REVERSED),
            rule: bare(self.rule).add_modifier(Modifier::DIM),
            line: bare(self.line).add_modifier(Modifier::DIM),
            glass: bare(self.glass).add_modifier(Modifier::BOLD),
            shimmer: bare(self.shimmer).add_modifier(Modifier::BOLD),
            title: bare(self.title).add_modifier(Modifier::BOLD),
            inactive: bare(self.inactive).add_modifier(Modifier::DIM),
            focus: bare(self.focus).add_modifier(Modifier::REVERSED),
            small: bare(self.small),
            bar: self.bar,
            count: self.count,
            mono: true,
        }
    }

    pub const fn mono_style(style: Style) -> Style {
        bare(style)
    }

    pub const fn base(mut self, style: Style) -> Self {
        self.base = style;
        self
    }

    pub const fn ink(mut self, style: Style) -> Self {
        self.ink = style;
        self
    }

    pub const fn strong(mut self, style: Style) -> Self {
        self.strong = style;
        self
    }

    pub const fn muted(mut self, style: Style) -> Self {
        self.muted = style;
        self
    }

    pub const fn accent(mut self, style: Style) -> Self {
        self.accent = style;
        self
    }

    pub const fn selected(mut self, style: Style) -> Self {
        self.selected = style;
        self
    }

    pub const fn good(mut self, style: Style) -> Self {
        self.good = style;
        self
    }

    pub const fn warn(mut self, style: Style) -> Self {
        self.warn = style;
        self
    }

    pub const fn bad(mut self, style: Style) -> Self {
        self.bad = style;
        self
    }

    pub const fn rule(mut self, style: Style) -> Self {
        self.rule = style;
        self
    }

    pub const fn line(mut self, style: Style) -> Self {
        self.line = style;
        self
    }

    pub const fn glass(mut self, style: Style) -> Self {
        self.glass = style;
        self
    }

    pub const fn shimmer(mut self, style: Style) -> Self {
        self.shimmer = style;
        self
    }

    pub const fn title(mut self, style: Style) -> Self {
        self.title = style;
        self
    }

    pub const fn inactive(mut self, style: Style) -> Self {
        self.inactive = style;
        self
    }

    pub const fn focus(mut self, style: Style) -> Self {
        self.focus = style;
        self
    }

    pub const fn small(mut self, style: Style) -> Self {
        self.small = style;
        self
    }

    pub const fn bar(mut self, bar: Option<Bar<'static>>) -> Self {
        self.bar = bar;
        self
    }

    pub const fn count(mut self, count: Option<Count<'static>>) -> Self {
        self.count = count;
        self
    }

    pub const fn tone(&self, tone: Tone) -> Style {
        match tone {
            Tone::Ink => self.ink,
            Tone::Strong => self.strong,
            Tone::Muted => self.muted,
            Tone::Accent => self.accent.add_modifier(Modifier::BOLD),
            Tone::Good => self.good,
            Tone::Warn => self.warn,
            Tone::Bad => self.bad,
        }
    }

    pub const fn header(&self) -> HeaderStyles {
        HeaderStyles::new()
            .accent(self.accent)
            .muted(self.muted)
            .rule(self.rule)
    }

    pub const fn footer(&self) -> FooterStyles {
        FooterStyles::new()
            .accent(self.accent)
            .muted(self.muted)
            .alert(self.bad)
            .ink(self.ink)
            .rule(self.line)
            .good(self.good)
    }

    pub const fn list(&self) -> ListStyles {
        ListStyles::new()
            .selected(self.selected)
            .ink(self.ink)
            .faint(self.muted)
            .base(self.base)
    }

    pub fn view<'a, S: Source>(
        &self,
        list: &'a mut List<S>,
        columns: &'a [Column<'a>],
    ) -> ListView<'a, S> {
        ListView::new(list, columns)
            .styles(self.list())
            .scrollbar(self.bar)
            .count(self.count)
    }

    pub fn hourglass<'a>(
        &self,
        elapsed: Duration,
        label: &'a str,
        hint: Option<&'a str>,
    ) -> Hourglass<'a> {
        Hourglass::new(elapsed)
            .label(label)
            .hint(hint.filter(|hint| !hint.is_empty()))
            .glass(self.glass)
            .accent(self.shimmer)
            .muted(self.muted)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::list::{Count, Row};
    use ratatui::buffer::Buffer;
    use ratatui::layout::Rect;
    use ratatui::widgets::Widget;

    const ACCENT: Color = Color::Rgb(0x40, 0x80, 0xff);

    const TONES: [Tone; 7] = [
        Tone::Ink,
        Tone::Strong,
        Tone::Muted,
        Tone::Accent,
        Tone::Good,
        Tone::Warn,
        Tone::Bad,
    ];

    fn every(palette: &Palette) -> [Style; 17] {
        [
            palette.base,
            palette.ink,
            palette.strong,
            palette.muted,
            palette.accent,
            palette.selected,
            palette.good,
            palette.warn,
            palette.bad,
            palette.rule,
            palette.line,
            palette.glass,
            palette.shimmer,
            palette.title,
            palette.inactive,
            palette.focus,
            palette.small,
        ]
    }

    type Pick = fn(&Palette) -> Style;

    fn has(style: Style, modifier: Modifier) -> bool {
        style.add_modifier.contains(modifier)
    }

    #[test]
    fn a_new_palette_paints_the_accent_and_keeps_plain_text_plain() {
        let palette = Palette::new(ACCENT);
        assert_eq!(palette.base, Style::new());
        assert_eq!(palette.ink, Style::new());
        assert_eq!(palette.small, Style::new());
        assert_eq!(palette.strong, Style::new().add_modifier(Modifier::BOLD));
        assert_eq!(palette.muted, Style::new().add_modifier(Modifier::DIM));
        assert_eq!(palette.line, palette.muted);
        assert_eq!(palette.inactive, palette.muted);
        assert_eq!(palette.accent, Style::new().fg(ACCENT));
        assert_eq!(palette.glass, palette.accent);
        assert_eq!(palette.shimmer, palette.accent);
        let lit_bold = Style::new().fg(ACCENT).add_modifier(Modifier::BOLD);
        assert_eq!(palette.selected, lit_bold);
        assert_eq!(palette.rule, lit_bold);
        assert_eq!(palette.title, lit_bold);
        assert_eq!(palette.focus, lit_bold.add_modifier(Modifier::REVERSED));
        assert_eq!(palette.bar, None);
        assert_eq!(palette.count, None);
        assert!(!palette.mono);
    }

    #[test]
    fn status_colours_stay_fixed_whatever_the_accent() {
        let one = Palette::new(Color::Red);
        let two = Palette::new(Color::Indexed(33));
        assert_eq!(one.good, two.good);
        assert_eq!(one.warn, two.warn);
        assert_eq!(one.bad, two.bad);
        assert_eq!(one.good.fg, Some(GOOD));
        assert_eq!(one.warn.fg, Some(WARN));
        assert_eq!(one.bad.fg, Some(BAD));
        assert_ne!(one.accent, two.accent);
    }

    #[test]
    fn mono_strips_every_colour_and_marks_the_palette() {
        let palette = Palette::new(ACCENT)
            .base(Style::new().bg(Color::Black).fg(Color::White))
            .small(Style::new().bg(Color::Blue))
            .mono();
        assert!(palette.mono);
        for style in every(&palette) {
            assert_eq!(style.fg, None, "{style:?}");
            assert_eq!(style.bg, None, "{style:?}");
        }
    }

    #[test]
    fn mono_tells_states_apart_by_weight_dimming_and_reversal() {
        let palette = Palette::new(ACCENT).mono();
        assert!(has(palette.strong, Modifier::BOLD));
        assert!(has(palette.muted, Modifier::DIM));
        assert!(has(palette.accent, Modifier::BOLD));
        assert!(has(palette.selected, Modifier::REVERSED));
        assert!(has(palette.good, Modifier::BOLD));
        assert!(has(palette.warn, Modifier::BOLD | Modifier::UNDERLINED));
        assert!(has(palette.bad, Modifier::REVERSED));
        assert!(has(palette.rule, Modifier::DIM));
        assert!(has(palette.line, Modifier::DIM));
        assert!(has(palette.inactive, Modifier::DIM));
        assert!(has(palette.focus, Modifier::REVERSED));
        assert!(has(palette.title, Modifier::BOLD));
        assert_ne!(palette.good, palette.bad);
        assert_ne!(palette.warn, palette.good);
        assert_ne!(palette.selected, palette.ink);
        assert_ne!(palette.muted, palette.ink);
    }

    #[test]
    fn mono_keeps_the_apps_own_modifiers_bar_and_count() {
        let count = Count::new("-", "of");
        let palette = Palette::new(ACCENT)
            .title(Style::new().fg(Color::Red).add_modifier(Modifier::ITALIC))
            .ink(Style::new().add_modifier(Modifier::UNDERLINED))
            .bar(Some(Bar::LINE))
            .count(Some(count))
            .mono();
        assert_eq!(
            palette.title,
            Style::new().add_modifier(Modifier::ITALIC | Modifier::BOLD)
        );
        assert_eq!(palette.ink, Style::new().add_modifier(Modifier::UNDERLINED));
        assert_eq!(palette.bar, Some(Bar::LINE));
        assert_eq!(palette.count, Some(count));
    }

    #[test]
    fn mono_twice_is_mono_once() {
        let once = Palette::new(ACCENT).mono();
        assert_eq!(once.mono(), once);
    }

    #[test]
    fn mono_style_drops_colours_and_keeps_modifiers() {
        let style = Style::new()
            .fg(Color::Red)
            .bg(Color::Blue)
            .add_modifier(Modifier::BOLD)
            .remove_modifier(Modifier::ITALIC);
        let bare = Palette::mono_style(style);
        assert_eq!(bare.fg, None);
        assert_eq!(bare.bg, None);
        assert_eq!(bare.add_modifier, Modifier::BOLD);
        assert_eq!(bare.sub_modifier, Modifier::ITALIC);
        assert_eq!(Palette::mono_style(Style::new()), Style::new());
    }

    #[test]
    fn tones_map_to_their_palette_styles_and_accent_is_bold() {
        let palette = Palette::new(ACCENT);
        assert_eq!(Tone::default(), Tone::Ink);
        assert_eq!(palette.tone(Tone::Ink), palette.ink);
        assert_eq!(palette.tone(Tone::Strong), palette.strong);
        assert_eq!(palette.tone(Tone::Muted), palette.muted);
        assert_eq!(
            palette.tone(Tone::Accent),
            Style::new().fg(ACCENT).add_modifier(Modifier::BOLD)
        );
        assert_eq!(palette.tone(Tone::Good), palette.good);
        assert_eq!(palette.tone(Tone::Warn), palette.warn);
        assert_eq!(palette.tone(Tone::Bad), palette.bad);
    }

    #[test]
    fn tones_follow_the_apps_overrides_and_mono() {
        let custom = Style::new().fg(Color::Cyan);
        let palette = Palette::new(ACCENT).good(custom).accent(custom);
        assert_eq!(palette.tone(Tone::Good), custom);
        assert_eq!(
            palette.tone(Tone::Accent),
            custom.add_modifier(Modifier::BOLD)
        );
        let mono = Palette::new(ACCENT).mono();
        for tone in TONES {
            assert_eq!(mono.tone(tone).fg, None, "{tone:?}");
        }
        let colour = Palette::new(ACCENT);
        let shown: Vec<Style> = TONES.iter().map(|&tone| colour.tone(tone)).collect();
        for (index, style) in shown.iter().enumerate() {
            for other in &shown[index + 1..] {
                assert_ne!(style, other);
            }
        }
    }

    #[test]
    fn each_setter_replaces_its_style_alone() {
        let style = Style::new()
            .fg(Color::Magenta)
            .add_modifier(Modifier::ITALIC);
        let base = Palette::new(ACCENT);
        let set: [(Palette, Pick); 17] = [
            (base.base(style), |p| p.base),
            (base.ink(style), |p| p.ink),
            (base.strong(style), |p| p.strong),
            (base.muted(style), |p| p.muted),
            (base.accent(style), |p| p.accent),
            (base.selected(style), |p| p.selected),
            (base.good(style), |p| p.good),
            (base.warn(style), |p| p.warn),
            (base.bad(style), |p| p.bad),
            (base.rule(style), |p| p.rule),
            (base.line(style), |p| p.line),
            (base.glass(style), |p| p.glass),
            (base.shimmer(style), |p| p.shimmer),
            (base.title(style), |p| p.title),
            (base.inactive(style), |p| p.inactive),
            (base.focus(style), |p| p.focus),
            (base.small(style), |p| p.small),
        ];
        for (index, (palette, get)) in set.iter().enumerate() {
            assert_eq!(get(palette), style, "setter {index}");
            let changed = every(palette)
                .iter()
                .zip(every(&base))
                .filter(|(now, was)| **now != *was)
                .count();
            assert_eq!(changed, 1, "setter {index}");
        }
    }

    fn distinct() -> Palette {
        Palette::new(ACCENT)
            .base(Style::new().bg(Color::Indexed(1)))
            .ink(Style::new().fg(Color::Indexed(2)))
            .strong(Style::new().fg(Color::Indexed(3)))
            .muted(Style::new().fg(Color::Indexed(4)))
            .accent(Style::new().fg(Color::Indexed(5)))
            .selected(Style::new().fg(Color::Indexed(6)))
            .good(Style::new().fg(Color::Indexed(7)))
            .warn(Style::new().fg(Color::Indexed(8)))
            .bad(Style::new().fg(Color::Indexed(9)))
            .rule(Style::new().fg(Color::Indexed(10)))
            .line(Style::new().fg(Color::Indexed(11)))
            .glass(Style::new().fg(Color::Indexed(12)))
            .shimmer(Style::new().fg(Color::Indexed(13)))
            .title(Style::new().fg(Color::Indexed(14)))
            .inactive(Style::new().fg(Color::Indexed(15)))
            .focus(Style::new().fg(Color::Indexed(16)))
            .small(Style::new().fg(Color::Indexed(17)))
    }

    #[test]
    fn the_distinct_palette_really_has_no_two_styles_alike() {
        let styles = every(&distinct());
        for (index, style) in styles.iter().enumerate() {
            for other in &styles[index + 1..] {
                assert_ne!(style, other);
            }
        }
    }

    #[test]
    fn the_hourglass_takes_the_palettes_glass_shimmer_and_muted_styles() {
        let palette = distinct();
        let took = Duration::from_millis(1_250);
        let expected = Hourglass::new(took)
            .label("Loading")
            .hint(Some("esc to stop"))
            .glass(Style::new().fg(Color::Indexed(12)))
            .accent(Style::new().fg(Color::Indexed(13)))
            .muted(Style::new().fg(Color::Indexed(4)));
        assert_eq!(
            palette.hourglass(took, "Loading", Some("esc to stop")),
            expected
        );
        let swapped = Hourglass::new(took)
            .label("Loading")
            .hint(Some("esc to stop"))
            .glass(palette.shimmer)
            .accent(palette.glass)
            .muted(palette.muted);
        assert_ne!(
            palette.hourglass(took, "Loading", Some("esc to stop")),
            swapped
        );
    }

    #[test]
    fn the_header_takes_the_palettes_accent_muted_and_rule() {
        let palette = distinct();
        let header = palette.header();
        assert_eq!(header.accent, Style::new().fg(Color::Indexed(5)));
        assert_eq!(header.muted, Style::new().fg(Color::Indexed(4)));
        assert_eq!(header.rule, Style::new().fg(Color::Indexed(10)));
    }

    #[test]
    fn the_footer_alerts_with_bad_rules_with_line_and_writes_with_ink() {
        let palette = distinct();
        let footer = palette.footer();
        assert_eq!(footer.accent, Style::new().fg(Color::Indexed(5)));
        assert_eq!(footer.muted, Style::new().fg(Color::Indexed(4)));
        assert_eq!(footer.alert, Style::new().fg(Color::Indexed(9)));
        assert_eq!(footer.ink, Style::new().fg(Color::Indexed(2)));
        assert_eq!(footer.rule, Style::new().fg(Color::Indexed(11)));
        assert_eq!(footer.good, Some(Style::new().fg(Color::Indexed(7))));
    }

    #[test]
    fn the_list_styles_take_selected_ink_muted_as_faint_and_base() {
        let palette = distinct();
        let expected = ListStyles::new()
            .selected(Style::new().fg(Color::Indexed(6)))
            .ink(Style::new().fg(Color::Indexed(2)))
            .faint(Style::new().fg(Color::Indexed(4)))
            .base(Style::new().bg(Color::Indexed(1)));
        assert_eq!(palette.list(), expected);
        let faint_from_line = ListStyles::new()
            .selected(palette.selected)
            .ink(palette.ink)
            .faint(palette.line)
            .base(palette.base);
        assert_ne!(palette.list(), faint_from_line);
    }

    #[test]
    fn an_empty_hourglass_hint_counts_as_no_hint() {
        let palette = Palette::new(ACCENT);
        let took = Duration::ZERO;
        assert_eq!(
            palette.hourglass(took, "Wait", Some("")),
            palette.hourglass(took, "Wait", None)
        );
        assert_ne!(
            palette.hourglass(took, "Wait", Some(" ")),
            palette.hourglass(took, "Wait", None)
        );
    }

    fn rows(count: usize) -> Vec<Row> {
        (0..count)
            .map(|index| Row::new([format!("row{index}")]))
            .collect()
    }

    fn draw(palette: &Palette, list: &mut List, height: u16) -> Buffer {
        let columns = [Column::new("Name", 4, 8)];
        let area = Rect::new(0, 0, 12, height);
        let mut buffer = Buffer::empty(area);
        palette
            .view(list, &columns)
            .header(false)
            .render(area, &mut buffer);
        buffer
    }

    fn line(buffer: &Buffer, y: u16) -> String {
        (0..buffer.area.width)
            .map(|x| buffer.cell((x, y)).map_or(" ", |cell| cell.symbol()))
            .collect()
    }

    #[test]
    fn the_list_view_paints_the_selected_row_with_the_palette() {
        let palette = Palette::new(ACCENT);
        let mut list = List::new().with_rows(rows(3));
        list.select(1);
        let buffer = draw(&palette, &mut list, 3);
        assert!(line(&buffer, 1).contains("row1"));
        let chosen = buffer.cell((11, 1)).map(|cell| cell.style());
        assert_eq!(chosen.and_then(|style| style.fg), Some(ACCENT));
        assert!(chosen.is_some_and(|style| has(style, Modifier::BOLD)));
        let other = buffer.cell((11, 0)).map(|cell| cell.style());
        assert_ne!(other.and_then(|style| style.fg), Some(ACCENT));
    }

    #[test]
    fn the_list_view_shows_the_palettes_scrollbar_only_when_rows_overflow() {
        let marks = ["│", "┃", "╽", "╿"];
        let edge = |buffer: &Buffer| -> Vec<String> {
            (0..buffer.area.height)
                .map(|y| {
                    buffer
                        .cell((buffer.area.width - 1, y))
                        .map_or(String::new(), |cell| cell.symbol().to_string())
                })
                .collect()
        };
        let barred = Palette::new(ACCENT).bar(Some(Bar::LINE));
        let mut long = List::new().with_rows(rows(10));
        let shown = edge(&draw(&barred, &mut long, 3));
        assert!(
            shown.iter().all(|mark| marks.contains(&mark.as_str())),
            "{shown:?}"
        );
        let mut short = List::new().with_rows(rows(2));
        let shown = edge(&draw(&barred, &mut short, 3));
        assert!(
            shown.iter().all(|mark| !marks.contains(&mark.as_str())),
            "{shown:?}"
        );
        let bare = Palette::new(ACCENT);
        let mut long = List::new().with_rows(rows(10));
        let shown = edge(&draw(&bare, &mut long, 3));
        assert!(
            shown.iter().all(|mark| !marks.contains(&mark.as_str())),
            "{shown:?}"
        );
    }
}
