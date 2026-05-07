# Release Pipeline Research: `canvacli`

This document captures the recommended release pipeline for `canvacli`, an
open-source Go CLI distributed via GitHub Releases and a Homebrew tap. It
targets `darwin/arm64`, `darwin/amd64`, `linux/amd64`, `linux/arm64`, and
`windows/amd64`.

## 1. GoReleaser

### Version

The latest stable GoReleaser is **v2.15.4** (April 2026); use the floating
constraint `~> v2` in CI so that patch releases are picked up automatically.
The companion `goreleaser/goreleaser-action` is on **v7.2.1** (April 2026), and
all examples below assume the v2 config schema. See
[GoReleaser releases](https://github.com/goreleaser/goreleaser/releases) and
[goreleaser-action releases](https://github.com/goreleaser/goreleaser-action/releases).

### Build matrix and binary naming

GoReleaser handles the full target matrix natively via `goos` / `goarch` and
`ignore` exclusions ([builds docs](https://goreleaser.com/customization/builds/go/)).
For our matrix we list four platforms and exclude `windows/arm64` (we do not
ship that variant). Set `env: [CGO_ENABLED=0]` so the cross-compile is purely
Go-native (this is the entire reason we picked `modernc.org/sqlite`; see
section 5).

### Archive format

Default archive format is `tar.gz`. Override Windows to `zip` via
`format_overrides` ([archive docs](https://goreleaser.com/customization/archive/)).
The default `name_template` already produces
`canvacli_<version>_<os>_<arch>.<ext>` style filenames, which matches what
Homebrew expects.

### Checksums

`checksum:` produces a single `canvacli_<version>_checksums.txt` using SHA-256
by default ([checksum docs](https://goreleaser.com/customization/checksum/)).
This file is consumed by the SLSA provenance generator (see below) and by
Homebrew, which derives `sha256` per-archive from it.

### Changelog from conventional commits

GoReleaser's `changelog` block has first-class support for grouping by
conventional-commit prefixes via regex
([changelog docs](https://goreleaser.com/customization/changelog/)). We use
`use: github` to pull author logins from the GitHub compare API, then group
into Features / Fixes / Performance / Refactors / Others, and exclude `docs:`
and `chore:` noise.

### SLSA provenance

For a public OSS CLI, SLSA Level 3 provenance is cheap insurance and signals
supply-chain hygiene. The official pattern uses `slsa-framework/slsa-github-generator`'s
`generator_generic_slsa3.yml` reusable workflow, fed the base64-encoded
contents of the GoReleaser checksum file as input
([GoReleaser SLSA blog](https://goreleaser.com/blog/slsa-generation-for-your-artifacts/),
[example repo](https://github.com/goreleaser/goreleaser-example-slsa-provenance)).
This requires `id-token: write` permission on the calling job.

### Version embedding via ldflags

GoReleaser's default ldflags already inject `main.version`, `main.commit`, and
`main.date`. In `cmd/canvacli/main.go` declare:

```go
var (
    version = "dev"
    commit  = "none"
    date    = "unknown"
)
```

and surface them through `--version`. No extra config is needed beyond
declaring the `ldflags` block (we also pass `-s -w` to strip the binary).

## 2. Homebrew tap

### Separate tap repo

Create a dedicated tap repository: **`<org>/homebrew-tap`** (Homebrew requires
the `homebrew-` prefix). Keeping the formula in its own repo means consumer
installs read `brew tap <org>/tap && brew install canvacli`, releases stay
small, and the main repo never sees `Formula/` churn. Including the formula
in the main repo via `brew tap <org>/canvacli https://github.com/<org>/canvacli`
also works but is non-idiomatic.

### Formula vs Cask

GoReleaser deprecated `brews:` in v2.10 in favor of `homebrew_casks:`
([formulas docs](https://goreleaser.com/customization/homebrew_formulas/)).
However, **Casks are macOS-only**; for a cross-platform CLI we want users on
Linux to `brew install` too, which means we still need a Formula. The
practical 2026 advice: keep using `brews:` until v3 lands (no announced date),
or maintain a hand-written Formula in the tap repo and have GoReleaser PR
updates to it. The skeleton below uses `brews:` because it remains functional
and is the lowest-friction path.

### Auto-update from GoReleaser

The `brews:` block points at the tap repo and supplies a commit author and a
commit message template
([homebrew docs](https://goreleaser.com/customization/homebrew/)). On every
tag, GoReleaser regenerates `Formula/canvacli.rb` and pushes it. Set
`pull_request.enabled: true` if you prefer review-first; for solo
maintainership a direct push is fine.

### PAT requirements (least privilege)

The default `GITHUB_TOKEN` cannot push to a sibling repo. Create a
**fine-grained PAT** scoped to **only the tap repo** with `Contents: read &
write` and `Metadata: read`. Store it as `HOMEBREW_TAP_TOKEN` in the main
repo's secrets. This is strictly less powerful than a classic PAT with `repo`
scope and is the recommended pattern.

### Formula template structure

GoReleaser generates the Ruby formula automatically; you rarely write it by
hand. The generated file uses `class Canvacli < Formula`, embeds per-platform
`url`/`sha256` blocks for each archive, and a `def install` that copies the
binary to `bin`. Add a `test do` block via `brews.test:` that runs
`system "#{bin}/canvacli", "--version"` so `brew test canvacli` works.

## 3. GitHub Actions workflow

### Trigger and basics

Trigger on `push` of tags matching `v*`. Required permissions on the release
job: `contents: write` (release assets), `packages: write` (only if pushing
images), and `id-token: write` (SLSA / cosign keyless).

### Action versions (current as of 2026-05)

- `actions/checkout@v5` (use `fetch-depth: 0` — required for changelog)
- `actions/setup-go@v6` ([releases](https://github.com/actions/setup-go/releases)),
  caching enabled by default
- `goreleaser/goreleaser-action@v7` with `version: "~> v2"`

### Caching

`actions/setup-go@v6` caches the Go module and build cache automatically based
on `go.sum`. Pass `cache-dependency-path: go.sum` only if your repo has
multiple modules.

### Required secrets

- `GITHUB_TOKEN` — provided by GitHub, used for the release itself.
- `HOMEBREW_TAP_TOKEN` — fine-grained PAT for the tap repo (see section 2).

### Smoke test

After GoReleaser builds the artifacts (use `goreleaser build --snapshot
--clean` in a separate pre-release job, or run the binary out of `dist/` in
the same job), exercise the headline commands so a busted release never
ships:

```bash
./dist/canvacli_linux_amd64_v1/canvacli --version
./dist/canvacli_linux_amd64_v1/canvacli schema --compact
```

Failures fail the workflow before the release is published.

## 4. Versioning

- **Tag format:** `vMAJOR.MINOR.PATCH`, e.g. `v1.0.0`. GoReleaser strips the
  leading `v` for `{{ .Version }}` automatically.
- **Pre-releases:** `v0.1.0-beta.1`, `v1.0.0-rc.1`. GoReleaser flags these as
  pre-releases on GitHub when the SemVer prerelease component is non-empty;
  Homebrew updates are skipped automatically for pre-releases when
  `brews.skip_upload: auto` is set.
- **`canvacli --version` output:** rendered from the `version`, `commit`, and
  `date` package vars populated by ldflags. Format suggestion:
  `canvacli 1.2.3 (commit abc1234, built 2026-05-07T12:00:00Z)`.

## 5. Pitfalls

### CGo + cross-compilation

The classic SQLite driver `mattn/go-sqlite3` requires CGo, which means each
target needs a working C cross-toolchain in CI — painful, slow, and brittle
on macOS-from-Linux builds. **`modernc.org/sqlite` is a CGo-free transpilation
of the SQLite C source to pure Go** and supports darwin amd64/arm64, linux
amd64/arm64/386/arm, and windows amd64
([package docs](https://pkg.go.dev/modernc.org/sqlite)). This is exactly the
right call for our matrix: keep `CGO_ENABLED=0`, build everything on a single
`ubuntu-latest` runner, no `zig cc` or osxcross required. The tradeoff is
roughly 2x slower SQLite throughput, which is irrelevant for a CLI's local
state store.

### Local Homebrew formula testing

Before tagging, validate the formula locally. With a checked-out tap repo:

```bash
brew install --build-from-source ./Formula/canvacli.rb
brew test canvacli
brew audit --strict --new-formula ./Formula/canvacli.rb
```

You can also dry-run the whole release with
`goreleaser release --snapshot --clean --skip=publish` to inspect `dist/`
without pushing anything.

### Other gotchas

- Always tag from `main` (or a release branch) — GoReleaser compares against
  the previous tag for the changelog and a detached commit will silently
  produce an empty changelog.
- Don't forget to push the tag (`git push --tags`) — `git push` alone does
  not.
- Pin all third-party actions to a major version (`@v7`) and use Dependabot
  to bump them; pinning to a SHA is overkill for OSS releases.

---

## Appendix A — `.goreleaser.yaml` skeleton

```yaml
# yaml-language-server: $schema=https://goreleaser.com/static/schema.json
version: 2

project_name: canvacli

before:
  hooks:
    - go mod tidy
    - go mod download

builds:
  - id: canvacli
    main: ./cmd/canvacli
    binary: canvacli
    env:
      - CGO_ENABLED=0
    flags:
      - -trimpath
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}
      - -X main.date={{.Date}}
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    ignore:
      - goos: windows
        goarch: arm64

archives:
  - id: canvacli
    name_template: >-
      {{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}
    format_overrides:
      - goos: windows
        formats: [zip]
    files:
      - LICENSE*
      - README*
      - CHANGELOG*

checksum:
  name_template: "{{ .ProjectName }}_{{ .Version }}_checksums.txt"
  algorithm: sha256

snapshot:
  version_template: "{{ incpatch .Version }}-next"

changelog:
  use: github
  sort: asc
  groups:
    - title: Features
      regexp: '^.*?feat(\([[:word:]]+\))??!?:.+$'
      order: 0
    - title: Bug Fixes
      regexp: '^.*?fix(\([[:word:]]+\))??!?:.+$'
      order: 1
    - title: Performance
      regexp: '^.*?perf(\([[:word:]]+\))??!?:.+$'
      order: 2
    - title: Refactors
      regexp: '^.*?refactor(\([[:word:]]+\))??!?:.+$'
      order: 3
    - title: Others
      order: 999
  filters:
    exclude:
      - "^docs:"
      - "^chore:"
      - "^test:"
      - "^ci:"
      - "merge conflict"
      - Merge pull request
      - Merge remote-tracking branch
      - Merge branch

release:
  github:
    owner: <ORG>
    name: canvacli
  draft: false
  prerelease: auto
  mode: replace

brews:
  - name: canvacli
    repository:
      owner: <ORG>
      name: homebrew-tap
      branch: main
      token: "{{ .Env.HOMEBREW_TAP_TOKEN }}"
    directory: Formula
    homepage: "https://github.com/<ORG>/canvacli"
    description: "Canvas CLI for <thing>."
    license: "MIT"
    skip_upload: auto
    commit_author:
      name: canvacli-releaser
      email: releases@<ORG>.dev
    commit_msg_template: "chore(brew): canvacli {{ .Tag }}"
    test: |
      system "#{bin}/canvacli", "--version"
    install: |
      bin.install "canvacli"
```

## Appendix B — `.github/workflows/release.yml` skeleton

```yaml
name: release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write
  id-token: write   # SLSA + cosign keyless

jobs:
  goreleaser:
    name: GoReleaser
    runs-on: ubuntu-latest
    outputs:
      hashes: ${{ steps.hashes.outputs.hashes }}
    steps:
      - name: Checkout
        uses: actions/checkout@v5
        with:
          fetch-depth: 0

      - name: Set up Go
        uses: actions/setup-go@v6
        with:
          go-version: stable
          cache: true

      - name: Run GoReleaser
        id: goreleaser
        uses: goreleaser/goreleaser-action@v7
        with:
          distribution: goreleaser
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}

      - name: Smoke test built binary
        run: |
          BIN=$(find dist -type f -name canvacli -path '*linux_amd64*' | head -n1)
          test -x "$BIN"
          "$BIN" --version
          "$BIN" schema --compact

      - name: Generate provenance subjects
        id: hashes
        env:
          ARTIFACTS: ${{ steps.goreleaser.outputs.artifacts }}
        run: |
          checksum_file=$(echo "$ARTIFACTS" | jq -r '.[] | select(.type=="Checksum") | .path')
          echo "hashes=$(base64 -w0 < "$checksum_file")" >> "$GITHUB_OUTPUT"

  provenance:
    needs: [goreleaser]
    permissions:
      actions: read
      id-token: write
      contents: write
    uses: slsa-framework/slsa-github-generator/.github/workflows/generator_generic_slsa3.yml@v2.1.0
    with:
      base64-subjects: "${{ needs.goreleaser.outputs.hashes }}"
      upload-assets: true
```

## Sources

- [GoReleaser docs](https://goreleaser.com/)
- [GoReleaser GitHub Actions](https://goreleaser.com/ci/actions/)
- [GoReleaser Homebrew](https://goreleaser.com/customization/homebrew/) /
  [Formulas (deprecated)](https://goreleaser.com/customization/homebrew_formulas/)
- [GoReleaser changelog](https://goreleaser.com/customization/changelog/),
  [archive](https://goreleaser.com/customization/archive/),
  [checksum](https://goreleaser.com/customization/checksum/),
  [builds](https://goreleaser.com/customization/builds/go/)
- [SLSA generation blog](https://goreleaser.com/blog/slsa-generation-for-your-artifacts/),
  [example repo](https://github.com/goreleaser/goreleaser-example-slsa-provenance)
- [`actions/setup-go`](https://github.com/actions/setup-go),
  [`goreleaser/goreleaser-action`](https://github.com/goreleaser/goreleaser-action)
- [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite)
- [Homebrew formula cookbook](https://docs.brew.sh/Formula-Cookbook)
