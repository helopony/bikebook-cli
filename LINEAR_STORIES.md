# Linear stories — `BikeBook CLI v1` (team: spoks)

Paste-ready issues for the project. Linear MCP transport is past removal date
(2026-04-08) so these couldn't be created automatically — re-point the Linear
MCP at `https://mcp.linear.app/mcp` and they can be created on the next pass.

Project description (paste into Project → Description):

> An open-source, agent-first Go CLI wrapping the BikeBook Workshop Public API
> (developers.bikebook.com). Generated typed client from `public-v1.json`,
> Cobra subcommands for all 48 operations, cursor pagination, mandatory
> idempotency keys on writes, structured JSON errors mirroring the API's
> envelope, single-binary distribution via GitHub Releases + Homebrew. Designed
> for AI agents (Claude Code, Cursor, Codex) as the primary user.
>
> Design doc: `RESEARCH.md` in the repo. Spec: `public-v1.json` (Workshop API v1).

---

## BBK-1 · Scaffold Go project + OpenAPI codegen pipeline

**Priority**: Urgent · **Estimate**: 2

Set up the Go module, Cobra skeleton, and `oapi-codegen` pipeline.

### Acceptance

- `go mod` initialised at `github.com/helopony/bikebook-cli`.
- `make generate` runs `oapi-codegen` against `public-v1.json` and produces `internal/api/types.go` + `internal/api/client.go`.
- `cmd/bikebook` builds. `bikebook --help` shows the root command and global flags. `bikebook --version` prints version + commit + base URL.
- CI: GitHub Actions runs `go vet`, `go test ./...`, `make generate` + `git diff --exit-code` (drift check).
- `.golangci.yml` configured with sane defaults.

---

## BBK-2 · Global agent contract: output, errors, exit codes

**Priority**: Urgent · **Estimate**: 3

Implement the cross-cutting agent contract from RESEARCH.md §3 before any resource commands.

### Acceptance

- Global flags: `--json`, `--raw`, `--quiet`, `--no-color`, `--api-base`, `--api-key`, `--env`, `--request-id`, `--idempotency-key`, `--debug`.
- TTY auto-detection: pretty when interactive, compact JSON when piped.
- Stdout = data, stderr = diagnostics. Always.
- Exit codes match the table in RESEARCH.md §3.2 (0/2/3/4/5/6/7/8/9).
- Error renderer emits the API's `ApiErrorResponse` envelope verbatim in `--json` mode, with an added `cli` block (`exit_code`, `http_status`, `hint`, `docs_url`). Local errors use `cli_*` codes.
- `NO_COLOR` env honoured. `BIKEBOOK_NON_INTERACTIVE=1` env honoured.
- Snapshot tests for each output mode against a fixture response.

---

## BBK-3 · Auth resolution + `bikebook config`

**Priority**: Urgent · **Estimate**: 2

Credential resolution + config file management.

### Acceptance

- Resolution order: `--api-key` flag → `BIKEBOOK_API_KEY` env → `~/.config/bikebook/config.toml`.
- Config file mode is `0600`. `config set api-key` updates without exposing the key in `ps`.
- `--env` defaults to the prefix of the resolved key (`bbk_live_` → live, `bbk_test_` → test). Mismatch is a hard fail (exit 2).
- Key is redacted to `bbk_live_***` in `--debug` and error trails. Never logged in full.
- Subcommands: `bikebook config get|set|list|unset` with `--json` support.

---

## BBK-4 · `bikebook doctor` + `bikebook version`

**Priority**: High · **Estimate**: 1

Health check + version reporting.

### Acceptance

- `bikebook doctor` performs: env detection, key resolution check, connectivity probe (`GET /jobs?limit=1`), and prints results as a table or JSON. Distinct exit codes per failure type.
- `bikebook version` prints `version`, `commit`, `built_at`, `api_base`, `latest_available` (cached daily). `--json` support.

---

## BBK-5 · `bikebook describe` — generated from OpenAPI

**Priority**: High · **Estimate**: 2

Machine-readable schema of the entire CLI surface. THE killer agent feature.

### Acceptance

- `bikebook describe` emits a JSON document listing every command, its flags (name, type, required, default, description), positional args, accepted input fields, expected output schema (from OpenAPI), and possible exit codes.
- `bikebook describe <cmd>` returns the slice for a single command with a worked example.
- Schema is built at compile-time from `public-v1.json` plus the Cobra command tree — no drift possible.
- Snapshot test asserts the schema is non-empty for every registered command.

---

## BBK-6 · `bikebook raw <METHOD> <PATH>` escape hatch

**Priority**: High · **Estimate**: 1

Signed pass-through to the API for operations not (yet) wrapped or for debugging.

### Acceptance

- `bikebook raw GET /jobs?limit=1` issues the request with auth + correlation headers and prints the response.
- Body via `--data @file.json` or stdin.
- Auto-generates `Idempotency-Key` for POST/PUT/PATCH unless `--idempotency-key` is passed.
- Headers added via `--header`. Useful for testing custom headers.

---

## BBK-7 · Read endpoints: all 31 GETs as the template-resource (Jobs first)

**Priority**: High · **Estimate**: 5

Implement all GET operations as thin wrappers over the generated client. Jobs is the canonical example and review target; the other 12 tags follow the same pattern.

### Acceptance

- `bikebook jobs list` + `bikebook jobs get` are the canonical template: flag mapping, cursor pagination follow (`--all`, `--max`), `--json` snapshot test, error mapping test.
- `--limit`, `--cursor`, and `--all` (auto-follow with `--max` cap) wired for every list endpoint.
- List endpoint `--json` output preserves API envelope verbatim (`{ data, pagination }`). `--raw` emits NDJSON of `data[]`.
- All 31 GETs covered (Assets / Businesses / Chat / Customers / Invoices / Invoice items / Job reports / Jobs / Services / Stock / Webhook endpoints / Webhook deliveries / Webhook events).
- Each subcommand has at least one happy-path test against a mocked client.

---

## BBK-8 · Write endpoints with idempotency: 10 POST + 5 PATCH + 2 DELETE

**Priority**: High · **Estimate**: 5

Add all write operations. Idempotency is mandatory on every POST per the spec.

### Acceptance

- All 17 write endpoints wired (assets / chat messages + attachments / customers update / invoice items / jobs create / part authorisation decisions / services / webhook endpoints + rotate-secret).
- `Idempotency-Key` auto-generated (UUIDv7) on every write, logged to stderr so agents can retry safely. Overridable via `--idempotency-key`.
- Destructive operations (`delete`, `rotate-secret`) require `--yes`.
- `--dry-run` prints the request that would be sent (method, URL, headers, body) without executing.
- Input accepted via flags, `--from-stdin` JSON, or `--from-file path.json`.
- 409 Conflict responses (e.g. idempotency replay mismatch) surface a clear hint.

---

## BBK-9 · Distribution: goreleaser + install.sh + Homebrew tap

**Priority**: Medium · **Estimate**: 3

Public binary releases.

### Acceptance

- `.goreleaser.yml` builds darwin/linux/windows × amd64/arm64.
- GitHub Releases workflow runs on tag push, attaches archives + checksums + SBOM.
- `install.sh` hosted on a stable URL — one-liner installs the right binary for the platform.
- Homebrew tap repo (`helopony/homebrew-tap`) created; formula auto-updated by goreleaser.
- `bikebook upgrade` self-updates the binary (opt-out via `BIKEBOOK_NO_UPGRADE=1`).
- Reproducible-ish builds: `-trimpath`, embedded version + commit.

---

## BBK-10 · Documentation for agents: llms.txt + AGENTS.md + Claude Code skill

**Priority**: Medium · **Estimate**: 2

Make the CLI maximally discoverable for agents.

### Acceptance

- `AGENTS.md` in the repo root with one short recipe per common task (max ~50 lines per recipe).
- `llms.txt` published at the docs URL per the llms.txt spec.
- Optional: a Claude Code skill file (`.claude/skills/bikebook.md`) that bundles install + common command patterns.
- README has a "for agents" section pointing at `bikebook describe`, `bikebook --help`, and `AGENTS.md`.

---

## BBK-11 · Confirm sandbox host with BikeBook

**Priority**: Low · **Estimate**: 1

Open question from §13: do `bbk_test_` keys route through `api.bikebook.com` with a key-based split, or through a separate sandbox host?

### Acceptance

- Answer documented in `RESEARCH.md`.
- `--env=test` either routes to the correct host or no-ops correctly.
