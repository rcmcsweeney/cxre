# Changelog

All notable changes to CXRE are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.1] - 2026-07-12

### Fixed

- Report reset-credit information that Codex explicitly marks unavailable with
  a dedicated actionable error instead of a generic protocol failure.
- Report the tagged module version for binaries installed with `go install`.
- Detect duplicate, conflicting, and unidentifiable credit rows without
  overstating result completeness.
- Classify child-process failures only after bounded stderr collection finishes.
- Distinguish user interruption from a network timeout and diagnose invalid
  `CXRE_CODEX` selections without echoing configured paths.
- Keep redirected warning output free of ANSI styling.
- Make the documented terminal countdowns and JSON credit counts agree.

### Changed

- Add a plain-language, OS-specific quick start while retaining the detailed
  output, verification, privacy, and protocol reference.
- Test the minimum Go version and run the full suite natively on Linux, macOS,
  and Windows.
- Build release artifacts once, smoke-test and attest those exact bytes, then
  publish the GitHub release and tested Homebrew formula.
- Include security, contribution, and packaging documentation in release
  archives so README links remain useful offline.

The successful JSON response remains schema version 1. The release adds
sanitized error codes and conventional interruption exit statuses.

## [0.1.0] - 2026-07-12

### Added

- `CXRE — Codex Resets` branding for the read-only reset overview.
- Fast, read-only reset-credit expiration lookup through Codex app-server.
- Five-hour and weekly percentage-left summaries with exact reset times and
  remaining countdowns.
- Exact local or UTC timestamps, relative countdowns, and soonest-first sorting.
- Responsive terminal output with graceful color and Unicode fallbacks.
- Stable JSON schema version 1 and sanitized machine-readable errors.
- Partial-data reporting when Codex knows the credit count but omits detail rows.
- Static builds for macOS, Linux, and Windows, plus checksums, SBOMs, and
  GitHub build provenance.
- Automated Homebrew tap publication and a future Scoop manifest template.

[Unreleased]: https://github.com/rcmcsweeney/cxre/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/rcmcsweeney/cxre/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/rcmcsweeney/cxre/releases/tag/v0.1.0
