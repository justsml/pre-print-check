package cli

import (
	"flag"
	"fmt"
	"html"
	"io"
	"os"
	"strings"

	"github.com/dan/pre-print-tools/internal/svgcheck"
)

const usage = `pre-print validates and repairs SVGs for print and web.

Usage:
  pre-print check [--target screen|paper|fabric|vinyl|20ft|4k] [--format terminal|md|html] FILE.svg
  pre-print fix [--target TARGET] [--unsafe] [-o OUTPUT.svg] FILE.svg

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
	target := fs.String("target", "", "output material or estimated size, e.g. screen, paper, fabric, vinyl, 20ft, 4k")
	format := fs.String("format", "terminal", "report format: terminal, md, or html")
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

	if err := writeReport(stdout, report, *format); err != nil {
		fmt.Fprintf(stderr, "check failed: %v\n", err)
		return 2
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}

func runFix(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("fix", flag.ContinueOnError)
	fs.SetOutput(stderr)
	target := fs.String("target", "", "output material or estimated size, e.g. screen, paper, fabric, vinyl, 20ft, 4k")
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

func writeReport(w io.Writer, report svgcheck.Report, format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "terminal", "text", "txt":
		writeTerminalReport(w, report)
	case "md", "markdown":
		writeMarkdownReport(w, report)
	case "html":
		writeHTMLReport(w, report)
	default:
		return fmt.Errorf("unsupported report format %q", format)
	}
	return nil
}

func writeTerminalReport(w io.Writer, report svgcheck.Report) {
	fmt.Fprintf(w, "%s\n", report.FriendlySummary())
	fmt.Fprintf(w, "%s\n", report.Summary())
	if report.Target.Raw != "" {
		fmt.Fprintf(w, "Target: %s\n", report.Target.Description())
	}
	if report.Meta.RasterImages > 0 || report.Meta.UniqueColors > 0 || report.Meta.Filters > 0 || report.Meta.FilterRefs > 0 {
		fmt.Fprintf(w, "Artwork signals: %d raster image(s), %d inline raster image(s), %d color value(s), %d unique color(s), %d filter definition(s), %d filter reference(s), %d shadow signal(s)\n",
			report.Meta.RasterImages,
			report.Meta.InlineRasterImages,
			report.Meta.ColorValues,
			report.Meta.UniqueColors,
			report.Meta.Filters,
			report.Meta.FilterRefs,
			report.Meta.Shadows,
		)
	}
	for _, issue := range report.Issues {
		rank := ""
		if issue.Rank != "" {
			rank = fmt.Sprintf(" rank=%s", issue.Rank)
		}
		fmt.Fprintf(w, "[%s%s] %s: %s\n", issue.Severity, rank, issue.Code, issue.Message)
	}
	if len(report.Issues) == 0 {
		fmt.Fprintln(w, "No issues found.")
	}
}

func writeMarkdownReport(w io.Writer, report svgcheck.Report) {
	fmt.Fprintln(w, "# Preflight Report")
	fmt.Fprintf(w, "\n%s\n\n", report.FriendlySummary())
	fmt.Fprintf(w, "- File: `%s`\n", report.Path)
	fmt.Fprintf(w, "- Metadata: %s\n", report.Summary())
	if report.Target.Raw != "" {
		fmt.Fprintf(w, "- Target: %s\n", report.Target.Description())
	}
	fmt.Fprintf(w, "- Artwork signals: %d raster image(s), %d inline raster image(s), %d color value(s), %d unique color(s), %d filter definition(s), %d filter reference(s), %d shadow signal(s)\n",
		report.Meta.RasterImages,
		report.Meta.InlineRasterImages,
		report.Meta.ColorValues,
		report.Meta.UniqueColors,
		report.Meta.Filters,
		report.Meta.FilterRefs,
		report.Meta.Shadows,
	)
	fmt.Fprintln(w, "\n## Findings")
	if len(report.Issues) == 0 {
		fmt.Fprintln(w, "\nNo issues found.")
		return
	}
	fmt.Fprintln(w, "\n| Severity | Rank | Code | Message |")
	fmt.Fprintln(w, "| --- | --- | --- | --- |")
	for _, issue := range report.Issues {
		rank := string(issue.Rank)
		if rank == "" {
			rank = "-"
		}
		fmt.Fprintf(w, "| %s | %s | `%s` | %s |\n", issue.Severity, rank, issue.Code, escapeMarkdownTable(issue.Message))
	}
}

func writeHTMLReport(w io.Writer, report svgcheck.Report) {
	fmt.Fprintln(w, "<!doctype html>")
	fmt.Fprintln(w, `<html lang="en"><meta charset="utf-8"><title>Preflight Report</title>`)
	fmt.Fprintln(w, `<style>body{font-family:system-ui,sans-serif;line-height:1.45;max-width:960px;margin:40px auto;padding:0 20px;color:#1f2328}table{border-collapse:collapse;width:100%}th,td{border:1px solid #d0d7de;padding:8px;text-align:left;vertical-align:top}.error{color:#b42318}.warning{color:#9a6700}.info{color:#0969da}code{background:#f6f8fa;padding:2px 4px;border-radius:4px}</style>`)
	fmt.Fprintln(w, "<body><h1>Preflight Report</h1>")
	fmt.Fprintf(w, "<p><strong>%s</strong></p>\n", html.EscapeString(report.FriendlySummary()))
	fmt.Fprintln(w, "<ul>")
	fmt.Fprintf(w, "<li>File: <code>%s</code></li>\n", html.EscapeString(report.Path))
	fmt.Fprintf(w, "<li>Metadata: %s</li>\n", html.EscapeString(report.Summary()))
	if report.Target.Raw != "" {
		fmt.Fprintf(w, "<li>Target: %s</li>\n", html.EscapeString(report.Target.Description()))
	}
	fmt.Fprintf(w, "<li>Artwork signals: %d raster image(s), %d inline raster image(s), %d color value(s), %d unique color(s), %d filter definition(s), %d filter reference(s), %d shadow signal(s)</li>\n",
		report.Meta.RasterImages,
		report.Meta.InlineRasterImages,
		report.Meta.ColorValues,
		report.Meta.UniqueColors,
		report.Meta.Filters,
		report.Meta.FilterRefs,
		report.Meta.Shadows,
	)
	fmt.Fprintln(w, "</ul><h2>Findings</h2>")
	if len(report.Issues) == 0 {
		fmt.Fprintln(w, "<p>No issues found.</p></body></html>")
		return
	}
	fmt.Fprintln(w, "<table><thead><tr><th>Severity</th><th>Rank</th><th>Code</th><th>Message</th></tr></thead><tbody>")
	for _, issue := range report.Issues {
		rank := string(issue.Rank)
		if rank == "" {
			rank = "-"
		}
		fmt.Fprintf(w, `<tr><td class="%s">%s</td><td>%s</td><td><code>%s</code></td><td>%s</td></tr>`+"\n",
			html.EscapeString(string(issue.Severity)),
			html.EscapeString(string(issue.Severity)),
			html.EscapeString(rank),
			html.EscapeString(issue.Code),
			html.EscapeString(issue.Message),
		)
	}
	fmt.Fprintln(w, "</tbody></table></body></html>")
}

func escapeMarkdownTable(value string) string {
	return strings.ReplaceAll(value, "|", `\|`)
}
