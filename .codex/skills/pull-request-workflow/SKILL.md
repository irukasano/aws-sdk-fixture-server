---
name: pull-request-workflow
description: "Draft and create a reviewed issue-linked GitHub pull request for the current branch."
---

# Pull Request Workflow

Use this skill when the user asks to prepare or create a GitHub pull request for the current branch. Do not use it to commit implementation work.

1. Inspect the current branch and `git status --short`. Stop without pushing or creating a PR when the worktree has uncommitted changes.
2. Extract the final Issue number from a branch named `feature/*#number` or `bugs/*#number`. Stop if it cannot be determined.
3. Obtain the Issue title and Assignees with `gh issue view #{number} --json title,assignees`. Stop if this fails. An empty Assignees list is valid.
4. Read `.github/PULL_REQUEST_TEMPLATE.md`, the relevant HLD and Plan, the committed diff, and recorded verification results. Stop without pushing or creating a PR when any of those inputs is unavailable, or when the changed scope, test results, or provisional decisions and concerns cannot be determined. Draft:
   - a short change-focused PR title derived from the summary;
   - a body whose first line is `fixes #{number}`, followed by a blank line and the template's sections;
   - a summary of roughly 200 Japanese characters or fewer;
   - a bullet list of every changed component under `変更範囲`;
   - tested perspectives and commands under `テスト`;
   - `なし` under `暫定判断・懸念` only when there are no provisional decisions or concerns.
5. Obtain the repository default branch with `gh repo view --json defaultBranchRef`. Stop without pushing or creating a PR if it cannot be obtained. Display the proposed title, full body, base branch, and Issue Assignees. Wait for explicit user approval before any external mutation.
6. Only after approval, push the current branch with `git push -u origin <current-branch>`. If it fails, report the error and do not create a PR.
7. Create the PR with `gh pr create`, explicitly passing the drafted title, body, repository default branch as `--base`, and one `--assignee` value per Issue Assignee. Do not pass an Assignee flag when the Issue has none.
8. If PR creation fails after a successful push, report the failure and that the remote branch remains. On success, output the created PR URL as the final result.

Never merge a PR. Never push or create a PR before the user approves the displayed title, body, base branch, and Assignees.
