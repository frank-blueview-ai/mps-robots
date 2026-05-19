const http = require("node:http");
const { chromium } = require("@playwright/test");

const ORA_NAME = process.env.ORA_NAME;
const ORA_PASSWORD = process.env.ORA_PASSWORD;
const PORT = Number(process.env.ORA_BRIDGE_PORT || 8787);
const HEADLESS = process.env.ORA_BRIDGE_HEADLESS !== "false";

if (!ORA_NAME || !ORA_PASSWORD) {
  console.error("Set ORA_NAME and ORA_PASSWORD before starting the bridge.");
  process.exit(1);
}

let browser;
let page;
let connected = false;

class HttpError extends Error {
  constructor(status, message, details = undefined) {
    super(message);
    this.status = status;
    this.details = details;
  }
}

function json(res, status, body) {
  const payload = JSON.stringify(body);
  res.writeHead(status, {
    "Content-Type": "application/json",
    "Content-Length": Buffer.byteLength(payload),
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
    "Access-Control-Allow-Headers": "Content-Type",
  });
  res.end(payload);
}

function readJson(req) {
  return new Promise((resolve, reject) => {
    let body = "";
    req.on("data", (chunk) => {
      body += chunk;
      if (body.length > 1024 * 1024) {
        reject(new Error("Request body too large"));
        req.destroy();
      }
    });
    req.on("end", () => {
      if (!body) {
        resolve({});
        return;
      }
      try {
        resolve(JSON.parse(body));
      } catch (error) {
        reject(error);
      }
    });
    req.on("error", reject);
  });
}

async function installOraChannelHook(targetPage) {
  await targetPage.addInitScript(() => {
    window.__oraBridge = {
      channels: {},
      nextId: 10000,
      pending: {},
      reports: {},
      lastStatus: null,
      sends: [],
      receives: [],
      sendCommand(cmd, data, timeoutMs = 5000) {
        const channel = this.channels.control;
        if (!channel || channel.readyState !== "open") {
          return Promise.reject(new Error("ORA control channel is not open"));
        }

        const id = this.nextId++;
        const payload = { cmd, data, id };

        return new Promise((resolve, reject) => {
          const timeout = setTimeout(() => {
            delete this.pending[id];
            reject(new Error(`ORA command timed out: ${cmd}`));
          }, timeoutMs);

          this.pending[id] = { resolve, reject, timeout, cmd };
          channel.send(JSON.stringify(payload));
        });
      },
      getStatus() {
        return {
          connected: Boolean(this.channels.control && this.channels.control.readyState === "open"),
          latest: this.lastStatus,
          reports: Object.keys(this.reports),
        };
      },
    };

    const NativeRTCPeerConnection = window.RTCPeerConnection;

    function printable(data) {
      if (typeof data === "string") return data.slice(0, 2000);
      if (data instanceof ArrayBuffer) return `[ArrayBuffer ${data.byteLength}]`;
      if (ArrayBuffer.isView(data)) return `[${data.constructor.name} ${data.byteLength}]`;
      return String(data).slice(0, 2000);
    }

    function handleMessage(channel, event) {
      const text = printable(event.data);
      window.__oraBridge.receives.push({ label: channel.label, at: Date.now(), data: text });

      if (typeof event.data !== "string") return;

      let message;
      try {
        message = JSON.parse(event.data);
      } catch {
        return;
      }

      if (message.type === "response" && message.id in window.__oraBridge.pending) {
        const pending = window.__oraBridge.pending[message.id];
        clearTimeout(pending.timeout);
        delete window.__oraBridge.pending[message.id];

        if (message.code === 0) {
          pending.resolve(message);
        } else {
          pending.reject(new Error(`${pending.cmd} failed with code ${message.code}: ${JSON.stringify(message.data)}`));
        }
        return;
      }

      if (message.type === "report") {
        window.__oraBridge.reports[message.cmd] = message.data;
        if (message.cmd === "devices_status_report") {
          window.__oraBridge.lastStatus = message.data;
        }
      }
    }

    function hookChannel(channel) {
      if (channel.__oraBridgeHooked) return;
      channel.__oraBridgeHooked = true;
      window.__oraBridge.channels[channel.label] = channel;

      const nativeSend = channel.send.bind(channel);
      channel.send = function(data) {
        window.__oraBridge.sends.push({ label: channel.label, at: Date.now(), data: printable(data) });
        return nativeSend(data);
      };

      channel.addEventListener("message", (event) => handleMessage(channel, event));
      channel.addEventListener("close", () => {
        if (window.__oraBridge.channels[channel.label] === channel) {
          delete window.__oraBridge.channels[channel.label];
        }
      });
    }

    window.RTCPeerConnection = function(...args) {
      const peer = new NativeRTCPeerConnection(...args);
      const nativeCreateDataChannel = peer.createDataChannel.bind(peer);

      peer.createDataChannel = function(label, options) {
        const channel = nativeCreateDataChannel(label, options);
        hookChannel(channel);
        return channel;
      };

      peer.addEventListener("datachannel", (event) => hookChannel(event.channel));
      return peer;
    };

    window.RTCPeerConnection.prototype = NativeRTCPeerConnection.prototype;
    window.RTCPeerConnection.prototype.constructor = window.RTCPeerConnection;
  });
}

async function connectEditor() {
  browser = await chromium.launch({ headless: HEADLESS });
  page = await browser.newPage();
  await installOraChannelHook(page);

  await page.goto("https://editor.ozobot.com/en/blockly", { waitUntil: "domcontentloaded" });

  const connectOra = page.getByRole("button", { name: "Connect ORA" });
  if ((await connectOra.count()) === 0) {
    await page.getByRole("button", { name: "+ ORA" }).click();
  }

  await page.getByRole("button", { name: "Connect ORA" }).click();
  await page.locator('input[name="ora_id"]').fill(ORA_NAME);
  await page.locator('input[name="ora_password"]').fill(ORA_PASSWORD);
  await page.getByRole("button", { name: "Connect", exact: true }).click();

  await page.getByText(ORA_NAME, { exact: false }).waitFor({ timeout: 90_000 });
  await page.waitForFunction(() => window.__oraBridge?.channels?.control?.readyState === "open", null, { timeout: 90_000 });
  connected = true;
}

async function bridgeStatus() {
  if (!page || page.isClosed()) return { connected: false, reason: "editor page is not open" };
  return await page.evaluate(() => window.__oraBridge.getStatus());
}

async function command(cmd, data, timeoutMs) {
  if (!connected) throw new Error("ORA bridge is not connected");
  return await page.evaluate(
    ({ cmd, data, timeoutMs }) => window.__oraBridge.sendCommand(cmd, data, timeoutMs),
    { cmd, data, timeoutMs },
  );
}

async function latestRobotStatus() {
  const status = await bridgeStatus();
  if (!status.connected) {
    throw new HttpError(503, status.reason || "ORA bridge is not connected", status);
  }
  if (!status.latest) {
    throw new HttpError(503, "ORA status has not been reported yet", status);
  }
  return status.latest;
}

function robotBlockReason(status) {
  if (!status.xarm_connected) {
    return "ORA arm is not connected";
  }

  const errorCode = status.xarm_error_code ?? status.xarm_error?.code ?? 0;
  if (errorCode) {
    const title = status.xarm_error?.title?.en || "Robot error";
    return `Error ${errorCode}: ${title}`;
  }

  if (!status.xarm_is_ready) {
    return `ORA arm is not ready; state ${status.xarm_state}`;
  }

  return null;
}

async function ensureRobotReady() {
  const status = await latestRobotStatus();
  const reason = robotBlockReason(status);
  if (reason) {
    throw new HttpError(409, reason, {
      xarm_state: status.xarm_state,
      xarm_error_code: status.xarm_error_code ?? status.xarm_error?.code ?? 0,
      xarm_error: status.xarm_error,
      xarm_is_ready: status.xarm_is_ready,
    });
  }
  return status;
}

async function ensureRobotCanSetReady() {
  const status = await latestRobotStatus();
  if (!status.xarm_connected) {
    throw new HttpError(409, "ORA arm is not connected");
  }

  const errorCode = status.xarm_error_code ?? status.xarm_error?.code ?? 0;
  if (errorCode) {
    const title = status.xarm_error?.title?.en || "Robot error";
    throw new HttpError(409, `Clear ORA error ${errorCode} first: ${title}`, {
      xarm_state: status.xarm_state,
      xarm_error_code: errorCode,
      xarm_error: status.xarm_error,
      xarm_is_ready: status.xarm_is_ready,
    });
  }

  return status;
}

function homeJointPayload(status) {
  const joints = status.xarm_initial_point;
  if (!Array.isArray(joints) || joints.length < 6) {
    throw new HttpError(409, "ORA initial joint position is not available");
  }

  return {
    I: Number(joints[0]),
    J: Number(joints[1]),
    K: Number(joints[2]),
    L: Number(joints[3]),
    M: Number(joints[4]),
    N: Number(joints[5]),
    O: 0,
    R: 0,
    wait: true,
    isControl: false,
    isClickMove: false,
  };
}

const server = http.createServer(async (req, res) => {
  if (req.method === "OPTIONS") {
    json(res, 204, {});
    return;
  }

  try {
    const url = new URL(req.url, `http://${req.headers.host || "localhost"}`);

    if (req.method === "GET" && url.pathname === "/status") {
      json(res, 200, await bridgeStatus());
      return;
    }

    if (req.method === "POST" && url.pathname === "/command") {
      const body = await readJson(req);
      json(res, 200, await command(body.cmd, body.data ?? null, body.timeoutMs ?? 5000));
      return;
    }

    if (req.method === "POST" && url.pathname === "/stop") {
      json(res, 200, await command("xarm_set_state", { state: 4 }, 5000));
      return;
    }

    if (req.method === "POST" && url.pathname === "/ready") {
      await ensureRobotCanSetReady();
      json(res, 200, await command("xarm_set_state", { state: 0 }, 5000));
      return;
    }

    if (req.method === "POST" && url.pathname === "/gripper") {
      await ensureRobotReady();
      const body = await readJson(req);
      json(res, 200, await command("xarm_set_lite6_gripper", { op: body.open ? "open" : "close" }, 10000));
      return;
    }

    if (req.method === "POST" && url.pathname === "/home") {
      const status = await ensureRobotReady();
      json(res, 200, await command("xarm_move_joint", homeJointPayload(status), 60000));
      return;
    }

    if (req.method === "POST" && url.pathname === "/move-step") {
      await ensureRobotReady();
      const body = await readJson(req);
      json(res, 200, await command("xarm_move_step", {
        isLoop: Boolean(body.isLoop),
        direction: body.direction,
        isMoveTool: false,
      }, body.timeoutMs ?? 5000));
      return;
    }

    if (req.method === "POST" && url.pathname === "/move-step-over") {
      json(res, 200, await command("xarm_move_step_over", null, 5000));
      return;
    }

    if (req.method === "POST" && url.pathname === "/move-line") {
      await ensureRobotReady();
      const body = await readJson(req);
      json(res, 200, await command("xarm_move_line", body, body.timeoutMs ?? 15000));
      return;
    }

    json(res, 404, { error: "not found" });
  } catch (error) {
    json(res, error.status || 500, { error: error.message, details: error.details });
  }
});

process.on("SIGINT", async () => {
  server.close();
  if (browser) await browser.close();
  process.exit(0);
});

connectEditor()
  .then(() => {
    server.listen(PORT, "127.0.0.1", () => {
      console.log(`ORA bridge listening on http://127.0.0.1:${PORT}`);
    });
  })
  .catch(async (error) => {
    console.error("Failed to start ORA bridge:", error);
    if (browser) await browser.close();
    process.exit(1);
  });
