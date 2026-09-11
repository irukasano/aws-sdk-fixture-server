#!/usr/bin/env bash

set -euo pipefail

readonly program_name="${0##*/}"

fail() {
  printf '%s: %s\n' "$program_name" "$1" >&2
  exit 1
}

if [[ $# -ne 2 || -z $1 || -z $2 ]]; then
  fail "usage: $program_name <review-thread-id> <body>"
fi

readonly thread_id="$1"
readonly body="$2"

if ! gh auth status >/dev/null 2>&1; then
  fail "GitHub CLI is not authenticated"
fi

readonly reply_query='mutation($threadId: ID!, $body: String!) {
  addPullRequestReviewThreadReply(input: {
    pullRequestReviewThreadId: $threadId,
    body: $body
  }) {
    comment { url }
  }
}'

response="$(gh api graphql -F threadId="$thread_id" -f body="$body" -f query="$reply_query")" || fail "could not reply to review thread $thread_id"

if ! reply_url="$(jq -er '.data.addPullRequestReviewThreadReply.comment.url' <<<"$response")"; then
  fail "review thread reply did not return a URL"
fi
[[ -n $reply_url ]] || fail "review thread reply did not return a URL"

printf '%s\n' "$reply_url"
