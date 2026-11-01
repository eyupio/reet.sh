package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/eyupio/reet.sh/internal/clientconfig"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Exit codes for `reet branches`.
const (
	branchesOK       = 0
	branchesRefused  = 1
	branchesUsage    = 2
	branchesHaveWork = 3
)

const branchesUsageText = `Usage: reet branches status [--dir PATH] [--base BRANCH] [--json]
       reet branches prune  [--dir PATH] [--base BRANCH] [--dry-run] [--yes] [--json]

Report the local branches in PATH whose work is already in BRANCH (or the
repository's default), say why each other one is kept, and prune only the safe
ones. Nothing is deleted without --yes, or by typing yes at the prompt.

  --dir PATH     the repository's top folder (default: the current folder)
  --base BRANCH  the branch merged work is measured against (default: the
                 repository's default, then main, then master)
  --dry-run      show what would be deleted and change nothing
  --yes          delete without asking (needed when there is no terminal)
  --json         print the plan or the result as JSON

Nothing unmerged, checked out in a worktree, current, default or the base is
ever deleted, and a branch is deleted only while it still points at the tip the
plan showed. Remote branches are never touched.

Exit: 0 nothing to do (or done), 1 refused or failed, 2 usage, 3 candidates
found while --dry-run or without --yes.
`

// runBranches is the `reet branches` command: a machine-local plan and prune,
// using the same proof and deletion code as the controller's machine answer.
func runBranches(args []string, out io.Writer) int {
	return runBranchesInput(args, os.Stdin, out)
}

func runBranchesInput(args []string, in io.Reader, out io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(out, branchesUsageText)
		return branchesOK
	}
	switch args[0] {
	case "status":
		return branchesStatus(args[1:], out)
	case "prune":
		return branchesPrune(args[1:], in, out)
	}
	fmt.Fprintf(os.Stderr, "reet branches: unknown command %q\n\n%s", args[0], branchesUsageText)
	return branchesUsage
}

func branchesFlags(name string) *flag.FlagSet {
	f := flag.NewFlagSet("branches "+name, flag.ContinueOnError)
	f.SetOutput(os.Stderr)
	return f
}

// branchesTarget resolves what the person asked for into the home and absolute
// folder the machine rules take.
func branchesTarget(dir, base string) (string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", "", fmt.Errorf("can't find your home folder: %w", err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	if base != "" && !terminalproto.ValidRefName(base) {
		return "", "", fmt.Errorf("--base %q is not a branch name", base)
	}
	return home, abs, nil
}

func branchesStatus(args []string, out io.Writer) int {
	f := branchesFlags("status")
	dir := f.String("dir", ".", "the repository's top folder")
	base := f.String("base", "", "the branch merged work is measured against")
	asJSON := f.Bool("json", false, "print the plan as JSON")
	if err := f.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return branchesOK
		}
		return branchesUsage
	}
	if f.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "reet branches status: unexpected arguments")
		return branchesUsage
	}
	home, folder, err := branchesTarget(*dir, *base)
	if err != nil {
		fmt.Fprintln(os.Stderr, "reet branches status:", err)
		return branchesUsage
	}
	plan := terminalhost.PlanBranchPrune(context.Background(), home, terminalproto.BranchPrunePlan{Dir: folder, Base: *base}, terminalhost.ExecGit)
	if plan.Reason != "" {
		fmt.Fprintln(os.Stderr, "reet branches status:", branchesRefusal(plan.Reason))
		return branchesRefused
	}
	if *asJSON {
		if err := json.NewEncoder(out).Encode(plan); err != nil {
			fmt.Fprintln(os.Stderr, "reet branches status:", err)
			return branchesRefused
		}
		return branchesOK
	}
	printBranchPlan(out, folder, plan)
	return branchesOK
}

func branchesPrune(args []string, in io.Reader, out io.Writer) int {
	f := branchesFlags("prune")
	dir := f.String("dir", ".", "the repository's top folder")
	base := f.String("base", "", "the branch merged work is measured against")
	dryRun := f.Bool("dry-run", false, "show what would be deleted and change nothing")
	yes := f.Bool("yes", false, "delete without asking")
	asJSON := f.Bool("json", false, "print the plan or result as JSON")
	if err := f.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return branchesOK
		}
		return branchesUsage
	}
	if f.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "reet branches prune: unexpected arguments")
		return branchesUsage
	}
	home, folder, err := branchesTarget(*dir, *base)
	if err != nil {
		fmt.Fprintln(os.Stderr, "reet branches prune:", err)
		return branchesUsage
	}
	plan := terminalhost.PlanBranchPrune(context.Background(), home, terminalproto.BranchPrunePlan{Dir: folder, Base: *base}, terminalhost.ExecGit)
	if plan.Reason != "" {
		fmt.Fprintln(os.Stderr, "reet branches prune:", branchesRefusal(plan.Reason))
		return branchesRefused
	}
	safe := branchesSafe(plan)
	if *asJSON && (*dryRun || !*yes) {
		if err := json.NewEncoder(out).Encode(plan); err != nil {
			fmt.Fprintln(os.Stderr, "reet branches prune:", err)
			return branchesRefused
		}
	} else if !*asJSON {
		printBranchPlan(out, folder, plan)
	}
	if len(safe) == 0 {
		if !*asJSON {
			fmt.Fprintln(out, "\nNothing is safe to prune.")
		}
		return branchesOK
	}
	if *dryRun {
		return branchesHaveWork
	}
	if !*yes {
		if !clientconfig.Interactive(in) {
			if !*asJSON {
				fmt.Fprintf(out, "\nNothing was deleted. Run again with --yes to delete these %d %s.\n", len(safe), branchesWord(len(safe)))
			}
			return branchesHaveWork
		}
		answer, err := clientconfig.Ask(in, out, fmt.Sprintf("Delete %d %s? Type yes", len(safe), branchesWord(len(safe))), "")
		if err != nil || strings.TrimSpace(answer) != "yes" {
			fmt.Fprintln(out, "Nothing was deleted.")
			return branchesRefused
		}
	}
	targets := make([]terminalproto.BranchPruneTarget, 0, len(safe))
	for _, b := range safe {
		targets = append(targets, terminalproto.BranchPruneTarget{Name: b.Name, OID: b.OID})
	}
	result := terminalhost.PruneBranches(context.Background(), home, terminalproto.BranchPrune{Dir: folder, Base: plan.Base, Branches: targets}, terminalhost.ExecGit)
	if result.Reason != "" {
		fmt.Fprintln(os.Stderr, "reet branches prune:", branchesRefusal(result.Reason))
		return branchesRefused
	}
	if *asJSON {
		if err := json.NewEncoder(out).Encode(result); err != nil {
			fmt.Fprintln(os.Stderr, "reet branches prune:", err)
			return branchesRefused
		}
		return branchesOK
	}
	fmt.Fprintf(out, "\nDeleted %d %s.\n", len(result.Deleted), branchesWord(len(result.Deleted)))
	for _, skipped := range result.Skipped {
		fmt.Fprintf(out, "  kept %s — %s\n", skipped.Name, branchesWhy(string(skipped.Skip), plan.Base))
	}
	return branchesOK
}

func branchesSafe(plan terminalproto.BranchPrunePlanResult) []terminalproto.BranchPruneBranch {
	out := make([]terminalproto.BranchPruneBranch, 0, len(plan.Branches))
	for _, b := range plan.Branches {
		if b.Skip == "" {
			out = append(out, b)
		}
	}
	return out
}

func printBranchPlan(out io.Writer, folder string, plan terminalproto.BranchPrunePlanResult) {
	fmt.Fprintf(out, "%s\n", folder)
	if plan.Base == "" {
		fmt.Fprintln(out, "No base branch to compare against; nothing can be pruned.")
	} else {
		fmt.Fprintf(out, "Base: %s\n", plan.Base)
	}
	safe := branchesSafe(plan)
	fmt.Fprintf(out, "\nWill be deleted (%d):\n", len(safe))
	for _, b := range safe {
		fmt.Fprintf(out, "  %s\n", b.Name)
	}
	kept := plan.Branches[:0:0]
	for _, b := range plan.Branches {
		if b.Skip != "" {
			kept = append(kept, b)
		}
	}
	if len(kept) > 0 {
		fmt.Fprintf(out, "\nKept (%d):\n", len(kept))
		for _, b := range kept {
			fmt.Fprintf(out, "  %s — %s\n", b.Name, branchesWhy(string(b.Skip), plan.Base))
		}
	}
	if plan.More {
		fmt.Fprintln(out, "\nOnly the first 200 branches are listed.")
	}
}

func branchesWhy(skip, base string) string {
	switch skip {
	case "not_merged":
		return "its work isn't in " + branchesBaseName(base)
	case "unpushed":
		return "it has commits its remote doesn't"
	case "checked_out":
		return "a worktree has it checked out"
	case "current":
		return "it is the branch checked out in the folder"
	case "default":
		return "it is the repository's default branch"
	case "base":
		return "it is the branch merged work is measured against"
	case "moved":
		return "it changed while you were looking"
	case "git_failed":
		return "git couldn't read or delete it"
	}
	return "reet kept it"
}

func branchesBaseName(base string) string {
	if base == "" {
		return "the base branch"
	}
	return base
}

func branchesWord(n int) string {
	if n == 1 {
		return "branch"
	}
	return "branches"
}

func branchesRefusal(reason terminalproto.BranchesReason) string {
	switch reason {
	case terminalproto.BranchesNotRepoRoot:
		return "that folder isn't the top of a git repository"
	case terminalproto.BranchesOutsideHome:
		return "that folder is outside your home folder"
	case terminalproto.BranchesDenied:
		return "this machine doesn't allow it just now"
	}
	return "git couldn't read the branches there"
}
