---
name: implementation-workflow
description: Implement an approved HLD and Plan with red-first tests, an independent HLD-alignment review, green verification, and the repository commit workflow. Use for planned implementation tasks, not design-only work.
---

# Implementation Workflow

Use this workflow only after the relevant HLD and Plan have explicit user approval.

1. Delegate a subagent to add the smallest tests that demonstrate the approved behavior. The subagent must not implement production behavior. Run the relevant tests and retain the command and failing output as proof that they are red.
2. Delegate a different subagent to review the red tests against the approved HLD and Plan. It must check coverage, expected behavior, non-goals, and whether the test can fail for the intended missing behavior. Resolve any findings and rerun the tests until the reviewer approves and they remain red.
3. Implement only the approved Plan scope. Run the relevant tests after each meaningful change and iterate until they are green.
4. Run the Plan's complete verification set, inspect its output, and review the diff against the approved HLD and Plan. Do not treat the task as complete if any required check is skipped or failing.
5. Invoke `$commit-workflow` to prepare and create the implementation commit. It must preserve its own scope checks and commit authorization requirements.

Stop rather than continue to implementation when a red test cannot be reproduced, the independent review finds HLD or Plan divergence, a new design decision is required, or the required verification cannot become green. Report the evidence and obtain updated direction first.
