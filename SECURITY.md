# Security policy

## Reporting a vulnerability

Please **do not** open a public issue. Report it privately through
[GitHub security advisories](https://github.com/Tukesh1/ghx/security/advisories/new) with steps to reproduce.
You will get a response within a few days, and a fix will be released as soon as possible.

## How ghx handles your credentials

- ghx reads a token from `GITHUB_TOKEN`, `GH_TOKEN` or `gh auth token`. It never writes the token to disk
  and only sends it to `api.github.com`.
- The dashboard cache (`~/.cache/ghx` or `~/Library/Caches/ghx`) may contain private repository names and
  titles, so it is written with `0600` permissions. Delete the directory to clear it.
- Text from GitHub (titles, logs, descriptions) is sanitized before rendering, so control characters and
  escape sequences in untrusted content cannot move the cursor or rewrite the screen.
- Mutations are never retried automatically, and destructive ones require a confirming keypress.
