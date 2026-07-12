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

- [ ] Authenticate GitHub CLI as `rcmcsweeney` with `gh auth login -h github.com`.
- [ ] Create the public repository `rcmcsweeney/cxre` with `main` as its default branch.
- [ ] Commit and push the complete source tree.
- [ ] Add the repository description: `Know exactly when your Codex reset credits expire.`
- [ ] Add repository topics such as `codex`, `cli`, `golang`, `developer-tools`, and `openai`.
- [ ] Confirm the CI workflow passes on Linux, macOS, and Windows.
- [ ] Enable private vulnerability reporting in the repository security settings.
- [ ] Review branch protection or rulesets for `main` after the initial push.

## Homebrew release prerequisite

- [ ] Create the public repository `rcmcsweeney/homebrew-tap` with a `main` branch.
- [ ] Create a fine-grained token scoped only to `rcmcsweeney/homebrew-tap` with
      **Contents: read and write** permission.
- [ ] Add that token to `rcmcsweeney/cxre` as the Actions secret
      `HOMEBREW_TAP_GITHUB_TOKEN`.
- [ ] Never put the token in a file, commit, issue, workflow log, or terminal screenshot.

## Release `v0.1.0`

- [ ] Confirm `CHANGELOG.md` and README examples are ready.
- [ ] Run `make ci` locally.
- [ ] Create and push the annotated tag: `git tag -a v0.1.0 -m "CXRE v0.1.0"`.
- [ ] Confirm the release workflow publishes five archives, checksums, five SBOMs,
      provenance, and release notes.
- [ ] Confirm every archive-native `--help` and `--version` smoke job passes.
- [ ] Verify an archive with `checksums.txt` and `gh attestation verify`.
- [ ] Install from Homebrew with `brew install rcmcsweeney/tap/cxre`.
- [ ] Run `cxre`, `cxre --utc`, and `cxre --json` from the installed release.

## Later improvements

- [ ] Publish the prepared Scoop manifest through `rcmcsweeney/scoop-bucket`.
- [ ] Evaluate macOS notarization and Windows code signing.
- [ ] Revisit GoReleaser's Homebrew cask support when migrating from the required
      `Formula/cxre.rb` distribution contract.
