# Agent-Executable Task Schema Extension

## Purpose

Extend `persephone_tasks` documents so that an agent receiving **only a `_key`** can query
the database and have everything needed to execute the task autonomously — no additional
prompting, no context from a parent conversation.

## Design Principles

1. **Self-contained**: Every input the agent needs is in the document
2. **Declarative procedure**: Steps are ordered, machine-parseable, human-readable
3. **State-aware**: Agent can resume from where a previous agent left off
4. **Schema-compatible**: Uses ArangoDB's schemaless extension — core Persephone fields untouched

## Schema Extension: `work_order`

All agent-executable fields live under a single nested object: `work_order`.
This keeps the extension cleanly separated from core Persephone fields.

```json
{
  "_key": "task_cr_01",
  "title": "PR #1: Foundation",
  "status": "in_review",
  "type": "task",
  "parent_key": "task_cr_epic",

  "work_order": {
    "version": "1.0",

    "objective": "Create a GitHub PR for code review of the foundation packages",

    "context": {
      "repo": "toddwbucy/Hermes",
      "repo_path": "/home/todd/olympus/Hermes",
      "base_branch": "code-review-base",
      "base_commit": "0510fe2",
      "source_branch": "main",
      "worktree_root": "/tmp/hermes-review"
    },

    "inputs": {
      "branch_name": "review/01-foundation",
      "pr_number": 1,
      "pr_title": "PR #1: Foundation — types, config, infrastructure",
      "packages": ["adapter (base)", "plugin", "config", "state", "event", ...],
      "file_manifest": ["internal/adapter/adapter.go", ...],
      "file_count": 37,
      "line_count": 5328
    },

    "dependencies": {
      "blocked_by": [],
      "blocks": ["task_cr_04", "task_cr_05", "task_cr_06", "task_cr_07"]
    },

    "procedure": [
      {
        "step": 1,
        "action": "create_worktree",
        "command": "git worktree add /tmp/hermes-review/pr-01-foundation code-review-base",
        "description": "Create isolated worktree from code-review-base"
      },
      {
        "step": 2,
        "action": "create_branch",
        "command": "cd /tmp/hermes-review/pr-01-foundation && git checkout -b review/01-foundation",
        "description": "Create feature branch for this PR"
      },
      {
        "step": 3,
        "action": "checkout_files",
        "command": "git checkout main -- <file_manifest>",
        "description": "Copy files from main into the worktree"
      },
      {
        "step": 4,
        "action": "commit",
        "command": "git add -A && git commit -m '<pr_title>'",
        "description": "Commit all files with descriptive message"
      },
      {
        "step": 5,
        "action": "push",
        "command": "git push -u origin review/01-foundation",
        "description": "Push branch to remote"
      },
      {
        "step": 6,
        "action": "create_pr",
        "command": "gh pr create --base code-review-base --title '<pr_title>' --body '<body>'",
        "description": "Create PR targeting code-review-base"
      },
      {
        "step": 7,
        "action": "wait_for_review",
        "duration_minutes": 10,
        "description": "Wait for automated code review bot"
      },
      {
        "step": 8,
        "action": "check_review",
        "command": "gh api repos/toddwbucy/Hermes/pulls/<pr_number>/comments",
        "description": "Check for review comments"
      },
      {
        "step": 9,
        "action": "conditional_loop",
        "condition": "review_comments_exist",
        "loop_to": 7,
        "fix_action": "Read comments, fix issues, commit, push",
        "description": "If comments exist, fix and re-check; else proceed"
      },
      {
        "step": 10,
        "action": "cleanup_worktree",
        "command": "git worktree remove /tmp/hermes-review/pr-01-foundation",
        "description": "Remove worktree after PR passes review"
      },
      {
        "step": 11,
        "action": "update_task",
        "description": "Update this task status to in_review, record PR URL and review result"
      }
    ],

    "success_criteria": [
      "PR created and targeting code-review-base",
      "Automated review passes with no unresolved comments",
      "Worktree cleaned up",
      "Task status updated to in_review"
    ],

    "state": {
      "current_step": 11,
      "completed_steps": [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11],
      "review_iterations": 0,
      "pr_url": "https://github.com/toddwbucy/Hermes/pull/1",
      "worktree_path": null,
      "notes": ["PR created successfully", "Worktree cleaned"]
    }
  }
}
```

## Field Reference

### `work_order.version`
Schema version for forward compatibility. Currently `"1.0"`.

### `work_order.objective`
One-sentence human-readable description of what the agent should accomplish.

### `work_order.context`
Shared environment context. For a code review task this includes repo info, base branches,
and worktree location. Other task types would have different context shapes.

### `work_order.inputs`
All task-specific data the agent needs. Shape varies by task type.

### `work_order.dependencies`
Task-level dependencies expressed as Persephone task keys.
- `blocked_by`: Tasks that must complete before this one can start
- `blocks`: Tasks that are waiting on this one

### `work_order.procedure`
Ordered list of steps. Each step has:
- `step`: Sequence number
- `action`: Machine-parseable action type
- `command` (optional): Exact shell command template
- `description`: Human-readable explanation
- `condition` / `loop_to` (optional): For conditional/looping steps
- `duration_minutes` (optional): For wait steps

### `work_order.success_criteria`
List of conditions that define task completion. Agent checks these before marking done.

### `work_order.state`
Mutable state updated by the agent as it works:
- `current_step`: Last completed step number
- `completed_steps`: Array of completed step numbers
- `review_iterations`: How many fix cycles
- `pr_url`: Output artifact
- `worktree_path`: Active worktree (null when cleaned)
- `notes`: Agent log entries

## Agent Query Pattern

```python
# Agent receives: task_key = "task_cr_01"
# Agent queries:
task = db.aql("""
  FOR doc IN persephone_tasks
    FILTER doc._key == @key
    RETURN doc
""", bind_vars={"key": task_key})

work_order = task["work_order"]
current_step = work_order["state"]["current_step"]
# Resume from current_step + 1
```

## Agent State Update Pattern

```python
# After completing step 6:
db.aql("""
  UPDATE @key WITH {
    work_order: {
      state: {
        current_step: 6,
        completed_steps: APPEND(doc.work_order.state.completed_steps, [6]),
        pr_url: @pr_url
      }
    },
    updated_at: DATE_ISO8601(DATE_NOW())
  } IN persephone_tasks
""", bind_vars={"key": task_key, "pr_url": url})
```

## Extensibility

The `work_order` object is designed for compositional growth:
- Different task types have different `inputs` shapes
- `procedure` steps can be extended with new `action` types
- `context` can be extended for non-code tasks (e.g., database migrations, deployments)
- `state` tracks whatever the agent needs for resumability
