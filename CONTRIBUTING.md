# Contributing to the migrazione-git-azure-devops project

Thank you for your interest! This document explains how to propose changes, open issues, and submit pull requests to this Go project.

## How can I help?

- Bug reports: open an Issue describing steps to reproduce, expected/actual output, and tool version (`--version`).
- Feature proposals: open an Issue with context, problem, proposed solution, and impact.
- Documentation: corrections and improvements to the README or code comments are welcome.
- Code: submit small, focused PRs with tests (where appropriate) and a clear description.

## Requirements

- Go 1.22+ (latest minor recommended)
- Git
- Optional:
  - golangci-lint for local linting
  - Docker/Buildx to test the image
  - GoReleaser for local snapshot builds

## Environment setup

```bash
git clone https://github.com/amusarra/migrazione-git-azure-devops.git
cd migrate-git-azure-devops
go mod tidy
```

Local build of the tool:

```bash
go build -o bin/migrate-git-azure-devops ./cmd/migrate-git-azure-devops
./bin/migrate-git-azure-devops --version
```

Snapshot with GoReleaser (artifacts in dist/):

```bash
goreleaser release --clean --snapshot --skip=publish
```

## PR workflow

1. Fork the repository and create a branch from `main`:
   - suggested branch name: type/short-scope-example (e.g. `feat/wizard-prompt`, `fix/http-302`)
2. Development:
   - format: `go fmt ./...`
   - analysis: `go vet ./...`
   - lint (optional but recommended): `golangci-lint run`
   - test (if present): `go test ./...`
   - build: `go build ./cmd/migrazione-git-azure-devops`
3. Keep changes small, with godoc comments on functions and complex blocks.
4. Update README if you change CLI usage or add flags.
5. Open the PR describing:
   - problem solved/feature
   - main changes
   - notes on compatibility, impacts, and how to test

## Commit style (Conventional Commits)

Use messages like:

- feat: add a new feature
- fix: fix a bug
- docs: update documentation
- refactor: refactoring without functional changes
- test: add/update tests
- build/ci: changes to build system or CI pipeline
- chore: maintenance tasks

Examples:

- `feat: added --version flag`
- `fix: handled HTTP 302 without following redirects`

## Code guidelines

- Keep functions small and cohesive; extract helpers into dedicated files (e.g. api.go, utils.go).
- Document functions and types with godoc comments.
- Always handle errors (do not ignore returns from Close/Remove).
- Do not print HTTP response bodies in clear text, except in `--trace`.
- Keep CLI output clear and stable; avoid unnecessary breaking changes.

## Version and Release

- The `version`, `commit`, `date` variables are set via `-ldflags` in build/release.
- Do not manually update the version in the code.
- Releases are tagged (SemVer) and managed by GitHub Actions + GoReleaser.
- The changelog is generated automatically.

## Security

- Do not include credentials/PAT in issues, PRs, or logs.
- For vulnerabilities, use GitHub Security Advisories or contact maintainers privately.
- Do not open public Issues with PoC exposing sensitive data.

## Code of Conduct

We adopt respectful and professional behavior. As a reference, the [Contributor Covenant](https://www.contributor-covenant.org/version/2/1/code_of_conduct/) is a good base.

## Questions?

Open an Issue with the “question” label or start a discussion. Thank you for your contribution!
