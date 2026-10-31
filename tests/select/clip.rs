use pito_tui::select::send;
use pito_tui::{COPY_MAX, base64};

use crate::common::{Sent, Sink};

#[test]
fn a_copy_goes_out_as_osc_52_and_to_the_clipboard_command() {
    let sink = Sink::default();
    assert!(send("alpha beta", &mut sink.clone()));
    assert_eq!(
        sink.sent(),
        [
            Sent::Osc("\x1b]52;c;YWxwaGEgYmV0YQ==\x07".to_string()),
            Sent::Command("alpha beta".to_string()),
        ]
    );
}

#[test]
fn several_rows_and_wide_glyphs_reach_both_paths_byte_for_byte() {
    let sink = Sink::default();
    let text = "日本 ok\nnext row";
    assert!(send(text, &mut sink.clone()));
    let osc = format!("\x1b]52;c;{}\x07", base64(text.as_bytes()));
    assert_eq!(
        sink.sent(),
        [Sent::Osc(osc), Sent::Command(text.to_string())]
    );
}

#[test]
fn text_up_to_copy_max_takes_both_paths() {
    let sink = Sink::default();
    let text = "a".repeat(COPY_MAX);
    assert!(send(&text, &mut sink.clone()));
    let sent = sink.sent();
    assert_eq!(sent.len(), 2);
    assert!(matches!(&sent[0], Sent::Osc(osc) if osc.starts_with("\x1b]52;c;YWFh")));
    assert_eq!(sent[1], Sent::Command(text));
}

#[test]
fn text_over_copy_max_skips_osc_52_and_still_reaches_the_command() {
    let sink = Sink::default();
    let text = "a".repeat(COPY_MAX + 1);
    assert!(send(&text, &mut sink.clone()));
    assert_eq!(sink.sent(), [Sent::Command(text)]);
    let wide = "é".repeat(COPY_MAX / 2 + 1);
    let other = Sink::default();
    assert!(send(&wide, &mut other.clone()));
    assert_eq!(other.sent(), [Sent::Command(wide)]);
}

#[test]
fn empty_text_sends_nothing() {
    let sink = Sink::default();
    assert!(!send("", &mut sink.clone()));
    assert!(sink.sent().is_empty());
}
