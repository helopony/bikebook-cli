# BikeBook CLI Agent Guide

Use `bikebook` as the stable interface to the BikeBook Workshop Public API. Keep stdout for returned data and stderr for diagnostics, idempotency keys, and errors.

## Discover Commands

Run this before guessing flags or payload shape:

```sh
bikebook describe --json
bikebook describe jobs list --json
bikebook jobs list --help
```

`describe` is generated from the Cobra command tree plus `public-v1.json`, so it is the best source for commands, flags, positional args, exit codes, and output contracts.

## Configure Authentication

Prefer profiles for repeatable agent runs:

```sh
printf '%s' "$BIKEBOOK_API_KEY" | bikebook config set api-key --profile default
bikebook config profiles use default
bikebook doctor --json
```

Resolution order is `--api-key`, `BIKEBOOK_API_KEY`, selected config profile. API keys are stored in `~/.bikebook/config.toml` with mode `0600` and redacted in config output.

## Read Resources

Use `--json` when you need the full API envelope:

```sh
bikebook --json jobs list --limit 50
bikebook --json jobs list --all --max 250
bikebook --json jobs get job_123
```

Use `--raw` for NDJSON rows from list `data[]`:

```sh
bikebook --raw customers list --all --max 1000
```

## Write Resources Safely

Every write emits an `Idempotency-Key` on stderr. Capture it if you may retry:

```sh
bikebook --json jobs create --from-file job.json
bikebook --json customers update cus_123 --field name="Ada Lovelace"
bikebook --json invoice-items delete item_123 --yes
```

Use dry-run before destructive or generated writes:

```sh
bikebook --json webhook-endpoints rotate-secret wh_123 --yes --dry-run
```

## Send Raw API Requests

Use `raw` only when a generated command is missing or when debugging HTTP behavior:

```sh
bikebook --json raw GET '/jobs?limit=1'
bikebook --json raw POST /jobs --data @job.json
```

## Handle Errors

Non-zero exits render structured errors on stderr. The `cli.exit_code`, `cli.http_status`, and optional `cli.hint` fields are safe to parse.

Common exits: `2` usage, `3` validation, `4` auth, `6` not found, `7` conflict, `8` rate limited, `9` network/upstream.

## Install And Upgrade

```sh
curl -fsSL https://raw.githubusercontent.com/helopony/bikebook-cli/main/install.sh | sh
bikebook upgrade
```

Set `BIKEBOOK_NO_UPGRADE=1` in locked-down environments.
