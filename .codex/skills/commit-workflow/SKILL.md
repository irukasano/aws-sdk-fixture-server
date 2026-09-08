---
name: commit-workflow
description: Create a scoped Git commit from approved Plan and HLD work, adding an Issue reference derived from feature/*#number or bugs/*#number branch names. Use when a user requests a commit or invokes the implementation workflow.
---

# Commit Workflow

Use this workflow when the user has requested a commit, or when `$implementation-workflow` invokes it after the user explicitly requested that implementation workflow. The latter invocation authorizes this workflow's commit.

1. Inspect the current branch, `git status`, and the unstaged and staged diffs. Identify only the files belonging to the approved Plan and HLD scope. Do not stage, modify, or commit unrelated work; if the boundary cannot be determined, stop and ask for direction.
2. Read the relevant HLD and Plan together with the scoped diff. Generate a concise commit summary that describes the implemented planned behavior, rather than only listing filenames.
3. If the branch name matches `feature/*#number` or `bugs/*#number`, take the final `number` after `#` and prefix the commit subject with `refs #number `. Otherwise, use the generated summary without an Issue prefix.
4. Stage only the scoped files, verify the staged diff and `git diff --cached --check`, then commit with the generated subject. Do not push.
5. Report the commit hash, subject, and whether the worktree contains unrelated remaining changes.

Never create an empty commit. Stop before committing if tests required by the approved Plan are not green, the staged diff includes work outside the approved scope, or neither the user nor an explicitly requested `$implementation-workflow` has authorized a commit.
