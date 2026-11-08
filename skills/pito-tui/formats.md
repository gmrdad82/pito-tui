# Formats

The progress lines, key scripts, sizes, text dumps and capture files are specified in
`docs/pages/reference/formats.md` in the repository. Key scripts and sizes are also in
[`headless.md`](headless.md).

Progress lines (`Cx::run`, version 1, one JSON object a line): `v` must be `1`; `state` is
required (`start`, `progress`, `done`, `fail`, `end`); `stage`, `fraction` (0 to 1), `msg` and
`ok` (on `end`) are read, other fields ignored. A line that is not JSON or has no `state`
goes into the activity's detail and reaches `Screen::heard` as text. `command::read(line)`
parses one line. The activity ends on the `end` line or the command's exit status.

Captures: one folder per scenario and one file per size (`tmp/before/00-home/80x24`); a screen's
folder is its two-digit index then its name, a script's folder its name.
