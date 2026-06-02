const { expect, test } = require("@playwright/test");

async function isGoServer(request) {
  const response = await request.get("/api/health").catch(() => null);
  if (!response || !response.ok()) return false;
  const text = await response.text();
  return text.includes('"server":"ora-go-server"');
}

async function deleteProjectsByTitle(request, title) {
  const response = await request.get("/api/projects").catch(() => null);
  if (!response || !response.ok()) return;

  const projects = await response.json();
  await Promise.all(projects
    .filter((project) => project.title === title)
    .map((project) => request.delete(`/api/projects/${project.id}`)));
}

async function deleteUsersByDisplayName(request, displayName) {
  const response = await request.get("/api/users").catch(() => null);
  if (!response || !response.ok()) return;

  const users = await response.json();
  await Promise.all(users
    .filter((user) => user.displayName === displayName)
    .map((user) => request.delete(`/api/users/${user.id}`)));
}

async function deleteClassesByName(request, name) {
  const response = await request.get("/api/classes").catch(() => null);
  if (!response || !response.ok()) return;

  const classes = await response.json();
  await Promise.all(classes
    .filter((classProfile) => classProfile.name === name)
    .map((classProfile) => request.delete(`/api/classes/${classProfile.id}`)));
}

function telemetryBody(status) {
  return `event: status\ndata: ${JSON.stringify({
    bridgeStatus: status.connected ? 200 : 0,
    payload: status,
    at: "2026-05-19T16:15:00.000Z",
  })}\n\n`;
}

async function mockBridgeStatus(page, status) {
  const context = page.context();

  await context.route("**/api/telemetry", async (route) => {
    await route.fulfill({
      status: 200,
      headers: {
        "Content-Type": "text/event-stream",
        "Cache-Control": "no-cache",
      },
      body: telemetryBody(status),
    });
  });

  await context.route("**/bridge/status", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(status),
    });
  });
}

async function mockBridgeCommands(page, calls) {
  const context = page.context();

  await context.route("**/bridge/move-step", async (route) => {
    calls.push({
      path: "/bridge/move-step",
      body: route.request().postDataJSON(),
    });
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ ok: true }),
    });
  });

  await context.route("**/bridge/move-step-over", async (route) => {
    calls.push({
      path: "/bridge/move-step-over",
      body: route.request().postDataJSON(),
    });
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ ok: true }),
    });
  });

  await context.route("**/bridge/clear-error", async (route) => {
    calls.push({
      path: "/bridge/clear-error",
      body: route.request().postDataJSON(),
    });
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ ok: true }),
    });
  });

  await context.route("**/bridge/home", async (route) => {
    calls.push({
      path: "/bridge/home",
      body: route.request().postDataJSON(),
    });
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ ok: true }),
    });
  });

  await context.route("**/bridge/joint-step", async (route) => {
    calls.push({
      path: "/bridge/joint-step",
      body: route.request().postDataJSON(),
    });
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ ok: true }),
    });
  });
}

async function expectCanvasPainted(canvas) {
  const stats = await canvas.evaluate((node) => {
    const target = node;
    const gl = target.getContext("webgl2") || target.getContext("webgl");
    if (!gl) {
      return { width: target.width, height: target.height, uniqueColors: 0, litPixels: 0 };
    }

    const width = target.width;
    const height = target.height;
    const pixels = new Uint8Array(width * height * 4);
    gl.readPixels(0, 0, width, height, gl.RGBA, gl.UNSIGNED_BYTE, pixels);

    const colors = new Set();
    let litPixels = 0;
    const stride = Math.max(4, Math.floor(pixels.length / 3000 / 4) * 4);
    for (let index = 0; index < pixels.length; index += stride) {
      const red = pixels[index];
      const green = pixels[index + 1];
      const blue = pixels[index + 2];
      const alpha = pixels[index + 3];
      if (alpha > 0 && red + green + blue > 55) {
        litPixels += 1;
      }
      colors.add(`${red >> 4}-${green >> 4}-${blue >> 4}-${alpha >> 4}`);
    }

    return { width, height, uniqueColors: colors.size, litPixels };
  });

  expect(stats.width).toBeGreaterThan(300);
  expect(stats.height).toBeGreaterThan(300);
  expect(stats.uniqueColors).toBeGreaterThan(12);
  expect(stats.litPixels).toBeGreaterThan(80);
}

test("local controller loads the React classroom interface", async ({ page, request }) => {
  test.skip(!(await isGoServer(request)), "Go server React UI is not running.");

  const messages = [];
  page.on("console", (message) => {
    if (message.type() === "error") {
      messages.push(message.text());
    }
  });

  await page.goto("/");

  await expect(page.getByRole("heading", { name: "ORA Arm Control" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Dashboard" })).toBeVisible();
  await expect(page.getByRole("tab", { name: "Dashboard" })).toBeVisible();
  await expect(page.getByRole("tab", { name: "Manual Control" })).toBeVisible();
  await expect(page.getByRole("tab", { name: "Projects" })).toBeVisible();
  await expect(page.getByRole("tab", { name: "Classroom" })).toBeVisible();
  await expect(page.getByRole("tab", { name: "Settings" })).toBeVisible();
  await expect(page.getByText("10.1.48.113")).toBeVisible();
  await expect(page.getByText(/Bridge online|Bridge offline/).first()).toBeVisible();

  await page.getByRole("tab", { name: "Manual Control" }).click();
  await expect(page.getByRole("heading", { name: "Manual Control" })).toBeVisible();
  await expect(page.getByLabel("XY movement pad")).toBeVisible();
  await expect(page.getByLabel("3D arm visualization")).toBeVisible();
  await expect(page.getByRole("button", { name: "Set Ready" })).toBeVisible();
  await expect(page.getByRole("button", { name: /Enable Live Control|Disable Live Control/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /Close Gripper|Open Gripper/ })).toBeVisible();
  await expect(page.getByRole("button", { name: "Pop Out Model" })).toBeVisible();

  expect(messages.filter((text) => (
    !text.includes("ORA command failed") &&
    !text.includes("Failed to load resource: the server responded with a status of 502")
  ))).toEqual([]);
});

test("Go classroom admin creates and reloads a student profile and class", async ({ page, request }) => {
  test.skip(!(await isGoServer(request)), "Go server classroom API is not running.");

  const profileName = `E2E Student ${Date.now()}`;
  const className = `E2E Robotics ${Date.now()}`;
  await deleteUsersByDisplayName(request, profileName);
  await deleteClassesByName(request, className);

  try {
    await page.goto("/");

    await page.getByRole("tab", { name: "Classroom" }).click();
    await expect(page.getByRole("heading", { name: "User Profiles" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Add Profile" })).toBeEnabled();

    await page.getByRole("textbox", { name: "Profile name" }).fill(profileName);
    await page.getByRole("combobox", { name: "Role" }).selectOption("operator");
    await page.getByRole("textbox", { name: "Email" }).fill("student@example.test");
    await page.getByRole("button", { name: "Add Profile" }).click();

    await expect(page.getByText(`${profileName} saved`).first()).toBeVisible();
    await expect(page.getByRole("button", { name: new RegExp(profileName) })).toBeVisible();

    await page.getByRole("textbox", { name: "Class name" }).fill(className);
    await page.getByRole("textbox", { name: "Term" }).fill("2026 Pilot");
    await page.getByRole("button", { name: "Add Class" }).click();

    await expect(page.getByText(`${className} saved`).first()).toBeVisible();
    await expect(page.getByRole("button", { name: new RegExp(className) })).toBeVisible();

    await page.reload();
    await page.getByRole("tab", { name: "Classroom" }).click();
    await expect(page.getByRole("button", { name: new RegExp(profileName) })).toBeVisible();
    await expect(page.getByRole("button", { name: new RegExp(className) })).toBeVisible();
  } finally {
    await deleteUsersByDisplayName(request, profileName);
    await deleteClassesByName(request, className);
  }
});

test("Go project workflow saves and reopens a student project", async ({ page, request }) => {
  test.skip(!(await isGoServer(request)), "Go server project API is not running.");

  const title = `E2E Project ${Date.now()}`;
  await deleteProjectsByTitle(request, title);

  try {
    await page.goto("/");

    await page.getByRole("tab", { name: "Projects" }).click();
    await expect(page.getByRole("button", { name: "Save" })).toBeEnabled();
    await page.getByRole("textbox", { name: "Project" }).fill(title);
    await page.getByRole("textbox", { name: "Student" }).fill("Test Student");
    await page.getByRole("textbox", { name: "Python draft" }).fill("move_home()\nopen_gripper()");

    await page.getByRole("button", { name: "Save" }).click();
    await expect(page.getByText(`${title} saved`)).toBeVisible();
    await expect(page.getByRole("button", { name: new RegExp(title) })).toBeVisible();

    await page.reload();
    await page.getByRole("tab", { name: "Projects" }).click();
    await expect(page.getByRole("button", { name: new RegExp(title) })).toBeVisible();
    await page.getByRole("button", { name: new RegExp(title) }).click();

    await expect(page.getByRole("textbox", { name: "Project" })).toHaveValue(title);
    await expect(page.getByRole("textbox", { name: "Student" })).toHaveValue("Test Student");
    await expect(page.getByRole("textbox", { name: "Python draft" })).toHaveValue("move_home()\nopen_gripper()");
  } finally {
    await deleteProjectsByTitle(request, title);
  }
});

test("telemetry stream renders ready pose and enables teacher-gated controls", async ({ page, request }) => {
  test.skip(!(await isGoServer(request)), "Go server telemetry API is not running.");
  await page.addInitScript(() => window.localStorage.removeItem("ora-control-panel-percent"));

  await mockBridgeStatus(page, {
    connected: true,
    latest: {
      xarm_connected: true,
      xarm_is_ready: true,
      xarm_state: 0,
      xarm_error_code: 0,
      xarm_tcp_pose: [123.4, -45.6, 210, 1, 2, 3],
      xarm_joint_pose: [10, 20, 30, 40, 50, 60],
    },
  });

  await page.goto("/");
  await page.getByRole("tab", { name: "Manual Control" }).click();

  await expect(page.getByLabel("Robot safety status")).toContainText("Ready");
  await expect(page.getByLabel("Control panel")).toBeVisible();
  await expect(page.getByLabel("3D model panel")).toBeVisible();
  await expectCanvasPainted(page.getByTestId("arm-3d-canvas"));
  await expect(page.getByLabel("TCP pose summary")).toContainText("X 123.4");
  await expect(page.getByLabel("TCP pose summary")).toContainText("Y -45.6");
  await expect(page.getByRole("button", { name: "Enable Live Control" })).toBeEnabled();

  await page.getByRole("button", { name: "Enable Live Control" }).click();
  await page.getByRole("button", { name: "X+" }).hover();
  await expect(page.getByLabel("Joint jog controls")).toBeVisible();

  await expect(page.getByLabel("TCP Pose readout")).toContainText("123.4");
  await expect(page.getByLabel("Joint Pose readout")).toContainText("60.0");
  await expect(page.getByTestId("lite6-chain-readout")).toContainText("ORA mesh model");
  await expect(page.getByTestId("lite6-chain-readout")).toContainText("J1 10.0");
  await expect(page.getByTestId("lite6-chain-readout")).toContainText("J6 60.0");

  const beforeResize = await page.getByLabel("Control panel").boundingBox();
  const resizeHandle = page.getByRole("button", { name: "Resize cockpit panels" });
  const handleBox = await resizeHandle.boundingBox();
  expect(beforeResize).not.toBeNull();
  expect(handleBox).not.toBeNull();
  await page.mouse.move(handleBox.x + handleBox.width / 2, handleBox.y + handleBox.height / 2);
  await page.mouse.down();
  await page.mouse.move(handleBox.x + handleBox.width / 2 + 120, handleBox.y + handleBox.height / 2);
  await page.mouse.up();
  const resizedPercent = await page.evaluate(() => Number(window.localStorage.getItem("ora-control-panel-percent")));
  expect(Math.abs(resizedPercent - 42)).toBeGreaterThan(10);

  await page.getByRole("button", { name: "Freeze Model" }).click();
  await expect(page.getByRole("button", { name: "Follow Live Pose" })).toBeVisible();
  await expect(page.getByLabel("3D arm visualization")).toContainText("Model frozen");
  await page.getByRole("button", { name: "Reset Camera" }).click();
  await page.getByRole("button", { name: "Lock Camera" }).click();
  await expect(page.getByRole("button", { name: "Unlock Camera" })).toBeVisible();

  const popupPromise = page.waitForEvent("popup");
  await page.getByRole("button", { name: "Pop Out Model" }).click();
  const popup = await popupPromise;
  await popup.waitForLoadState("domcontentloaded");
  await expect(popup.getByLabel("Detached 3D arm model")).toBeVisible();
  await expectCanvasPainted(popup.getByTestId("arm-3d-canvas"));
  await popup.close();
});

test("manual teach pendant holds movement until release", async ({ page, request }) => {
  test.skip(!(await isGoServer(request)), "Go server telemetry API is not running.");

  const calls = [];
  await mockBridgeStatus(page, {
    connected: true,
    latest: {
      xarm_connected: true,
      xarm_is_ready: true,
      xarm_state: 0,
      xarm_error_code: 0,
      xarm_tcp_pose: [198.8, 3.2, 137, 179.9, 0, 0.1],
      xarm_joint_pose: [0, 10, 31.8, 0, 21.8, 0],
    },
  });
  await mockBridgeCommands(page, calls);

  await page.goto("/");
  await page.getByRole("tab", { name: "Manual Control" }).click();
  await page.getByRole("button", { name: "Enable Live Control" }).click();
  await page.getByRole("combobox", { name: "Step" }).selectOption("large");

  const xPlus = page.getByRole("button", { name: "X+" });
  const box = await xPlus.boundingBox();
  expect(box).not.toBeNull();

  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await expect.poll(() => calls.length).toBe(1);
  expect(calls[0]).toMatchObject({
    path: "/bridge/move-step",
    body: {
      isLoop: true,
      direction: "position-x-increase",
      isMoveTool: false,
      stepSize: "large",
      speedScale: 0.35,
    },
  });

  await page.mouse.up();
  await expect.poll(() => calls.length).toBe(2);
  expect(calls[1]).toMatchObject({
    path: "/bridge/move-step-over",
  });
});

test("joint jog controls send relative joint step commands", async ({ page, request }) => {
  test.skip(!(await isGoServer(request)), "Go server telemetry API is not running.");

  const calls = [];
  await mockBridgeStatus(page, {
    connected: true,
    latest: {
      xarm_connected: true,
      xarm_is_ready: true,
      xarm_state: 0,
      xarm_error_code: 0,
      xarm_tcp_pose: [86.7, 3.9, 90.6, 180, 0, 0],
      xarm_joint_pose: [0, 0, 0, 0, 0, 0],
    },
  });
  await mockBridgeCommands(page, calls);

  await page.goto("/");
  await page.getByRole("tab", { name: "Manual Control" }).click();
  await page.getByRole("button", { name: "Enable Live Control" }).click();
  await expect(page.getByLabel("Joint jog controls")).toContainText("0.5°");

  await page.getByRole("button", { name: "J2 positive" }).click();
  await expect.poll(() => calls.length).toBe(1);
  expect(calls[0]).toMatchObject({
    path: "/bridge/joint-step",
    body: {
      jointIndex: 1,
      deltaDegrees: 0.5,
    },
  });

  calls.length = 0;
  await page.getByRole("combobox", { name: "Step" }).selectOption("large");
  await page.getByRole("button", { name: "J6 negative" }).click();
  await expect.poll(() => calls.length).toBe(1);
  expect(calls[0]).toMatchObject({
    path: "/bridge/joint-step",
    body: {
      jointIndex: 5,
      deltaDegrees: -5,
    },
  });
});

test("manual X Y Z controls send the expected ORA direction commands", async ({ page, request }) => {
  test.skip(!(await isGoServer(request)), "Go server telemetry API is not running.");

  const calls = [];
  await mockBridgeStatus(page, {
    connected: true,
    latest: {
      xarm_connected: true,
      xarm_is_ready: true,
      xarm_state: 0,
      xarm_error_code: 0,
      xarm_tcp_pose: [86.7, 3.9, 90.6, 180, 0, 0],
      xarm_joint_pose: [0, 0, 0, 0, 0, 0],
      xarm_initial_point: [0, 9.93, 31.8, 0, 21.87, 0],
    },
  });
  await mockBridgeCommands(page, calls);

  await page.goto("/");
  await page.getByRole("tab", { name: "Manual Control" }).click();
  await page.getByRole("button", { name: "Enable Live Control" }).click();

  const expectedDirections = [
    ["X+", "position-x-increase"],
    ["X-", "position-x-decrease"],
    ["Y+", "position-y-increase"],
    ["Y-", "position-y-decrease"],
    ["Z Up", "position-z-increase"],
    ["Z Down", "position-z-decrease"],
  ];

  for (const [label, direction] of expectedDirections) {
    calls.length = 0;
    const button = page.getByRole("button", { name: label });
    const box = await button.boundingBox();
    expect(box).not.toBeNull();

    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    await page.mouse.down();
    await expect.poll(() => calls.length).toBe(1);
    expect(calls[0]).toMatchObject({
      path: "/bridge/move-step",
      body: { direction },
    });

    await page.mouse.up();
    await expect.poll(() => calls.length).toBe(2);
    expect(calls[1]).toMatchObject({ path: "/bridge/move-step-over" });
  }
});

test("initial position sends ORA home command from live control", async ({ page, request }) => {
  test.skip(!(await isGoServer(request)), "Go server telemetry API is not running.");

  const calls = [];
  await mockBridgeStatus(page, {
    connected: true,
    latest: {
      xarm_connected: true,
      xarm_is_ready: true,
      xarm_state: 0,
      xarm_error_code: 0,
      xarm_tcp_pose: [198.8, 3.2, 137, 179.9, 0, 0.1],
      xarm_joint_pose: [0, 10, 31.8, 0, 21.8, 0],
    },
  });
  await mockBridgeCommands(page, calls);

  await page.goto("/");
  await page.getByRole("tab", { name: "Manual Control" }).click();
  await page.getByRole("button", { name: "Enable Live Control" }).click();
  await page.getByRole("button", { name: "Initial Position" }).click();

  await expect.poll(() => calls.length).toBe(1);
  expect(calls[0]).toMatchObject({
    path: "/bridge/home",
  });
});

test("telemetry stream blocks motion and shows stopped-state guidance", async ({ page, request }) => {
  test.skip(!(await isGoServer(request)), "Go server telemetry API is not running.");

  await mockBridgeStatus(page, {
    connected: true,
    latest: {
      xarm_connected: true,
      xarm_is_ready: false,
      xarm_state: 5,
      xarm_error_code: 0,
      xarm_tcp_pose: [0, 0, 120, 0, 0, 0],
      xarm_joint_pose: [0, 0, 0, 0, 0, 0],
    },
  });

  await page.goto("/");
  await page.getByRole("tab", { name: "Manual Control" }).click();

  await expect(page.getByLabel("Robot safety status")).toContainText("Stopped: click Set Ready");
  await expect(page.getByRole("button", { name: "Enable Live Control" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Set Ready" })).toBeEnabled();
});

test("telemetry stream surfaces ORA self-collision errors", async ({ page, request }) => {
  test.skip(!(await isGoServer(request)), "Go server telemetry API is not running.");

  await mockBridgeStatus(page, {
    connected: true,
    latest: {
      xarm_connected: true,
      xarm_is_ready: false,
      xarm_state: 4,
      xarm_error_code: 22,
      xarm_error: { title: { en: "Self-Collision Error" } },
      xarm_tcp_pose: [10, 20, 30, 0, 0, 0],
      xarm_joint_pose: [1, 2, 3, 4, 5, 6],
    },
  });

  await page.goto("/");
  await page.getByRole("tab", { name: "Manual Control" }).click();

  await expect(page.getByLabel("Robot safety status")).toContainText("Error 22: Self-Collision Error");
  const eventLog = page.getByRole("region", { name: "Station event log" });
  await expect(eventLog).toContainText("Self-Collision Error");
  await page.getByRole("button", { name: "Clear station event log" }).click();
  await expect(eventLog).toBeVisible();
  await expect(page.getByRole("button", { name: "Enable Live Control" })).toBeDisabled();
});

test("manual recovery exposes clear error before ready or home", async ({ page, request }) => {
  test.skip(!(await isGoServer(request)), "Go server telemetry API is not running.");

  const calls = [];
  await mockBridgeStatus(page, {
    connected: true,
    latest: {
      xarm_connected: true,
      xarm_is_ready: false,
      xarm_state: 4,
      xarm_error_code: 21,
      xarm_error: { title: { en: "Kinematic Error" } },
      xarm_tcp_pose: [138.9, -211.6, 478.5, 179.9, 0, 0.1],
      xarm_joint_pose: [-56.3, 32.9, 154.5, 0.7, 121.4, -56],
    },
  });
  await mockBridgeCommands(page, calls);

  await page.goto("/");
  await page.getByRole("tab", { name: "Manual Control" }).click();

  await expect(page.getByLabel("Robot safety status")).toContainText("Error 21: Kinematic Error");
  await expect(page.getByRole("button", { name: "Enable Live Control" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Initial Position" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Clear Error" })).toBeEnabled();

  await page.getByRole("button", { name: "Clear Error" }).click();
  await expect.poll(() => calls.length).toBe(1);
  expect(calls[0]).toMatchObject({ path: "/bridge/clear-error" });
});
