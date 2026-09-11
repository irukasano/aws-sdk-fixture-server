#!/usr/bin/env bash

set -euo pipefail

readonly program_name="${0##*/}"

fail() {
  printf '%s: %s\n' "$program_name" "$1" >&2
  exit 1
}

if [[ $# -ne 1 || ! $1 =~ ^[1-9][0-9]*$ ]]; then
  fail "usage: $program_name <issue-or-pull-request-number>"
fi

readonly number="$1"
temp_dir="$(mktemp -d)"
trap 'rm -rf "$temp_dir"' EXIT
error_file="$temp_dir/error"

if pull_request="$(gh pr view "$number" --json number,title,url,state 2>"$error_file")"; then
  jq -c '{number, title, url}' <<<"$pull_request"
  exit 0
fi

if ! rg -qi 'could not resolve to a pullrequest|pull request.*not found|no pull requests found' "$error_file"; then
  printf '%s: could not view pull request #%s: %s\n' "$program_name" "$number" "$(<"$error_file")" >&2
  exit 1
fi

if ! issue="$(gh issue view "$number" --json closedByPullRequestsReferences 2>"$error_file")"; then
  printf '%s: could not view issue #%s: %s\n' "$program_name" "$number" "$(<"$error_file")" >&2
  exit 1
fi

mapfile -t candidates < <(
  jq -c '.closedByPullRequestsReferences[] | {number, title, url}' <<<"$issue"
)

case ${#candidates[@]} in
  1)
    candidate_number="$(jq -r '.number' <<<"${candidates[0]}")"
    if ! pull_request="$(gh pr view "$candidate_number" --json number,title,url,state 2>"$error_file")"; then
      printf '%s: could not view linked pull request #%s: %s\n' "$program_name" "$candidate_number" "$(<"$error_file")" >&2
      exit 1
    fi
    jq -c '{number, title, url}' <<<"$pull_request"
    ;;
  0)
    fail "issue #$number has no linked open pull request"
    ;;
  *)
    printf '%s: issue #%s has multiple linked open pull requests:\n' "$program_name" "$number" >&2
    printf '%s\n' "${candidates[@]}" >&2
    exit 1
    ;;
esac
