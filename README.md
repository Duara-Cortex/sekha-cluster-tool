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

# Attention & Recall Parameters
CLUSTER_SALIENCE_THRESHOLD=0.45
CLUSTER_RECALL_TOP_K=5
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

### 1. Cluster Status & Latency Probe (`status`)
Checks reachability and response latency across configured cluster endpoints:

```bash
sekha-cluster-tool status
# Or with explicit parameters:
sekha-cluster-tool status \
  --sensory-url "http://<sensory-node>:8081" \
  --working-url "http://<working-node>:8083" \
  --knowledge-url "http://<knowledge-node>:8084"
```

### 2. Sensory Filtering (`filter`)
Dispatches raw text or queries an in-memory buffer:

```bash
sekha-cluster-tool filter \
  --text "Raw telemetry string or syslog line" \
  --directive "Identify hardware faults" \
  --threshold 0.45
```

### 3. Associative Recall (`recall`)
Queries the relational knowledge graph for contextual entities (returns lean schema without embeddings by default):

```bash
sekha-cluster-tool recall \
  --query "Hardware failure policies" \
  --top-k 3 \
  -a "#project:kestrel" \
  --anchor-mode boost \
  --include-embeddings
```

### 4. Working Deliberation (`deliberate`)
Invokes the working memory scratchpad to formulate a reasoning step:

```bash
sekha-cluster-tool deliberate \
  --task "Mitigate hardware fault" \
  --input "Critical core thermal warning" \
  --context "[policy: Thermal Policy] Threshold 70C triggers auxiliary fan override"
```

### 5. Episodic Consolidation (`consolidate`)
Commits completed deliberation traces for background Hebbian reinforcement and decay:

```bash
sekha-cluster-tool consolidate \
  --goal "Mitigate hardware fault" \
  --outcome "success" \
  --session-id "sess-101" \
  -a "#project:kestrel" \
  --sync
```

### 6. Closed-Loop Cognitive Cycle (`orchestrate`)
Coordinates the entire 4-stage loop in a single command, collecting stage telemetry:

```bash
sekha-cluster-tool orchestrate \
  --input "syslog telemetry stream" \
  --directive "Investigate and resolve alert" \
  -a "#project:kestrel" \
  --sync
```

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

## 📄 Licence

Apache 2.0. Authored by the **Duara Cortex** team.
