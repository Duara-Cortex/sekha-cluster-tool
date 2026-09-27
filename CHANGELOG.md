# Changelog

## v1.0.13

Packs Node 2's context to Node 2's real budget and reports what Node 2 actually used.

### Changed
- **The Node 2 budget matches Node 2.** The prompt budget is context − max(output reserve, `--max-tokens`) − template reserve.
  - The output reserve is new: `--output-reserve`, env `CLUSTER_DELIBERATE_OUTPUT_RESERVE`, default 512. It matches Node 2's `OutputReserve`.
  - Before, the tool subtracted only `max_tokens` (256).
  - Defaults give 4096 − 512 − 256 = **3328**, where it used to be 3456. `prompt_limit_tokens` is now 3584 and equals Node 2's `prompt_window_tokens`.
- **The template reserve default drops from 384 to 256.** That is the measured ~184-token Node 2 template plus a margin. An `.env` that still sets `CLUSTER_DELIBERATE_PROMPT_RESERVE=384` keeps 384.
- **The token estimator is now `heuristic-v2`, replacing `conservative-heuristic-v1`.**
  - An ASCII word costs 1 token for its first 4 letters and 1 per 3 letters after that. v1 charged 1 per 3 letters, so a common word such as "remember" cost 3.
  - Words split at lower-to-upper case changes.
  - Each newline now counts as a token. v1 counted a single newline as 0, which could underestimate.
  - Digits, punctuation and non-ASCII are priced as before.
  - The estimate is still monotonic in prefix length, so truncation stays exact.
- `env show` now always shows `deliberate_context_tokens`, `deliberate_output_reserve_tokens` and `deliberate_prompt_reserve_tokens`, resolved to their defaults when unset.
- The Node 2 deliberation deadline now includes the template reserve when sizing for the prompt.

### Added
- **Prepacked requests.** Each `orchestrate` deliberation sends `prepacked: true` and `prompt_budget_tokens` (the budget the tool packed into), so Node 2 doesn't cut the context a second time.
- **Node 2's side of `context_budget`.** It is in the `--full` output and the concise counts.
  - `node2_usage` is `reported` or `unknown`.
  - `node2_context_usage` is Node 2's `context_usage` object, unchanged.
  - `node2_second_cut` is `true` when Node 2 kept fewer chunks or facts than it received, or truncated any.
  - `output_reserve_tokens` appears in the `--full` report.
- **`within_budget` reflects Node 2's real prompt.** When Node 2 reports `context_usage`, it compares `actual_prompt_tokens` with Node 2's `prompt_window_tokens`.
  - Without `context_usage` (older Node 2), it falls back to `prompt_tokens` against the tool's prompt limit. The Node 2 counts are then omitted rather than zero.
  - `node2_second_cut` never changes status or exit code.
- Token calibration fixtures (`internal/orchestrator/testdata/token_calibration.json`). A unit test checks that the estimator never underestimates Node 2's count and stays within 1.3× in aggregate.

### Calibration (measured live)
- _Pending: estimated/actual ratios on the BEAM 100k_001 chunks are recorded here after the live run._

### Unchanged (harness compatibility)
- `stages[].stage_name` values, stage statuses (`success`/`failed`/`over_budget`), `status`/`loop_complete`/exit codes, and the concise top-level field order.
- Existing `context_budget` fields keep their names and meaning. New fields are only added.

## v1.0.12

### Changed
- **`orchestrate` output is concise by default.** It is a few KB for any input up to the 1 MiB cap; an 87 KB input used to produce about 227 KB. `status`, `is_complete`, `stages[]`, `final_thought`, `proposed_action`, `trace_id`, `session_id` and `total_duration_ms` come first, so stage outcomes fit in a 2 KB preview. `loop_complete` follows. `stages[]` entries carry `stage_name`, `status`, `error` and `duration_ms`. `sensory`, `recall`, `deliberation` and `consolidation` are summarised as counts, and the relevance gate as thresholds plus `nodes_in` / `nodes_kept` / `nodes_dropped`. Chunk text, recalled nodes and the per-node `kept[]`/`dropped[]` lists are gone from the default output.
- **`filter` output is counts-only by default**, with `chunks_omitted`. The chunk text was the input echoed back.
- **Honest status and exit code for `orchestrate`.** `status` is now `completed` only when no stage failed, `partial` when some stages failed and `failed` when all did (it used to be `completed` unconditionally). A partial or failed cycle exits `2`, with the JSON result still on stdout. Exit `1` still means the `{"status":"error",...}` shape.
- **Stage deadlines come from config instead of a payload-size formula.** `PayloadTimeout` (5s + 2s per 64 KiB) is removed. It gave 9s for an 87 KB synchronous consolidate and cut off a live run.
  - Consolidate (`consolidate`, `orchestrate` stage 4): `--consolidate-timeout` > `CLUSTER_CONSOLIDATE_TIMEOUT_MS` > default 120s.
  - Sensory filter (`filter`, `orchestrate` stage 1): `--sensory-timeout` > `CLUSTER_SENSORY_TIMEOUT_MS` > default 30s.
  - On `filter` and `consolidate`, `--timeout` also sets that deadline. On `orchestrate`, `--timeout` now affects only stage 2 recall, where it used to be the base for stages 1 and 4 as well.
  - A deadline error names the value used and where it came from (flag, OS env or `.env` path).
  - The Node 2 deliberation deadline is unchanged: `CLUSTER_DELIBERATE_TIMEOUT_MS` is a floor, widened for prompt size.

### Added
- `--full` on `orchestrate` and `filter` (operators only) restores the complete previous output.
- `loop_complete` on `orchestrate`: `true` only when all four stages report `success`. `is_complete` is unchanged and is still Node 2's deliberation flag.
- `CLUSTER_SENSORY_TIMEOUT_MS` and `CLUSTER_CONSOLIDATE_TIMEOUT_MS` in `--help`, `env init` and `env show` (the latter also shows each value's source).

### Fixed
- An unreadable or non-PEM TLS CA certificate now fails loudly with the file path and source, instead of silently falling back to system trust.

### Unchanged (harness compatibility)
- The `stages[].stage_name` values and `success`/`failed` statuses, one JSON object on stdout with diagnostics on stderr, the `outputError` shape, and all payload flag semantics (repeatable `--input`/`--text`/`--trace`, no-separator concatenation, the 1 MiB cap).
