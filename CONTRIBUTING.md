# Contributing to ghx

Thanks for helping! ghx is a single Go binary built on [Bubbletea](https://github.com/charmbracelet/bubbletea)
and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

## Develop

```sh
git clone https://github.com/Tukesh1/ghx && cd ghx
make demo     # run with the offline dataset — no GitHub token needed
make check    # go vet, unit tests, and a render check of every view at three terminal sizes
```

`./ghx --demo --snapshot --tab 6 --size 160x48` prints one rendered frame, handy for reviewing
layout changes without a TTY.

## Ground rules

- **Every frame must be exactly the terminal size.** `make check` and `TestEveryFrameFitsTheTerminal`
  fail if any line is a cell too wide or the frame has an extra row — that is what keeps the header
  pinned. Run text from GitHub through the existing helpers (`fit`, `wrap`), never print it raw.
- **Demo mode must keep working.** New data needs a realistic entry in `internal/github/mock.go`
  (or `MockAnalytics`) so the view can be developed and tested offline.
- **Mutations go through `gh`** (see `internal/github/rest.go`) and anything destructive needs a
  confirm-twice key, like `m m` for merge.
- Keep the action-first focus: features most GitHub users need daily, not every API field.

## Releasing

Push a tag: `git tag v0.3.0 && git push origin v0.3.0`. The Release workflow runs GoReleaser, which
builds macOS/Linux/Windows binaries, publishes `ghx_<os>_<arch>` archives with `checksums.txt`,
and `install.sh` picks them up automatically.
