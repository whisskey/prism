# prism

[![CI](https://github.com/whisskey/prism/actions/workflows/ci.yml/badge.svg)](https://github.com/whisskey/prism/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25.5+-00ADD8?logo=go)](https://go.dev/)

Code-first OpenAPI specification generator for Go web frameworks.

prism parses chi, gin, echo, and fiber source code, infers routes and schemas, and writes an OpenAPI 3.0/3.1 document without running your application. Analysis is static (`go/ast` plus type checking), so generation stays fast and CI-friendly. The output is a standard `openapi.yaml` you can preview with tools like Scalar or Redoc.

## Table of contents

- [Why static analysis?](#why-static-analysis)
- [Requirements](#requirements)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Supported frameworks](#supported-frameworks)
- [Commands](#commands)
- [Configuration](#configuration)
- [CI/CD](#cicd)
- [Development](#development)
- [Contributing](#contributing)
- [License](#license)

## Why static analysis?

| Criteria | prism | Runtime reflection |
|----------|-------|-------------------|
| Dependencies | None at generate time | Full app and database stack |
| Speed | Source parsing only | Service startup required |
| Safety | No code execution | Handler code runs |
| CI fit | Lightweight | Heavy setup |

## Requirements

- Go 1.25.5 or later

## Installation

```bash
# Homebrew
brew install prism/tap/prism

# Go install
go install github.com/whisskey/prism/cmd@latest

# From source
git clone https://github.com/whisskey/prism.git
cd prism
go build -o prism ./cmd
```

## Quick start

```bash
prism init                 # create config (auto-detect framework)
prism generate             # write openapi.yaml
prism check --ci           # verify spec matches code
prism diff                 # show diff against generated spec
prism lint ./...           # report inference warnings
prism watch                # regenerate on file changes
```

## Supported frameworks

| Framework | Import | Generation | Lint |
|-----------|--------|------------|------|
| **chi** | `go-chi/chi` | Routes, middleware, struct schemas | Route extraction |
| **gin** | `gin-gonic/gin` | Binding tags, handler-last routes | Route extraction |
| **echo** | `labstack/echo` | Echo patterns, validator tags | Route extraction |
| **fiber** | `gofiber/fiber` | fibergen deep inference | fibergen analysis |

Set `--framework auto` to detect the framework from `go.mod`.

## Commands

| Command | Description |
|---------|-------------|
| `init` | Create a `prism.yaml` config file |
| `generate` | Generate an OpenAPI document from source |
| `check` | Verify the committed spec matches the code |
| `diff` | Compare two OpenAPI documents |
| `print` | Print the spec to stdout |
| `watch` | Regenerate when source files change |
| `lint` | Report OpenAPI inference warnings |
| `version` | Print version information |

### Generate

```bash
prism generate --mode full          # full | routes-only | schemas-only
prism generate --merge              # merge into an existing spec
prism generate --dry-run            # print to stdout, do not write
prism generate --fibergen           # use fibergen catalog (Fiber)
prism generate --framework chi .
```

### Lint

`prism lint` reports gaps in route and schema inference. Pass `--strict` to exit with an error when warnings are present.

For **chi**, **gin**, and **echo**, lint checks:

- unresolved handlers (anonymous, wrapped, or missing)
- low route discovery confidence
- POST, PUT, or PATCH routes without an inferred request body
- missing or incomplete response definitions

For **fiber**, lint uses the fibergen analyzer for deeper handler and type inference.

```bash
prism lint ./...
prism lint --framework gin ./internal/...
prism lint --strict ./...
```

**golangci-lint plugin** (Fiber only):

```bash
go build -o prism-gcl ./internal/golangci/plugin
prism lint --with-golangci ./...
```

### Annotations

Add `@openapi` godoc tags when static inference is not enough:

```go
// ListUsers returns a paginated list.
// @openapi:summary List users
// @openapi:success 200 {array} UserResponse
// @openapi:failure 401 {object} ErrorResponse
func ListUsers(w http.ResponseWriter, r *http.Request) { ... }
```

## Configuration

prism loads config from the first file found: `prism.yaml`, `prism.json`, `.prism.yaml`, or `.prism.json`.

```yaml
framework: chi        # chi | gin | echo | fiber | auto
output: openapi.yaml
format: yaml           # yaml | json

source:
  paths:
    - ./internal
    - ./cmd
  include:
    - "**/*.go"
  exclude:
    - "**/*_test.go"
    - "**/mocks/**"

openapi:
  version: "3.0.3"
  info:
    title: My API
    version: 1.0.0
  servers:
    - url: http://localhost:8080

generation:
  mode: full
  merge: false
```

Example configs: [`tests/mock/chi/prism.yaml`](tests/mock/chi/prism.yaml). Fixture apps for all frameworks are in [`tests/mock/`](tests/mock/).

## CI/CD

Run prism in your API repository to keep the OpenAPI spec aligned with the code.

The snippets below are workflow `steps` entries. Place them inside a job in `.github/workflows/*.yml`.

**Check only**

```yaml
- run: go install github.com/whisskey/prism/cmd@latest
- run: prism check --ci
```

**Check and lint**

```yaml
- run: go install github.com/whisskey/prism/cmd@latest
- run: prism lint --strict ./...
- run: prism check --ci
```

**Composite action**

```yaml
- uses: whisskey/prism/.github/actions/prism@main
  with:
    command: check
    ci: "true"
```

**Pre-commit hook**

```bash
prism check --strict || exit 1
```

Reference workflow with more jobs: [`.github/workflows/prism-example.yml`](.github/workflows/prism-example.yml) (manual `workflow_dispatch` only). This repository runs its own tests in [`.github/workflows/ci.yml`](.github/workflows/ci.yml).

## Development

```bash
go test ./...                      # unit and plugin tests
go test ./tests/e2e/...           # CLI tests (generate, lint)
go test ./tests/integration/...    # plugin integration tests
go build -o prism ./cmd
./prism --help
```

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for setup, testing, commit conventions, and the pull request workflow.

For questions and bugs, open an [issue](https://github.com/whisskey/prism/issues).

## License

[MIT](LICENSE)
