import { test, expect } from "@microsoft/tui-test";
import { tcxBinary, fullAutoArgs, EXPECT_TIMEOUT } from "./helpers.js";

// Ask the LLM to call update_plan with 3 specific steps, then say a canary word.
// --full-auto auto-approves any tool calls.
test.use({
  program: {
    file: tcxBinary,
    args: [
      ...fullAutoArgs,
      "-m",
      'Call the update_plan tool with explanation "Working on it" and exactly 3 steps: ' +
        '1) "Read existing code" (completed), ' +
        '2) "Write migration script" (in_progress), ' +
        '3) "Run tests" (pending). ' +
        "After calling update_plan, say exactly: plan7749done",
    ],
  },
  rows: 30,
  columns: 120,
});

test("displays plan header in viewport", async ({ terminal }) => {
  // Wait for the canary word confirming the LLM completed
  await expect(
    terminal.getByText(/plan7749done/gi, { full: true, strict: false })
  ).toBeVisible({ timeout: EXPECT_TIMEOUT });

  // Plan header should be rendered
  await expect(
    terminal.getByText(/Plan/g, { full: true, strict: false })
  ).toBeVisible({ timeout: EXPECT_TIMEOUT });
});

test("displays Updated verb for update_plan tool call", async ({ terminal }) => {
  await expect(
    terminal.getByText(/plan7749done/gi, { full: true, strict: false })
  ).toBeVisible({ timeout: EXPECT_TIMEOUT });

  // The formatToolCall renders "Updated" as the verb for update_plan
  await expect(
    terminal.getByText(/Updated/g, { full: true, strict: false })
  ).toBeVisible({ timeout: EXPECT_TIMEOUT });
});
