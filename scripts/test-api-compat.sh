#!/usr/bin/env bash
# Integration tests against real Git tags and the pinned apidiff executable.
set -euo pipefail
script_dir="$(cd "$(dirname "$0")" && pwd)"
temporary="$(mktemp -d "${TMPDIR:-/tmp}/gotq-api-gate-test.XXXXXX")"
trap 'rm -rf "$temporary"' EXIT
export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null
export GIT_AUTHOR_NAME='API gate test' GIT_AUTHOR_EMAIL='api-test@example.invalid'
export GIT_COMMITTER_NAME="$GIT_AUTHOR_NAME" GIT_COMMITTER_EMAIL="$GIT_AUTHOR_EMAIL"
export GOWORK=off
unset GOTQ_API_BASE_REF
mkdir "$temporary/repo"
cd "$temporary/repo"
git init -q
printf 'module example.com/api\n\ngo 1.25.0\n' >go.mod
printf 'package api\nfunc Existing() int { return 1 }\n' >api.go
mkdir child
printf 'package child\nfunc Existing() int { return 1 }\n' >child/child.go
git add .
git commit -qm initial
git tag v1.0.0
git commit --allow-empty -qm development

expect_success() {
  if ! "$script_dir/check-api-compat.sh" "$@" >"$temporary/output" 2>&1; then
    cat "$temporary/output"; exit 1
  fi
}
expect_failure() {
  local diagnostic="$1"
  shift
  if "$script_dir/check-api-compat.sh" "$@" >"$temporary/output" 2>&1; then
    echo "expected gate failure: $diagnostic"; cat "$temporary/output"; exit 1
  fi
  if ! grep -Fq "$diagnostic" "$temporary/output"; then
    cat "$temporary/output"; exit 1
  fi
}
expect_success
printf '\nfunc Added() {}\n' >>api.go
expect_failure 'need a review record'
expect_success --write-review
expect_success
printf '\nfunc Unreviewed() {}\n' >>child/child.go
expect_failure 'does not match the reviewed changes'
printf 'package child\nfunc Existing() int { return 1 }\n' >child/child.go
# A forged review file must not authorize an incompatible signature.
printf 'package api\nfunc Existing() string { return "" }\nfunc Added() {}\n' >api.go
printf 'forged approval\n' >docs/api-diffs/v1.0.0.txt
expect_failure 'incompatible public API changes'
expect_failure 'incompatible public API changes' --write-review
printf 'package api\nfunc Existing() int { return 1 }\nfunc Added() {}\n' >api.go
expect_success --write-review
# Removal in a subpackage must also be rejected.
printf 'package child\n' >child/child.go
expect_failure 'incompatible public API changes'
printf 'package child\nfunc Existing() int { return 1 }\n' >child/child.go
# Baselines are tied to commit IDs, not only mutable tag names.
git add api.go docs
git commit -qm addition
git tag -f v1.0.0 >/dev/null
GOTQ_API_BASE_REF=v1.0.0 expect_failure 'does not match the reviewed changes'
GOTQ_API_BASE_REF=../../bad expect_failure 'invalid API baseline ref'
GOTQ_API_BASE_REF=v9.9.9 expect_failure ''
printf 'API compatibility gate integration tests passed\n'
