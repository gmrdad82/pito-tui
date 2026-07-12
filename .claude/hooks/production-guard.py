#!/usr/bin/env python3
"""PreToolUse hook (Bash): PRODUCTION GUARD — owner order 2026-07-12.

Claude must never touch the production instance (app.pitomd.com) from
this repo. Two enforcement layers:

  1. Any Bash command that mentions app.pitomd.com is BLOCKED outright.
  2. Any `vhs <tape>` invocation is inspected: if the tape launches
     pito-tui WITHOUT an explicit `-instance` flag, it would inherit
     ~/.config/pito-tui/config.toml's default server — which may point
     at production — so it is BLOCKED until the tape pins an instance.
     (This is exactly how one capture grazed production on 2026-07-12.)

Read-only analysis of the tool call JSON on stdin; exit 2 blocks.
"""
import json
import os
import re
import sys

PROD_HOST = "app.pitomd.com"


def find_tapes(command):
    """Best-effort: paths passed to a vhs invocation."""
    tapes = []
    for m in re.finditer(r"vhs\s+([^\s;|&]+\.tape)", command):
        tapes.append(m.group(1))
    return tapes


def main():
    try:
        data = json.load(sys.stdin)
    except Exception:
        return 0
    if data.get("tool_name") != "Bash":
        return 0
    command = str((data.get("tool_input") or {}).get("command", ""))

    if PROD_HOST in command:
        print(
            f"PRODUCTION GUARD: this command references {PROD_HOST} — Claude "
            "must not touch the production instance from this repo (owner "
            "order 2026-07-12). Use https://dev.pitomd.com.",
            file=sys.stderr,
        )
        return 2

    for tape in find_tapes(command):
        # Resolve relative to any `cd <dir>` earlier in the same command.
        candidates = [tape]
        for cd in re.finditer(r"cd\s+([^\s;|&]+)", command):
            candidates.append(os.path.join(os.path.expanduser(cd.group(1)), tape))
        path = next((c for c in candidates if os.path.isfile(os.path.expanduser(c))), None)
        if path is None:
            continue  # tape written later in a heredoc — layer 1 already screened the text
        try:
            content = open(os.path.expanduser(path)).read()
        except OSError:
            continue
        if PROD_HOST in content:
            print(
                f"PRODUCTION GUARD: tape {tape} references {PROD_HOST} — blocked.",
                file=sys.stderr,
            )
            return 2
        launches = re.findall(r'Type\s+"[^"]*pito-tui[^"]*"', content)
        for launch in launches:
            if "-instance" not in launch:
                print(
                    f"PRODUCTION GUARD: tape {tape} launches pito-tui without "
                    "an explicit -instance flag — it would inherit the config "
                    "default, which may be production. Pin it: "
                    'Type "... pito-tui -instance https://dev.pitomd.com ...".',
                    file=sys.stderr,
                )
                return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
