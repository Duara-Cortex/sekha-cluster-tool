# Changelog

## v1.0.11

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
