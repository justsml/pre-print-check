# Repository Guidelines

## Project Structure & Module Organization

- `main.go` is the executable entry point.
- `internal/cli/` owns command parsing, user-facing output, exit codes, and file I/O.
- `internal/svgcheck/` owns SVG inspection, target parsing, issue generation, and fixes.
- Tests live beside the package they cover, currently in `internal/svgcheck/check_test.go`.
- `README.md` documents user-facing behavior and example CLI usage.

Keep new code inside `internal/` unless it is part of the top-level command bootstrap.

## Build, Test, and Development Commands

- `make build` builds the CLI binary as `./pre-print`.
- `make test` runs `go test ./...` across all packages.
- `make vet` runs `go vet ./...` for static checks.
- `./pre-print check --target vinyl art.svg` runs a local preflight check.
- `./pre-print fix -o art.fixed.svg art.svg` writes a conservatively repaired SVG.

Run `make test` before opening a PR. Run `make vet` when changing parsing, file handling, or reporting.

## Coding Style & Naming Conventions

Use standard Go formatting: run `gofmt` on changed `.go` files. Prefer clear package-local helpers. Keep CLI concerns in `internal/cli` and SVG analysis in `internal/svgcheck`.

Use Go naming conventions: exported identifiers use `PascalCase`, unexported identifiers use `camelCase`, and test functions use `TestName`. Issue codes are lowercase kebab-case strings such as `missing-viewbox` or `raster-not-cuttable`; keep new codes stable and descriptive because they appear in reports.

## Testing Guidelines

The project uses Go's built-in `testing` package. Add table-driven tests for target parsing or rule matrices, and focused tests for new checker or fixer behavior. Prefer assertions on issue codes, severity, ranks, and output changes rather than full-report text comparisons.

Name tests after behavior, for example `TestVinylTargetFlagsNonCuttableContent`. Run all tests with:

```sh
make test
```

## Commit & Pull Request Guidelines

Recent commits use short, imperative summaries, for example `Add material targets for SVG checks` and `Handle bare SVG roots in checks`. Follow that style: start with a verb and mention the affected behavior.

PRs should include a short description, test results (`make test`, and `make vet` when relevant), and before/after examples for user-facing CLI output changes. Link related issues when available. Include sample SVG snippets or command output when adding new preflight rules.

## Security & Configuration Tips

Treat SVG input as untrusted. Keep unsafe transformations gated behind explicit options such as `--unsafe`, avoid executing or resolving external SVG resources, and preserve conservative default behavior for print workflows.
