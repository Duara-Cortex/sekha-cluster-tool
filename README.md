# sekha-cluster-tool

Deterministic, compiled agent orchestration harness and CLI for **Tripartite Cognitive Clusters** (Sensory Attention Gate, Working Scratchpad Memory, and Long-Term Knowledge Graph Store).

`sekha-cluster-tool` is completely decoupled from any specific deployment environment or physical hardware IP addresses. By default, all node endpoint addresses are **blank (`""`)** and must be specified by the machine building it or configured at runtime via `.env` files, real OS environment variables, or CLI flags.

---

## 🏛️ Cognitive Layers & Architecture

The tool coordinates four discrete stages across three distributed cognitive layers:

```
[Raw Sensory Input / Stream]
              │
              ▼
   Stage 1: Sensory Attention Gate (:8081)
   Lightweight CPU entropy/density salience gating (<1ms)
              │
              ▼
   Stage 2: Long-Term Knowledge Store (:8084)
   Associative recall: top-k vector similarity + recency decay
              │
              ▼
   Stage 3: Working Memory Scratchpad (:8083)
   Multi-step deliberation with local SLM inference
              │
              ▼
   Stage 4: Memory Consolidation & Decay (:8084/:8085)
   Episodic trace commitment for graph fusion and Hebbian decay
```

---

## ⚙️ Configuration & Precedence

All endpoint addresses default to blank (`""`). Configuration is resolved using the 12-factor standard with the following precedence order:

1. **CLI Flags** (`--sensory-url`, `--working-url`, `--knowledge-url`, `--api-key`)
2. **Real OS Environment Variables** (`CLUSTER_SENSORY_URL`, `CLUSTER_WORKING_URL`, `CLUSTER_KNOWLEDGE_URL`, `CLUSTER_API_KEY`)
3. **Environment File** (`.env`, `.env.local`, `--env-file <path>`, or `~/.config/sekha-cluster-tool/.env`)
4. **Compile-time Builder Injection** (`make build SENSORY_URL=...`)

### Generating a Starter `.env` File
To generate a blank starter `.env` template in the current directory:

```bash
sekha-cluster-tool env init
```

This creates `.env` with all addresses defaulting to blank:
```env
# ==============================================================================
# Tri-Node Cognitive Cluster Environment Configuration (.env)
# All endpoint addresses default to blank ("") and must be configured by the machine.
# ==============================================================================

# Sensory Attention Layer (e.g. http://<sensory-node>:8081)
CLUSTER_SENSORY_URL=

# Working Memory Scratchpad Layer (e.g. http://<working-node>:8083)
CLUSTER_WORKING_URL=

# Long-Term Knowledge Graph Layer (e.g. http://<knowledge-node>:8084)
CLUSTER_KNOWLEDGE_URL=

# Authentication (optional API key for Node 1 protected endpoints; fallback: SEKHA_API_KEY)
CLUSTER_API_KEY=

# Timeout Budgets (milliseconds)
CLUSTER_DEFAULT_TIMEOUT_MS=1500
CLUSTER_DELIBERATE_TIMEOUT_MS=8000
# Node 3 filter deadline (filter, orchestrate stage 1; default 30000)
CLUSTER_SENSORY_TIMEOUT_MS=30000
# Node 1 consolidate deadline (consolidate, orchestrate stage 4; default 120000)
CLUSTER_CONSOLIDATE_TIMEOUT_MS=120000

# Attention & Recall Parameters
CLUSTER_SALIENCE_THRESHOLD=0.45
CLUSTER_RECALL_TOP_K=5
CLUSTER_RECALL_MIN_SIM=0.50

# Payload cap & Node 2 context budget
CLUSTER_MAX_INPUT_BYTES=1048576
CLUSTER_DELIBERATE_CONTEXT_TOKENS=4096
CLUSTER_DELIBERATE_PROMPT_RESERVE=384

# Transport Encryption & TLS (optional)
# CLUSTER_TLS_CA_CERT=/path/to/ca.crt
# CLUSTER_INSECURE=false
```

To inspect the actively resolved configuration:
```bash
sekha-cluster-tool env show
sekha-cluster-tool env show --env-file /path/to/.env
```

---

## 📦 Building & Installation

### Option 1: Build with Blank Defaults (Runtime Configured via `.env`)
```bash
git clone https://github.com/Duara-Cortex/sekha-cluster-tool.git
cd sekha-cluster-tool

make test
make build
make install
```

### Option 2: Build with Baked-In Addresses (Configured by the Machine Building It)
The machine building the binary can optionally bake in endpoint addresses at compile time via make variables:

```bash
make build SENSORY_URL="http://<sensory-node>:8081" \
           WORKING_URL="http://<working-node>:8083" \
           KNOWLEDGE_URL="http://<knowledge-node>:8084"
```

### Option 3: Release Installer (`curl`)
```bash
curl -fsSL https://raw.githubusercontent.com/Duara-Cortex/sekha-cluster-tool/main/scripts/install.sh | bash
```

---

## 🛠️ Subcommands & Usage

All subcommands output **strict, formatted JSON to `stdout`**, allowing direct deserialisation by agent runtimes and test harnesses. Diagnostic step logs are routed to `stderr` when `--verbose` is specified.

### 1. Cluster Status & Terminal Dashboard (`status`)
Probes all three cluster layers concurrently (500ms default timeout budget) and renders a human-readable ASCII dashboard by default:

```bash
sekha-cluster-tool status
```

**Terminal Dashboard Output:**
```text
======================================================================
              Sekha Tri-Node Edge Cognitive Cluster Status            
======================================================================
Cluster State: ALL NODES HEALTHY (3/3 Online) | Trace ID: trc-abc123
Config Source: /home/admin/.env

[●] Node 3: Sensory Layer (http://192.168.8.183:8081)
    • Status:       HEALTHY (9.8 ms)
    • Buffer Usage: 10.3 MB / 64.0 MB (16.2% fill)
    • Throughput:   50,386 ingested | 0 dropped
    • Gating:       30,503 salient / 30,836 evaluated (1.1% noise reduced)

[●] Node 2: Working Memory Layer (http://192.168.8.175:8083)
    • Status:       HEALTHY (10.9 ms)
    • SLM Engine:   REACHABLE (llama-server :8082)
    • Service:      sekha-working-scratchpad (uptime: 4d 14h)

[●] Node 1: Long-Term Knowledge Layer (http://192.168.8.213:8084)
    • Status:       HEALTHY (10.4 ms)
    • Graph Scale:  229 nodes | 882 relational edges
    • Embedder:     REACHABLE (384-D :8086)
    • Service:      sekha-knowledge-store (uptime: 1d 02h)
======================================================================
```

If a node is offline or degraded, it displays `[✗] OFFLINE (<latency> ms)` with the exact connection error and sets cluster state to `DEGRADED (2/3 Online)`.

**Machine JSON Output (`--json` or `--format json`):**
```bash
sekha-cluster-tool status --json
```

### 2. Preflight Connectivity Ping (`ping`)
Lightweight preflight connectivity check across all three layers running concurrently. Exits with **code 0** if all nodes respond, or **code 1** if any node is unreachable:

```bash
sekha-cluster-tool ping
```

**Terminal Output:**
```text
[OK] Node 3 (Sensory)    - 9.8ms   (http://192.168.8.183:8081)
[OK] Node 2 (Working)    - 11.2ms  (http://192.168.8.175:8083)
[OK] Node 1 (Knowledge)  - 10.4ms  (http://192.168.8.213:8084)
Cluster: HEALTHY (3/3 nodes online)
```

**JSON Output (`--json`):**
```bash
sekha-cluster-tool ping --json
```
```json
{
  "all_healthy": true,
  "nodes": {
    "sensory": { "reachable": true, "ping_ms": 9.8, "url": "http://192.168.8.183:8081" },
    "working": { "reachable": true, "ping_ms": 11.2, "url": "http://192.168.8.175:8083" },
    "knowledge": { "reachable": true, "ping_ms": 10.4, "url": "http://192.168.8.213:8084" }
  },
  "timestamp": "2026-09-24T18:50:00Z"
}
```

### 3. Sensory Filtering (`filter`)
Dispatches raw text or queries an in-memory buffer:

```bash
sekha-cluster-tool filter \
  --text "Raw telemetry string or syslog line" \
  --directive "Identify hardware faults" \
  --threshold 0.45
```

By default `filter` prints counts only (`total_chunks`, `salient_chunks`, `noise_discarded`, `reduction_rate`, `latency_ms`, `chunks_omitted`), because the chunk text is the input split up and would make the output as large as the input. Pass `--full` to get every chunk with its text and salience.

### 4. Associative Recall (`recall`)
Queries the relational knowledge graph for contextual entities (returns lean schema without embeddings by default):

```bash
sekha-cluster-tool recall \
  --query "Hardware failure policies" \
  --top-k 3 \
  -a "#project:kestrel" \
  --anchor-mode boost \
  --include-embeddings
```

### 5. Working Deliberation (`deliberate`)
Invokes the working memory scratchpad to formulate a reasoning step:

```bash
sekha-cluster-tool deliberate \
  --task "Mitigate hardware fault" \
  --input "Critical core thermal warning" \
  --context "[policy: Thermal Policy] Threshold 70C triggers auxiliary fan override"
```

### 6. Episodic Consolidation (`consolidate`)
Commits completed deliberation traces for background Hebbian reinforcement and decay.

**Positional Shortcut (Quick Memorisation):**
Persist facts directly without manual JSON escaping or `--trace`:
```bash
sekha-cluster-tool consolidate "<label>" "<summary>" [--anchor "<anchor>"] [--sync]

# Example:
sekha-cluster-tool consolidate "Project Kestrel" "INGEST_PORT: 51742; AUTH_HEADER: X-Kestrel-Key" \
  -a "#project:kestrel" \
  --sync
```

**Full Episodic Trace Mode:**
```bash
sekha-cluster-tool consolidate \
  --trace '{"session_id":"sess-101","sensory_context":[...]}' \
  -a "#project:kestrel" \
  --sync
```
Or via flag-based trace metadata:
```bash
sekha-cluster-tool consolidate \
  --goal "Mitigate hardware fault" \
  --outcome "success" \
  --session-id "sess-101" \
  -a "#project:kestrel" \
  --sync
```

### 7. Closed-Loop Cognitive Cycle (`orchestrate`)
Coordinates the entire 4-stage loop in a single command, collecting stage telemetry. Supports `--task` as an alias for `--directive`:

```bash
sekha-cluster-tool orchestrate \
  --input "syslog telemetry stream" \
  --task "Investigate and resolve alert" \
  -a "#project:kestrel" \
  --sync
```

#### Output, status and exit code
The default output is concise: a few KB for any input up to the 1 MiB cap. These fields come first, in this order:

| Field | Meaning |
|---|---|
| `status` | `completed` (no stage failed), `partial` (some stages failed), `failed` (every stage failed) |
| `is_complete` | Node 2's own deliberation flag. It does **not** mean the loop completed. |
| `stages[]` | `stage_name`, `status` (`success` / `failed`, or `over_budget` for stage 3), `error`, `duration_ms` |
| `final_thought`, `proposed_action` | Node 2's step |
| `trace_id`, `session_id`, `total_duration_ms` | |
| `loop_complete` | `true` only when all four stages report `success` |

Then come counts-only summaries: `sensory` (chunk counts, reduction rate, latency; no chunk text), `recall` (node and edge counts, latency, and the relevance gate's thresholds and `nodes_in` / `nodes_kept` / `nodes_dropped`), `deliberation` (status, token counts and rates, context-budget counts) and `consolidation` (status, `synchronous`, entity/node/edge counts, message). Error and message text is capped at 300 bytes.

`--full` (for operators) prints the complete response instead: chunk text, recalled nodes, the per-node `kept[]`/`dropped[]` gate decisions and the packing decisions.

Exit codes: `0` when `status` is `completed`, `2` when it is `partial` or `failed` (the JSON result is still printed on stdout), and `1` for errors that stop the cycle before it runs (the `{"status":"error",...}` shape). A stage 3 `over_budget` result counts as having run: `status` stays `completed` and the exit code is 0, but `loop_complete` is `false`.

#### Stage deadlines
Each stage takes its deadline from config (flag > OS env > `.env` > default):

| Stage | Setting | Default |
|---|---|---|
| 1 sensory filter | `--sensory-timeout`, `CLUSTER_SENSORY_TIMEOUT_MS` | 30s |
| 2 recall | `--timeout`, `CLUSTER_DEFAULT_TIMEOUT_MS` | 1.5s |
| 3 deliberate | `CLUSTER_DELIBERATE_TIMEOUT_MS` (floor, widened for prompt size and `--max-tokens`) | 8s |
| 4 consolidate | `--consolidate-timeout`, `CLUSTER_CONSOLIDATE_TIMEOUT_MS` | 120s |

Standalone `filter` and `consolidate` use the same sensory and consolidate settings, and there `--timeout` sets that deadline too. When a deadline expires, the error names the value and where it came from, e.g. `consolidate deadline of 9s exceeded (set by CLUSTER_CONSOLIDATE_TIMEOUT_MS in .env file (/path/.env); ...)`.

#### Relevance gating (Stage 2 → Stage 3)
Recalled nodes pass to Node 2 only if all of these hold:
- `sim_score` ≥ `--min-sim` (default `0.50`, env `CLUSTER_RECALL_MIN_SIM`). The blended `score` is not used, because recency and anchor boosts lift the episode the previous cycle just consolidated. Nodes with no `sim_score` (anchor-only hits in Node 1's lean schema) are admitted only if they match a requested anchor and pass the term check below.
- When `-a` anchors are given and the node lists anchors, at least one of them matches.
- The node's distinctive terms overlap the current input. At least 2 terms and 15% of the node's terms must appear in the input, and the directive's own words don't count toward this.

The recall query is the directive plus an excerpt of up to 512 runes from the most salient chunk. With `--full`, `stages[1].relevance_gate` lists every kept and dropped node with its `sim_score`, term overlap and drop reason (`missing_sim_score`, `below_sim_floor`, `anchor_mismatch`, `insufficient_term_overlap`). `recall.nodes` in the output contains only the kept nodes.

#### Node 2 context budget (Stage 3)
The prompt budget is `--context-tokens` (default `4096`, env `CLUSTER_DELIBERATE_CONTEXT_TOKENS`) minus `--max-tokens`, minus `--prompt-reserve` (default `384`, env `CLUSTER_DELIBERATE_PROMPT_RESERVE`) for the Node 2 template. Tokens are estimated conservatively, erring high: every digit, symbol and non-ASCII byte pair counts as one token. Recalled facts get at most a quarter of the budget, ranked by `sim_score`. Chunks fill the rest in salience order. The first chunk that doesn't fit is truncated and lower ranked chunks are dropped. Kept chunks go to Node 2 in their original order.

With `--full`, `stages[2].context_budget` reports the budget, the estimated prompt tokens, Node 2's reported `prompt_tokens`, `within_budget`, and each truncation or drop with its token counts. If the objective alone can't fit, nothing is sent to Node 2 and the stage fails with an explicit error. Stage 4 still consolidates the full, untruncated sensory stream.

---

## 📦 Large Payloads

`--input` / `--text` (on `filter`, `orchestrate` and `consolidate`) and `consolidate --trace` are **repeatable**. On `consolidate`, `--input` is stored as a `sensory_context` item alongside `--goal` / `--session-id`. Occurrences are joined in order with no separator, so a payload can be split at any byte offset:

```bash
sekha-cluster-tool orchestrate --task "Answer from the transcript" \
  --input "$PART1" --input "$PART2" --input "$PART3"
```

- **OS limits:** Linux caps a single argument at 128 KiB (`MAX_ARG_STRLEN`) and rejects larger ones before the CLI starts, so split anything larger into ≤120 KiB parts. macOS allows a single argument of 256 KB+ but caps all arguments plus the environment at about 1 MiB (`ARG_MAX`).
- **Input cap:** the combined input from inline flags, `--file` or stdin (`--file -`) is capped at 1 MiB. Raise it with `--max-input-bytes` or `CLUSTER_MAX_INPUT_BYTES`. Input over the cap fails with an actionable error before anything is sent. It is **never truncated**.
- **`--file`** is an optional convenience for human operators. It can't be combined with inline input.
- **Timeouts:** the Node 3 filter and Node 1 consolidate deadlines come from `CLUSTER_SENSORY_TIMEOUT_MS` and `CLUSTER_CONSOLIDATE_TIMEOUT_MS` (see [Stage deadlines](#stage-deadlines)); set them in `.env` for your cluster and payload sizes. The Node 2 deadline scales with the packed prompt size and `--max-tokens`.

---

## 🌐 Distributed Tracing (`X-Trace-ID`)

Every request automatically injects an `X-Trace-ID` (e.g. `trc-a1b2c3d4e5f60718`) into upstream HTTP headers. Specify an explicit trace ID to correlate cluster operations across logs:

```bash
sekha-cluster-tool orchestrate \
  --input "sensor data" \
  --trace-id "turn-98765" \
  --verbose
```

---

## 🔒 Transport Layer Security (TLS)

When connecting to HTTPS cluster endpoints (e.g. Node 1 using a private or self-signed CA certificate), configure the root CA certificate to establish trust:

- **Flag:** `--tls-ca-cert <path>`
- **Environment:** `CLUSTER_TLS_CA_CERT=<path>`

### Certificate Validation & Diagnostics
If a TLS CA certificate path is configured, `sekha-cluster-tool` validates the certificate file upfront and **fails loudly** with actionable diagnostics if:
1. **The file is missing or unreadable:** the error reports the configured path, the resolved absolute path (if relative), the configuration source (flag, environment variable, or `.env` file), and the exact OS error (`no such file or directory`, `permission denied`, etc.).
2. **The file is a directory:** the error reports `read <path>: is a directory`.
3. **The file contains no valid PEM certificates:** the error reports that the file contains no valid PEM certificates and suggests inspecting the certificate with:
   ```bash
   openssl x509 -in <path> -noout -subject
   ```

In `status` and `ping`, the CA failure is surfaced directly on the affected HTTPS node in both the terminal dashboard and `--json` machine output instead of a generic OFFLINE/x509 verification error. All other subcommands (`filter`, `recall`, `deliberate`, `consolidate`, `orchestrate`) fail immediately with structured JSON error output.

### Insecure Verification & System Trust
- **No CA configured:** system root certificates are used by default.
- **Bypass verification:** pass `--insecure` or set `CLUSTER_INSECURE=true` to skip TLS certificate verification (`InsecureSkipVerify: true`) for testing or development environments.

---

## 📄 Licence

Apache 2.0. Authored by the **Duara Cortex** team.

