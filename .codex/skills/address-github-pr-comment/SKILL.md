---
name: address-github-pr-comment
description: Address unreplied line comments on a GitHub pull request, including comment-by-comment triage, verified fixes, replies, and a final push.
metadata:
  short-description: Address unreplied PR line comments
---

# Address GitHub PR Comments

Use this skill when the user asks to handle review feedback on a GitHub pull request or on the single open pull request linked to an Issue. Do not use it for general PR summaries, top-level review comments, or creating a new PR.

## Resolve the pull request

The input is one positive integer in the current repository. Run `.codex/bin/github-resolve-pr-from-number.sh <number>` and use its JSON output. If it fails, stop and report its error.

The resolver first checks `gh pr view`. If the number is not a PR, it checks the Issue's `closedByPullRequestsReferences` and accepts exactly one linked open PR. It stops without Git or GitHub changes for no candidate, multiple candidates, or an API failure.

## Find candidates

Run `.codex/bin/github-list-unreplied-review-comments.sh <pr-number>`. It obtains `target_login` from the currently authenticated `gh` account and returns JSON Lines for only these review threads:

- the thread is unresolved;
- its root comment is a line comment;
- the final comment was not posted by `target_login`.

The comment need not mention `target_login`. If the script fails, stop and report its error. If it returns no lines, report that there are no unreplied line comments and end without pushing.

Never resolve a thread, even after replying.

## Handle one comment at a time

Process the returned threads sequentially. Read the complete thread, diff hunk, and relevant code before deciding.

- If the point is not valid, reply in the thread with a concise explanation.
- If it is valid but no change is needed, reply with the reason.
- If validity is unclear, stop and ask the user; do not guess.
- If it is valid but requires a specification change or scope decision, stop and ask the user before changing code or replying.
- If it is valid and needs a change, create a comment-specific subtask and do all of the following before replying:
  1. Record the implementation plan in the current session's todo file.
  2. Add the smallest red test proving the intended change.
  3. Have an independent reviewer check that the red test matches the agreed scope and can fail for the intended missing behavior. Resolve findings and keep the test red.
  4. Implement the approved change and run the relevant tests until green.
  5. Have an independent reviewer check the implementation against the agreed scope.
  6. Run the complete planned verification and inspect the diff.
  7. Commit only the scoped, green change.
  8. Reply in the review thread with what changed and how it was verified.

Reply only with `.codex/bin/github-reply-to-review-thread.sh <thread-id> <body>`. It posts solely to the existing review thread and prints the reply URL on success; it does not support PR-body or top-level comments. Do not resolve the thread. If any test, review, commit, or reply fails, stop and report the failure; do not continue to later comments.

After every candidate has been handled successfully, push the current branch once. If push fails, report the error. Do not create, merge, or resolve a pull request.
