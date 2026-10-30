# Notices

The look this shell draws with is in the style of HEY's terminal UI by 37signals
([basecamp/hey-cli](https://github.com/basecamp/hey-cli)): the centred rules
and nested numbered tabs of the header, the "key label · key label" hints and
"Yes ▸ No" confirm of the footer, the selected-row marker of the list, and the
hourglass.

The hourglass's 13 frames (`pito_tui::hourglass::FRAMES`: 7 drain, then 6
flip) are copied verbatim from hey-cli's `internal/tui/loading.go`, and its
drain and flip timing follows that file's `hourglassFrameIndex`. They are used
under hey-cli's MIT licence, reproduced here in full:

```text
MIT License

Copyright (c) 2026 37signals, LLC

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

No other code or text of hey-cli is copied into this crate.

The PITO MD wordmark in docs/images/pitomd-wordmark.svg, shown at the README's
Licence section, is copied by bin/marks from the PITO MD brand masters
at aa3aee8b232a1b848995bf3d9a012a01d4aff8ec. The PITO MD name and
its logos are © Cătălin Ilinca, all rights reserved and not covered by the MIT
licence (TRADEMARKS.md).
