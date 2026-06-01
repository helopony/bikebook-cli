# Adding BikeBook CLI To Agents

This is the short setup path for giving an agent reliable access to the BikeBook Workshop Public API through `bikebook`.

## 1. Install The CLI

Install on macOS or Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/helopony/bikebook-cli/main/install.sh | sh
```

Other options:

```sh
brew install helopony/tap/bikebook
go install github.com/helopony/bikebook-cli/cmd/bikebook@latest
```

Verify:

```sh
bikebook version --json
bikebook doctor --json
```

Set `BIKEBOOK_NO_UPGRADE=1` in locked-down agent environments.

## 2. Configure Auth

For repeatable agent runs, prefer a saved profile:

```sh
printf '%s' "$BIKEBOOK_API_KEY" | bikebook config set api-key --profile default
bikebook config profiles use default
bikebook doctor --json
```

Agents can also pass `--api-key` or rely on `BIKEBOOK_API_KEY`. Resolution order is `--api-key`, `BIKEBOOK_API_KEY`, then selected profile.

## 3. Add Agent Instructions

Copy or reference `AGENTS.md` in repositories where agents should use BikeBook. The core rule is:

```md
Use `bikebook` as the stable interface to the BikeBook Workshop Public API. Run `bikebook describe --json` before guessing command flags or payloads. Keep stdout for returned data and stderr for diagnostics, idempotency keys, and errors.
```

## 4. Install Runtime-Specific Skills

The `skills/` directory in this repository is the distribution source. Copy the matching skill into the target agent's expected local location after installing `bikebook`.

Claude Code:

```sh
mkdir -p .claude/skills
cp skills/claude/bikebook.md /path/to/target-repo/.claude/skills/bikebook.md
```

Codex or ChatGPT:

```sh
mkdir -p .codex/skills
cp -R skills/codex/bikebook /path/to/target-repo/.codex/skills/
```

For a personal Codex install, copy the skill into the Codex skills directory:

```sh
mkdir -p "${CODEX_HOME:-$HOME/.codex}/skills"
cp -R skills/codex/bikebook "${CODEX_HOME:-$HOME/.codex}/skills/"
```

## 5. Agent Usage Pattern

Start every new task with discovery:

```sh
bikebook describe --json
bikebook describe jobs list --json
bikebook jobs list --help
```

Read with structured output:

```sh
bikebook --json jobs list --limit 50
bikebook --json jobs get job_123
```

Use NDJSON for row streaming:

```sh
bikebook --raw customers list --all --max 1000
```

Use dry-run before destructive or generated writes:

```sh
bikebook --json webhook-endpoints rotate-secret wh_123 --yes --dry-run
```

Capture the stderr `Idempotency-Key` from writes before retrying.
