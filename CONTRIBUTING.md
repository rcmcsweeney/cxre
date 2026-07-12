# Contributing to CXRE

Thanks for helping make reset-credit expirations clearer. CXRE is small on
purpose: changes should preserve fast startup, read-only behavior, predictable
output, and a narrow v1 feature set.

By participating, be respectful, assume good intent, and keep technical
discussion focused on the work.

## Before you start

- Search existing issues and pull requests before opening a duplicate.
- Open an issue before a large architectural change or new user-facing command.
- Do not post credentials, real app-server payloads, account identifiers, or
  unredacted diagnostic output.
- Security reports belong in the private process described in
  [SECURITY.md](SECURITY.md), not a public issue.

## Set up

You need:

- Go 1.25 or newer;
- Git;
- Codex CLI 0.143.0 or newer only for the opt-in live integration test;
- GoReleaser v2 and Syft only when testing release snapshots.

```sh
git clone https://github.com/rcmcsweeney/cxre.git
cd cxre
go mod download
make check
```

The normal test suite never requires Codex credentials or network access. It
uses a fake JSONL app-server process.

## Architecture

The public entry point lives in `cmd/cxre`. Internal packages keep these
responsibilities separate:

- CLI parsing and command dispatch;
- Codex child-process and JSONL protocol transport;
- reset-credit normalization, ordering, and completeness;
- human terminal and JSON presentation;
- version/build metadata.

Keep protocol structs at the transport boundary. Keep terminal styling out of
business logic. Never allow raw backend errors or child-process stderr to flow
directly into user-facing output.

## Make a change

1. Create a focused branch.
2. Add or update tests with the implementation.
3. Run formatting and the full local checks.
4. Update README examples and `CHANGELOG.md` when behavior changes.
5. Open a pull request explaining the user-visible result and security impact.

Useful targets:

```sh
make fmt
make test
make test-race
make check
make vulncheck
make coverage
make snapshot
```

`make tidy-check` uses `go mod tidy -diff`, so it checks module files without
rewriting them. CI also smoke-tests `--help` and `--version` on macOS, Linux,
and Windows.

## Testing app-server behavior

Fake-process tests should cover both successful and hostile input, including:

- initialization and request ordering;
- interleaved notifications;
- malformed, unknown, and truncated messages;
- timeouts, signals, crashes, and bounded stderr;
- missing, ChatGPT, API-key-only, and Bedrock authentication modes;
- zero, partial, capped, count-only, and non-expiring credits;
- sentinel secrets that must never reach stdout, stderr, JSON errors, or logs.

Do not add real credentials or captured account payloads as fixtures.

The live test is explicitly opt-in:

```sh
CXRE_INTEGRATION=1 go test ./...
```

It must assert schema invariants only. It must not save real IDs, timestamps,
or account information in snapshots or test logs.

## User-facing compatibility

- Treat JSON schema version 1 and documented error codes as public contracts.
- Prefer additive JSON fields; incompatible changes require a schema bump.
- Unknown flags and positional commands must continue to exit 2.
- Human output may improve, but redirected output must remain plain and useful.
- Partial data is not zero data: preserve the authoritative count and warning.
- Additional commands require an explicit design discussion; v1 remains focused
  on reset expirations.

## Security checklist

Before submitting transport, authentication, error, or logging changes, verify:

- no code reads `auth.json` or an operating-system credential store;
- no token-like value is stored or printed;
- no direct endpoint or telemetry destination was introduced;
- raw app-server stderr and response bodies remain bounded and private;
- child cleanup works on timeout and interruption;
- `--json` errors write one sanitized object to stderr and nothing to stdout.

## Commits and pull requests

Keep commits reviewable and use an imperative subject, for example
`render narrow terminals as stacked rows`. A pull request should include:

- what changed and why;
- tests run;
- sample output for presentation changes, using fictional data;
- compatibility, privacy, or release implications.

Maintainers may ask to split unrelated changes. Contributions are accepted
under the repository's [MIT license](LICENSE).

## Releases

Maintainers release with an annotated Semantic Versioning tag such as
`v0.1.0`. The release workflow validates and tests the repository, then
GoReleaser publishes artifacts and updates the Homebrew tap. See
[`packaging/homebrew`](packaging/homebrew) for the one-time tap setup.
