---
name: bikebook
description: Use the `bikebook` command-line tool as the stable interface to the BikeBook Workshop Public API. Use when Claude needs to install or verify BikeBook CLI, configure API authentication, discover supported commands and flags, read BikeBook resources, perform idempotent writes, inspect raw HTTP behavior, parse structured CLI errors, or help another agent integrate the CLI.
---

# BikeBook CLI

Use this skill when working with the BikeBook Workshop Public API through the `bikebook` command-line tool.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/helopony/bikebook-cli/main/install.sh | sh
```

Verify:

```sh
bikebook version --json
bikebook doctor --json
```

## Auth

Prefer a profile instead of passing API keys in command arguments:

```sh
printf '%s' "$BIKEBOOK_API_KEY" | bikebook config set api-key --profile default
bikebook config profiles use default
```

API key resolution is `--api-key`, then `BIKEBOOK_API_KEY`, then the selected saved profile.

## Discover The CLI

```sh
bikebook describe --json
bikebook describe jobs list --json
bikebook <group> <command> --help
```

Use `describe` before inventing flags. It is generated from the registered Cobra tree and the bundled OpenAPI spec.

## Read Pattern

```sh
bikebook --json jobs list --limit 50
bikebook --json jobs list --all --max 250
bikebook --raw customers list --all --max 1000
```

`--json` keeps the API envelope. `--raw` emits NDJSON rows from list `data[]`.

## Write Pattern

```sh
bikebook --json jobs create --from-file job.json
bikebook --json customers update cus_123 --field name="Ada Lovelace"
bikebook --json webhook-endpoints delete wh_123 --yes
```

Every write prints an `Idempotency-Key` to stderr. Reuse it with `--idempotency-key` when retrying the same operation.

## Dry Run

```sh
bikebook --json jobs create --from-file job.json --dry-run
bikebook --json webhook-endpoints rotate-secret wh_123 --yes --dry-run
```

Dry-run prints method, URL, redacted headers, and body without sending the request.

## Error Handling

Data is always stdout. Errors and diagnostics are stderr. Parse `cli.exit_code`, `cli.http_status`, and `cli.hint` from structured errors.
