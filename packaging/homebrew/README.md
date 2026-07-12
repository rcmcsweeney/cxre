# Homebrew tap publication

Stable CXRE tags generate `Formula/cxre.rb` with GoReleaser. The release
workflow smoke-tests and attests the exact archives, publishes them, then
audits, installs, and tests the generated formula before copying it to
`rcmcsweeney/homebrew-tap`. The formula contains the real release URLs and
SHA-256 values, so this repository intentionally does not check in a
placeholder with invalid checksums.

## One-time maintainer setup

1. Confirm the public repository `rcmcsweeney/homebrew-tap` exists with a
   `main` branch.
2. Create a fine-grained GitHub personal access token restricted to that
   repository with **Contents: read and write** permission.
3. Add it to `rcmcsweeney/cxre` as the Actions repository secret
   `HOMEBREW_TAP_GITHUB_TOKEN`, then move it into the tag-restricted `release`
   environment before publishing with the staged workflow.
4. Protect the secret and rotate it immediately if it is ever printed or
   exposed.
5. Push a stable Semantic Versioning tag such as `v0.1.0`.

The build and smoke jobs never receive the token. Only the final tap-publication
job can read it, after the GitHub release is public and the generated formula
has passed `brew audit`, `brew install`, and `brew test`.

## Verify a publication

```sh
brew update
brew install rcmcsweeney/tap/cxre
cxre --version
brew audit --strict rcmcsweeney/tap/cxre
brew test rcmcsweeney/tap/cxre
```

The formula test executes only `cxre --version`; release automation never needs
a Codex sign-in.

GoReleaser 2.10 and newer recommends Homebrew casks for prebuilt executables.
CXRE v0.1 keeps the approved formula-based interface so users can install with
`brew install rcmcsweeney/tap/cxre`. A future packaging-only migration can move
to a cask without changing the CXRE executable or its CLI contract.
