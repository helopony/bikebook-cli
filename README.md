# bikebook

Agent-first command-line interface for the [BikeBook Workshop Public API][api].

> Status: v1 implementation in progress. See [`RESEARCH.md`](./RESEARCH.md)
> for the design doc and [`LINEAR_STORIES.md`](./LINEAR_STORIES.md) for the
> implementation plan.

## Why

AI agents are the primary consumer of this CLI. The interface is built around
the agent-friendly conventions distilled in `RESEARCH.md`: structured JSON
output, deterministic exit codes, mandatory idempotency on writes, and a
machine-readable `bikebook describe` schema generated from the API's OpenAPI
spec.

## Install

```sh
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/helopony/bikebook-cli/main/install.sh | sh

# Homebrew
brew install helopony/tap/bikebook

# From source
go install github.com/helopony/bikebook-cli/cmd/bikebook@latest
```

## For agents

- `bikebook describe` — full machine-readable schema of every command, flag, and exit code, generated from `public-v1.json`.
- `bikebook --help` and `bikebook <cmd> --help` — per-command help.
- `bikebook upgrade` — self-update from the latest GitHub Release; set `BIKEBOOK_NO_UPGRADE=1` to disable.
- [`AGENTS.md`](./AGENTS.md) — task-shaped command recipes for agents.
- [`llms.txt`](./llms.txt) — curated Markdown link map for LLM tools.
- [`.claude/skills/bikebook.md`](./.claude/skills/bikebook.md) — Claude Code skill with install and common command patterns.

## Spec

The full API spec lives at [`public-v1.json`](./public-v1.json) (OpenAPI 3.0.1,
48 operations across 13 resource tags). The CLI is a thin, faithful projection
of this spec — every subcommand maps to exactly one HTTP operation.

## License

[MIT](./LICENSE)

[api]: https://developers.bikebook.com/
