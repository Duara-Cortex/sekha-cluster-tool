#!/bin/sh
# ==============================================================================
# setup-env.sh
# Interactive prompt script to configure .env for the machine building/running
# the Tri-Node Cognitive Cluster Tool.
# ==============================================================================

set -e

ENV_FILE="${1:-.env}"

echo "==========================================================="
echo "   Tri-Node Cognitive Cluster Environment Setup"
echo "   Configuring destination: ${ENV_FILE}"
echo "==========================================================="
echo "Please enter the endpoint addresses for each cognitive layer."
echo "Addresses can be local to your machine (localhost), on your local network,"
echo "or remote across the internet. Press [Enter] to leave blank if unconfigured."
echo ""

# Sensory Layer
printf "Enter Sensory Layer URL (e.g. path to your sensory layer): "
read -r SENSORY_URL

# Working Memory Layer
printf "Enter Working Memory Layer URL (e.g. path to your working memory layer): "
read -r WORKING_URL

# Knowledge Store Layer
printf "Enter Long-Term Knowledge Layer URL (e.g. path to your knowledge store layer): "
read -r KNOWLEDGE_URL

echo ""
echo "--- Timing & Cognitive Defaults (press Enter to accept defaults) ---"

printf "Default Timeout in milliseconds [1500]: "
read -r TIMEOUT_MS
TIMEOUT_MS="${TIMEOUT_MS:-1500}"

printf "Deliberation Timeout in milliseconds [8000]: "
read -r DELIB_MS
DELIB_MS="${DELIB_MS:-8000}"

printf "Salience Attention Threshold [0.45]: "
read -r THRESHOLD
THRESHOLD="${THRESHOLD:-0.45}"

printf "Recall Top-K Graph Nodes [5]: "
read -r TOP_K
TOP_K="${TOP_K:-5}"

cat <<EOF > "$ENV_FILE"
# ==============================================================================
# Tri-Node Cognitive Cluster Environment Configuration (.env)
# Generated interactively by make env on $(date -u +"%Y-%m-%dT%H:%M:%SZ")
# ==============================================================================

# Sensory Attention Layer
CLUSTER_SENSORY_URL=${SENSORY_URL}

# Working Memory Scratchpad Layer
CLUSTER_WORKING_URL=${WORKING_URL}

# Long-Term Knowledge Graph Store Layer
CLUSTER_KNOWLEDGE_URL=${KNOWLEDGE_URL}

# Timeout Budgets (milliseconds)
CLUSTER_DEFAULT_TIMEOUT_MS=${TIMEOUT_MS}
CLUSTER_DELIBERATE_TIMEOUT_MS=${DELIB_MS}

# Attention & Recall Parameters
CLUSTER_SALIENCE_THRESHOLD=${THRESHOLD}
CLUSTER_RECALL_TOP_K=${TOP_K}
EOF

echo ""
echo "==========================================================="
echo "   Successfully created ${ENV_FILE}!"
echo "   You can inspect your active configuration with:"
echo "     ./bin/sekha-cluster-tool env show"
echo "==========================================================="
