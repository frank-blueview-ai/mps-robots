const ORA = {
  deviceName: "ORA-FEA252",
  ip: "10.1.48.113",
  proxyBase: "/bridge",
  editorUrl: "https://editor.ozobot.com/en/blockly",
};

const API = {
  status: "/status",
  ready: "/ready",
  home: "/home",
  moveLine: "/move-line",
  moveStep: "/move-step",
  moveStepOver: "/move-step-over",
  stop: "/stop",
  gripper: "/gripper",
};

const MOVE_THROTTLE_MS = 100;
const STEP_ONLINE_MS = 180;
const CLIENT_TIMEOUT_MS = {
  status: 10000,
  ready: 10000,
  home: 65000,
  moveLine: 20000,
  moveStep: 10000,
  moveStepOver: 10000,
  stop: 10000,
  gripper: 15000,
};

let gripperOpen = true;
let lastMoveTime = 0;
let verticalTimer = null;
let activeStepDirection = null;
let lastVector = { x: 0, y: 0 };
let liveControlEnabled = false;

const state = {
  connected: false,
  safeToMove: false,
  busy: false,
  lastCommand: "idle",
  robotStatus: null,
};

function $(id) {
  return document.getElementById(id);
}

function setText(id, value) {
  const element = $(id);
  if (element) element.textContent = value;
}

function updateStatus(connected, message) {
  state.connected = connected;
  const statusText = $("conn-text");
  const statusDot = $("status-dot");

  statusText.textContent = message || (connected ? "Connected" : "Offline");
  statusText.className = connected && state.safeToMove ? "status-connected" : "status-disconnected";
  statusDot.className = connected ? "status-dot status-dot-connected" : "status-dot status-dot-disconnected";
}

function movementBlockedMessage(status) {
  if (!status) return "Waiting for ORA status";
  if (!status.xarm_connected) return "ORA arm disconnected";

  const errorCode = status.xarm_error_code ?? status.xarm_error?.code ?? 0;
  if (errorCode) {
    const title = status.xarm_error?.title?.en || "Robot error";
    return `Error ${errorCode}: ${title}`;
  }

  if (!status.xarm_is_ready) {
    if (status.xarm_state === 5) return "Stopped: click Set Ready";
    return `Movement locked: state ${status.xarm_state}`;
  }
  return "";
}

function setMotionControlsEnabled(enabled) {
  ["home-btn", "grip-btn", "z-up-btn", "z-down-btn"].forEach((id) => {
    const element = $(id);
    if (element) element.disabled = !enabled;
  });
}

function canSetReady(status) {
  if (!state.connected || !status || !status.xarm_connected) return false;
  const errorCode = status.xarm_error_code ?? status.xarm_error?.code ?? 0;
  return !errorCode && !status.xarm_is_ready;
}

function applyRobotStatus(body) {
  const status = body?.latest || null;
  state.robotStatus = status;

  const connected = Boolean(body?.connected);
  const blocked = connected ? movementBlockedMessage(status) : "Bridge offline";
  state.safeToMove = connected && !blocked;

  if (!state.safeToMove && liveControlEnabled) {
    liveControlEnabled = false;
    $("live-btn").textContent = "Enable Live Control";
    $("live-btn").classList.remove("armed");
    stopStepOnline();
  }

  $("live-btn").disabled = !state.safeToMove;
  $("ready-btn").disabled = !canSetReady(status);
  setMotionControlsEnabled(liveControlEnabled && state.safeToMove);
  updateStatus(connected, state.safeToMove ? "Connected" : blocked);
}

function updateCommand(command) {
  state.lastCommand = command;
  setText("last-command", command);
}

function endpoint(path) {
  return `${ORA.proxyBase}${path}`;
}

async function fetchWithTimeout(url, options = {}, timeoutMs = 2500) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), timeoutMs);

  try {
    return await fetch(url, {
      ...options,
      signal: controller.signal,
    });
  } finally {
    clearTimeout(timeout);
  }
}

async function sendCommand(action, params = {}) {
  const path = API[action];
  if (!path) {
    throw new Error(`Unknown ORA action: ${action}`);
  }

  if (!liveControlEnabled && action !== "stop" && action !== "ready" && action !== "moveStepOver") {
    updateStatus(state.connected, "Controls locked");
    updateCommand("locked");
    return null;
  }

  if (!state.safeToMove && action !== "stop" && action !== "ready" && action !== "moveStepOver") {
    const message = movementBlockedMessage(state.robotStatus);
    updateStatus(state.connected, message);
    updateCommand("blocked");
    return null;
  }

  updateCommand(action);

  try {
    const response = await fetchWithTimeout(endpoint(path), {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(params),
    }, CLIENT_TIMEOUT_MS[action] ?? 10000);

    if (!response.ok) {
      let message = `HTTP ${response.status}`;
      try {
        const body = await response.json();
        message = body.error || message;
      } catch {}
      throw new Error(message);
    }

    if (action !== "moveStepOver") {
      await checkStatus();
    }
    return response;
  } catch (error) {
    console.error("ORA command failed:", error);
    const message = error.name === "AbortError" ? `${action} timed out` : error.message || "ORA command failed";
    updateStatus(state.connected, message);
    throw error;
  }
}

async function checkStatus() {
  try {
    const response = await fetchWithTimeout(endpoint(API.status), {
      method: "GET",
      cache: "no-store",
    }, CLIENT_TIMEOUT_MS.status);

    if (!response.ok) {
      state.safeToMove = false;
      const message = response.status === 502 || response.status === 504 ? "Bridge offline" : `HTTP ${response.status}`;
      updateStatus(false, message);
      return;
    }

    applyRobotStatus(await response.json());
  } catch (error) {
    state.safeToMove = false;
    setMotionControlsEnabled(false);
    $("ready-btn").disabled = true;
    $("live-btn").disabled = true;
    updateStatus(false, "Bridge offline");
  }
}

function emergencyStop() {
  stopVerticalMotion();
  stopStepOnline();
  lastVector = { x: 0, y: 0 };
  setText("val-x", "0.00");
  setText("val-y", "0.00");
  drawArmPreview(0, 0);
  sendCommand("stop", {}).catch(() => {});
}

function setReady() {
  sendCommand("ready").catch(() => {});
}

function toggleLiveControl() {
  if (!state.safeToMove) {
    updateStatus(state.connected, movementBlockedMessage(state.robotStatus));
    return;
  }

  liveControlEnabled = !liveControlEnabled;
  $("live-btn").textContent = liveControlEnabled ? "Disable Live Control" : "Enable Live Control";
  $("live-btn").classList.toggle("armed", liveControlEnabled);
  setMotionControlsEnabled(liveControlEnabled && state.safeToMove);
  updateStatus(state.connected, liveControlEnabled ? "Live control enabled" : "Controls locked");
}

function moveHome() {
  sendCommand("home").catch(() => {});
}

function toggleGripper() {
  gripperOpen = !gripperOpen;
  const label = gripperOpen ? "Close Gripper" : "Open Gripper";
  $("grip-btn").textContent = label;
  sendCommand("gripper", { open: gripperOpen }).catch(() => {});
}

function moveVertical(direction) {
  startStep(direction > 0 ? "position-z-increase" : "position-z-decrease");
}

function startVerticalMotion(direction) {
  stopVerticalMotion();
  moveVertical(direction);
  verticalTimer = setInterval(() => moveVertical(direction), STEP_ONLINE_MS);
}

function stopVerticalMotion() {
  if (verticalTimer) {
    clearInterval(verticalTimer);
    verticalTimer = null;
  }
  stopStepOnline();
}

function stepPayload(direction) {
  return {
    isLoop: true,
    direction,
    isMoveTool: false,
  };
}

function startStep(direction) {
  if (!liveControlEnabled || !state.safeToMove) return;
  if (activeStepDirection === direction) return;

  stopStepOnline();
  activeStepDirection = direction;
  sendCommand("moveStep", stepPayload(direction)).catch(() => {});
}

function stopStepOnline() {
  if (activeStepDirection) {
    activeStepDirection = null;
    sendCommand("moveStepOver").catch(() => {});
  }
}

function joystickDirection(x, y) {
  if (Math.abs(x) < 0.15 && Math.abs(y) < 0.15) return null;
  if (Math.abs(x) >= Math.abs(y)) {
    return x > 0 ? "position-x-increase" : "position-x-decrease";
  }
  return y > 0 ? "position-y-increase" : "position-y-decrease";
}

function drawArmPreview(x, y) {
  const canvas = $("arm-preview");
  const ctx = canvas.getContext("2d");
  const width = canvas.width;
  const height = canvas.height;
  const centerX = width / 2;
  const baseY = height - 28;
  const reach = Math.min(width, height) * 0.32;
  const targetX = centerX + x * reach;
  const targetY = baseY - reach - y * reach;

  ctx.clearRect(0, 0, width, height);

  ctx.fillStyle = "#171717";
  ctx.fillRect(0, 0, width, height);

  ctx.strokeStyle = "#3a3a3a";
  ctx.lineWidth = 1;
  for (let i = 32; i < width; i += 32) {
    ctx.beginPath();
    ctx.moveTo(i, 0);
    ctx.lineTo(i, height);
    ctx.stroke();
  }
  for (let i = 32; i < height; i += 32) {
    ctx.beginPath();
    ctx.moveTo(0, i);
    ctx.lineTo(width, i);
    ctx.stroke();
  }

  ctx.strokeStyle = "#d9a441";
  ctx.lineWidth = 12;
  ctx.lineCap = "round";
  ctx.beginPath();
  ctx.moveTo(centerX, baseY);
  ctx.lineTo((centerX + targetX) / 2, baseY - reach * 0.75);
  ctx.lineTo(targetX, targetY);
  ctx.stroke();

  ctx.fillStyle = "#58c67a";
  ctx.beginPath();
  ctx.arc(centerX, baseY, 16, 0, Math.PI * 2);
  ctx.fill();

  ctx.fillStyle = gripperOpen ? "#f5f5f5" : "#f28b54";
  ctx.beginPath();
  ctx.arc(targetX, targetY, 10, 0, Math.PI * 2);
  ctx.fill();
}

function bindHoldButton(id, direction) {
  const button = $(id);
  const start = (event) => {
    event.preventDefault();
    startVerticalMotion(direction);
  };

  button.addEventListener("pointerdown", start);
  button.addEventListener("pointerup", stopVerticalMotion);
  button.addEventListener("pointerleave", stopVerticalMotion);
  button.addEventListener("pointercancel", stopVerticalMotion);
}

function initJoystick() {
  if (!window.nipplejs) {
    updateStatus(false, "Joystick library missing");
    return;
  }

  const joystick = window.nipplejs.create({
    zone: $("joystick-zone"),
    mode: "static",
    position: { left: "50%", top: "50%" },
    color: "#58c67a",
    size: 150,
  });

  joystick.on("move", (evt, data) => {
    if (!data.vector) return;

    const x = Number(data.vector.x.toFixed(2));
    const y = Number(data.vector.y.toFixed(2));
    lastVector = { x, y };

    setText("val-x", x.toFixed(2));
    setText("val-y", y.toFixed(2));
    drawArmPreview(x, y);

    const now = Date.now();
    if (now - lastMoveTime > MOVE_THROTTLE_MS) {
      const direction = joystickDirection(x, y);
      if (direction) startStep(direction);
      lastMoveTime = now;
    }
  });

  joystick.on("end", () => {
    lastVector = { x: 0, y: 0 };
    setText("val-x", "0.00");
    setText("val-y", "0.00");
    drawArmPreview(0, 0);
    stopStepOnline();
  });
}

function init() {
  setText("device-name", ORA.deviceName);
  setText("ip-display", ORA.ip);
  $("editor-link").href = ORA.editorUrl;

  $("stop-btn").addEventListener("click", emergencyStop);
  $("ready-btn").addEventListener("click", setReady);
  $("live-btn").addEventListener("click", toggleLiveControl);
  $("home-btn").addEventListener("click", moveHome);
  $("grip-btn").addEventListener("click", toggleGripper);
  $("connect-btn").addEventListener("click", checkStatus);

  bindHoldButton("z-up-btn", 1);
  bindHoldButton("z-down-btn", -1);

  drawArmPreview(0, 0);
  setMotionControlsEnabled(false);
  initJoystick();
  checkStatus();
  setInterval(checkStatus, 5000);
}

window.addEventListener("DOMContentLoaded", init);
