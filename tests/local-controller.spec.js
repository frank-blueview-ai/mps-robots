const { expect, test } = require("@playwright/test");

test("local controller loads and reports current ORA transport boundary", async ({ page }) => {
  const messages = [];
  page.on("console", (message) => {
    if (message.type() === "error") {
      messages.push(message.text());
    }
  });

  await page.goto("/");

  await expect(page.getByRole("heading", { name: "ORA Arm Control" })).toBeVisible();
  await expect(page.getByText("ORA-FEA252 at 10.1.48.113")).toBeVisible();
  await expect(page.locator("#joystick-zone")).toBeVisible();
  await expect(page.locator("#arm-preview")).toBeVisible();

  await page.getByRole("button", { name: "Check" }).click();
  await expect(page.locator("#conn-text")).toHaveText(/Connected|Bridge offline|Error \d+:|Movement locked|ORA arm disconnected|Stopped: click Set Ready/);

  await expect(page.getByRole("button", { name: "Set Ready" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Enable Live Control" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Close Gripper" })).toBeVisible();

  expect(messages.filter((text) => (
    !text.includes("ORA command failed") &&
    !text.includes("Failed to load resource: the server responded with a status of 502")
  ))).toEqual([]);
});
