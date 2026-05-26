# BikeBook CLI — Design Research

Working notes for building an agent-first CLI on top of the **BikeBook Workshop
Public API** (docs: <https://developers.bikebook.com/>). Goal: nail the design
before writing code, so the first commit reflects current (2026) best practice
for agent-consumable CLIs.

## 0. What we know about the target API

Facts pulled directly from `public-v1.json` (OpenAPI 3.0.1):

- **Title**: Workshop API · **Version**: v1
- **Path prefix**: `/public/v1/`
- **Auth**: HTTP Bearer (`Authorization: Bearer bbk_live_...`). The key prefix encodes the environment (`bbk_live_` vs presumably `bbk_test_`).
- **JSON convention**: `snake_case`, prefixed string IDs (`org_`, `job_`, `cus_`, etc.).
- **Idempotency**: `Idempotency-Key` header is **required** on every write (POST).
- **Request correlation**: optional `X-Bikebook-Request-Id` header on requests; echoed back in error envelopes as `request_id`.
- **Pagination**: cursor-based. List endpoints take `?limit=&cursor=` and return `{ "data": [...], "pagination": { "has_more": bool, "next_cursor": string|null } }` (`ApiPagination`).
- **Error envelope** (`ApiErrorResponse`):
  ```json
  { "error": { "code": "...", "message": "...", "parameter": "..." }, "request_id": "..." }
  ```
  `code` is the stable machine-readable identifier (e.g. `invalid_request`, `resource_not_found`).
- **HTTP status codes used**: 200, 201, 400, 401, 403, 404, 409, 429.
- **No `servers` block** in the spec — base URL still needs to be confirmed.

**Surface**: 36 paths, 48 operations (31 GET, 10 POST, 5 PATCH, 2 DELETE) across 13 resource tags:

| Tag | Reads | Writes | Notes |
|---|---|---|---|
| Assets | list / list-for-customer / get | create / update | customer-owned bikes & equipment |
| Businesses | list / get / list services / availability / availability slots / next slot | — | read-only catalogue + scheduling |
| Chat | list messages | send message / upload attachment | per-customer threads |
| Customers | list / get | update | |
| Invoices | list / get | — | |
| Invoice items | get | create / update / delete | |
| Job reports | list / list-for-job / get / get-for-job | — | |
| Jobs | list / get / list part authorisations | create / submit part-authorisation decisions | core workflow |
| Services | list / get | create / update | scoped under business |
| Stock | list / get | — | stock variations / SKUs |
| Webhook endpoints | list / get | create / update / delete / rotate secret | |
| Webhook deliveries | list-for-endpoint / get | replay | |
| Webhook events | list event types | — | catalogue |

This shape (cursor pagination, stable error codes, mandatory idempotency keys,
correlation IDs, snake_case, prefixed IDs) is the Stripe / Plaid / Linear
playbook — the design below mirrors their CLIs deliberately.

## 1. Goals & non-goals

**Goals**
- AI agents (Claude Code, Cursor, Codex, custom) are the *primary* user, humans secondary.
- Install on any machine with one command, no language toolchain required.
- Every command, flag, input, output, and error is discoverable from the CLI itself.
- Output is deterministic, structured, and cheap on tokens.
- The CLI is a thin, faithful projection of the API — no hidden magic.

**Non-goals**
- TUI / interactive wizards as the default path.
- Hiding the underlying API. Agents should be able to reason 1:1 between CLI commands and HTTP endpoints.
- Bundling product opinions (we are not building a "bike research assistant" — we are exposing the API).

## 2. Why CLI (and not MCP, SDK, or raw HTTP)?

Recent comparative studies (2025–2026) show CLIs outperforming MCP servers for
agent use on identical tasks: ~10–32× cheaper in tokens and dramatically higher
reliability. The reason is structural:

- CLIs are **stateless, composable, and pipeable** — agents already know how to use them via `bash`.
- Help text is **lazy-loaded on demand** instead of being dumped into the context up front (as MCP tool schemas are).
- Output can be **filtered with `jq`, `grep`, `head`** before re-entering the agent's context.
- One install gives **every** agent (Claude, Cursor, Codex, scripts, humans) the same surface.

We can still ship an MCP wrapper later; CLI first is the right primitive.

## 3. Agent-first design checklist

The non-negotiables for v1, distilled from Speakeasy, Vercel, GitHub CLI, Stripe CLI, and Anthropic's own conventions:

### 3.1 Output contract

| Flag | Behavior |
|---|---|
| *(default, TTY)* | Pretty human output, colors, tables. |
| *(default, non-TTY)* | Auto-switch to compact JSON. Never emit ANSI codes when stdout is piped. |
| `--json` | Force pretty-printed JSON on stdout regardless of TTY. |
| `--raw` | Compact single-line JSON (one object per line / NDJSON for lists). Saves tokens. |
| `--quiet` / `-q` | Suppress all progress/logging on stderr. Stdout still emits data. |
| `--no-color` | Disable ANSI. Also honor `NO_COLOR` env. |

**Rule**: stdout = data, stderr = diagnostics. Always. An agent should be able to `cmd ... | jq` without contamination.

### 3.2 Exit codes

Mapped directly to the HTTP status codes the API returns (200, 201, 400, 401, 403, 404, 409, 429), plus local-failure codes:

| Code | Meaning | Trigger |
|---|---|---|
| 0 | Success | 2xx |
| 1 | Generic error (last resort) | unknown |
| 2 | Usage error (bad flags, missing arg) | local |
| 3 | Validation / bad request | 400 |
| 4 | Authentication failed | 401 |
| 5 | Forbidden | 403 |
| 6 | Not found | 404 |
| 7 | Conflict (e.g. idempotency replay mismatch) | 409 |
| 8 | Rate limited | 429 |
| 9 | Network / upstream failure | timeout, 5xx |

Agents recover differently from "rate limited" vs "auth failed" vs "validation" — encoding this in the exit code beats parsing English.

### 3.3 Structured errors

Errors go to **stderr** in `--json` mode (and when stdout is non-TTY). We emit
the API's own envelope verbatim, plus a small `cli` block for local context.
No renames, no translation — agents that already know the API recognise the shape:

```json
{
  "error": {
    "code": "resource_not_found",
    "message": "Job job_abc123 not found",
    "parameter": "job_id"
  },
  "request_id": "req_01HXYZ…",
  "cli": {
    "exit_code": 6,
    "http_status": 404,
    "hint": "try `bikebook jobs list --customer-id=…` to find a valid job id",
    "docs_url": "https://developers.bikebook.com/reference#Jobs_Job"
  }
}
```

When the failure is local (network, validation before send), `error.code`
uses CLI-prefixed values (`cli_network_error`, `cli_invalid_input`) so agents
can tell API failures from client failures.

### 3.4 Non-interactive by default for agents

- No command ever prompts unless `--interactive` is passed.
- Destructive operations refuse to run without `--yes` (or `--force` for hard cases).
- Every prompt has a flag equivalent so the agent never needs a TTY.
- Honor `BIKEBOOK_NON_INTERACTIVE=1` and auto-detect non-TTY stdin.

### 3.5 Discoverability — built into the binary

| Command | Purpose |
|---|---|
| `bikebook --help` | Top-level help. Lists commands, global flags, and one-line examples. |
| `bikebook <cmd> --help` | Per-command help with full flag list, input schema, output shape, examples. |
| `bikebook --version` | Print version + commit SHA + API base URL. |
| `bikebook describe` | **Machine-readable schema of the entire CLI** (commands, flags, types, exit codes) as JSON. The single most agent-useful command. |
| `bikebook describe <cmd>` | JSON schema for a single command incl. example input/output. |
| `bikebook doctor` | Diagnose env, auth, connectivity. JSON output supported. |

`describe` is the killer feature — it lets an agent learn the full surface area
from one call without scraping `--help`.

### 3.6 Idempotency & pagination

- **Pagination** is cursor-based (confirmed: `ApiPagination` with `has_more` + `next_cursor`). Flags: `--limit N`, `--cursor STR`. Add `--all` to auto-follow `next_cursor` until exhausted (with a hard cap, e.g. `--max 10000`).
- List output preserves the API envelope verbatim in `--json` mode (`{ data, pagination }`). In `--raw` mode we emit NDJSON of `data[]` for streaming pipelines (`jq -c`, `xargs`).
- **Idempotency** is mandatory for every write. Default behaviour: the CLI generates a UUIDv7 and sends it in `Idempotency-Key`. `--idempotency-key STR` overrides (for retries from agent state). The generated key is always logged to stderr so an agent can reuse it after a network blip.
- **Request correlation**: the CLI generates an `X-Bikebook-Request-Id` per call (overridable via `--request-id`) and surfaces it in errors + `--debug` output for easy support-ticket cross-reference.

### 3.7 Input flexibility

- Accept input via positional args, named flags, **and** stdin JSON (`--from-stdin`). Agents often have JSON already in hand.
- Field names match the API exactly. No renames, no camelCase↔snake_case translation surprises.

### 3.8 Caching & rate-limit etiquette

- Cache idempotent GETs on disk (`~/.cache/bikebook/`) with a short TTL; bypass with `--no-cache`.
- Surface `Retry-After` from the upstream as the `hint` field on rate-limit errors.

## 4. Distribution — "install on any machine very easily"

Ranked by friction for the target user (a developer or an agent on a fresh box):

| Channel | Friction | Notes |
|---|---|---|
| **`curl … | sh` installer** | Lowest | One line, no toolchain. Downloads the right prebuilt binary for the platform. Industry standard (Stripe, Vercel, Bun, Deno, Rustup). |
| **Homebrew** | Low (macOS/Linux) | `brew install bikebook`. Worth doing once we have a tap. |
| **npm `-g`** | Low | Only if we ship Node — adds Node dependency. |
| **`pip install`** | Low | Only if we ship Python — adds Python dependency. |
| **Docker image** | Medium | Useful for CI; awkward for local agent loops (`docker run` boilerplate per call). |
| **GitHub Releases (manual)** | Medium | Always provide as a fallback. |
| **`go install` / `cargo install`** | Medium | Requires toolchain; nice for contributors. |

**Recommendation**: single static binary (Go or Rust), cross-compiled in CI to
the standard matrix (darwin-arm64, darwin-amd64, linux-arm64, linux-amd64,
windows-amd64), published to GitHub Releases, fronted by a short shell
installer. Add Homebrew once stable.

The agent install becomes:

```bash
curl -fsSL https://bikebook.example.com/install.sh | sh
```

…and that's it. No `node`, no `python`, no version skew.

## 5. Language choice & codegen

Now that we have an OpenAPI 3.0.1 spec with 74 schemas and 48 operations,
**OpenAPI codegen is the right primitive** — hand-writing 48 request
wrappers is toil, and the spec will evolve. The language decision now hinges
on which ecosystem has the best codegen story.

| Language | Codegen tool | Pros | Cons |
|---|---|---|---|
| **Go** | `oapi-codegen` (types + typed client) | Single binary, ~5 ms cold start, Cobra/Viper are state-of-the-art for agent CLIs, `goreleaser` ships the full release matrix. | Verbose error handling. |
| **Rust** | `progenitor` / `openapi-generator` | Smallest binary, fastest. | Slower iteration, smaller pool for a workshop API. |
| **TypeScript (Bun-compiled)** | `openapi-typescript` + `openapi-fetch` (or `orval`) | Best-in-class typed client, TS authoring, `bun build --compile` ships a binary. | Larger binary (~50–80 MB), slower cold start than Go, Bun-compiled CLI ecosystem is still young. |
| **Python (PyInstaller)** | `openapi-python-client` | Easy to write. | Heavy binary, slow cold start, packaging pain. |

**Recommendation: Go + `oapi-codegen`**. Cold-start latency matters a lot
in agent loops (agents may invoke the CLI hundreds of times in a session), Go
hits ~5 ms vs Bun's ~30–80 ms, the deliverable is a single static binary, and
the spec → typed client pipeline is mature.

**Codegen pipeline**:
1. `public-v1.json` is committed to the repo (already done).
2. `make generate` runs `oapi-codegen` to produce `internal/api/types.go` and `internal/api/client.go`.
3. `describe` is generated *from the OpenAPI spec at build time*, not hand-written — guarantees it never drifts.
4. New endpoints = re-run `make generate`, write the thin Cobra subcommand, ship.

If the team has a strong TS preference, **TypeScript via Bun** is the runner-up
(`openapi-fetch` is excellent), accepting the cold-start cost.

## 6. Command structure

Pattern: `bikebook <resource> <verb> [args] [--flags]`. Resource names use the
API's plural snake-cased path segments, with hyphens for CLI ergonomics.
Verbs map 1:1 to operations.

**Resource commands** (one subcommand per spec operation):

```
bikebook assets             list | get | create | update | list-for-customer
bikebook businesses         list | get | services | availability | availability-slots | next-available-slot
bikebook chat               messages | send | upload-attachment
bikebook customers          list | get | update
bikebook invoices           list | get
bikebook invoice-items      get | create | update | delete
bikebook job-reports        list | get | list-for-job | get-for-job
bikebook jobs               list | get | create | part-authorisations | submit-part-authorisations
bikebook services           list | get | create | update
bikebook stock              list | get
bikebook webhook-endpoints  list | get | create | update | delete | rotate-secret
bikebook webhook-deliveries list-for-endpoint | get | replay
bikebook webhook-events     list
```

**Plus the always-present agent surface**:

```
bikebook describe [command]   # machine-readable schema of the whole CLI (generated from OpenAPI)
bikebook doctor               # env + auth + connectivity check (calls GET /jobs?limit=1)
bikebook version              # version, commit, api base, latest available
bikebook config get|set|list  # api-base, api-key, default --env, default --limit
bikebook raw <METHOD> <PATH>  # signed pass-through to the API for endpoints we haven't wrapped yet
bikebook webhooks listen      # forward live events to a localhost URL, verify signatures, pretty-print
bikebook webhooks trigger     # replay an existing delivery by id (uses POST /webhook_deliveries/{id}/replay)
```

**Design rules:**
- One subcommand = one HTTP operation. No client-side joins, no hidden multi-call sequences.
- Subcommand and flag names use the **exact field names** from the spec (`business_id`, `customer_email`, `frame_model`) so an agent that has read the OpenAPI doesn't need a translation table.
- `--api-base` / `BIKEBOOK_API_BASE` and `--env=live|test` for environment selection. The default `--env` is derived from the key prefix (`bbk_live_` vs `bbk_test_`).
- `bikebook raw` is the unconditional escape hatch — always available, even after the wrapper drifts from the API.
- Writes require `--yes` for destructive ops (delete, rotate-secret); `--dry-run` prints the request that would be sent without executing it.

## 7. Authentication

Confirmed from the spec: HTTP Bearer with a Workshop-managed API key
(`Authorization: Bearer bbk_live_...`). The `bbk_live_` / `bbk_test_` prefix
is the environment signal.

**Credential resolution order** (highest priority first):
1. `--api-key` flag — emits a stderr warning that the key now lives in shell history
2. `BIKEBOOK_API_KEY` env — **preferred for agents**: no file IO, no prompts, scoped to the process
3. `~/.config/bikebook/config.toml` written by `bikebook config set api-key …` (file mode 0600)

**Rules:**
- Never log the raw key. Redact to `bbk_live_***` in `--debug` output and any error trail.
- `--env` defaults to whichever prefix the resolved key has. Conflict between `--env=live` and a `bbk_test_` key is a hard fail (exit code 2).
- `bikebook doctor` validates the key by issuing the cheapest authenticated call (`GET /public/v1/jobs?limit=1`) and reports the masked key prefix, env, and round-trip time.
- Per-call override: agents that juggle multiple workshops can pass `BIKEBOOK_API_KEY=…` inline (`BIKEBOOK_API_KEY=bbk_… bikebook jobs list`) without touching config files.

## 8. Documentation for agents

Three layers, each cheap to maintain and each readable by agents:

1. **In-binary**: `--help`, `describe`, and `doctor`. Always up to date because they're generated from code.
2. **`llms.txt` on the install/docs site**: a curated index of the most useful documentation URLs, formatted per the llms.txt spec. Now part of Chrome Lighthouse's Agentic Browsing audit (May 2026), so this is table stakes.
3. **A `SKILLS.md` (or `AGENTS.md`) in the repo**: short, task-shaped recipes ("How do I look up a bike from a serial number's metadata?") that an agent can grep before issuing commands. Keep each recipe ≤50 lines (per Speakeasy guidance).

Optionally publish a Claude Code / Cursor "skill" file that bundles the CLI with usage notes for one-step install in agent runtimes.

## 9. Versioning & updates

- Semver. Breaking output shape changes are major version bumps.
- `bikebook version` prints `version`, `commit`, `built_at`, `api_base`, `latest_available` (cached daily).
- `bikebook upgrade` self-updates (downloads the latest binary into the same path it's installed at). Opt-out via env var for locked-down environments.
- Document a **stability contract** for `--json` output: field additions are non-breaking, removals/renames require a major bump.

## 10. Observability & debugging

- `--debug` (or `BIKEBOOK_DEBUG=1`) prints the outgoing HTTP request and response (headers, body, timing) to stderr in JSON.
- Include a `request_id` (locally generated UUID) in debug output and in error payloads — agents can quote it back when reporting issues.
- Optional `--trace` writes a full HAR-style trace to a file.

## 11. Quality bar before "1.0"

- 100% of commands have a `--help`, an entry in `describe`, and a JSON-mode example in `SKILLS.md`.
- `--json` output schema is documented and covered by snapshot tests.
- Exit codes are covered by tests (one test per code).
- Cold-start latency ≤30 ms on a recent laptop.
- A fresh agent in a clean Docker container can install the CLI, list manufacturers, and look up a specific bike using only `--help` and `describe` — no other docs.

## 12. Recommended stack (concrete)

- **Language**: Go (1.23+)
- **CLI framework**: `spf13/cobra` + `spf13/viper` (env/flag/file config merge)
- **HTTP**: stdlib `net/http` + small typed client; no SDK generator needed for 3 endpoints
- **Output**: custom renderer with `--json` / `--raw` / human modes, NDJSON for streams
- **Release**: `goreleaser` → GitHub Releases (darwin/linux/windows × amd64/arm64) + Homebrew tap + `install.sh`
- **CI**: GitHub Actions (lint, test, release)
- **Docs site**: a single static page + `llms.txt` + `SKILLS.md` mirrored from the repo

## 13. Locked decisions

Resolved on 2026-05-26 with the user:

| Question | Decision |
|---|---|
| Ownership / support | **Open-source consumer CLI** in `helopony` GitHub org. Public, MIT-licensed, no obligation to mirror official BikeBook branding. |
| Language | **Go + `oapi-codegen`** — single static binary, ~5 ms cold start, typed client generated from `public-v1.json`. |
| Binary name | **`bikebook`** |
| Production base URL | **`https://api.bikebook.com/public/v1`** (path prefix included in the base) |
| v1 scope | **All 48 operations at once** via codegen — generate the typed client, then write thin Cobra wrappers for every operation. |
| Distribution | **Public**: GitHub Releases (darwin/linux/windows × amd64/arm64) + `curl \| sh` installer + Homebrew tap. |
| Webhook listener | **Skip for v1.** Ship `webhook_endpoints`/`webhook_deliveries`/`webhook_events` CRUD only; defer Stripe-style `listen` to a later milestone. |

Still open (low priority, can be answered during implementation):

- Sandbox host: are `bbk_test_` keys routed via the same `api.bikebook.com` host with a key-based split, or a separate hostname? (Will discover from `doctor` or by asking BikeBook.)

## 14. Proposed step-by-step path

Per your "step by step" preference, suggested first slices:

1. **Resolve §13 (1) (2) (3) (5)** — base URL, ownership, language, binary name.
2. **Scaffold**: empty Cobra project, `make generate` wired to `oapi-codegen`, output: `bikebook --help`, `bikebook --version`, `bikebook describe` (reading from the bundled spec).
3. **Agent contract end-to-end**: implement `doctor`, `config`, `raw` — no resource commands yet. This proves auth, error envelope, exit codes, and JSON/raw/human output on real traffic.
4. **First real resource**: `bikebook jobs list` + `bikebook jobs get`. Use it as the template: types, rendering, pagination follow, error mapping, snapshot test of `--json` output, entry in `SKILLS.md`.
5. **Mass-add reads** for the other 12 tags using the same template.
6. **Writes**: jobs/create, customers/update, invoice-items/create+update+delete, services/create+update, chat/send + upload-attachment, assets/create + update, part-authorisation submission. All with `Idempotency-Key` plumbing and `--dry-run`.
7. **Webhooks**: `webhook-endpoints` CRUD + `rotate-secret`, `webhook-deliveries` list/get/replay, `webhook-events` list. Then `webhooks listen` if §13 (7) is yes.
8. **Distribution**: `goreleaser`, `install.sh`, Homebrew tap, hosted `llms.txt`, `SKILLS.md`, optional Claude Code skill bundle.
