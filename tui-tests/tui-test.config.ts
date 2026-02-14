import { defineConfig } from "@microsoft/tui-test";

export default defineConfig({
  timeout: 120_000,
  expect: { timeout: 60_000 },
  retries: 1,
  trace: true,
  workers: 1,
});
