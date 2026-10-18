#!/usr/bin/env bash
set -euo pipefail

write_review=false
case "${1:-}" in
  '') ;;
  --write-review) write_review=true ;;
  *) echo "usage: $0 [--write-review]" >&2; exit 2 ;;
esac
if (( $# > 1 )); then
  echo "usage: $0 [--write-review]" >&2
  exit 2
fi

root="$(git rev-parse --show-toplevel)"
base_ref="${GOTQ_API_BASE_REF:-}"
if [[ "$(git -C "$root" rev-parse --is-shallow-repository)" == true ]]; then
  echo "::error::API comparison requires full history and release tags (fetch-depth: 0)"
  exit 1
fi
if [[ -z "$base_ref" ]]; then
  if ! base_ref="$(git -C "$root" describe --tags --match 'v[0-9]*' --abbrev=0 HEAD^ 2>/dev/null)"; then
    echo "No previous release tag; skipping public API comparison"
    exit 0
  fi
fi
if [[ ! "$base_ref" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.-]+)?$ ]]; then
  echo "::error::invalid API baseline ref: $base_ref"
  exit 1
fi
base_commit="$(git -C "$root" rev-parse --verify "refs/tags/$base_ref^{commit}")"
if ! command -v apidiff >/dev/null 2>&1; then
  echo "::error::apidiff is required; install the pinned version documented in CONTRIBUTING.md"
  exit 1
fi

module="$(cd "$root" && go list -m)"
temporary="$(mktemp -d "${TMPDIR:-/tmp}/gotq-apidiff.XXXXXX")"
trap 'rm -rf "$temporary"' EXIT
mkdir "$temporary/old"
# An archive keeps the comparison read-only with respect to the caller's Git
# worktrees, index, and refs. Export the current tree including uncommitted edits.
git -C "$root" archive "$base_commit" | tar -x -C "$temporary/old"
(cd "$temporary/old" && apidiff -m -w "$temporary/old.export" "$module")
(cd "$root" && apidiff -m -w "$temporary/new.export" "$module")

# A review record can never authorize an incompatible change in the v1 line.
apidiff -m -incompatible "$temporary/old.export" "$temporary/new.export" >"$temporary/incompatible.txt"
if [[ -s "$temporary/incompatible.txt" ]]; then
  cat "$temporary/incompatible.txt"
  echo "::error::incompatible public API changes from $base_ref are forbidden"
  exit 1
fi
apidiff -m "$temporary/old.export" "$temporary/new.export" >"$temporary/diff.txt"
{
  printf 'Baseline: %s (%s)\n' "$base_ref" "$base_commit"
  cat "$temporary/diff.txt"
} >"$temporary/review.txt"
expected="$root/docs/api-diffs/$base_ref.txt"

if [[ "$write_review" == true ]]; then
  mkdir -p "$(dirname "$expected")"
  cp "$temporary/review.txt" "$expected"
  echo "Wrote $expected; review and commit the exact compatible API delta"
  exit 0
fi
if [[ ! -f "$expected" && ! -s "$temporary/diff.txt" ]]; then
  echo "Public API is unchanged from $base_ref"
  exit 0
fi
if [[ ! -f "$expected" ]]; then
  cat "$temporary/diff.txt"
  echo "::error::compatible public API changes need a review record: scripts/check-api-compat.sh --write-review"
  exit 1
fi
if ! diff -u "$expected" "$temporary/review.txt"; then
  echo "::error::public API delta does not match the reviewed changes from $base_ref"
  exit 1
fi
echo "Public API is compatible with $base_ref and matches the reviewed delta"
