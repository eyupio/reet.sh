---
title: "Run parallel AI coding agents with Git worktrees · reet.sh"
description: "Give concurrent coding agent sessions separate Git worktrees, inspect their changes and remove worktrees deliberately after reviewing the result."
layout: article
topic: WORKFLOWS
category: remote
art: worktrees
art_icon: branch
art_label: ROOM TO WORK
card_title: "Give each coding agent its own worktree."
hero_title: "More than one agent."
hero_accent: "Room for each to work."
related:
  - projects.md
  - terminal-worktrees.md
---

# Run parallel AI coding agents with Git worktrees

Two agent sessions can run in different terminal tabs and still edit the same files. A separate terminal changes how you reach the process; a separate Git worktree gives that process its own checkout and branch.

Reet can request a fresh worktree when launching an agent on a machine that supports it. That lets you keep tasks apart while using the same underlying repository.

## When a worktree helps

Use separate worktrees for tasks that can be reviewed independently: a docs improvement, a bug fix and an unrelated test change, for example. Each agent sees a working directory for its own task rather than changes appearing underneath it from a second agent.

A worktree is not a separate machine or a security boundary. Processes still share the OS account, installed tools, network access and other resources on that host. A project may also refer to files or services outside the checkout.

## Prepare a folder project

A Reet project represents one folder on one machine. Add the repository to the project rail, choose the agent you expect to use, and review its launch defaults.

The defaults can include a base branch, whether to start sessions in worktrees, a model and supported effort setting, and a starter prompt. A saved default is useful context, but review it before starting a different kind of task.

See [Projects](projects.md) for the complete controls and the relationship between a machine, folder and project.

## Start with a fresh worktree

For a terminal agent launch on a capable machine:

1. Choose the folder project.
2. Choose the coding agent and opening prompt.
3. Select a **fresh worktree** and review the base branch shown in the launch settings.
4. Press **Start**.

The machine creates or adopts the requested worktree before starting the agent there. Worktree support is negotiated with the machine; a connector that cannot safely handle the request refuses it rather than silently starting in the shared checkout.

The base follows the launch and project settings. Without an explicitly selected base, the machine uses the checkout’s current commit. Check that commit before running several tasks that need the same starting point.

!!! note "Gaffer uses the project folder"
    OpenCode, Cline or AGY in Gaffer starts in the chosen project folder. Its launch path does not offer the same worktree option as a terminal launch. Choose the supported terminal mode when a fresh worktree is required; see [Start an agent session](start-agent-session.md).

## Keep task boundaries visible

Give each session a prompt with a clear outcome and a limited scope. For example, ask one agent to improve the installation guide and another to fix a specific failing test, rather than asking both to make broad changes to the same module.

Check the session’s folder and branch before editing. A session title is a useful label, but the actual working directory and Git branch determine where changes land.

Separate worktrees reduce accidental overlap inside the checkout. They do not make two changes compatible when you integrate them later. Review overlapping dependencies and file changes in the normal way.

## Review each result

Use the session workspace’s **Changes** view to inspect the worktree diff. Run the repository’s relevant checks in that worktree, then review how the change should reach the main development branch.

A clean diff can still be incorrect, and an exited agent does not prove a successful task. Keep code review and project checks attached to each result.

If a later task depends on an earlier result, sequence the work deliberately. A worktree created before the earlier result landed will not automatically acquire that change.

## Remove worktrees deliberately

Reet does not remove worktrees automatically and never deletes their branches. Before removing one, inspect the working tree for changes and check whether its commits have been pushed or otherwise preserved.

The removal flow reports changed or unpushed work and requires you to make the relevant choice. Do not treat closing a terminal as preserving or integrating its work.

The [worktree reference](terminal-worktrees.md) documents the machine-side path rules, status checks, removal conditions and negotiated capabilities.

## A useful first trial

Start with two small tasks in two worktrees on one machine. Confirm that each session has a distinct directory and branch, inspect the two diffs, and run the relevant checks. Integrate or retain the results before cleaning up.

That gives you a concrete view of the workflow before you spread larger tasks across more projects and machines.
