#!/usr/bin/env bash
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
base_ref="${GOTQ_API_BASE_REF:-}"

if [[ -z "$base_ref" ]]; then
  if ! base_ref="$(git -C "$root" describe --tags --abbrev=0 HEAD^ 2>/dev/null)"; then
    echo "No previous release tag; skipping public API comparison"
    exit 0
  fi
fi
if [[ ! "$base_ref" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.-]+)?$ ]]; then
  echo "::error::invalid API baseline ref: $base_ref"
  exit 1
fi
if ! command -v apidiff >/dev/null 2>&1; then
  echo "::error::apidiff is required; install the pinned version documented in CONTRIBUTING.md"
  exit 1
fi

module="$(cd "$root" && go list -m)"
temporary="$(mktemp -d "${TMPDIR:-/tmp}/gotq-apidiff.XXXXXX")"
old_worktree="$temporary/old"

cleanup() {
  git -C "$root" worktree remove --force "$old_worktree" >/dev/null 2>&1 || true
  rm -rf "$temporary"
}
trap cleanup EXIT

git -C "$root" worktree add --detach "$old_worktree" "$base_ref" >/dev/null
(
  cd "$old_worktree"
  apidiff -m -w "$temporary/old.export" "$module"
)
(
  cd "$root"
  apidiff -m -w "$temporary/new.export" "$module"
)
apidiff -m "$temporary/old.export" "$temporary/new.export" >"$temporary/actual.txt"

if [[ -s "$temporary/actual.txt" ]]; then
  cat "$temporary/actual.txt"
  echo "::error::public API differs from $base_ref"
  exit 1
fi

echo "Public API is unchanged from $base_ref"
