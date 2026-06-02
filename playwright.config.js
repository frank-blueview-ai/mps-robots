const { defineConfig } = require("@playwright/test");

const webServerKind = process.env.ORA_WEB_SERVER || "go";
const webServerCommand = webServerKind === "go"
  ? "powershell -ExecutionPolicy Bypass -File .\\scripts\\start-go-server.ps1 start"
  : "powershell -ExecutionPolicy Bypass -File .\\scripts\\start-nginx.ps1";

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
    command: webServerCommand,
    url: "http://localhost:8080",
    reuseExistingServer: true,
    timeout: 15_000,
  },
});
