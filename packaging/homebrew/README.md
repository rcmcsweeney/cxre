# Homebrew tap publication

Stable CXRE tags publish `Formula/cxre.rb` to
`rcmcsweeney/homebrew-tap` through GoReleaser. The generated formula contains
the real release URLs and SHA-256 values, so this repository intentionally does
not check in a placeholder formula with invalid checksums.

## One-time maintainer setup

1. Create the public repository `rcmcsweeney/homebrew-tap` with a `main` branch.
2. Create a fine-grained GitHub personal access token restricted to that
   repository with **Contents: read and write** permission.
3. Add it to `rcmcsweeney/cxre` as the Actions repository secret
   `HOMEBREW_TAP_GITHUB_TOKEN`.
4. Protect the secret and rotate it immediately if it is ever printed or
   exposed.
5. Push a stable Semantic Versioning tag such as `v0.1.0`.

The tag-driven release workflow supplies the token only to GoReleaser.
GoReleaser writes `Formula/cxre.rb` to the tap's `main` branch after it has
computed the release archive checksums.

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
