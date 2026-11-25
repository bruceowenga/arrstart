// Command arrstart is the headless apply CLI and future wizard binary.
//
// WP1.1a: implements `version` and `apply --answers <file> [--dry-run]`.
// `wizard` prints a not-implemented hint. Subcommands after apply arrive in
// later WPs (doctor, status, update, backup, add, remove, rewire, uninstall).
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/bruceowenga/arrstart/internal/apply"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "wizard not implemented yet; use `arrstart apply`")
		printUsage(os.Stderr)
		return 2
	}

	switch args[0] {
	case "version":
		fmt.Printf("version=%s commit=%s date=%s\n", version, commit, date)
		return 0
	case "apply":
		return runApply(args[1:])
	case "wizard":
		fmt.Fprintln(os.Stderr, "wizard not implemented yet; use `arrstart apply`")
		printUsage(os.Stderr)
		return 2
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %q\n", args[0])
		printUsage(os.Stderr)
		return 2
	}
}

func runApply(args []string) int {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		answersFile string
		dryRun      bool
	)
	fs.StringVar(&answersFile, "answers", "", "path to answers.yaml")
	fs.BoolVar(&dryRun, "dry-run", false, "resolve and print plan without executing")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, "error: invalid arguments")
		printUsage(os.Stderr)
		return 2
	}

	if answersFile == "" {
		fmt.Fprintln(os.Stderr, "error: --answers is required")
		printUsage(os.Stderr)
		return 2
	}

	a, err := apply.LoadAnswers(answersFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}

	if dryRun {
		plan, err := apply.Resolve(a)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 2
		}
		fmt.Print(apply.FormatPlan(plan))
		return 0
	}

	plan, err := apply.Resolve(a)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}

	orch := apply.NewOrchestrator(
		&apply.ResolveStage{Answers: a, Plan: plan},
		&apply.GenerateStage{},
		&apply.ComposeUpStage{ComposeFile: "docker-compose.yml"},
		&apply.WaitHealthyStage{},
		&apply.WireStage{},
		&apply.SummaryStage{},
	)
	if err := orch.Run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: arrstart <command> [flags]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  version   Print version, commit, and build date")
	fmt.Fprintln(w, "  apply     Apply answers.yaml (headless):")
	fmt.Fprintln(w, "              arrstart apply --answers answers.yaml [--dry-run]")
	fmt.Fprintln(w, "  wizard    Not implemented yet; use `arrstart apply`")
}
