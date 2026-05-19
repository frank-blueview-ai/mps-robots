const { defineConfig } = require("@playwright/test");

module.exports = defineConfig({
  testDir: "./tests",
  timeout: 45_000,
  expect: {
    timeout: 10_000,
  },
  use: {
    baseURL: "http://localhost:8080",
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
  },
  webServer: {
    command: "powershell -ExecutionPolicy Bypass -File .\\scripts\\start-nginx.ps1",
    url: "http://localhost:8080",
    reuseExistingServer: true,
    timeout: 15_000,
  },
});
