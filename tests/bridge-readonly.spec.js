const { expect, test } = require("@playwright/test");

test("local bridge exposes connected ORA status and read-only command path", async ({ request }) => {
  const status = await request.get("http://localhost:8080/bridge/status", { timeout: 10_000 }).catch(() => null);
  test.skip(!status || !status.ok(), "ORA bridge is not running.");

  expect(status.ok()).toBeTruthy();

  const statusBody = await status.json();
  expect(statusBody.connected).toBe(true);
  expect(statusBody.latest.xarm_connected).toBe(true);
  expect(statusBody.latest).toHaveProperty("xarm_state");

  const command = await request.post("http://localhost:8080/bridge/command", {
    data: {
      cmd: "xarm_get_approx_motion",
      data: null,
      timeoutMs: 5000,
    },
    timeout: 10_000,
  });
  expect(command.ok()).toBeTruthy();
  await expect(await command.json()).toMatchObject({ type: "response", code: 0 });
});
