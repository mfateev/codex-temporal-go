import { test, expect } from "@microsoft/tui-test";

const tcxBinary = process.env.TCX_BINARY || "../tcx";
const temporalHost = process.env.TEMPORAL_HOST || "localhost:18233";

// Explicit timeout for expect matchers — tui-test's config-based timeout
// does not propagate to worker processes in this version.
const EXPECT_TIMEOUT = 60_000;

test.use({
  program: {
    file: tcxBinary,
    args: [
      "--temporal-host", temporalHost,
      "--full-auto",
      "--model", "gpt-4o-mini",
      "--no-color",
      "-m", "Say exactly the word: pineapple",
    ],
  },
  rows: 30,
  columns: 120,
});

test("tcx starts session and displays LLM response", async ({ terminal }) => {
  // TUI should render and start a session
  await expect(
    terminal.getByText(/Started session/g, { full: true, strict: false })
  ).toBeVisible({ timeout: EXPECT_TIMEOUT });

  // LLM should respond with the word "pineapple" somewhere in the output
  await expect(
    terminal.getByText(/pineapple/gi, { full: true, strict: false })
  ).toBeVisible({ timeout: EXPECT_TIMEOUT });
});
