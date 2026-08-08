# Dependency and Go Upgrade Design

## Goal

Update the project to Go 1.26.5 and current compatible module releases, then remove only static-analysis findings that are demonstrably dead or behavior-preserving.

## Scope

- Set the module Go version and toolchain to Go 1.26.5.
- Update direct dependencies to their latest releases, update explicitly listed stale or vulnerable transitive modules, and let `go mod tidy` select the rest.
- Keep existing public APIs and runtime behavior unchanged.
- Remove unused private aliases, helpers, and test declarations reported by `staticcheck` after confirming they have no callers.
- Apply behavior-preserving boolean and redundant-return simplifications.
- Do not split large files or introduce new abstractions or dependencies.

## Upgrade Flow

1. Update the Go directive and toolchain.
2. Upgrade direct modules and run `go mod tidy`.
3. Run the full tests to catch dependency incompatibilities before source cleanup.
4. Use the current `staticcheck` failures as the red check, apply the smallest cleanup, and rerun it.
5. Finish with tests, `go vet`, `staticcheck`, and `govulncheck`.

## Error Handling

If a latest module release breaks compilation or tests, identify the incompatible module and retain the newest version that passes the existing suite. No compatibility shim will be added unless the dependency's supported API requires a small direct migration.

## Verification

- `go test ./...`
- `go vet ./...`
- `GOTOOLCHAIN=go1.26.5 go run honnef.co/go/tools/cmd/staticcheck@latest ./...`
- `GOTOOLCHAIN=go1.26.5 go run golang.org/x/vuln/cmd/govulncheck@latest ./...`
- `go mod tidy -diff` produces no diff
