const { expect, test } = require("@playwright/test");

const ORA_NAME = process.env.ORA_NAME;
const ORA_PASSWORD = process.env.ORA_PASSWORD;

test.skip(!ORA_NAME || !ORA_PASSWORD, "Set ORA_NAME and ORA_PASSWORD to run the official-editor ORA connection E2E.");

test("official Ozobot Editor can connect to ORA through its native bridge", async ({ page }) => {
  test.setTimeout(120_000);

  await page.goto("https://editor.ozobot.com/en/blockly", { waitUntil: "domcontentloaded" });

  const connectOra = page.getByRole("button", { name: "Connect ORA" });
  if ((await connectOra.count()) === 0) {
    const addOra = page.getByRole("button", { name: "+ ORA" });
    await expect(addOra).toBeVisible({ timeout: 30_000 });
    await addOra.click();
  }

  await expect(connectOra).toBeVisible({ timeout: 30_000 });
  await connectOra.click();

  await page.locator('input[name="ora_id"]').fill(ORA_NAME);
  await page.locator('input[name="ora_password"]').fill(ORA_PASSWORD);

  const connectButton = page.getByRole("button", { name: "Connect", exact: true });
  await expect(connectButton).toBeEnabled();
  await connectButton.click();

  await expect(page.getByText(ORA_NAME, { exact: false })).toBeVisible({ timeout: 90_000 });
  await expect(page.getByRole("button", { name: /^(Disconnect|Abort and disconnect)$/ })).toBeVisible({ timeout: 90_000 });
  await expect(page.getByRole("button", { name: "RUN" })).toBeVisible({ timeout: 90_000 });
});
