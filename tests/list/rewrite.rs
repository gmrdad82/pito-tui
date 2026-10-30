use crate::common::*;
use pito_tui::list::{Cell, Column, List, Mark, Part, Row};

const TWO: [Column<'static>; 2] = [Column::new("", 3, 3), Column::new("", 1, 0)];

fn show(list: &mut List) -> Vec<String> {
    draw_with(list, &TWO, 16, 4, |view| view.styles(STYLES)).text
}

fn marked() -> List {
    List::new().with_rows([
        Row::new(["a", "1"]).mark(Mark::new("⧗")),
        Row::new(["b", "2"]).mark(Mark::new("⧗")),
        Row::new(["c", "3"]).mark(Mark::new("⧗")),
    ])
}

#[test]
fn a_row_rewritten_in_place_draws_its_new_text() {
    let mut list = marked();
    let row = list.row_mut(1).unwrap();
    row.cells_mut()[1].set_text("42");
    row.leading_mut().unwrap().set_text("✓");
    assert_eq!(
        show(&mut list),
        ["▌ ⧗ a    1", "  ✓ b    42", "  ⧗ c    3", ""]
    );
    assert!(list.row_mut(3).is_none());
    assert!(Row::blank().leading_mut().is_none());
    assert!(Row::blank().cells_mut().is_empty());
}

#[test]
fn set_text_reuses_the_strings_buffer() {
    let mut cell = Cell::new(String::with_capacity(32));
    cell.set_text("short");
    let at = cell.text().as_ptr();
    cell.set_text("a longer text that fits");
    assert_eq!(cell.text().as_ptr(), at);
    assert_eq!(cell.text(), "a longer text that fits");
    let mut mark = Mark::new(String::with_capacity(8));
    mark.set_text("⧗");
    let at = mark.text().as_ptr();
    mark.set_text("✓✓");
    assert_eq!(mark.text().as_ptr(), at);
    assert_eq!(mark.text(), "✓✓");
}

#[test]
fn the_mark_column_follows_rewritten_marks() {
    let mut list = marked();
    list.row_mut(0)
        .unwrap()
        .leading_mut()
        .unwrap()
        .set_text("10");
    assert_eq!(
        show(&mut list),
        ["▌ 10 a    1", "  ⧗  b    2", "  ⧗  c    3", ""]
    );
    list.row_mut(1)
        .unwrap()
        .leading_mut()
        .unwrap()
        .set_text("11");
    list.row_mut(0)
        .unwrap()
        .leading_mut()
        .unwrap()
        .set_text("9");
    assert_eq!(
        show(&mut list),
        ["▌ 9  a    1", "  11 b    2", "  ⧗  c    3", ""]
    );
    list.row_mut(1)
        .unwrap()
        .leading_mut()
        .unwrap()
        .set_text("8");
    assert_eq!(
        show(&mut list),
        ["▌ 9 a    1", "  8 b    2", "  ⧗ c    3", ""]
    );
    list.row_mut(2).unwrap().leading_mut().unwrap().set_text("");
    list.row_mut(1).unwrap().leading_mut().unwrap().set_text("");
    list.row_mut(0).unwrap().leading_mut().unwrap().set_text("");
    assert_eq!(show(&mut list), ["▌ a    1", "  b    2", "  c    3", ""]);
    list.row_mut(2)
        .unwrap()
        .leading_mut()
        .unwrap()
        .set_text("✗");
    list.push(Row::new(["d", "4"]).mark(Mark::new("...")));
    assert_eq!(
        show(&mut list),
        [
            "▌     a    1",
            "      b    2",
            "  ✗   c    3",
            "  ... d    4"
        ]
    );
    list.row_mut(3)
        .unwrap()
        .leading_mut()
        .unwrap()
        .set_text("•");
    list.set_rows([Row::new(["e", "5"]).mark(Mark::new("✓"))]);
    assert_eq!(show(&mut list), ["▌ ✓ e    5", "", "", ""]);
}

#[test]
fn every_widest_mark_narrowed_in_one_frame_narrows_the_column() {
    let mut list = List::new().with_rows(
        (0..200)
            .map(|n| Row::new([format!("{}", n % 10), String::from("x")]).mark(Mark::new("10"))),
    );
    assert_eq!(show(&mut list)[1], "  10 1    x");
    for index in 0..200 {
        list.row_mut(index)
            .unwrap()
            .leading_mut()
            .unwrap()
            .set_text("9");
    }
    assert_eq!(show(&mut list)[1], "  9 1    x");
    list.row_mut(150)
        .unwrap()
        .leading_mut()
        .unwrap()
        .set_text("100");
    for index in 0..150 {
        list.row_mut(index)
            .unwrap()
            .leading_mut()
            .unwrap()
            .set_text("");
    }
    list.select(150);
    let drawn = show(&mut list);
    assert_eq!(drawn[3], "▌ 100 0    x");
    list.row_mut(150)
        .unwrap()
        .leading_mut()
        .unwrap()
        .set_text("");
    assert_eq!(show(&mut list)[3], "▌   0    x");
}

#[test]
fn set_part_rewrites_one_part_and_keeps_the_others() {
    let mut cell = Cell::parts([
        Part::new("Done").style(GREEN),
        Part::new(" · 3").faint(),
        Part::new("!"),
    ]);
    assert!(cell.set_part(1, " · 12"));
    assert_eq!(cell.text(), "Done · 12!");
    assert!(cell.set_part(0, "Ok"));
    assert!(cell.set_part(2, ""));
    assert!(!cell.set_part(3, "x"));
    assert_eq!(
        cell,
        Cell::parts([
            Part::new("Ok").style(GREEN),
            Part::new(" · 12").faint(),
            Part::new(""),
        ])
    );
    let mut plain = Cell::new("one").faint();
    assert!(plain.set_part(0, "two"));
    assert!(!plain.set_part(1, "three"));
    assert_eq!(plain, Cell::new("two").faint());
}

#[test]
fn set_text_makes_a_cell_of_parts_one_part_in_the_cells_style() {
    let mut cell = Cell::parts([Part::new("a").style(GREEN), Part::new("b")]).faint();
    cell.set_text("xy");
    assert_eq!(cell, Cell::new("xy").faint());
    let mut rest = Cell::new("note").rest();
    rest.set_text("more");
    assert_eq!(rest, Cell::new("more").rest());
}

#[test]
fn a_heading_rewritten_in_place_keeps_its_parts_styles() {
    let mut list = List::new().with_rows([
        Row::heading([Part::new("Running").style(GREEN), Part::new(" · 2").faint()]),
        Row::new(["a", "1"]),
        Row::new(["b", "2"]),
    ]);
    assert!(list.row_mut(0).unwrap().cells_mut()[0].set_part(1, " · 10"));
    let drawn = draw_with(&mut list, &TWO, 16, 3, |view| view.styles(STYLES));
    assert_eq!(drawn.text[0], "  Running · 10");
    assert_eq!(drawn.marks[0], "..gggggggfffff");
    assert_eq!(list.selected(), Some(1));
}

#[test]
fn rewriting_keeps_the_selection_and_the_range() {
    let mut list = marked();
    list.select(2);
    list.select_range(Some((1, 2)));
    list.row_mut(2).unwrap().cells_mut()[0].set_text("z");
    assert_eq!(list.selected(), Some(2));
    assert_eq!(list.range(), Some((1, 2)));
    assert_eq!(list.selected_row().unwrap().cells()[0].text(), "z");
    let drawn = draw_with(&mut list, &TWO, 16, 4, |view| view.styles(STYLES));
    assert_eq!(drawn.text[2], "▌ ⧗ z    3");
    assert_eq!(drawn.marks[1], "S".repeat(16));
}
