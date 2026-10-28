# Changelog

All notable changes to ghx. The format follows [Keep a Changelog](https://keepachangelog.com) and the project
uses [semantic versioning](https://semver.org).

## [Unreleased] — v0.2.0

### Fixed
- **Live mode loads reliably.** The dashboard was one large GraphQL query that GitHub often aborted after ~10s
  (HTTP 502), after which ghx silently showed demo data. It now runs small queries in parallel (about 2× faster: ~3.4s instead of ~8s on a real account);
  a section that fails is reported in the header instead of failing the whole screen.
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
- Truncated column headers ("QUICK ACTI…"), indistinguishable long repository names (now cut in the middle),
  half-empty charts, "1 languages"-style plurals, and fake dropdown carets in the header.

### Added
- **Profile tab** (`1`): your github.com profile in the terminal — avatar drawn as pixel art, name, status, bio,
  followers, organizations, links and top languages; your profile README; pinned repositories (or your most
  starred ones) as cards; the contribution graph and commits by repository. The other tabs move one key up
  (Command Center is `2`, Issues `6`); ghx still opens on the Command Center.
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

## [v0.1.0]

First release: Command Center, Pull Requests, Actions, Repositories with Insights, Issues, notifications
inbox, four themes, demo mode, and install scripts for macOS, Linux and Windows.
