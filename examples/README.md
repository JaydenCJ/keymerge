# keymerge examples

Runnable, offline and self-contained.

## demo-merge.sh

Builds a small git repository where two branches make the classic
package.json edits — one bumps a dependency and adds a script, the other
adds a new dependency and a keyword. Line-based merging always conflicts
here (the edits touch adjacent lines); keymerge merges it cleanly through
a real `git merge`.

```bash
bash examples/demo-merge.sh
```

## package-json/ — a fake conflict, merged

The three-way fixture behind the demo. Merge it directly, without git:

```bash
keymerge merge package-json/base.json package-json/ours.json package-json/theirs.json -o -
```

Exit code 0: every edit survives, key order and style stay intact.

## conflict/ — a real collision, surfaced precisely

Both sides bumped `version` and rewrote the same `start` script. That is
a genuine disagreement, so keymerge writes conflict markers around those
two members only — everything else merges:

```bash
keymerge check conflict/base.json conflict/ours.json conflict/theirs.json
keymerge merge conflict/base.json conflict/ours.json conflict/theirs.json -o -
```

`check` lists the colliding paths (`/version`, `/scripts/start`) and exits
1 without writing anything — handy in scripts and pre-merge sanity checks.

Like scripts/smoke.sh, the demo pins author dates and isolates git
configuration, so its output is identical on every machine.
