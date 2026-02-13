#!/usr/bin/env bash
#
# TUI E2E test runner for temporal-agent-harness.
#
# Builds tcx + worker, starts Temporal dev server and worker,
# runs tui-test, then tears everything down.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TEMPORAL_PORT=18233
TEMPORAL_UI_PORT=18234

# PIDs to clean up
TEMPORAL_PID=""
WORKER_PID=""

cleanup() {
  echo ""
  echo "==> Cleaning up..."
  if [ -n "$WORKER_PID" ] && kill -0 "$WORKER_PID" 2>/dev/null; then
    echo "    Stopping worker (PID $WORKER_PID)"
    kill "$WORKER_PID" 2>/dev/null || true
    wait "$WORKER_PID" 2>/dev/null || true
  fi
  if [ -n "$TEMPORAL_PID" ] && kill -0 "$TEMPORAL_PID" 2>/dev/null; then
    echo "    Stopping Temporal dev server (PID $TEMPORAL_PID)"
    kill "$TEMPORAL_PID" 2>/dev/null || true
    wait "$TEMPORAL_PID" 2>/dev/null || true
  fi
  echo "==> Cleanup complete"
}
trap cleanup EXIT

# --- 1. Check prerequisites ---

echo "==> Checking prerequisites..."

if [ -z "${OPENAI_API_KEY:-}" ] && [ -z "${ANTHROPIC_API_KEY:-}" ]; then
  echo "ERROR: At least one LLM API key required: OPENAI_API_KEY or ANTHROPIC_API_KEY"
  exit 1
fi

# Find temporal CLI
TEMPORAL_BIN=""
if command -v temporal &>/dev/null; then
  TEMPORAL_BIN="temporal"
elif [ -x "$HOME/.temporalio/bin/temporal" ]; then
  TEMPORAL_BIN="$HOME/.temporalio/bin/temporal"
else
  echo "ERROR: temporal CLI not found. Install from https://docs.temporal.io/cli"
  exit 1
fi
echo "    temporal CLI: $TEMPORAL_BIN"

if ! command -v go &>/dev/null; then
  echo "ERROR: go not found in PATH"
  exit 1
fi

if ! command -v node &>/dev/null; then
  echo "ERROR: node not found in PATH"
  exit 1
fi

# --- 2. Build binaries ---

echo "==> Building tcx binary..."
(cd "$PROJECT_ROOT" && go build -o "$PROJECT_ROOT/tcx" ./cmd/tcx)
echo "    Built: $PROJECT_ROOT/tcx"

echo "==> Building worker binary..."
(cd "$PROJECT_ROOT" && go build -o "$PROJECT_ROOT/worker" ./cmd/worker)
echo "    Built: $PROJECT_ROOT/worker"

# --- 3. Start Temporal dev server ---

echo "==> Starting Temporal dev server on port $TEMPORAL_PORT..."
$TEMPORAL_BIN server start-dev \
  --port "$TEMPORAL_PORT" \
  --ui-port "$TEMPORAL_UI_PORT" \
  --headless \
  --log-format json \
  --log-level error \
  &>/dev/null &
TEMPORAL_PID=$!
echo "    Temporal PID: $TEMPORAL_PID"

# Wait for Temporal to be ready (TCP probe)
echo "==> Waiting for Temporal to be ready..."
for i in $(seq 1 30); do
  if bash -c "echo >/dev/tcp/localhost/$TEMPORAL_PORT" 2>/dev/null; then
    echo "    Temporal ready after ${i}s"
    break
  fi
  if [ "$i" -eq 30 ]; then
    echo "ERROR: Temporal did not start within 30s"
    exit 1
  fi
  sleep 1
done

# --- 4. Start worker ---

echo "==> Starting worker..."
TEMPORAL_HOST_URL="localhost:$TEMPORAL_PORT" \
  "$PROJECT_ROOT/worker" &>/dev/null &
WORKER_PID=$!
echo "    Worker PID: $WORKER_PID"

# Give worker a moment to register with Temporal
sleep 2

# Verify worker is still running
if ! kill -0 "$WORKER_PID" 2>/dev/null; then
  echo "ERROR: Worker exited prematurely"
  exit 1
fi

# --- 5. Install npm dependencies (if needed) ---

if [ ! -d "$SCRIPT_DIR/node_modules" ]; then
  echo "==> Installing npm dependencies..."
  (cd "$SCRIPT_DIR" && npm install)
fi

# --- 6. Run tui-test ---

echo "==> Running TUI tests..."
echo ""

export TCX_BINARY="$PROJECT_ROOT/tcx"
export TEMPORAL_HOST="localhost:$TEMPORAL_PORT"

cd "$SCRIPT_DIR"
set +e
npx @microsoft/tui-test
TEST_EXIT=$?
set -e

echo ""
if [ $TEST_EXIT -eq 0 ]; then
  echo "==> All TUI tests passed!"
else
  echo "==> TUI tests failed (exit code: $TEST_EXIT)"
fi

exit $TEST_EXIT
