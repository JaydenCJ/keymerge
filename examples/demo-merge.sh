#!/usr/bin/env bash
# Runnable demo: builds a tiny git repository where two branches edit
# package.json in ways that ALWAYS collide textually but never collide
# structurally — then merges them cleanly with keymerge installed as the
# merge driver. Offline, deterministic, safe to re-run.
#
# Usage: bash examples/demo-merge.sh [workdir]
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="${1:-$(mktemp -d)}"
REPO="$WORK/demo-repo"
rm -rf "$REPO"

export GIT_CONFIG_GLOBAL=/dev/null
export GIT_CONFIG_SYSTEM=/dev/null
export GIT_AUTHOR_NAME="Dev" GIT_AUTHOR_EMAIL="dev@example.test"
export GIT_COMMITTER_NAME="Dev" GIT_COMMITTER_EMAIL="dev@example.test"
export GIT_AUTHOR_DATE="2026-01-01T10:00:00+00:00"
export GIT_COMMITTER_DATE="2026-01-01T10:00:00+00:00"

echo "==> building keymerge"
(cd "$ROOT" && go build -o "$WORK/keymerge" ./cmd/keymerge)
export PATH="$WORK:$PATH"

echo "==> creating a repository with a package.json"
git init -q "$REPO" && git -C "$REPO" checkout -q -b main
cp "$ROOT/examples/package-json/base.json" "$REPO/package.json"
keymerge install -C "$REPO" --pattern '*.json'
git -C "$REPO" add -A && git -C "$REPO" commit -q --no-gpg-sign -m "base"

echo "==> branch 'feature' adds pino and an http keyword"
git -C "$REPO" checkout -qb feature
cp "$ROOT/examples/package-json/theirs.json" "$REPO/package.json"
git -C "$REPO" commit -aq --no-gpg-sign -m "add pino + http keyword"

echo "==> branch 'main' bumps zod and adds a lint script"
git -C "$REPO" checkout -q main
cp "$ROOT/examples/package-json/ours.json" "$REPO/package.json"
git -C "$REPO" commit -aq --no-gpg-sign -m "bump zod + add lint script"

echo "==> merging (would be a guaranteed textual conflict)"
git -C "$REPO" merge --no-edit feature

echo "==> merged package.json:"
cat "$REPO/package.json"
echo "==> done; repo left at $REPO for inspection"
