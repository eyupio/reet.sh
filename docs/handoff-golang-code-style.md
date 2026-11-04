# Handoff: Go code style recommendations

## What this PR does

Applies the mechanical, low-risk parts of the `samber/cc-skills-golang@golang-code-style`
guidelines across the Go codebase. It is a style-only change set: no intended
behaviour change, no public API change, no schema change.

The work was driven by an AST audit over all 347 non-test Go files using three
rules:

1. `if` conditions with 3 or more operands (`&&` / `||` leaves).
2. `if` bodies that end in `return` / `break` / `continue` / `goto` and still
   carry an `else`.
3. `for i := 0; i < n; i++` index loops.

## Completed so far

`internal/store` and `internal/api` landed in the first PR. `internal/terminalproto`
and `internal/terminalhost` landed in the second. This PR adds
`internal/setup`, `internal/terminalbroker`, `internal/scan`,
`internal/terminalagent` and `internal/acpagent`.

### Index loops (`range`)

- 20 index loops converted to `range`.
- 8 left index-based and annotated with a comment because the body advances the
  index itself (escaped bytes, JSONC comments, git `-z` rename paths). `range`
  cannot express those.

### Unnecessary `else` after a terminating `if`

- 71 `else` branches removed or restructured.
- 58 needed the `if` init statement hoisted first.
- 6 hoisted statements changed from `:=` to `=` where the variable already
  existed in the enclosing block.
- 5 nested `else if` chains were restructured by hand. A naive `else` removal is
  **unsound** for a nested `else if`: `if A { … } else if B { return } else { C }`
  must not become `… if B { return }; C`, because `C` then also runs when `A`
  is true.

### Complex conditions

- `internal/store`: 129 → 0 (first PR).
- `internal/api`: 87 → 0 (first PR).
- `internal/terminalproto`: 97 → 0.
- `internal/terminalhost`: 73 → 0.
- `internal/setup`: 66 → 0 (previous PR).
- `internal/terminalbroker`: 46 → 0 (previous PR).
- `internal/scan`: 43 → 0 (previous PR).
- `internal/terminalagent`: 41 → 0 (previous PR).
- `internal/acpagent`: 32 → 0 (previous PR).
- `internal/agenthealth`: 26 → 0 (this PR).
- `internal/backup`: 19 → 0 (this PR).
- `internal/lifecycle`: 18 → 0 (this PR).

Conditions were grouped into named booleans, with expensive or
short-circuit-sensitive checks kept inline (for example a nil check before a
dereference, or a map index after a non-negative check).

## Not done (remaining work)

Current AST audit after this PR:

| Rule | Remaining |
| --- | --- |
| `cond>=3` | 136 |
| `params>4` | 187 |
| `index-loop` | 8 (all intentional, commented) |

Top packages by remaining complex conditions:

| Package | `cond>=3` |
| --- | --- |
| `cmd/reet` | 17 |
| `internal/agentsoftware` | 13 |
| `internal/skillbundle` | 12 |
| `internal/terminalcontrol` | 11 |
| `internal/clientconfig` | 10 |
| `internal/skillplan` | 9 |
| `internal/mail` | 7 |
| `internal/skillpreview` | 7 |

`params>4` is untouched. It is a design change (options structs and call-site
updates), not a mechanical rewrite. Functions with 5 and 6 parameters are the
bulk (about 100 and 50 respectively).

Also untouched from the same style guide:

- lines over ~120 characters (about 2,180 occurrences, many are long strings or
  comments),
- nil slice/map initialisation (some nil values are intentional, e.g. JSON
  `null` versus `[]`),
- keyed composite literals,
- `strconv` versus `fmt` and `%q` in errors.

## Verification

- `gofmt` clean across the repository.
- `go build ./...` passes.
- `go test ./internal/agenthealth ./internal/backup ./internal/lifecycle -count=1` passes.
- `go test ./internal/scan -count=1` passes apart from the network-dependent
  test listed below.
- Full `go test ./...` has two failures that also reproduce on pristine
  `main`:
  - `internal/clientconfig` — `TestPromptsAreBoundedAndDoNotConsumeLaterConfirmation`
    fails with `prompted from a pipe` in this non-interactive environment.
  - `internal/scan` — `TestRunReportRefusesAnInteractiveLessScanWithoutServer`
    is network-dependent and fails against the same baseline.

## Re-running the audit

The audit used a temporary `go/ast` program, not committed. To reproduce the
counts, walk non-test `.go` files and count:

- `*ast.IfStmt` conditions where the recursive count of `token.LAND` /
  `token.LOR` leaves is 3 or more;
- `*ast.IfStmt` with `Else != nil` whose `Body` last statement is a `ReturnStmt`
  or a terminating `BranchStmt`;
- `*ast.ForStmt` with init `i := 0`, condition `i < …`, post `i++`.

## Suggested next step

Continue with `cmd/reet` (17 conditions) using the same approach, then
`internal/agentsoftware` (13), `internal/skillbundle` (12) and
`internal/terminalcontrol` (11). Do not start with `params>4` unless a package
is being touched for another reason; options-struct refactors should be
reviewed on their own.

Regressions caught locally during these PRs and fixed: `hasControl` briefly
returned the wrong boolean, an `if`/`else` in `unpushedCount` was mangled while
extracting its condition, and two inverted predicates (`pending question` and
model-option duplicate detection) were caught by their existing tests. All are
covered by the package tests.
