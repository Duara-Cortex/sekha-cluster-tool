#!/bin/sh
# ==============================================================================
# sekha-cluster-tool Installer Script
# Deterministic, package-managed binary installer with SHA-256 verification.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Duara-Cortex/sekha-cluster-tool/main/scripts/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/Duara-Cortex/sekha-cluster-tool/main/scripts/install.sh | VERSION=v1.0.0 bash
# ==============================================================================

set -e

REPO="Duara-Cortex/sekha-cluster-tool"
TOOL_NAME="sekha-cluster-tool"
DEFAULT_VERSION="v1.0.0"

# Target installation directory
if [ -z "$INSTALL_DIR" ]; then
    if [ "$(id -u)" -eq 0 ]; then
        INSTALL_DIR="/usr/local/bin"
    else
        INSTALL_DIR="${HOME}/.local/bin"
    fi
fi

# Detect Operating System
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
    darwin) OS="darwin" ;;
    linux)  OS="linux" ;;
    *)
        echo "Error: Unsupported operating system '$OS'. Supported: darwin, linux" >&2
        exit 1
        ;;
esac

# Detect CPU Architecture
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *)
        echo "Error: Unsupported CPU architecture '$ARCH'. Supported: arm64, amd64" >&2
        exit 1
        ;;
esac

# Determine Version to install
VERSION="${VERSION:-$DEFAULT_VERSION}"

echo "==========================================================="
echo "   Installing ${TOOL_NAME} (${VERSION})"
echo "   Target Platform: ${OS}/${ARCH}"
echo "   Destination:     ${INSTALL_DIR}/${TOOL_NAME}"
echo "==========================================================="

TMP_DIR="$(mktemp -d -t sekha-install.XXXXXX)"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

TARBALL_NAME="${TOOL_NAME}_${VERSION}_${OS}_${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL_NAME}"
CHECKSUM_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"

# Download binary archive and checksums
echo "==> Downloading release artifact: ${DOWNLOAD_URL}..."
if ! curl -fsSL -o "${TMP_DIR}/${TARBALL_NAME}" "$DOWNLOAD_URL"; then
    echo "Warning: Direct release artifact download failed." >&2
    echo "Checking if source repository is available locally for compilation fallback..." >&2

    if command -v go >/dev/null 2>&1; then
        echo "==> Go compiler detected. Compiling from source..."

        # Resolve .env file
        ENV_PATH="${ENV_FILE:-${CLUSTER_ENV_FILE:-.env}}"
        if [ ! -f "$ENV_PATH" ] && [ -f "${HOME}/.config/sekha-cluster-tool/.env" ]; then
            ENV_PATH="${HOME}/.config/sekha-cluster-tool/.env"
        fi

        # Extract URLs from env file if not already provided in environment
        if [ -f "$ENV_PATH" ]; then
            if [ -z "$SENSORY_URL" ]; then
                SENSORY_URL="$(grep -E '^CLUSTER_SENSORY_URL=' "$ENV_PATH" | head -n 1 | cut -d= -f2- | tr -d '\r"' | tr -d "'")"
            fi
            if [ -z "$WORKING_URL" ]; then
                WORKING_URL="$(grep -E '^CLUSTER_WORKING_URL=' "$ENV_PATH" | head -n 1 | cut -d= -f2- | tr -d '\r"' | tr -d "'")"
            fi
            if [ -z "$KNOWLEDGE_URL" ]; then
                KNOWLEDGE_URL="$(grep -E '^CLUSTER_KNOWLEDGE_URL=' "$ENV_PATH" | head -n 1 | cut -d= -f2- | tr -d '\r"' | tr -d "'")"
            fi
        fi

        CONFIG_PKG="github.com/Duara-Cortex/sekha-cluster-tool/internal/config"
        BUILD_LDFLAGS="-s -w -X 'main.Version=${VERSION}'"
        if [ -n "$SENSORY_URL" ]; then
            BUILD_LDFLAGS="${BUILD_LDFLAGS} -X '${CONFIG_PKG}.BuildSensoryURL=${SENSORY_URL}'"
        fi
        if [ -n "$WORKING_URL" ]; then
            BUILD_LDFLAGS="${BUILD_LDFLAGS} -X '${CONFIG_PKG}.BuildWorkingURL=${WORKING_URL}'"
        fi
        if [ -n "$KNOWLEDGE_URL" ]; then
            BUILD_LDFLAGS="${BUILD_LDFLAGS} -X '${CONFIG_PKG}.BuildKnowledgeURL=${KNOWLEDGE_URL}'"
        fi

        SCRIPT_DIR="$(cd "$(dirname "$0")" 2>/dev/null && pwd || echo "")"
        REPO_ROOT=""
        if [ -n "$SCRIPT_DIR" ] && [ -f "${SCRIPT_DIR}/../go.mod" ]; then
            REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
        elif [ -f "./go.mod" ]; then
            REPO_ROOT="$(pwd)"
        fi

        if [ -n "$REPO_ROOT" ] && [ -d "${REPO_ROOT}/cmd/${TOOL_NAME}" ]; then
            (cd "$REPO_ROOT" && go build -ldflags="${BUILD_LDFLAGS}" -o "${INSTALL_DIR}/${TOOL_NAME}" "./cmd/${TOOL_NAME}") || {
                echo "Error: Local source compilation failed." >&2
                exit 1
            }
        else
            GOBIN="$INSTALL_DIR" go install -ldflags="${BUILD_LDFLAGS}" "github.com/${REPO}/cmd/${TOOL_NAME}@${VERSION}" || {
                echo "Error: Source compilation fallback failed." >&2
                exit 1
            }
        fi
        echo "==> Successfully installed ${TOOL_NAME} to ${INSTALL_DIR}/${TOOL_NAME}"
        exit 0
    else
        echo "Error: Release download failed and Go compiler is not installed." >&2
        echo "Please verify release tags at https://github.com/${REPO}/releases" >&2
        exit 1
    fi
fi

# Verify checksum if checksums.txt is available
echo "==> Verifying SHA-256 checksum..."
if curl -fsSL -o "${TMP_DIR}/checksums.txt" "$CHECKSUM_URL"; then
    cd "$TMP_DIR"
    if command -v sha256sum >/dev/null 2>&1; then
        grep "$TARBALL_NAME" checksums.txt | sha256sum -c - || {
            echo "Error: Checksum verification failed!" >&2
            exit 1
        }
    elif command -v shasum >/dev/null 2>&1; then
        grep "$TARBALL_NAME" checksums.txt | shasum -a 256 -c - || {
            echo "Error: Checksum verification failed!" >&2
            exit 1
        }
    else
        echo "Warning: sha256sum/shasum not found; skipping cryptographic verification."
    fi
    cd - >/dev/null
else
    echo "Notice: Remote checksums.txt not published for ${VERSION}; proceeding with caution."
fi

# Extract and install binary
echo "==> Extracting binary artifact..."
tar -xzf "${TMP_DIR}/${TARBALL_NAME}" -C "$TMP_DIR"

mkdir -p "$INSTALL_DIR"
EXTRACTED_BIN="$(find "$TMP_DIR" -type f -name "${TOOL_NAME}*" ! -name "*.tar.gz" | head -n 1)"

if [ -z "$EXTRACTED_BIN" ]; then
    echo "Error: Failed to locate extracted ${TOOL_NAME} binary." >&2
    exit 1
fi

cp "$EXTRACTED_BIN" "${INSTALL_DIR}/${TOOL_NAME}"
chmod 755 "${INSTALL_DIR}/${TOOL_NAME}"

echo "==========================================================="
echo "   Successfully installed ${TOOL_NAME} to:"
echo "   ${INSTALL_DIR}/${TOOL_NAME}"
echo "==========================================================="

# Copy .env to bin folder if valid, or prompt user
if [ -f ".env" ]; then
    SENSORY="$(grep -E '^CLUSTER_SENSORY_URL=' .env 2>/dev/null | cut -d= -f2- | tr -d '\r"' | tr -d "'")"
    WORKING="$(grep -E '^CLUSTER_WORKING_URL=' .env 2>/dev/null | cut -d= -f2- | tr -d '\r"' | tr -d "'")"
    KNOWLEDGE="$(grep -E '^CLUSTER_KNOWLEDGE_URL=' .env 2>/dev/null | cut -d= -f2- | tr -d '\r"' | tr -d "'")"

    if [ -n "$SENSORY" ] && [ -n "$WORKING" ] && [ -n "$KNOWLEDGE" ] && \
       ! echo "$SENSORY" | grep -q "<" && ! echo "$WORKING" | grep -q "<" && ! echo "$KNOWLEDGE" | grep -q "<"; then
        cp ".env" "${INSTALL_DIR}/.env"
        echo "✓ Copied verified .env to ${INSTALL_DIR}/.env"
    else
        echo "⚠️  Local .env is incomplete. Run './scripts/setup-env.sh' to configure your endpoints."
    fi
else
    echo "ℹ️  No .env file found. Run './scripts/setup-env.sh' to configure your endpoints."
fi

# Check PATH
case ":$PATH:" in
    *:"$INSTALL_DIR":*) ;;
    *)
        echo ""
        echo "Note: '${INSTALL_DIR}' is not currently in your \$PATH."
        echo "Add it by running:"
        echo "  export PATH=\"\$PATH:${INSTALL_DIR}\""
        echo ""
        ;;
esac

"${INSTALL_DIR}/${TOOL_NAME}" version
