# bikebook

Agent-first command-line interface for the [BikeBook Workshop Public API][api].

## Why

AI agents are the primary consumer of this CLI. The interface is built around
agent-friendly conventions: structured JSON output, deterministic exit codes,
mandatory idempotency on writes, and a machine-readable `bikebook describe`
schema generated from the API's OpenAPI spec.

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
- [`AGENT_SETUP.md`](./AGENT_SETUP.md) — simple manual for adding the CLI to agent runtimes.
- [`llms.txt`](./llms.txt) — curated Markdown link map for LLM tools.
- [`skills/claude/bikebook.md`](./skills/claude/bikebook.md) — Claude Code skill distribution file.
- [`skills/codex/bikebook/SKILL.md`](./skills/codex/bikebook/SKILL.md) — Codex and ChatGPT skill distribution package.

## Spec

The full API spec lives at [`public-v1.json`](./public-v1.json) (OpenAPI 3.0.1,
85 operations across 16 resource tags). The CLI is a thin, faithful projection
of this spec — every subcommand maps to exactly one HTTP operation.

## License

[MIT](./LICENSE)

[api]: https://developers.bikebook.com/
