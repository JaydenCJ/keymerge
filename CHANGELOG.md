# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-07-12

### Added

- Key-level three-way merge engine for JSON: one-sided changes win,
  convergent changes collapse, deletions propagate, and only genuine
  collisions (edit/edit, add/add, delete/edit, type clashes) conflict —
  each addressed by an RFC 6901 JSON Pointer.
- Order-preserving JSON document model: object member order and raw
  number literals survive the merge byte-for-byte; duplicate object keys
  are rejected with the exact line and column.
- Semantic equality that ignores object member order and compares numbers
  by mathematical value, so formatter noise (key reordering, `1` vs `1.0`)
  never produces a conflict.
- diff3 array merging with LCS alignment, recursion into both-edited
  single elements (arrays of objects merge field-by-field), plus `atomic`
  and `union` strategies selectable with `--arrays`.
- Conflict rendering as git-style whole-line markers placed exactly at the
  colliding member, honoring git's marker size (`%L`) and custom labels;
  taking either side of every block yields valid JSON.
- Output style preservation: indent unit (2/4 spaces, tabs), LF/CRLF and
  trailing-newline detection from the ours side.
- `merge` (git driver contract: rewrite `%A`, exit 0/1), `check` (dry run
  listing collision paths), and `install` (writes git config and
  `.gitattributes` idempotently) subcommands.
- Runnable examples (`examples/demo-merge.sh`, fixture triples) and a
  merge-rules reference (`docs/merge-rules.md`).
- 88 deterministic offline tests (unit + in-process CLI integration
  against real temporary git repositories) and `scripts/smoke.sh`, which
  drives an actual `git merge` through the installed driver.

[0.1.0]: https://github.com/JaydenCJ/keymerge/releases/tag/v0.1.0
