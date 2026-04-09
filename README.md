# anytype-gh

[![Go Report Card](https://goreportcard.com/badge/github.com/alesr/anytype-gh)](https://goreportcard.com/report/github.com/alesr/anytype-gh)
[![Go Reference](https://pkg.go.dev/badge/github.com/alesr/anytype-gh.svg)](https://pkg.go.dev/github.com/alesr/anytype-gh)
[![codecov](https://codecov.io/gh/alesr/anytype-gh/graph/badge.svg?token=mv0hyu13HW)](https://codecov.io/gh/alesr/anytype-gh)

`anytype-gh` is an interactive CLI that syncs a GitHub repository `README.md`
(including private repos) into an Anytype page.

## What it does

- lists repositories visible to your GitHub token (archived repositories are skipped)
- fetches the selected repository README from GitHub
- creates, updates, or recreates an Anytype page for that README
- persists local state so repeated runs are idempotent (SHA-based skip/update)

## Requirements

- Go 1.26+
- Anytype desktop app installed and running
- GitHub token with access to your target repos (`GH_TOKEN`)

Create a token at https://github.com/settings/personal-access-tokens with permission to read repositories.

## Install

```bash
go install github.com/alesr/anytype-gh/cmd/anytype-gh@latest
```

## macOS prebuilt binaries

- `anytype-gh-darwin-arm64` (Apple Silicon)
- `anytype-gh-darwin-amd64` (Intel)

Download the right asset for your Mac, then:

```bash
chmod +x anytype-gh-darwin-<arch>
mv anytype-gh-darwin-<arch> anytype-gh
./anytype-gh
```

## Configuration

Create `.env.local` in the project root:

```dotenv
GH_TOKEN=github_pat_xxx
ANYTYPE_BASE_URL=http://localhost:31009 # optional
```

Configuration precedence:

- process environment variables (highest priority)
- `.env.local` file values as fallback

`.env.local` lookup order:

1. current working directory
2. `~/.config/anytype-gh/.env.local`
3. executable directory

State is persisted at `~/.config/anytype-gh/state.json` with restrictive file
permissions (`0600`).

## Run

Run the interactive CLI:

```bash
anytype-gh
```

First, authenticate (Anytype app must be running). Then choose the repository you want to fetch the README from, and sync it with Anytype to create or update a page with the README content.


## Sync behavior

For the selected repository:

- target page title is `README - <owner>/<repo>`
- if README SHA is unchanged and mapping exists, sync is skipped
- if SHA changed, existing page is updated
- if mapped object no longer exists, page is recreated
- state is updated with object ID, SHA, sync timestamp, and space ID
