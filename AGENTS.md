# Repository Guidelines

## Project Structure & Module Organization
- Go module at the repo root; put CLI entrypoints in `cmd/classifier/`, shared logic in `internal/`, and use `pkg/` only for APIs meant for reuse.
- Keep tests beside the code in `_test.go`; store fixtures in `testdata/`.
- Config samples belong in `configs/` (e.g., `configs/local.yaml.example`); large assets or models should live in `assets/` with a short README on provenance.
- Helper scripts go in `scripts/`; keep generated artifacts out of version control (coverage files are already ignored).

## Build, Test, and Development Commands
- Install/refresh dependencies: `go mod tidy`.
- Format and vet before committing:
  ```sh
  gofmt -w .
  go vet ./...
  ```
- Build everything: `go build ./...`.
- Test suite (run race/coverage before merging):
  ```sh
  go test ./...
  go test -race ./...
  go test -cover ./... -coverprofile=coverage.out
  ```
- If linting is added (`golangci-lint`), run `golangci-lint run ./...`.

## Coding Style & Naming Conventions
- Follow standard Go style (tabs, gofmt/goimports). Exported identifiers need doc comments; prefer short names (`Classifier`, `NewModel`, `ErrInvalidInput`).
- Avoid package name stutter (`classifier.Service` instead of `classifier.Classifier`). Interfaces stay small and are named by behavior (`Loader`, `Runner`).
- Return `error` last, wrap with context (`fmt.Errorf("loading model: %w", err)`), and pass `context.Context` through public entrypoints.

## Testing Guidelines
- Use the standard `testing` package; prefer table-driven tests named like `TestPredict_InvalidInput`. Keep unit tests fast; mark integration tests with build tags if added later.
- Prefer fixtures in `testdata/` and deterministic inputs. Aim for strong coverage of core logic; regenerate `coverage.out` via the command above but do not commit it.

## Commit & Pull Request Guidelines
- History is minimal; use clear, imperative commit messages (Conventional Commits welcome, e.g., `feat: add tokenizer`). Separate behavior changes from refactors when possible.
- Pull requests need a short intent summary, linked issues, and notes on testing (`go test`, `go test -race`). Add screenshots or sample outputs when changing user-facing behavior (CLI messages, logs, generated files).

## Security & Configuration Tips
- Never commit secrets; `.env` is ignored—add `.env.example` with safe defaults when configuration is required.
- Validate external inputs early, keep defaults bounded, and document system dependencies in `README.md` or `configs/`.
