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

`internal/store` and `internal/api` landed in the first PR. This PR adds
`internal/terminalproto` and `internal/terminalhost`.

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
- `internal/terminalproto`: 97 → 0 (this PR).
- `internal/terminalhost`: 73 → 0 (this PR).

Conditions were grouped into named booleans, with expensive or
short-circuit-sensitive checks kept inline (for example a nil check before a
dereference, or a map index after a non-negative check).

## Not done (remaining work)

Current AST audit after this PR:

| Rule | Remaining |
| --- | --- |
| `cond>=3` | 397 |
| `params>4` | 181 |
| `index-loop` | 8 (all intentional, commented) |

Top packages by remaining complex conditions:

| Package | `cond>=3` |
| --- | --- |
| `internal/setup` | 66 |
| `internal/scan` | 43 |
| `internal/terminalbroker` | 38 |
| `internal/terminalagent` | 33 |
| `internal/agenthealth` | 26 |
| `internal/acpagent` | 20 |
| `internal/backup` | 19 |
| `internal/lifecycle` | 18 |
| `cmd/reet` | 17 |

`params>4` is untouched. It is a design change (options structs and call-site
updates), not a mechanical rewrite. Functions with 5 and 6 parameters are the
bulk (99 and 50 respectively).

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
- `go test ./internal/terminalproto -count=1` passes.
- `go test ./internal/terminalhost -count=1` passes.
- `go test ./internal/api ./internal/terminalbroker -count=1` passes.
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

Continue with `internal/setup` (66 conditions) using the same approach, then
`internal/scan` (43) and `internal/terminalbroker` (38). Do not start with
`params>4` unless a package is being touched for another reason; options-struct
refactors should be reviewed on their own.

Two regressions were caught locally during this PR and are fixed:
`hasControl` briefly returned the wrong boolean, and an `if`/`else` in
`unpushedCount` was mangled while extracting its condition. Both are covered by
the existing `internal/terminalhost` tests.
