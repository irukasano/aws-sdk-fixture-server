#!/usr/bin/env bash

set -euo pipefail

readonly program_name="${0##*/}"

fail() {
  printf '%s: %s\n' "$program_name" "$1" >&2
  exit 1
}

if [[ $# -ne 1 || ! $1 =~ ^[1-9][0-9]*$ ]]; then
  fail "usage: $program_name <pull-request-number>"
fi

readonly pull_request_number="$1"

if ! gh auth status >/dev/null 2>&1; then
  fail "GitHub CLI is not authenticated"
fi

target_login="$(gh api user --jq '.login')" || fail "could not determine the current GitHub login"
[[ -n $target_login ]] || fail "could not determine the current GitHub login"

repository="$(gh repo view --json nameWithOwner --jq '.nameWithOwner')" || fail "could not determine the current repository"
[[ $repository == */* ]] || fail "could not determine the current repository"
readonly owner="${repository%%/*}"
readonly name="${repository#*/}"

readonly review_threads_query='query($owner: String!, $name: String!, $number: Int!, $cursor: String) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      reviewThreads(first: 100, after: $cursor) {
        nodes {
          id
          isResolved
          comments(first: 100) {
            nodes {
              id
              author { login }
              body
              url
              path
              line
              originalLine
              diffHunk
              createdAt
            }
            pageInfo { hasNextPage endCursor }
          }
        }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
}'

readonly thread_comments_query='query($threadId: ID!, $cursor: String) {
  node(id: $threadId) {
    ... on PullRequestReviewThread {
      comments(first: 100, after: $cursor) {
        nodes {
          id
          author { login }
          body
          url
          path
          line
          originalLine
          diffHunk
          createdAt
        }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
}'

temp_dir="$(mktemp -d)"
trap 'rm -rf "$temp_dir"' EXIT
threads_file="$temp_dir/threads.jsonl"

cursor=""
while :; do
  if [[ -n $cursor ]]; then
    response="$(gh api graphql -F owner="$owner" -F name="$name" -F number="$pull_request_number" -F cursor="$cursor" -f query="$review_threads_query")" || fail "could not retrieve review threads"
  else
    response="$(gh api graphql -F owner="$owner" -F name="$name" -F number="$pull_request_number" -f query="$review_threads_query")" || fail "could not retrieve review threads"
  fi

  if ! jq -e '.data.repository.pullRequest.reviewThreads' >/dev/null <<<"$response"; then
    fail "pull request #$pull_request_number was not found or review threads could not be retrieved"
  fi

  jq -c '.data.repository.pullRequest.reviewThreads.nodes[]' <<<"$response" >>"$threads_file"
  has_next_page="$(jq -r '.data.repository.pullRequest.reviewThreads.pageInfo.hasNextPage' <<<"$response")"
  cursor="$(jq -r '.data.repository.pullRequest.reviewThreads.pageInfo.endCursor // empty' <<<"$response")"
  [[ $has_next_page == true ]] || break
  [[ -n $cursor ]] || fail "review thread pagination returned no cursor"
done

while IFS= read -r thread; do
  thread_id="$(jq -r '.id' <<<"$thread")"
  comments="$(jq -c '.comments.nodes' <<<"$thread")"
  has_next_page="$(jq -r '.comments.pageInfo.hasNextPage' <<<"$thread")"
  cursor="$(jq -r '.comments.pageInfo.endCursor // empty' <<<"$thread")"

  while [[ $has_next_page == true ]]; do
    [[ -n $cursor ]] || fail "comment pagination returned no cursor for review thread $thread_id"
    response="$(gh api graphql -F threadId="$thread_id" -F cursor="$cursor" -f query="$thread_comments_query")" || fail "could not retrieve comments for review thread $thread_id"
    if ! jq -e '.data.node.comments' >/dev/null <<<"$response"; then
      fail "could not retrieve comments for review thread $thread_id"
    fi
    next_comments="$(jq -c '.data.node.comments.nodes' <<<"$response")"
    comments="$(jq -cn --argjson current "$comments" --argjson next "$next_comments" '$current + $next')"
    has_next_page="$(jq -r '.data.node.comments.pageInfo.hasNextPage' <<<"$response")"
    cursor="$(jq -r '.data.node.comments.pageInfo.endCursor // empty' <<<"$response")"
  done

  jq -cn \
    --argjson thread "$thread" \
    --argjson comments "$comments" \
    --arg pull_request_number "$pull_request_number" \
    --arg target_login "$target_login" '
      $comments[0] as $root
      | $comments[-1] as $last
      | select($thread.isResolved == false)
      | select($root.originalLine != null)
      | select(($last.author.login // "") != $target_login)
      | {
          thread_id: $thread.id,
          pull_request_number: ($pull_request_number | tonumber),
          path: $root.path,
          line: ($root.line // $root.originalLine),
          original_line: $root.originalLine,
          diff_hunk: $root.diffHunk,
          comments: $comments,
          last_comment: $last
        }'
done <"$threads_file"
