# Contributing

## Development setup

```bash
git clone https://github.com/open-mon-stack/open-mon-stack.git
cd open-mon-stack
go build -o open-mon-stack .
./open-mon-stack
```

## Running tests

```bash
go test ./...

# Single package
go test ./internal/deploy/...
```

## Submitting changes

1. Fork the repository and create a branch from `main`
2. Make your changes and ensure tests pass (`go test ./...`)
3. Open a pull request against `main`

## Commit style

Use conventional commits: `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`

The release workflow uses these prefixes to generate changelogs.
