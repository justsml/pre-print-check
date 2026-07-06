package cli

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/dan/pre-print-tools/internal/svgcheck"
)

const usage = `pre-print-tools validates and repairs SVGs for print and web.

Usage:
  pre-print-tools check [--target 20ft|1.2m|90in|4k|8k] FILE.svg
  pre-print-tools fix [--target TARGET] [--unsafe] [-o OUTPUT.svg] FILE.svg

Commands:
  check    Report print/web risks in an SVG
  fix      Apply conservative automatic SVG fixes
`

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}

	switch args[0] {
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "fix":
		return runFix(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	target := fs.String("target", "", "estimated target size, e.g. 20ft, 1.2m, 90in, 4k, 8k")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "check requires exactly one SVG file")
		return 2
	}

	report, err := svgcheck.CheckFile(fs.Arg(0), *target)
	if err != nil {
		fmt.Fprintf(stderr, "check failed: %v\n", err)
		return 1
	}

	writeReport(stdout, report)
	if report.HasErrors() {
		return 1
	}
	return 0
}

func runFix(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("fix", flag.ContinueOnError)
	fs.SetOutput(stderr)
	target := fs.String("target", "", "estimated target size, e.g. 20ft, 1.2m, 90in, 4k, 8k")
	output := fs.String("o", "", "output path; defaults to overwriting FILE.svg")
	unsafe := fs.Bool("unsafe", false, "allow broader transformations that may change rendering")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "fix requires exactly one SVG file")
		return 2
	}

	inputPath := fs.Arg(0)
	input, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Fprintf(stderr, "fix failed: %v\n", err)
		return 1
	}

	result, err := svgcheck.Fix(input, svgcheck.FixOptions{
		Target: *target,
		Unsafe: *unsafe,
	})
	if err != nil {
		fmt.Fprintf(stderr, "fix failed: %v\n", err)
		return 1
	}

	outPath := *output
	if outPath == "" {
		outPath = inputPath
	}
	if err := os.WriteFile(outPath, result.SVG, 0o644); err != nil {
		fmt.Fprintf(stderr, "fix failed: %v\n", err)
		return 1
	}

	if len(result.Changes) == 0 {
		fmt.Fprintf(stdout, "No changes needed: %s\n", outPath)
		return 0
	}

	fmt.Fprintf(stdout, "Wrote %s\n", outPath)
	for _, change := range result.Changes {
		fmt.Fprintf(stdout, "- %s\n", change)
	}
	return 0
}

func writeReport(w io.Writer, report svgcheck.Report) {
	fmt.Fprintf(w, "%s\n", report.Summary())
	if report.Target.Raw != "" {
		fmt.Fprintf(w, "Target: %s\n", report.Target.Description())
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(w, "[%s] %s: %s\n", issue.Severity, issue.Code, issue.Message)
	}
	if len(report.Issues) == 0 {
		fmt.Fprintln(w, "No issues found.")
	}
}
