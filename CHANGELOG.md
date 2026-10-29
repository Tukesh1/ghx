# Changelog

All notable changes to ghx. The format follows [Keep a Changelog](https://keepachangelog.com) and the project
uses [semantic versioning](https://semver.org).

## [Unreleased]

## [v0.2.0] — 2026-10-29

### Highlights
- **New Profile tab**: your github.com profile in the terminal, with your avatar drawn as pixel art.
- **Live mode loads reliably** and about twice as fast, and it degrades gracefully instead of silently showing
  demo data.
- **Actions work with just a token**: approve, merge, comment, re-run and more call the GitHub API directly.

### Changed
- Tabs are renumbered so the Profile comes first: `1` Profile, `2` Command Center, `3` Pull Requests,
  `4` Actions, `5` Repositories, `6` Issues. ghx still opens on the Command Center, and `--snapshot --tab`
  follows the new numbers (`7` is repository insights).
- On narrow terminals, inactive tabs show only their key and icon so all six fit at 80 columns.

### Fixed
- **Live mode loads reliably.** The dashboard was one large GraphQL query that GitHub often aborted after ~10s
  (HTTP 502), after which ghx silently showed demo data. It now runs small queries in parallel (about 2×
  faster: ~3.4s instead of ~8s on a real account); a section that fails is reported in the header instead of
  failing the whole screen.
- A failed first load shows an error screen with the reason and automatic retries (5s → 2m backoff) instead of
  pretending with demo data; press `d` to explore the demo on purpose.
- A failed refresh no longer retries on every tick; the last good data stays on screen.
- Fixed a timer leak: every fetch and action started an extra tick loop, so CPU use and spinner speed grew
  the longer ghx ran.
- `--snapshot` showed sample analytics for real repositories; Insights now only ever show real data.
- `c` (checkout) ran `gh pr checkout` in whatever directory ghx was started from. It now only runs inside a
  clone of the PR's repository and copies the command otherwise.
- PRs in repositories without a review policy showed "Review Req" and were never "ready to merge".
- CI chips counted only the first 10 checks ("12/12 passed" on a failing PR); they now use GitHub's counts for
  every check, and failing checks are listed first.
- `--org` filtered your 15 most recent repositories instead of querying the organization's own.
- The notifications drawer could not scroll past the first screen, and opening a notification went to the
  repository instead of the thread.
- Failure log excerpts started with unrelated `with:`/`env:` echo lines; they now start at the failing step.
- `install.sh` could hang for over a minute when GitHub's download CDN left a connection open. Downloads now
  time out after 10s and retry, and the binary download shows a progress bar.
- `install.ps1` was very slow on Windows PowerShell 5.1 (its progress bar redraws for every packet) and could
  fail on old TLS defaults. Both are fixed, and downloads have timeouts.
- Truncated column headers ("QUICK ACTI…"), indistinguishable long repository names (now cut in the middle),
  half-empty charts, "1 languages"-style plurals, and fake dropdown carets in the header.

### Added
- **Profile tab** (`1`): your github.com profile in the terminal. It shows your avatar drawn as pixel art, name,
  status, bio, followers, public organizations, links, stars earned and top languages; your profile README;
  pinned repositories (or your most-starred ones) as cards; and the contribution graph with commits by
  repository. Like github.com, it never shows private organization memberships or private repositories, so
  screenshots are safe to share. `enter` opens a pinned repository, `c` copies its clone command.
- `O` opens your profile on github.com.
- Actions (approve, merge, close, comment, re-run, dispatch, mark read) call the GitHub API directly — they
  work with just a `GITHUB_TOKEN`, no GitHub CLI required, and report clear errors.
- Comment composer: `c` on an issue or `C` on any PR or issue writes a Markdown comment without leaving ghx.
- Merge uses the method the repository allows (squash, merge commit or rebase) and deletes your own branch.
- `y` copies the selected item's link; `enter` opens it on the PR, Actions and Issues tabs.
- GitHub Light theme; the chosen theme and glyph set are remembered between runs.
- Context-aware footer showing the keys for what is selected; clickable header controls.
- Clear messages for expired tokens (401) and rate limits, with the reset time.
- `--refresh` flag, `GHX_CONFIG_DIR` / `GHX_CACHE_DIR`, `R` to refresh from anywhere, `ctrl+d`/`ctrl+u` paging.
- Tests against a fake GitHub API, UI behaviour tests, race detector and golangci-lint in CI.
- `tools/screenshot` regenerates the README screenshots; `docs/demo.tape` records a demo GIF with VHS.

## [v0.1.0] — 2026-10-27

First release: Command Center, Pull Requests, Actions, Repositories with Insights, Issues, notifications
inbox, four themes, demo mode, and install scripts for macOS, Linux and Windows.

[Unreleased]: https://github.com/Tukesh1/ghx/compare/v0.2.0...HEAD
[v0.2.0]: https://github.com/Tukesh1/ghx/compare/v0.1.0...v0.2.0
[v0.1.0]: https://github.com/Tukesh1/ghx/releases/tag/v0.1.0
