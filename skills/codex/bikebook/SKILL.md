---
name: bikebook
description: Use the `bikebook` command-line tool as the stable interface to the BikeBook Workshop Public API. Use when Codex or ChatGPT needs to install or verify BikeBook CLI, configure API authentication, discover supported commands and flags, read BikeBook resources, perform idempotent writes, inspect raw HTTP behavior, parse structured CLI errors, or help another agent integrate the CLI.
---

# BikeBook CLI

## Overview

Use `bikebook` for all BikeBook Workshop Public API work. Keep stdout for returned data and stderr for diagnostics, idempotency keys, and structured errors.

## Install And Verify

Install with the stable script unless the environment already provides `bikebook`:

```sh
curl -fsSL https://raw.githubusercontent.com/helopony/bikebook-cli/main/install.sh | sh
```

Verify:

```sh
bikebook version --json
bikebook doctor --json
```

Set `BIKEBOOK_NO_UPGRADE=1` in locked-down environments. Otherwise use `bikebook upgrade` to update the binary.

## Configure Authentication

Prefer a saved profile for repeatable agent runs:

```sh
printf '%s' "$BIKEBOOK_API_KEY" | bikebook config set api-key --profile default
bikebook config profiles use default
bikebook doctor --json
```

API key resolution order is `--api-key`, then `BIKEBOOK_API_KEY`, then the selected config profile. Saved keys live in `~/.bikebook/config.toml` with mode `0600` and are redacted in config output.

## Discover Before Calling

Run discovery before guessing flags, positional arguments, payload shape, output contracts, or exit codes:

```sh
bikebook describe --json
bikebook describe jobs list --json
bikebook jobs list --help
```

`describe` is generated from the Cobra command tree plus `public-v1.json`, so treat it as the authoritative command schema.

## Read Resources

Use `--json` when the full API envelope matters:

```sh
bikebook --json jobs list --limit 50
bikebook --json jobs list --all --max 250
bikebook --json jobs get job_123
```

Use `--raw` for NDJSON rows from list `data[]`:

```sh
bikebook --raw customers list --all --max 1000
```

## Write Safely

Every write emits an `Idempotency-Key` on stderr. Capture it when a retry may be needed, and reuse it with `--idempotency-key` for the same operation.

```sh
bikebook --json jobs create --from-file job.json
bikebook --json customers update cus_123 --field name="Ada Lovelace"
bikebook --json invoice-items delete item_123 --yes
```

Use dry-run before destructive operations or generated writes:

```sh
bikebook --json webhook-endpoints rotate-secret wh_123 --yes --dry-run
```

## Raw Requests

Use `raw` only when a generated command is missing or while debugging HTTP behavior:

```sh
bikebook --json raw GET '/jobs?limit=1'
bikebook --json raw POST /jobs --data @job.json
```

## Error Handling

Non-zero exits render structured errors on stderr. Parse `cli.exit_code`, `cli.http_status`, and optional `cli.hint` when an agent needs deterministic control flow.

Common exit codes:

- `2`: usage
- `3`: validation
- `4`: auth
- `6`: not found
- `7`: conflict
- `8`: rate limited
- `9`: network or upstream
