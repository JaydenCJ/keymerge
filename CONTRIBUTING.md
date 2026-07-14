# Contributing to keymerge

Issues, discussions and pull requests are all welcome.

## Getting started

You need Go ≥1.22 and git ≥2.30; nothing else.

```bash
git clone https://github.com/JaydenCJ/keymerge && cd keymerge
go build ./...
go test ./...
bash scripts/smoke.sh
```

`scripts/smoke.sh` builds the binary, merges the shipped fixtures, then
drives a real `git merge` in a temp repository with keymerge installed as
the merge driver — both the clean case and the collision case. It must
finish by printing `SMOKE OK`.

## Before you open a pull request

1. `gofmt -l .` reports nothing (formatting is enforced).
2. `go vet ./...` passes with no findings.
3. `go test ./...` passes (88 deterministic tests, no network).
4. `bash scripts/smoke.sh` prints `SMOKE OK`.
5. Add tests for behavior changes; keep logic in pure, unit-testable
   modules (the parser, merge engine and renderer never touch the
   filesystem or shell out — only `internal/cli` does).

## Ground rules

- Keep dependencies at zero; adding one needs strong justification in the
  PR. keymerge is standard library only.
- No network calls, ever — the only external interface is the local `git`
  binary, and only in `install`. No telemetry.
- Merge rules are a contract: any change to the decision matrix needs a
  row update in `docs/merge-rules.md` and a test per affected cell.
- Code comments and doc comments are written in English.
- Determinism first: identical inputs must produce byte-identical output,
  including key order, conflict order and message order.

## Reporting bugs

Include the output of `keymerge version`, the three input files (base,
ours, theirs — minimized if possible), the exact command, and what you
expected. For driver issues inside git, also include your
`.gitattributes` line and `git config --get-regexp 'merge\.keymerge'`.

## Security

Please do not open public issues for security problems; use GitHub's
private vulnerability reporting on this repository instead.
