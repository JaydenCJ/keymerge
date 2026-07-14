#!/usr/bin/env bash
# End-to-end smoke test for keymerge: builds the binary, merges the shipped
# fixtures directly, then drives a real `git merge` with keymerge installed
# as the merge driver — the clean case AND the real-collision case. No
# network, idempotent, finishes in seconds.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

fail() {
  echo "SMOKE FAIL: $*" >&2
  exit 1
}

BIN="$WORKDIR/keymerge"
REPO="$WORKDIR/repo"
FIX="$ROOT/examples/package-json"
CFIX="$ROOT/examples/conflict"

# Isolate git completely from the host user's configuration.
export GIT_CONFIG_GLOBAL=/dev/null
export GIT_CONFIG_SYSTEM=/dev/null
export GIT_AUTHOR_NAME="Dev"
export GIT_AUTHOR_EMAIL="dev@example.test"
export GIT_COMMITTER_NAME="Dev"
export GIT_COMMITTER_EMAIL="dev@example.test"
export GIT_AUTHOR_DATE="2026-01-01T10:00:00+00:00"
export GIT_COMMITTER_DATE="2026-01-01T10:00:00+00:00"
export PATH="$WORKDIR:$PATH"

echo "1. build"
(cd "$ROOT" && go build -o "$BIN" ./cmd/keymerge) || fail "go build failed"

echo "2. version matches manifest"
VER="$("$BIN" version)" || fail "version exited nonzero"
[ "$VER" = "keymerge 0.1.0" ] || fail "version mismatch"

echo "3. direct merge of the shipped fixtures is clean"
OUT="$("$BIN" merge "$FIX/base.json" "$FIX/ours.json" "$FIX/theirs.json" -o -)" \
  || fail "fixture merge should exit 0"
echo "$OUT" | grep -q '"zod": "\^3.24.1"'  || fail "ours' zod bump missing"
echo "$OUT" | grep -q '"pino": "\^9.0.0"'  || fail "theirs' pino addition missing"
echo "$OUT" | grep -q '"lint": "eslint ."' || fail "ours' lint script missing"
echo "$OUT" | grep -q '"http"'             || fail "theirs' keyword missing"

echo "4. check reports the real collision with a pointer path"
set +e
CHK="$("$BIN" check "$CFIX/base.json" "$CFIX/ours.json" "$CFIX/theirs.json")"
[ $? -eq 1 ] || { set -e; fail "check should exit 1 on conflicts"; }
set -e
echo "$CHK" | grep -q "/version" || fail "conflict path /version missing"
echo "$CHK" | grep -q "edit/edit" || fail "conflict kind missing"

echo "5. git repo with the driver installed via 'keymerge install'"
git init -q "$REPO"
git -C "$REPO" checkout -q -b main
cp "$FIX/base.json" "$REPO/package.json"
INS="$("$BIN" install -C "$REPO" --pattern '*.json')" || fail "install exited nonzero"
echo "$INS" | grep -q '.gitattributes: added' || fail "install did not write .gitattributes"
git -C "$REPO" add -A
git -C "$REPO" commit -q --no-gpg-sign -m "base"

echo "6. textual conflict, structural non-conflict: git merges cleanly"
git -C "$REPO" checkout -qb feature
cp "$FIX/theirs.json" "$REPO/package.json"
git -C "$REPO" commit -aq --no-gpg-sign -m "add pino and http keyword"
git -C "$REPO" checkout -q main
cp "$FIX/ours.json" "$REPO/package.json"
git -C "$REPO" commit -aq --no-gpg-sign -m "bump zod, add lint script"
git -C "$REPO" merge -q --no-edit feature || fail "git merge should be clean"
grep -q '"pino": "\^9.0.0"' "$REPO/package.json" || fail "merged file lost pino"
grep -q '"zod": "\^3.24.1"' "$REPO/package.json" || fail "merged file lost zod bump"
"$BIN" check /dev/null "$REPO/package.json" "$REPO/package.json" >/dev/null \
  || fail "merged package.json is not valid JSON"

echo "7. real collision: git stops, markers land on the exact key"
git -C "$REPO" checkout -qb bump-a
sed -i.bak 's/"1.4.0"/"1.5.0"/' "$REPO/package.json" && rm -f "$REPO/package.json.bak"
git -C "$REPO" commit -aq --no-gpg-sign -m "bump to 1.5.0"
git -C "$REPO" checkout -q main
git -C "$REPO" checkout -qb bump-b
sed -i.bak 's/"1.4.0"/"2.0.0"/' "$REPO/package.json" && rm -f "$REPO/package.json.bak"
git -C "$REPO" commit -aq --no-gpg-sign -m "bump to 2.0.0"
git -C "$REPO" checkout -q bump-a
if git -C "$REPO" merge -q --no-edit bump-b 2>/dev/null; then
  fail "colliding version bumps must not merge"
fi
grep -q '^<<<<<<< ours$' "$REPO/package.json" || fail "conflict markers missing"
grep -q '"version": "1.5.0",' "$REPO/package.json" || fail "ours candidate missing"
grep -q '"version": "2.0.0",' "$REPO/package.json" || fail "theirs candidate missing"
git -C "$REPO" merge --abort

echo "8. usage errors exit 2"
set +e
"$BIN" merge only two >/dev/null 2>&1
[ $? -eq 2 ] || { set -e; fail "wrong arg count should exit 2"; }
set -e

echo "SMOKE OK"
