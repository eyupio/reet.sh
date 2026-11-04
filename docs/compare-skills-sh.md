# skills.sh and reet.sh: directory or control room?

skills.sh describes itself as **The Agent Skills Directory**: a public place to discover and install skills for AI agents. reet.sh is not a directory. It is a self-hosted control room for the skills you already use, the machines they run on, and the setups around them.

The short answer: use skills.sh to find skills, and use reet.sh to keep a private library, review changes, and put the right skills on the right machines.

## What each is good at

**skills.sh is good at discovery.** Its public pages make it easy to browse a large catalogue and copy an install command. If you want to see what exists, or publish a skill for other people to find, a public directory is the right shape.

**reet.sh is good at control.** It keeps a library you host, brings discovered skills back to that library, and distributes a reviewed selection to one machine or many. It also runs the agents on those machines, keeps sessions, and backs up the instance and personal shelves.

## Choose by the job

| You want to... | Start with |
| --- | --- |
| Browse a public catalogue of agent skills | skills.sh |
| Publish a skill for anyone to discover | skills.sh, or a reet.sh hosted skill page if you want the same binary to serve it |
| Keep a private skill library for a team or a fleet | reet.sh |
| Send the same skills to several machines and review the plan first | reet.sh Sync |
| See which machine has which skill, and whether a scan is stale | reet.sh Machines and Sync |
| Run Claude Code, Codex, OpenCode or Gemini CLI on a remote machine | reet.sh sessions |
| Back up skills and setups before changing a machine | reet.sh backups |

## They work together

reet.sh accepts skills.sh URLs, `owner/repo/skill` references and `npx skills add` commands. Paste one into the shelf, or use the bookmarklets on a skills.sh page, and reet fetches the details without running the command for you.

A sensible workflow is:

1. Find a skill on skills.sh.
2. Save it to your reet.sh shelf or a list.
3. Choose the machines that should have it.
4. Preview the exact plan and approve it.
5. Keep the library and the machines in step as the skill changes.

That keeps public discovery separate from the private decision about what runs where.

## Where reet.sh is deliberately different

- **You host it.** One Go binary and one SQLite file, on your hardware or a VPS you control. There is no reet-hosted control plane for your fleet.
- **Review is part of the flow.** A saved selection is not an install. The machine returns a concrete plan, and a person approves it.
- **The library is private by default.** Personal notes, machine inventories and team lists are not published with a public skill page.
- **No telemetry.** reet does not send usage data by default.
- **It is not a marketplace.** There is no ranking, paid placement or public install count to game. The community page is a deliberately published directory of lists and skills, not a popularity contest.

## Where skills.sh is the better answer

- You want the largest public catalogue to browse.
- You are publishing a skill and want discovery more than private distribution.
- You only need an install command and do not need a controller, accounts or machine management.

reet.sh is also not a replacement for the skills CLI. It works with the same skill format and install references; the CLI remains the thing that installs a skill on a machine when you are not using reet's reviewed sync.

## Try it

- [Install reet.sh](binary-installation.md)
- [Sync skills between machines](sync-skills-between-machines.md)
- [Connect your shelf to an AI client](mcp.md)
- [Public community](https://reet.sh/community)

If you only need to find a skill today, use skills.sh. If you need to know why a skill is on a machine, who approved it, and how to put the same set on the next machine, that is the job reet.sh is built for.
