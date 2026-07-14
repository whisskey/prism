# Contributing to prism

Thank you for your interest in contributing. Bug reports, feature requests, and pull requests are welcome.

## Before you start

- Search [existing issues](https://github.com/whisskey/prism/issues) to avoid duplicates.
- For larger changes, open an issue first to discuss the approach.
- Read the [README](README.md) for project scope and supported frameworks.

## Development setup

Requirements:

- Go 1.25.5 or later

```bash
git clone https://github.com/whisskey/prism.git
cd prism
go build -o prism ./cmd
```

## Running tests

Run the same checks as CI before opening a pull request:

```bash
# Unit and plugin tests
go test ./internal/... ./pkg/... -count=1

# Plugin integration tests
go test ./tests/integration/... -count=1

# CLI end-to-end tests
go test ./tests/e2e/... -count=1
```

Build verification:

```bash
go build -o prism ./cmd
./prism version
go build -o prism-gcl ./internal/golangci/plugin
```

## Project layout

| Path | Purpose |
|------|---------|
| `cmd/` | CLI entrypoint |
| `internal/cli/` | Cobra commands |
| `internal/plugins/` | Framework plugins (chi, gin, echo, fiber) |
| `internal/spec/` | OpenAPI assembly, merge, and diff |
| `internal/typecheck/` | Static analysis pipeline |
| `internal/analyzer/fibergen/` | Fiber deep inference |
| `pkg/openapi/` | Public OpenAPI types |
| `tests/integration/` | Plugin integration tests |
| `tests/e2e/` | CLI end-to-end tests |
| `tests/mock/` | Framework fixture apps |

When adding framework behavior, include or update tests in the matching plugin package and, when relevant, integration or e2e coverage.

## Pull request workflow

1. Fork the repository and create a branch from `main`.
2. Make focused changes with tests where behavior changes.
3. Run the test commands listed above.
4. Open a pull request against `main` with:
   - a clear summary of the change
   - the motivation or issue link, if applicable
   - notes on testing performed

Keep pull requests small and reviewable when possible.

## Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(cli): add --format flag
fix(plugins): infer wrapped gin handlers
test(spec): cover merge edge cases
chore(deps): bump cobra
docs: update configuration examples
refactor(typecheck): simplify loader
```

Guidelines:

- Use a short subject line; no body unless necessary.
- Pick a scope that matches the affected area (`cli`, `plugins`, `spec`, `typecheck`, etc.).
- Use `feat` for user-facing changes, `fix` for bug fixes, `test` for test-only changes.

## Code guidelines

- Match the style and structure of the surrounding package.
- Prefer extending existing helpers over duplicating logic.
- Keep changes minimal and scoped to the task.
- Add comments only for non-obvious behavior.
- Do not commit generated OpenAPI files, local config overrides, or build artifacts.

## Reporting bugs

Open a [bug report](https://github.com/whisskey/prism/issues/new) with:

- Go version (`go version`)
- prism version (`prism version`) or commit SHA
- framework in use (chi, gin, echo, or fiber)
- minimal code or config that reproduces the issue
- expected vs actual output

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
