# Security policy

CXRE touches an authentication boundary even though it never reads or stores a
credential. Reports that could expose a Codex session, account data, or
user-controlled terminal are taken seriously.

## Supported versions

Security fixes are provided for the latest released minor version. During the
0.x series, users should update to the newest release before reporting a
problem that may already be fixed.

| Version | Supported |
| --- | --- |
| Latest release | Yes |
| Older releases | No |

## Report a vulnerability

Use GitHub's private vulnerability reporting page:

<https://github.com/rcmcsweeney/cxre/security/advisories/new>

Do not open a public issue for suspected credential disclosure, command
execution, protocol injection, terminal escape injection, or dependency
compromise. If private reporting is unavailable, open a public issue containing
only a request for a private contact channel; do not include technical details.

Include, when safe:

- the CXRE version and operating system;
- the Codex CLI version and authentication mode, but no account identity;
- concise reproduction steps using synthetic data;
- expected and observed behavior;
- your assessment of impact.

Never attach tokens, `auth.json`, keychain exports, full environment dumps,
real app-server payloads, or unredacted logs. The maintainers will acknowledge
the report, investigate, coordinate a fix, and credit reporters who want to be
credited. Disclosure timing is agreed privately after a fix is available.

## Security design

CXRE follows a narrow, read-only trust model:

- It resolves the user-selected `CXRE_CODEX` executable or `codex` on `PATH`.
- It starts one `codex app-server --stdio` child process and uses documented
  account-read and rate-limit-read requests.
- It passes `refreshToken: false` and accepts only ChatGPT authentication for
  the v0.1 command.
- It does not read `auth.json`, access an operating-system keychain, store
  credentials, or ask users to paste tokens.
- It opens no network connection itself. The Codex child owns required OpenAI
  service communication and its existing authentication lifecycle.
- It buffers only a bounded amount of child stderr for internal classification
  and never echoes that content or a raw protocol body.
- It exposes only counts, expiration times, countdowns, completeness, and
  allowlisted errors—not opaque IDs or account details.
- It applies a deadline and cleans up the child on timeout or interruption.

Setting `CXRE_CODEX` explicitly trusts that executable as Codex. Treat changes
to `PATH`, `CXRE_CODEX`, `CODEX_HOME`, and the executable itself with the same
caution as any other local developer tool configuration.

## Release integrity

Official artifacts are attached to GitHub Releases and include SHA-256
checksums and per-archive SBOMs. The release workflow creates GitHub artifact
attestations. Initial releases are not code-signed or notarized.

Verify a downloaded archive against both `checksums.txt` and the repository's
GitHub provenance before executing it. Package-manager metadata is updated by
the same tag-driven workflow.

## Out of scope

The following are not vulnerabilities by themselves:

- a local administrator replacing the trusted Codex or CXRE executable;
- expected macOS Gatekeeper or Windows SmartScreen warnings for unsigned
  initial releases;
- service availability or reset-credit accuracy originating upstream, provided
  CXRE reports completeness honestly;
- unsupported API-key-only or Bedrock authentication returning a sanitized
  error.
