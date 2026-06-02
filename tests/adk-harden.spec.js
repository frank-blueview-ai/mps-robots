const { test, expect } = require("@playwright/test");
const { spawn } = require("child_process");
const http = require("http");
const fs = require("fs");
const path = require("path");

const ARTIFACT_DIR = "C:/Users/fperez/.gemini/antigravity-ide/brain/a3c9a6e7-b19a-44ca-90e6-6df4c487cb9a/artifacts/e2e";

test.describe("ADK Harden E2E Tests", () => {
  let goServerProcess;
  let mockBridgeServer;
  const port = 8082;
  const bridgePort = 8788;

  test.beforeAll(async () => {
    // 1. Ensure artifact directory exists
    fs.mkdirSync(ARTIFACT_DIR, { recursive: true });

    // 2. Start mock bridge server on bridgePort
    mockBridgeServer = http.createServer((req, res) => {
      res.setHeader("Content-Type", "application/json");
      if (req.url === "/status") {
        res.writeHead(200);
        res.end(JSON.stringify({
          connected: true,
          latest: {
            xarm_connected: true,
            xarm_is_ready: true,
            xarm_state: 0,
            xarm_error_code: 0,
            xarm_tcp_pose: [200.0, 0.0, 100.0, 180, 0, 0],
            xarm_joint_pose: [0, 0, 0, 0, 0, 0]
          }
        }));
      } else {
        res.writeHead(200);
        res.end(JSON.stringify({ status: "ok" }));
      }
    });

    await new Promise((resolve) => {
      mockBridgeServer.listen(bridgePort, "127.0.0.1", resolve);
    });

    // 3. Start Go server with custom configuration
    goServerProcess = spawn("go", [
      "run",
      "cmd/ora-server/main.go",
      "-addr", `127.0.0.1:${port}`,
      "-bridge-url", `http://127.0.0.1:${bridgePort}`,
      "-adk-arm-mode", "live",
      "-adk-enable-motion", "true",
      "-adk-require-confirm", "true",
      "-adk-allow-raw-cartesian", "false",
      "-adk-test-fake-agent", "true"
    ], {
      env: {
        ...process.env,
        GEMINI_API_KEY: "fake_key",
        ADK_TEST_FAKE_AGENT: "true"
      }
    });

    // Wait for the server to be ready
    await new Promise((resolve, reject) => {
      const timeout = setTimeout(() => reject(new Error("Go server start timeout")), 15000);
      goServerProcess.stdout.on("data", (data) => {
        const text = data.toString();
        if (text.includes("listening on http://")) {
          clearTimeout(timeout);
          resolve();
        }
      });
      goServerProcess.stderr.on("data", (data) => {
        console.error("[Go Server Err]", data.toString());
      });
    });
  });

  test.afterAll(async () => {
    if (goServerProcess) {
      goServerProcess.kill();
    }
    if (mockBridgeServer) {
      await new Promise((resolve) => mockBridgeServer.close(resolve));
    }
  });

  test("1. Simulator view loads and displays safety warning", async ({ page }) => {
    await page.goto(`http://localhost:${port}/sim/view`);
    
    // Assert warning banner is visible and has correct text
    const banner = page.locator("text=Approximate visual simulation only. Passing simulation does not prove real-world robot safety.");
    await expect(banner).toBeVisible();

    // Verify canvas container exists
    const canvas = page.locator("#canvas-container");
    await expect(canvas).toBeVisible();

    // Take screenshot
    await page.screenshot({ path: path.join(ARTIFACT_DIR, "sim-view.png") });
  });

  test("2. Simulator state/reset", async ({ request }) => {
    const res = await request.get(`http://localhost:${port}/api/sim/state`);
    expect(res.ok()).toBe(true);
    const json = await res.json();
    expect(json.mode).toBe("live");
    expect(json.objects.length).toBeGreaterThan(0);

    const resetRes = await request.post(`http://localhost:${port}/api/sim/reset`);
    expect(resetRes.ok()).toBe(true);

    fs.writeFileSync(path.join(ARTIFACT_DIR, "sim-state.json"), JSON.stringify(json, null, 2));
  });

  test("3. Direct simulator scenario runner", async ({ request }) => {
    const res = await request.post(`http://localhost:${port}/api/sim/scenarios/run`, {
      data: { scenario: "basic_pick_place.yaml" }
    });
    expect(res.ok()).toBe(true);
    const json = await res.json();
    expect(json.status).toBe("passed");

    fs.writeFileSync(path.join(ARTIFACT_DIR, "direct-scenario-basic-pick-place.json"), JSON.stringify(json, null, 2));
  });

  test("4. ADK-in-loop scenario runner using deterministic fake mode", async ({ request }) => {
    const res = await request.post(`http://localhost:${port}/api/sim/scenarios/run-agent`, {
      data: { scenario: "basic_pick_place.yaml" }
    });
    expect(res.ok()).toBe(true);
    const json = await res.json();
    expect(json.pass).toBe(true);
    expect(json.toolCalls.length).toBeGreaterThan(0);

    fs.writeFileSync(path.join(ARTIFACT_DIR, "agent-scenario-basic-pick-place.json"), JSON.stringify(json, null, 2));
  });

  test("5. Live raw Cartesian lockout API test", async ({ request }) => {
    const res = await request.post(`http://localhost:${port}/api/agent/chat`, {
      data: { message: "move to x=150, y=0, z=100 which is raw Cartesian" }
    });
    expect(res.ok()).toBe(true);
    const json = await res.json();
    
    expect(json.error).toContain("Live raw Cartesian movement is disabled");

    fs.writeFileSync(path.join(ARTIFACT_DIR, "live-raw-cartesian-rejected.json"), JSON.stringify(json, null, 2));
  });

  test("6. Confirmation ID test", async ({ request }) => {
    const res = await request.post(`http://localhost:${port}/api/agent/chat`, {
      data: { message: "Pick the cube" }
    });
    expect(res.ok()).toBe(true);
    const json = await res.json();
    
    expect(json.confirmationRequired).toBe(true);
    expect(json.confirmationCallID).toBeDefined();

    const wrongConfirm = await request.post(`http://localhost:${port}/api/agent/confirm`, {
      data: {
        sessionId: json.sessionId,
        confirmationCallId: "fc_wrong_id",
        confirmed: true
      }
    });
    expect(wrongConfirm.ok()).toBe(false);
    expect(wrongConfirm.status()).toBe(400);

    const rightConfirm = await request.post(`http://localhost:${port}/api/agent/confirm`, {
      data: {
        sessionId: json.sessionId,
        confirmationCallId: json.confirmationCallID,
        confirmed: true
      }
    });
    expect(rightConfirm.ok()).toBe(true);
    const confirmJson = await rightConfirm.json();

    fs.writeFileSync(path.join(ARTIFACT_DIR, "confirmation-flow.json"), JSON.stringify(confirmJson, null, 2));
  });
});
