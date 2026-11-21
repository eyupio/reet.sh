---
title: Skills for the work in hand
description: Contextual skill suggestions for agent sessions and every Factory stage, with maintained system defaults and honest receipts.
---

# Skills for the work in hand

Gaffer's **Skills for this task** panel suggests a small set of relevant skills
in an agent chat. Suggestions use the session title and your draft; matching
runs locally on the controller without a model call. **Review skill** shows the
instructions. **Add to message** inserts editable text for you to read and send.
Native terminals show a compact suggestions panel. On a phone held sideways,
open **Suggested skills** in the toolbar to keep the terminal spacious. Adding
guidance inserts without Enter and retains the terminal's paste safeguards.

**Pause** stops suggestions for this work in this browser. **Resume** brings
them back. A failure leaves your draft alone and offers retry.

## Factory stages

Select any stage in a pipeline to see suggestions for that stage, including
stages that have not run yet. For a future pending stage, **Use for this stage**
pins the reviewed guidance to its plan. Select up to two small skills, totalling
2,000 bytes of formatted guidance. The stage's opening prompt includes them
when it starts, with the existing stage boundary and merge policy retained.
Changing a system skill later does not change instructions already selected.

A stage that has started cannot have its opening prompt rewritten. Follow its
session link and add guidance there. A completed stage remains readable.

## Integrated Unslop

**Unslop** is a built-in, selectable UX/UI skill for design, code and review work. The compact reet adaptation fits beside a task in a chat message or Factory stage prompt and needs no external plugin. It preserves credit to Damir Sokolovsky (@heydamir), [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/), and its upstream link. Administrators can maintain or disable it like other system skills. It is an adaptation, not the complete upstream toolkit and reference bundle.

## Built-ins, your library and community

- **Reet built-in:** small, original, versioned instructions that need no install.
- **Your library:** your Active saved skills, loaded from their current immutable
  revision when you review them. Long skills are available through the existing
  installed skill workflow or MCP rather than being cut into a short message.
- **Curated source:** a researched upstream candidate, with a source link. It is
  discovery only; inspect its bundle and dependencies before adding it.
- **Community:** metadata from deliberately public lists on this instance.
  Explore and add a useful skill to your own library before using it here.

Suggestions do not install plugins, execute scripts or grant permissions. A
skill's supporting scripts and lifecycle hooks need their own native setup.
Personal and project instructions continue to apply.

## Dynamic lookup from an agent

Connect the agent to reet's hosted MCP server through **Account → MCP access**.
Two tools provide progressive loading:

1. `reet_recommend_skills`: pass the task and optional Factory stage; receive
   up to four metadata matches with reasons.
2. `reet_load_skill`: fetch one selected system or Active personal skill's exact
   current instructions and revision.

The tools require an active writer account and a scoped MCP connection. They
do not execute skills, install supporting files or record external agents as
having followed the guidance. Existing agents without that MCP connection can
use the session panel's ordinary prompt text. Reet does not silently configure
an MCP connection or claim a terminal can report structured tool usage.

## Skill insights

Open **More → Skill insights** to see recorded suggestions, choices, prepared
Factory prompts and links to their sessions. Filter agent sessions and Factory
stages, see which skills you choose and read recent receipts. The page shows up
to 500 receipts. Records are cleaned up after 90 days when new activity arrives.

**Chosen for draft** records a choice, not a sent message. **Opening prompt
prepared** means reet included the guidance in a stage launch accepted by the
machine; it does not prove the agent followed it. Tokens and check results belong
to the whole session. Current insights do not claim time, money or token savings.

## Maintain system skills

An owner or administrator opens **Instance administration → System skills**.
Inspect/edit a skill, add keywords, choose its Factory stage, disable it, or add
one. Inline guidance is limited to 1,000 bytes so it fits beside the person's
task. An upstream reference is offered as a source to explore, not executable
guidance. Avoid putting private project information into instance-wide entries.

Repository defaults live in `internal/skillbuddy/defaults.json`. Instance edits
override those defaults and survive upgrades. **Restore repo default** removes
that entry's override. Saves reject stale versions rather than overwriting
another administrator's change; changes are audited. The catalogue and receipts
are part of whole-instance database backups.
