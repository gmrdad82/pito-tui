# Changelog

All notable changes to pito-tui are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/); from 1.0.0 onward the
project follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- **Repo scaffold** — Go module (`github.com/gmrdad82/pito-tui`, Go 1.26 via
  mise), AGPL-3.0 license, and the Deadpan Butler Slack notifier ported
  verbatim from [pito](https://github.com/gmrdad82/pito). The plan doc's JSON
  contract is corrected against the real pito codebase: TOTP-only `/session`,
  the actual `POST /chat` parameters and its three replies, `GET
  /resume.json` for the conversation picker, web-only verb notices, the
  `TuiChannel` cable channel, and the full event-kind list.
