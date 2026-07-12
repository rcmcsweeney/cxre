# CXRE

> **Know when your Codex limits reset and reset credits expire.**

[![CI](https://github.com/rcmcsweeney/cxre/actions/workflows/ci.yml/badge.svg)](https://github.com/rcmcsweeney/cxre/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/rcmcsweeney/cxre)](https://github.com/rcmcsweeney/cxre/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

CXRE (Codex Resets) is a small, read-only CLI that shows the exact
expiration time for every available Codex manual reset credit. A compact
summary above the credit table also shows the five-hour and weekly limits,
percentage left, exact reset time, and time remaining. CXRE is a single native
executable and sorts the credits that expire first.

CXRE is an **unofficial community tool**. It is not affiliated with, endorsed
by, or maintained by OpenAI.

![CXRE reset-credit table showing four expirations](docs/cxre-terminal.png)

The screenshot shows the reset-credit table. The complete fictional example
below also shows the usage-limit summary.

## Requirements

- [Codex CLI](https://developers.openai.com/codex/cli) 0.143.0 or newer on
  `PATH`.
- A ChatGPT account already signed in through Codex. Run `codex login` if
  needed.

CXRE deliberately delegates authentication to Codex. API-key-only and Amazon
Bedrock Codex environments are not supported in v0.1.

## Install

### Homebrew

```sh
brew install rcmcsweeney/tap/cxre
```

The fully qualified command is intentional: on a fresh machine it adds the tap
and installs CXRE in one step while trusting only this formula. Each stable
release publishes or updates the formula automatically. If no stable release
is listed yet, install from source until the first formula is published. Codex
itself remains a separate prerequisite.

### GitHub Releases

Download the archive for your platform from
[GitHub Releases](https://github.com/rcmcsweeney/cxre/releases), extract it,
and place `cxre` (or `cxre.exe`) somewhere on your `PATH`.

Release archives are available for:

- macOS: Apple Silicon and Intel
- Linux: AMD64 and ARM64
- Windows: AMD64

Each release includes `checksums.txt`, per-archive SBOMs, and GitHub artifact
provenance. Verify an archive with:

```sh
sha256sum -c checksums.txt --ignore-missing
gh attestation verify cxre_0.1.0_Darwin_arm64.tar.gz -R rcmcsweeney/cxre
```

On macOS, use `shasum -a 256` if `sha256sum` is unavailable. Initial CXRE
releases are not code-signed or notarized, so macOS Gatekeeper or Windows
SmartScreen may ask for confirmation even when the checksum and provenance are
valid.

### From source

With Go 1.25 or newer:

```sh
go install github.com/rcmcsweeney/cxre/cmd/cxre@latest
```

### Scoop

A Scoop manifest is prepared under [`packaging/scoop`](packaging/scoop), but no
public bucket is published yet. See its README if you maintain a Scoop bucket.

## Use

```text
cxre             Show usage limits and reset-credit expiration times.
cxre --json      Emit stable machine-readable JSON.
cxre --utc       Display timestamps in UTC.
cxre --version   Display build version information.
cxre --help      Show help.
cxre -h          Show help.
```

Options may be combined, for example `cxre --json --utc`. CXRE v0.1 accepts no
positional commands. That leaves room for future commands without making the
first release more complicated.

### Terminal output

```text
CXRE — Codex Resets

Usage limits
Window  Left  Resets                            Remaining
---------------------------------------------------------
5h       63%  Sun 12 Jul 2026 5:00:00 PM NZST   3h 45m
Weekly   39%  Sat 18 Jul 2026 12:00:00 PM NZST  5d 22h

Available reset credits: 3

Expires                           Remaining
------------------------------------------------
Sun 12 Jul 2026 8:42:17 PM NZST  4h 12m
Mon 20 Jul 2026 9:00:00 AM NZST  7d 16h
Sun 02 Aug 2026 4:03:51 PM NZST  21d 23h
```

Times use the operating system's local timezone unless `--utc` is set. Limit
percentages show the amount left, matching Codex's presentation. Credits are
sorted by the earliest expiration; credits that do not expire appear last.
Countdowns are floored and use compact units:

- days and hours at one day or more;
- hours and minutes at one hour or more;
- minutes and seconds at one minute or more;
- seconds below one minute;
- `expired` for a timestamp at or before the current time.

On an interactive terminal, CXRE uses restrained color and Unicode status
marks. It automatically disables ANSI styling when output is redirected,
`TERM=dumb` or `NO_COLOR` is set, or the Windows console cannot support it;
Unicode marks appear only on a capable locale and console. Below 60 columns,
the table changes to stacked rows instead of truncating timestamps.

### JSON schema v1

`--json` writes one JSON document to stdout and no decorative text. `--utc`
changes RFC 3339 strings and the reported timezone; Unix values are unchanged.
In local mode the `timezone` field uses the operating system's IANA zone name
when available, with the active timezone abbreviation as a portable fallback.

```json
{
  "schema_version": 1,
  "generated_at": "2026-07-12T13:14:49+12:00",
  "timezone": "Pacific/Auckland",
  "limits": {
    "five_hour": {
      "used_percent": 37,
      "remaining_percent": 63,
      "resets_at": "2026-07-12T17:00:00+12:00",
      "resets_at_unix": 1783832400,
      "remaining_seconds": 13511,
      "reset_due": false
    },
    "weekly": {
      "used_percent": 61,
      "remaining_percent": 39,
      "resets_at": "2026-07-18T12:00:00+12:00",
      "resets_at_unix": 1784332800,
      "remaining_seconds": 513911,
      "reset_due": false
    }
  },
  "available_count": 3,
  "detailed_count": 3,
  "missing_count": 0,
  "complete": true,
  "credits": [
    {
      "expires_at": "2026-07-12T20:42:17+12:00",
      "expires_at_unix": 1783845737,
      "remaining_seconds": 26848,
      "expired": false,
      "does_not_expire": false
    }
  ],
  "warnings": []
}
```

`limits.five_hour` and `limits.weekly` are `null` when Codex does not provide a
recognized window. If a percentage is available without a reset timestamp, the
window remains present while `resets_at`, `resets_at_unix`,
`remaining_seconds`, and `reset_due` are `null`; human output shows `—` for the
unknown values. A missing or partial usage window does not change the
reset-credit `complete` field or the exit code. JSON exposes both the upstream
`used_percent` value and the derived, clamped `remaining_percent` shown in the
terminal.

For a credit that never expires, `expires_at`, `expires_at_unix`, and
`remaining_seconds` are `null`, while `does_not_expire` is `true`. CXRE never
puts opaque credit IDs, account details, titles, or descriptions in this
output.

The schema is versioned independently of the executable. Additive fields may
appear within schema version 1; incompatible changes require a new
`schema_version`.

### Partial data

Codex can report an authoritative available count while returning fewer
individual expiry rows. CXRE does not invent the missing timestamps. It shows
the known rows, emits a warning, sets `complete` to `false`, and reports the
difference in `missing_count`. This is a successful query and exits 0. An
explicit count of zero is also successful; a missing reset-credit summary is
an operational error.

## Errors and exit codes

| Exit | Meaning |
| ---: | --- |
| `0` | Successful query, including explicitly empty or partial data |
| `1` | Authentication, Codex, timeout, network, or protocol failure |
| `2` | Invalid flags or positional arguments |

Human errors are short and actionable. With `--json`, stdout stays empty and
stderr contains one sanitized object:

```json
{
  "error": {
    "code": "auth_missing",
    "message": "Unable to find Codex authentication.",
    "action": "Run `codex login`, sign in with ChatGPT, then run `cxre` again."
  }
}
```

Stable error codes are `usage`, `codex_not_found`, `auth_missing`,
`unsupported_auth`, `codex_too_old`, `timeout`, `network`, `protocol`, and
`output`. Backend response bodies, child-process stderr, and credentials are
never copied into user-facing errors.

### Troubleshooting

**CXRE cannot find Codex**

Confirm `codex --version` works in the same shell. For an unusual installation,
set `CXRE_CODEX` to the Codex executable path.

**CXRE cannot find authentication**

```sh
codex login
cxre
```

You never need to copy a token into CXRE.

**CXRE says Codex is too old**

Update Codex to 0.143.0 or newer, then retry. CXRE also feature-detects reset
expiry details because the protocol can evolve independently of version
numbers.

**The result is incomplete**

CXRE displayed every expiry row Codex provided. Update Codex and retry later;
the reported count remains authoritative, and the warning identifies how many
timestamps are unavailable.

## Privacy and security

Credentials are passwords. CXRE is intentionally designed so it never needs
to possess them:

1. It starts one `codex app-server --stdio` child process.
2. It initializes the documented
   [Codex app-server protocol](https://developers.openai.com/codex/app-server).
3. It asks Codex for `account/read` with `refreshToken: false`, then
   `account/rateLimits/read`.
4. It normalizes recognized quota windows, reset-credit counts, and expiration
   timestamps in memory, terminates the child, and renders the result.

CXRE does **not** read Codex's `auth.json`, query an operating-system keychain,
store credentials, print tokens, send telemetry, consume reset credits, or
make direct network requests. Codex owns its normal credential caching and
service communication, as described by the
[Codex authentication documentation](https://developers.openai.com/codex/auth).
CXRE inherits the normal process environment, including `CODEX_HOME`, without
searching private credential paths.

CXRE uses only the account type, recognized quota windows, and reset-credit
rate-limit data needed for this command; the app-server response passes through
memory while it is decoded.
Raw app-server messages and stderr are never logged. See
[SECURITY.md](SECURITY.md) for vulnerability reporting.

## Development

```sh
make build        # bin/cxre
make test         # unit and fake app-server tests
make test-race    # race detector
make check        # format, module, vet, and tests
make vulncheck    # Go vulnerability database
make snapshot     # local GoReleaser snapshot
```

The optional live integration test uses an existing Codex sign-in and records
no real identifiers or timestamps:

```sh
CXRE_INTEGRATION=1 go test ./...
```

The code is split into small internal packages for CLI dispatch, Codex JSONL
transport, usage-limit and reset-credit domain logic, terminal/JSON rendering,
and build metadata. See [CONTRIBUTING.md](CONTRIBUTING.md) before proposing a
change.

Releases follow Semantic Versioning. A `vX.Y.Z` tag runs tests, builds static
archives, generates checksums and SBOMs, publishes a GitHub Release, records
provenance, updates `rcmcsweeney/homebrew-tap`, and runs each archive's help and
version paths on a matching native hosted runner.

## Scope

Version 1 remains focused on reset expirations; the two standard usage windows
provide concise context above that data. The internal command registry leaves
space for possible future commands such as `cxre status`, `cxre limits`, and
`cxre account`, but none are promised yet.

## License

[MIT](LICENSE) © 2026 CXRE contributors.
