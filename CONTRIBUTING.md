# Contributing to ghx

Thanks for helping! ghx is a single Go binary built on [Bubble Tea](https://github.com/charmbracelet/bubbletea)
and [Lip Gloss](https://github.com/charmbracelet/lipgloss). Bug reports, ideas and pull requests are all welcome.

## Develop

```sh
git clone https://github.com/Tukesh1/ghx && cd ghx
make demo       # run with the offline dataset — no GitHub token needed
make run        # run against your account (uses gh auth / GITHUB_TOKEN)
make check      # go vet, race tests, and a render check of every view at three terminal sizes
```

`make snapshot TAB=7 SIZE=160x48` prints one rendered frame (tab 7 is repository insights), handy for reviewing
layout changes without a TTY. Drop `--demo` (`./ghx --snapshot --tab 1`) to render your live data.

## Ground rules

- **Every frame must be exactly the terminal size.** `make check` and `TestEveryFrameFitsTheTerminal` fail if
  any line is a cell too wide or the frame has an extra row — that is what keeps the header pinned. Run text
  from GitHub through the existing helpers (`fit`, `fitMid`, `wrap`), never print it raw.
- **Demo mode must keep working.** New data needs a realistic entry in `internal/github/mock.go`
  (or `MockAnalytics`) so the view can be developed and tested offline. Real repositories never show mock data.
- **Mutations go through `internal/github/rest.go`**, are never retried automatically, and anything destructive
  needs a confirm-twice key, like `m m` for merge. Simulate the change in demo mode via the `after` callback.
- **One tick loop.** Only the `tickMsg` handler schedules the next tick; don't return `tick(...)` elsewhere.
- **Degrade, don't fail.** A failing section becomes a `Dashboard.Warnings` entry, not an error screen.
- Keep the action-first focus: features most GitHub users need daily, not every API field.

## Tests

`go test -race ./...` covers the API client against a fake GitHub server (`internal/github/client_test.go`),
UI behaviour such as backoff, the comment composer and demo mutations (`internal/ui/behavior_test.go`), and
frame geometry for every theme and glyph set (`internal/ui/ui_test.go`). Please add a test with your change.

## Screenshots

The README images are generated from the demo dataset:

```sh
make screenshots    # writes docs/screenshots/*.svg
```

For an animated demo, install [VHS](https://github.com/charmbracelet/vhs) and run `vhs docs/demo.tape`.

## Releasing

Update `CHANGELOG.md`, then push a tag: `git tag v0.2.0 && git push origin v0.2.0`. The Release workflow runs
GoReleaser, which builds macOS/Linux/Windows binaries and publishes `ghx_<os>_<arch>` archives with
`checksums.txt`; `install.sh` and `install.ps1` pick them up automatically.
