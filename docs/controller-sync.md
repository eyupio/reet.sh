# Reet.sh skills and enrolled machines

Every connected machine follows the account's library: Active skills are installed, Archived skills are kept off machines, and Deleted skills are removed. The controller sends saved revisions and digests; the connector reconciles the supported home folders and reports each result. See [the everyday workflow](sync-skills-between-machines.md).

## Connect a machine

Use **Machines → Connect a machine**. Install the binary, make an enrolment code and copy the connection command shown by your instance. The code works once, for 15 minutes, and only to enrol a machine. Enrolment asks for it at a hidden prompt, or takes `--code` or `REET_ENROL_CODE`:

For an installation with `REET_URL` and `REET_TOKEN` already set privately:

```sh
reet agent enroll --env work-laptop --allow-apply --code reet_enrol_…
reet agent run
```

Enrollment binds the installation to its controller, actual hostname and home, and saves private credentials locally: the machine's id and its own secret, never a personal API token. After enrolment the agent authenticates with that secret alone, and it opens only the routes the agent uses (its poll, results, update stages, skills report, content observations, skill imports, private skill downloads, inventory scan and the private control connection); removing the machine under Machines revokes it.

A personal token from `--token`, `REET_TOKEN`, an env file or the saved token still enrols, with a warning. Against a controller that takes the machine's secret alone the token is not kept; against an older controller it is kept so the machine goes on working. Machines enrolled with a personal token before keep working, and the Machines page marks a machine whose last request used one. To move such a machine to its own secret, remove it under Machines, make an enrolment code, and on the machine run:

```sh
rm ~/.config/reet/agent.json && reet agent enroll --server https://your-site --env work-laptop --allow-apply --code reet_enrol_…
```

Then revoke the old personal token from **Account → API tokens** in a signed-in browser (tokens cannot be listed, made or revoked with a token). Do not copy its configuration to another machine. A running agent polls the controller and refreshes inventory; it does not start another web server. Use the installer or your process supervisor to keep it running after logout.

## Allow local writes

The machine must consent to file changes. `--allow-apply` records that consent at enrollment; to change it later, run on the machine:

```sh
reet agent allow-apply status
reet agent allow-apply on
# Stop managed writes locally:
reet agent allow-apply off
```

The web UI cannot grant this permission. A running connector reads changes to its local consent. **Pause sync** in a machine's settings is the controller-side way to leave it alone temporarily.

## Check progress and retry

Open **Sync** for the fleet sentence, each machine and recent activity. **Retry** clears a failed machine's wait so it tries on its next poll. Offline, paused, unsupported and failed states remain visible rather than being counted as successful.

```sh
reet agent status
reet doctor
```

If a machine needs an update, use its update control or the normal binary upgrade instructions. A connector below skills protocol 2 cannot receive skill work from the new controller. `reet scan` remains an inventory command; it does not select skills for the library.

## Recover a machine copy

Replacements and removals are backed up under a dated `.reet-backups` directory beside each managed skills folder. Backups are retained for 30 days. List them on the machine:

```sh
reet agent restore --list
reet agent restore skill-name
reet agent restore skill-name --date 2026-11-19
```

An older job backup can also be restored with `reet agent restore --backup PATH`, using the exact path from the listing. Use the same `--config PATH` when an installation has a custom enrollment file.

A local restore does not change the library. If sync remains enabled, the next reconcile follows reet again. To keep an edit across machines, adopt it with **Use this version** in the opened skill; to recover an earlier library revision, use **History → Put back**. Pause sync before investigating a local backup if you need it left in place.

## What replaced plans and previews

There are no per-machine skill plans to create or approve. Add to the library, Archive, Restore and Delete express the desired state. Lists and tags organise it. Unknown skills require **Keep** or **Archive** before they join that state.

The former per-machine pins, ownership adoption and preview APIs are retired. Historical design contracts are not instructions for the current controller. The old `reet sync` and `reet agent library-import` commands explain their replacement without changing files.

## Follow a controller that has moved

When a controller moves (see [backups](backups.md#moving-a-controller-the-order-of-it)), machines follow it on their own; nothing has to be run on them.

1. The old controller is fenced and given the new address, at the fence or later (the address can be given or changed while the fence stands, without lifting it).
2. A running agent's next request is refused by the fence, which names the address. The agent saves it in its enrollment as `movingTo`, so a restart, or the old controller being switched off, does not lose it.
3. Once a minute the agent asks the new address to prove it holds this machine's record. When the new controller is up and passes both checks below, the agent saves the new address, clears `movingTo` and restarts itself.
4. It tells the old controller it has gone (`POST /api/v2/sync/moved`, with its own credentials, to the controller it already uses). The old controller's Move page counts it as moved. A report the old controller does not answer is kept in the enrollment (`leftFrom`) and sent again once a minute, across restarts, until the old controller answers or a week has passed.

A machine switched off during the move catches up when its agent next starts: if it already heard the address it uses the saved one; if not, it hears it from the old controller, so keep that one fenced and running until every machine shows as moved. If the fence is lifted instead, the agent forgets the saved address as soon as the controller answers it without the fence, and a fence that stops naming an address, or names one this machine will not follow, replaces it too. The saved address only stands in for the old controller when that controller does not answer at all (the connection fails, or a proxy in front of it reports it down), never because of a failure on the machine itself.

**The machine does not hand its credentials to just any address.** The address is a hint and nothing more. Before it trusts the new controller the machine runs two checks, in order, and changes nothing unless both pass:

1. **The controller proves it holds this machine's record.** The machine sends its id and a fresh random nonce, with no credential. A controller stores only a hash of a machine's secret, so it answers with an HMAC of the nonce keyed by that hash (`POST /api/v2/sync/agent-proof`). The machine hashes its own secret and checks the answer. A controller that does not hold the record cannot make it, an answer recorded earlier cannot be replayed because the nonce is new each time, and the answer reveals neither the hash nor the secret. A redirect is not followed.
2. **The controller accepts the credentials.** Only now are the machine's credentials sent, once, to ask who they belong to; the answer must be this machine.

A place that cannot prove the record is sent no credential, and the enrollment keeps its address. That is the normal state for a while: the fence usually goes up before the new controller has the copy. While it waits, the agent's log says once that the controller has moved, not every cycle. An address that is not an HTTPS origin (one with a path, or plain HTTP anywhere but this machine itself) is not followed on its own; the machine tells the old controller so, and its Move page shows it as needing a hand.

### Moving a machine by hand

For a machine with following turned off, or to point a machine at a controller yourself:

```sh
reet agent set-server https://reet.new.example
```

It runs the same two checks, keeps the machine's identity, secret and settings, changes only its saved address, clears any saved `movingTo`, tells the old controller it has gone, and restarts the agent as `reet agent restart` does. The address must be an origin (no path, query or credentials) over HTTPS; plain HTTP is accepted only for this machine itself. Use `--config PATH` for a non-default enrollment file and `--service-prefix` as the other agent commands do. If a check fails, the command says which one and why (for example that the controller has no record of this machine, has itself moved, or does not support the proof request yet), and the enrollment file is left exactly as it was.

To stop a machine following on its own:

```sh
reet agent follow-moves off      # on | off | status; on by default
```

Like `allow-apply`, this can only be changed by a command on the machine. Restart a running agent for it to take effect. A machine with following off tells the fenced controller once, so the Move page shows it as needing a hand.
