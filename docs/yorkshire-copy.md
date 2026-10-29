# Yorkshire copy matrix

This is the working companion to [Reet’s Yorkshire voice](voice-and-copy.md). It turns the voice guidance into concrete strings and a typed dictionary, `web/src/lib/copy.ts`.

The rule of thumb: **standard text carries the meaning; Yorkshire carries the warmth.** A dialect string may replace a standard one only on low-risk surfaces. Destructive actions, authentication, system health, security warnings, form labels and CLI syntax always keep the standard string, with dialect optionally available as a supporting line.

## Surfaces

| Surface | Dialect allowed as replacement? | Typical use |
| --- | --- | --- |
| `critical` | No | Delete, revoke, terminate, errors, auth, security notices |
| `navigation` | No | Tabs, back links, primary navigation, cancellation |
| `form` | No | Field labels, validation, save controls |
| `status` | No | Operational / degraded / down / paused labels |
| `success` | Yes | Completed actions and confirmations |
| `empty` | Yes | Empty states and 404 pages, with a plain next step |
| `onboarding` | Yes | Greetings and first-run guidance |
| `microcopy` | Yes | Tooltips, badges, transient helper text |
| `marketing` | Yes | Landing copy and hero CTAs (auth CTAs stay standard) |
| `command` | No | Shell syntax, diagnostics, exit-code text |

`t(key)` applies the `reet` variant only on the five “Yes” surfaces. `support(key)` returns an optional companion line that is safe beside a standard label on any surface.

## Copy audit

### Header, navigation and hero

| Key | Standard | Reet variant | Notes |
| --- | --- | --- | --- |
| `nav.home` | Home | — | Navigation stays standard. |
| `nav.sessions` | Sessions | — | |
| `nav.machines` | Machines | — | |
| `nav.skills` | Skills | — | |
| `nav.account` | Account | — | |
| `nav.signIn` | Sign in | — | Authentication must never be a puzzle. |
| `nav.createAccount` | Create an account | — | |
| `hero.eyebrow` | Your AI control room. | Ey up. Your AI control room. | Greeting plus the page’s purpose. |
| `hero.title` | One control room. For your AI fleet. | — | The promise stays literal. |
| `hero.cta` | Get started | Reet, let’s get crackin’ | Marketing only; use `nav.signIn` for auth. |
| `hero.secondaryCta` | See how it works | Have a nosy | |
| `marketing.toolkit` | A very good agent toolkit. | A reet good agent toolkit. | |
| `marketing.control` | Proper control, on your own kit | — | Already carries the voice. |

### Forms, buttons and confirmations

| Key | Standard | Reet variant / support | Notes |
| --- | --- | --- | --- |
| `action.save` | Save changes | — | Form control stays standard. |
| `action.cancel` | Cancel | — | |
| `action.delete` | Delete | — | Destructive. No accent. |
| `action.revoke` | Revoke access | — | Destructive. No accent. |
| `action.terminate` | Terminate session | — | Destructive. No accent. |
| `action.forget` | Forget machine | — | Destructive. No accent. |
| `action.restore` | Restore | — | |
| `form.required` | This field is required. | Support: Give us summat to work with. | Precision first; warmth second. |
| `form.invalidEmail` | Enter a valid email address. | — | |
| `form.saveFailed` | Could not save your changes. Try again. | — | Failure stays direct. |
| `success.saved` | Saved. | Sorted. Saved. | States the outcome explicitly. |
| `success.updated` | Updated. | Reet, updated. | |
| `success.copied` | Copied to clipboard. | Reet, copied to clipboard. | Clipboard action remains named. |
| `success.created` | Created. | Reet, that’s made. | |
| `success.invited` | Invitation sent. | Reet, invite’s away. | |
| `success.taskComplete` | Task complete. | Reet, job’s a good ’un. | |
| `security.deleteWarning` | This cannot be undone. | — | Security copy stays standard. |
| `security.revokeWarning` | This revokes access immediately. The machine will need to be connected again. | — | |
| `security.secretOnce` | Copy this token now. It will not be shown again. | — | |

### System health and status badges

| Key | Standard label | Support line | Notes |
| --- | --- | --- | --- |
| `status.operational` | Operational | All reet — everything’s ticking over. | Colour and label stay standard. |
| `status.degraded` | Degraded | Summat’s not reet. Some parts are slower or failing. | Never require the idiom to understand health. |
| `status.partialOutage` | Partial outage | Summat’s not reet for some machines. | |
| `status.majorOutage` | Major outage | Nowt is responding right now. | |
| `status.down` | Down | Nowt is responding right now. | |
| `status.maintenance` | Maintenance | Planned work. Back shortly. | |
| `status.unknown` | Unknown | We have not heard from it yet. | |
| `status.checking` | Checking… | Hold your horses. | |
| `status.paused` | Paused | Hold your horses — this one is paused. | |
| `status.throttled` | Rate limited | Hold your horses — too many requests just now. | |
| `status.offline` | Offline | Nowt heard from this machine lately. | |
| `status.synced` | Everything in sync | All reet. | |
| `status.syncing` | Syncing… | Hold your horses — changes are on their way. | |
| `status.healthy` | Healthy and operational | Reet as rain. | |

### Errors, 404 and empty states

| Key | Standard | Reet variant / support | Notes |
| --- | --- | --- | --- |
| `error.generic.title` | Something went wrong | Support: Summat’s not reet. The detail below is the reliable bit. | Alert title stays standard. |
| `error.network` | Could not reach the server. Check your connection and try again. | Support: We’re not reet sure the request got there. | |
| `error.unauthorized` | Your session has expired. Sign in again to continue. | — | Authentication error: standard only. |
| `error.forbidden` | You do not have permission to do that. | Support: Ask an owner or admin on this instance. | |
| `error.rateLimited` | Too many requests. Wait a moment and try again. | Support: Hold your horses — then have another go. | |
| `error.server` | The server could not complete that request. | Support: Hold your horses and try again in a moment. | |
| `error.timeout` | The request timed out. | Support: Hold your horses — the machine may still be working. | |
| `error.copyFailed` | Could not copy to the clipboard. | Support: Use your browser’s copy command instead. | |
| `notFound.title` | Page not found | — | Heading stays indexable and unambiguous. |
| `notFound.lede` | That address does not lead anywhere on this instance. It may have moved, or the link may be mistyped. | Nowt lives at that address on this instance. It may have moved, or the link may be mistyped. | The second sentence is the fallback. |
| `notFound.back` | Back to Home | — | Navigation stays standard. |
| `empty.generic.title` | Nothing here yet. | Nowt here yet. | Always pair with a next step. |
| `empty.generic.body` | Add something to get started. | Get crackin’ and add summat. | |
| `empty.search.title` | No results match your search. | Nowt matches just yet. | |
| `empty.search.body` | Try another search or clear your filters. | Have another go, or clear your filters. | |
| `empty.machines.title` | No machines connected yet. | Nowt connected yet. | |
| `empty.machines.body` | Connect a machine with the reet CLI to see it here. | Get a machine connected with the reet CLI and it’ll show up here. | |
| `empty.sessions.title` | No sessions yet. | Nowt running yet. | |
| `empty.sessions.body` | Start a session from Home. | Start one from Home when you’re ready. | |
| `empty.skills.title` | No skills yet. | Nowt on the shelf yet. | |
| `empty.skills.body` | Add your first skill to get started. | Whack your first skill on the shelf. | |
| `empty.notifications.title` | Nothing needs you. | Nowt needs you. | |
| `empty.lists.title` | No lists yet. | Nowt in the drawer yet. | |
| `empty.community.title` | Nothing matches yet. | Nowt matches just yet. | |

### Onboarding, tooltips and microcopy

| Key | Standard | Reet variant | Notes |
| --- | --- | --- | --- |
| `onboarding.greeting` | Welcome. | Ey up. | |
| `onboarding.welcomeBack` | Welcome back. | Ey up. Good to see you again. | |
| `onboarding.firstRun.title` | Let’s get started. | Reet, let’s get crackin’. | Primary CTA stays standard. |
| `onboarding.firstRun.body` | Connect your first machine to get started. | Connect your first machine and we’ll get you sorted. | |
| `onboarding.emptyFleet` | Your fleet is empty. | Nowt in the fleet yet. | |
| `onboarding.stepDone` | Step complete. | Reet, job’s a good ’un. | |
| `onboarding.ready` | You are ready to go. | All reet — you’re ready to go. | |
| `onboarding.noFaff` | No unnecessary complexity. | No faff, just code. | |
| `microcopy.greeting` | Hello | Ey up | |
| `microcopy.good` | Good | Reet good | |
| `microcopy.veryGood` | Very good | Reet grand | |
| `microcopy.exactlyRight` | Exactly right | Too reet | |
| `microcopy.fast` | Very fast | Reet quick | Intensifier. |
| `microcopy.reliable` | Very reliable | Reet solid | Intensifier. |
| `microcopy.happy` | Happy with that | Chuffed with that | |
| `microcopy.friction` | No unnecessary complexity | No faff, just code | |
| `microcopy.processing` | Processing… | Hold your horses… | |
| `microcopy.allGood` | All good | All reet | |
| `microcopy.done` | Done | Job’s a good ’un | |
| `microcopy.badge` | New notification | Ey up — summat new | Badge stays short. |

### CLI and terminal output

| Key | Standard | Reet variant | Notes |
| --- | --- | --- | --- |
| `cli.installSuccess` | reet installed successfully. | Reet, installed. | Human-facing completion only. |
| `cli.scanSuccess` | Scan complete. | Reet, scan’s done. | |
| `cli.syncSuccess` | Sync complete. | All reet — sync’s done. | |
| `cli.waiting` | Waiting for the machine… | Hold your horses — waiting for the machine… | Status label stays standard if machine-readable. |
| `cli.error` | Error: | — | Never localise the diagnostic prefix. |
| `cli.hint` | Run `reet --help` for usage. | — | Commands stay exact. |
| `cli.lowLatency` | Low latency | Reet quick | Support: Round-trip time is below the healthy threshold. |
| `cli.stableConnection` | Stable connection | Reet solid | Support: No missed probes in the current window. |
| `cli.noResults` | No results. | Nowt found. | |

## Implementation

```ts
import { t, support } from './copy'

t('empty.skills.title')            // “Nowt on the shelf yet.” (empty surface)
t('status.degraded')               // “Degraded” (status surface, dialect ignored)
support('status.degraded')         // “Summat’s not reet. Some parts are slower or failing.”
t('success.copied', { dialect: false }) // “Copied to clipboard.”
```

- Keep interpolation tokens in both variants, e.g. `{count}`; pass values with `t(key, { vars: { count: 3 } })`.
- `t()` enforces the guardrail in code: callers cannot accidentally put dialect into a critical label by passing `dialect: true`.
- `support()` is the escape hatch for pairing a warm line with a standard label, tooltip or error title.
- When a string is not in the dictionary, prefer adding a typed key over hard-coding a one-off, so the standard fallback is always recorded.
