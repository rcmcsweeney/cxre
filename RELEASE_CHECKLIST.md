# CXRE maintainer checklist

Use this checklist for the initial `v0.1.0` publication. Do not create the
release tag until the repository CI workflow is green.

## Local project

- [x] Initialize the Go module as `github.com/rcmcsweeney/cxre`.
- [x] Implement and document the v0.1.0 CLI.
- [x] Run unit tests, fake app-server tests, the race detector, and `go vet`.
- [x] Run `govulncheck` with no known vulnerabilities found.
- [x] Cross-build all five supported release targets.
- [x] Build a complete GoReleaser snapshot with archives, checksums, and SBOMs.
- [x] Verify the live read-only Codex integration without retaining account data.

## GitHub repository

- [x] Authenticate GitHub CLI as `rcmcsweeney` with `gh auth login -h github.com`.
- [x] Create the public repository `rcmcsweeney/cxre` with `main` as its default branch.
- [x] Commit and push the complete source tree.
- [x] Add the repository description: `Know when your Codex limits reset and reset credits expire.`
- [x] Add repository topics such as `codex`, `cli`, `golang`, `developer-tools`, and `openai`.
- [x] Confirm the CI workflow passes on Linux, macOS, and Windows.
- [x] Enable private vulnerability reporting in the repository security settings.
- [ ] Review branch protection or rulesets for `main` after the initial push.

## Homebrew release prerequisite

- [x] Create the public repository `rcmcsweeney/homebrew-tap` with a `main` branch.
- [x] Create a fine-grained token scoped only to `rcmcsweeney/homebrew-tap` with
      **Contents: read and write** permission.
- [x] Add that token to `rcmcsweeney/cxre` as the Actions secret
      `HOMEBREW_TAP_GITHUB_TOKEN`.
- [x] Never put the token in a file, commit, issue, workflow log, or terminal screenshot.

## Release `v0.1.0`

- [x] Confirm `CHANGELOG.md` and README examples are ready.
- [x] Replace `docs/cxre-terminal.png` with a fresh real screenshot showing the
      `CXRE — Codex Resets` header and usage-limit summary.
- [x] Run `make ci` locally.
- [x] Create and push the annotated tag: `git tag -a v0.1.0 -m "CXRE v0.1.0"`.
- [x] Confirm the release workflow publishes five archives, checksums, five SBOMs,
      provenance, and release notes.
- [x] Confirm every archive-native `--help` and `--version` smoke job passes.
- [x] Verify an archive with `checksums.txt` and `gh attestation verify`.
- [x] Install from Homebrew with `brew install rcmcsweeney/tap/cxre`.
- [x] Run `cxre`, `cxre --utc`, and `cxre --json` from the installed release.

## Later improvements

- [ ] Publish the prepared Scoop manifest through `rcmcsweeney/scoop-bucket`.
- [ ] Evaluate macOS notarization and Windows code signing.
- [ ] Revisit GoReleaser's Homebrew cask support when migrating from the required
      `Formula/cxre.rb` distribution contract.

## Release `v0.1.1`

Preparation may be completed without publishing. Do not create or push the tag
until release publication is explicitly authorized.

- [ ] Move the v0.1.1 entries from `Unreleased` into a dated changelog section.
- [ ] Confirm successful JSON remains schema version 1 and the documented error
      codes and interruption exit statuses match the executable.
- [ ] Run formatting, module tidy-diff, vet, unit tests, race tests, and the
      vulnerability scan with Go 1.26.
- [ ] Run the Go 1.25 compatibility job with `GOTOOLCHAIN=local`.
- [ ] Confirm the full fake app-server suite passes on Linux, macOS, and Windows.
- [ ] Run the opt-in live read-only integration without retaining account data.
- [ ] Confirm the release snapshot contains five archives, five SBOMs,
      `checksums.txt`, the generated Homebrew formula, and all README-linked
      support documentation.
- [ ] Protect source `main` with the stable `CI required` check, required pull
      requests with zero mandatory approvals, resolved conversations, and no
      force-push or deletion.
- [ ] Protect `v*.*.*` tags from update or deletion and restrict creation to the
      maintainer/release identity.
- [ ] Protect the Homebrew tap's `main` branch from force-push or deletion and
      restrict formula writes to the release identity.
- [ ] Move `HOMEBREW_TAP_GITHUB_TOKEN` into a tag-restricted `release`
      environment before the first run of the new publish job.
- [ ] Create and push the annotated tag only after all preceding checks pass.
- [ ] Confirm all five checksum-verified archives pass native smoke tests before
      a draft release is created.
- [ ] Confirm attestations cover the exact tested archives, SBOMs, and checksum
      manifest before the draft is made public.
- [ ] Confirm the public release contains exactly five archives, five SBOMs, and
      `checksums.txt`, with notes taken from the v0.1.1 changelog section.
- [ ] Audit, install, and test the generated formula before updating the tap.
- [ ] Install `github.com/rcmcsweeney/cxre/cmd/cxre@v0.1.1` with Go 1.25 and
      confirm `cxre --version` reports `cxre 0.1.1`, not `cxre dev`.
- [ ] Run `cxre`, `cxre --utc`, and `cxre --json` from the released Homebrew
      installation.
