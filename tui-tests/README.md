# TUI E2E Tests

End-to-end tests for the `tcx` terminal UI using [@microsoft/tui-test](https://github.com/microsoft/tui-test).

These tests spawn `tcx` in a real PTY via node-pty, render output through xterm.js, and run assertions against the terminal buffer.

## Prerequisites

- **Node.js** >= 16
- **Go** >= 1.24
- **Temporal CLI** (`temporal` in PATH or `~/.temporalio/bin/temporal`)
- **LLM API key**: `OPENAI_API_KEY` or `ANTHROPIC_API_KEY` set in environment

## Running

```bash
cd tui-tests
./run.sh
```

The script automatically:
1. Builds `tcx` and `worker` binaries
2. Starts a Temporal dev server on port 18233
3. Starts the worker process
4. Runs all `*.test.ts` files via tui-test
5. Tears down services on exit

## Adding Tests

Create a new `*.test.ts` file:

```ts
import { test, expect } from "@microsoft/tui-test";

test.use({
  program: {
    file: process.env.TCX_BINARY || "../tcx",
    args: ["--temporal-host", process.env.TEMPORAL_HOST || "localhost:18233", ...],
  },
  rows: 30,
  columns: 120,
});

test("description", async ({ terminal }) => {
  await expect(terminal.getByText(/pattern/)).toBeVisible();
});
```

## Debugging

Failed test traces are saved to `tui-traces/`. These contain terminal snapshots that can be replayed to diagnose failures.
