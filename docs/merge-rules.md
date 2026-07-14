# keymerge merge rules

This document is the contract for what merges cleanly and what conflicts.
Every cell of the matrix has a corresponding test in `internal/merge`.

## Notation

For each key (or array region), three states are compared against the
common ancestor: **base** (the merge base, git's `%O`), **ours** (`%A`,
the current branch) and **theirs** (`%B`, the branch being merged).
"Changed" means *semantically* different from base — see
[What counts as a change](#what-counts-as-a-change).

## The decision matrix

| ours vs base | theirs vs base | result |
|---|---|---|
| unchanged | unchanged | keep |
| changed | unchanged | take ours |
| unchanged | changed | take theirs |
| changed | changed, **identically** | take the shared value (converged) |
| edited | edited differently, both objects | **recurse** per key |
| edited | edited differently, both arrays | **array strategy** (below) |
| edited | edited differently, scalars | conflict `edit/edit` |
| edited | edited to another kind | conflict `type` |
| deleted | edited | conflict `delete/edit` |
| edited | deleted | conflict `edit/delete` |
| deleted | deleted | delete |
| added | added, identically | keep |
| added | added differently (objects) | **recurse** per key |
| added | added differently (scalars) | conflict `add/add` |

Every conflict is addressed by an [RFC 6901](https://www.rfc-editor.org/rfc/rfc6901)
JSON Pointer (e.g. `/dependencies/react`, with `~`/`/` escaped as `~0`/`~1`),
reported on stderr in `merge` and on stdout in `check`.

## What counts as a change

Comparison is semantic, not textual:

- **Object member order is ignored** — a formatter reordering keys is not
  an edit and can never conflict.
- **Numbers compare by mathematical value** — `1`, `1.0` and `1e0` are the
  same value (exact rational comparison; literals with exponents beyond
  ±1000 fall back to literal comparison).
- **Absent ≠ null** — deleting a key and setting it to `null` are
  different edits, and combining them is a real collision.

## Array strategies (`--arrays`)

| Strategy | Behavior |
|---|---|
| `merge` (default) | diff3: elements are aligned against the base with an LCS diff; regions only one side changed take that side; a both-edited single element recurses (arrays of objects merge field-by-field); overlapping different changes conflict as a spliced run |
| `atomic` | any array changed differently on both sides is a single conflict |
| `union` | ours as-is, then theirs' additions appended; an element removed by either side stays removed; never conflicts — for order-insensitive scalar lists |

Note the honest edge: with `merge`, both sides appending *different*
elements at the same position **is** a conflict — their relative order is
genuinely ambiguous. Use `union` for lists where order does not matter.

Arrays whose LCS table would exceed 4,000,000 cells (about 2000×2000
elements) degrade to one atomic conflict instead of burning memory.

## Conflict output format

Markers wrap whole lines exactly at the collision, so editors and merge
tools that understand git conflicts work unmodified:

```text
{
  "name": "shop-api",
<<<<<<< ours
  "version": "1.5.0",
=======
  "version": "2.0.0",
>>>>>>> theirs
  ...
}
```

- Marker length honors git's `%L` (`-m`), labels honor `--ours-label` /
  `--theirs-label`.
- A side that deleted the value contributes an empty block.
- Taking either side of every block always yields valid JSON (commas are
  placed per side).

## Formatting of the result

The merged document is re-serialized: the indent unit (2/4 spaces or
tabs), LF/CRLF flavor and trailing newline are detected from the ours
side, member order follows ours with theirs' additions inserted next to
the neighbors they had in theirs, and number literals are emitted exactly
as they were parsed. Inline (single-line) container layout is not
preserved — a compact array becomes one element per line.

## Failure behavior

Invalid JSON on any side aborts the merge with the exact line/column,
leaves the working file untouched, and exits 3 — git then marks the path
conflicted with your version intact. Duplicate object keys are treated as
invalid input, because a key-level merge over duplicates is ambiguous.

Exit codes: `0` clean, `1` conflicts, `2` usage error, `3` runtime error.
