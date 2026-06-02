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

const APP_API = {
  projects: "/api/projects",
  users: "/api/users",
  classes: "/api/classes",
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
let currentProject = null;
let currentUser = null;
let currentClass = null;
let projectStorageAvailable = false;
let classroomStorageAvailable = false;

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

async function appRequest(path, options = {}, timeoutMs = 10000) {
  const response = await fetchWithTimeout(path, {
    ...options,
    headers: {
      ...(options.body ? { "Content-Type": "application/json" } : {}),
      ...(options.headers || {}),
    },
  }, timeoutMs);

  if (!response.ok) {
    let message = `HTTP ${response.status}`;
    try {
      const body = await response.json();
      message = body.error || message;
    } catch {}
    throw new Error(message);
  }

  if (response.status === 204) return null;
  return await response.json();
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

function setProjectStatus(message) {
  setText("project-status", message);
}

function projectSummary(project) {
  if (!project) return "No project loaded";
  const updated = project.updatedAt ? new Date(project.updatedAt).toLocaleString() : "not saved";
  return `${project.title || "Untitled Project"} saved ${updated}`;
}

function setProjectForm(project) {
  currentProject = project;
  $("project-title").value = project?.title || "";
  $("project-owner").value = project?.owner || "";
  $("project-python").value = project?.python || "";
  updateProjectControls();
  setProjectStatus(projectSummary(project));
}

function collectProjectPayload() {
  return {
    title: $("project-title").value.trim() || "Untitled Project",
    owner: $("project-owner").value.trim(),
    mode: "blocks-and-python",
    blockly: currentProject?.blockly || { workspaceVersion: 1, blocks: [] },
    python: $("project-python").value,
    generatedPython: currentProject?.generatedPython || "",
    safetyProfileId: currentProject?.safetyProfileId || "pilot-default",
    metadata: {
      savedFrom: "local-controller",
      lastManualCommand: state.lastCommand,
    },
  };
}

function updateProjectControls() {
  const hasProject = Boolean(currentProject?.id);
  ["duplicate-project-btn", "export-project-btn", "delete-project-btn"].forEach((id) => {
    const element = $(id);
    if (element) element.disabled = !hasProject || !projectStorageAvailable;
  });

  $("save-project-btn").disabled = !projectStorageAvailable;
}

function renderProjectList(projects) {
  const list = $("project-list");
  list.innerHTML = "";

  if (!projectStorageAvailable) {
    const item = document.createElement("p");
    item.className = "project-status";
    item.textContent = "Start the Go server to use project storage.";
    list.appendChild(item);
    return;
  }

  if (!projects.length) {
    const item = document.createElement("p");
    item.className = "project-status";
    item.textContent = "No saved projects";
    list.appendChild(item);
    return;
  }

  projects.forEach((project) => {
    const row = document.createElement("div");
    row.setAttribute("role", "listitem");

    const button = document.createElement("button");
    button.type = "button";
    button.className = "project-item";
    if (project.id === currentProject?.id) button.classList.add("active");

    const title = document.createElement("span");
    title.className = "project-item-title";
    title.textContent = project.title || "Untitled Project";

    const meta = document.createElement("span");
    meta.className = "project-item-meta";
    const owner = project.owner ? `${project.owner} · ` : "";
    const updated = project.updatedAt ? new Date(project.updatedAt).toLocaleDateString() : "unsaved";
    meta.textContent = `${owner}${updated}`;

    button.append(title, meta);
    button.addEventListener("click", () => openProject(project.id));
    row.appendChild(button);
    list.appendChild(row);
  });
}

async function loadProjects() {
  try {
    const projects = await appRequest(APP_API.projects, { method: "GET" });
    projectStorageAvailable = true;
    renderProjectList(projects);
    updateProjectControls();
    if (!currentProject) setProjectStatus("Ready");
  } catch {
    projectStorageAvailable = false;
    renderProjectList([]);
    updateProjectControls();
    setProjectStatus("Project storage unavailable");
  }
}

async function openProject(id) {
  try {
    const project = await appRequest(`${APP_API.projects}/${encodeURIComponent(id)}`, { method: "GET" });
    setProjectForm(project);
    await loadProjects();
  } catch (error) {
    setProjectStatus(error.message || "Could not open project");
  }
}

async function saveProject() {
  if (!projectStorageAvailable) {
    setProjectStatus("Project storage unavailable");
    return;
  }

  try {
    const payload = collectProjectPayload();
    const path = currentProject?.id ? `${APP_API.projects}/${encodeURIComponent(currentProject.id)}` : APP_API.projects;
    const method = currentProject?.id ? "PUT" : "POST";
    const project = await appRequest(path, {
      method,
      body: JSON.stringify(payload),
    });
    setProjectForm(project);
    await loadProjects();
  } catch (error) {
    setProjectStatus(error.message || "Could not save project");
  }
}

function newProject() {
  currentProject = null;
  $("project-title").value = "";
  $("project-owner").value = "";
  $("project-python").value = "";
  updateProjectControls();
  setProjectStatus(projectStorageAvailable ? "New project" : "Project storage unavailable");
  loadProjects();
}

async function duplicateProject() {
  if (!currentProject?.id || !projectStorageAvailable) return;

  try {
    const payload = collectProjectPayload();
    payload.title = `Copy of ${payload.title}`;
    const project = await appRequest(APP_API.projects, {
      method: "POST",
      body: JSON.stringify(payload),
    });
    setProjectForm(project);
    await loadProjects();
  } catch (error) {
    setProjectStatus(error.message || "Could not duplicate project");
  }
}

async function deleteProject() {
  if (!currentProject?.id || !projectStorageAvailable) return;
  if (!window.confirm(`Delete "${currentProject.title || "Untitled Project"}"?`)) return;

  try {
    await appRequest(`${APP_API.projects}/${encodeURIComponent(currentProject.id)}`, { method: "DELETE" });
    newProject();
    await loadProjects();
    setProjectStatus("Project deleted");
  } catch (error) {
    setProjectStatus(error.message || "Could not delete project");
  }
}

function exportProject() {
  if (!currentProject?.id) return;

  const filename = `${(currentProject.title || "ora-project").replace(/[^a-z0-9_-]+/gi, "-").replace(/^-|-$/g, "") || "ora-project"}.json`;
  const blob = new Blob([JSON.stringify(currentProject, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
  setProjectStatus("Project exported");
}

function importProject() {
  $("project-import-file").click();
}

async function handleProjectImport(event) {
  const [file] = event.target.files || [];
  event.target.value = "";
  if (!file || !projectStorageAvailable) return;

  try {
    const text = await file.text();
    const project = JSON.parse(text);
    const payload = {
      ...project,
      id: undefined,
      title: project.title ? `Imported ${project.title}` : "Imported Project",
    };
    const saved = await appRequest(APP_API.projects, {
      method: "POST",
      body: JSON.stringify(payload),
    });
    setProjectForm(saved);
    await loadProjects();
  } catch (error) {
    setProjectStatus(error.message || "Could not import project");
  }
}

function setUserStatus(message) {
  setText("user-status", message);
}

function setClassStatus(message) {
  setText("class-status", message);
}

function roleLabel(role) {
  return {
    admin: "Admin",
    teacher: "Teacher",
    student: "Student author",
    operator: "Selected student operator",
  }[role] || role || "Student author";
}

function setUserForm(user) {
  currentUser = user;
  $("user-display-name").value = user?.displayName || "";
  $("user-role").value = user?.role || "student";
  $("user-email").value = user?.email || "";
  updateClassroomControls();
  setUserStatus(user ? `${user.displayName} selected` : "No profile selected");
}

function setClassForm(classProfile) {
  currentClass = classProfile;
  $("class-name").value = classProfile?.name || "";
  $("class-term").value = classProfile?.term || "";
  updateClassroomControls();
  setClassStatus(classProfile ? `${classProfile.name} selected` : "No class selected");
}

function updateClassroomControls() {
  $("save-user-btn").disabled = !classroomStorageAvailable;
  $("save-class-btn").disabled = !classroomStorageAvailable;
  $("delete-user-btn").disabled = !classroomStorageAvailable || !currentUser?.id || currentUser.id === "station-admin";
  $("delete-class-btn").disabled = !classroomStorageAvailable || !currentClass?.id;
  $("save-user-btn").textContent = currentUser?.id ? "Update Profile" : "Add Profile";
  $("save-class-btn").textContent = currentClass?.id ? "Update Class" : "Add Class";
}

function renderUsers(users) {
  const list = $("user-list");
  list.innerHTML = "";

  if (!classroomStorageAvailable) {
    const item = document.createElement("p");
    item.className = "project-status";
    item.textContent = "Start the Go server to manage profiles.";
    list.appendChild(item);
    return;
  }

  if (!users.length) {
    const item = document.createElement("p");
    item.className = "project-status";
    item.textContent = "No profiles";
    list.appendChild(item);
    return;
  }

  users.forEach((user) => {
    const row = document.createElement("div");
    row.setAttribute("role", "listitem");

    const button = document.createElement("button");
    button.type = "button";
    button.className = "project-item";
    if (user.id === currentUser?.id) button.classList.add("active");
    if (!user.active) button.classList.add("inactive");

    const title = document.createElement("span");
    title.className = "project-item-title";
    title.textContent = user.displayName || "Unnamed profile";

    const meta = document.createElement("span");
    meta.className = "project-item-meta";
    const email = user.email ? ` · ${user.email}` : "";
    meta.textContent = `${roleLabel(user.role)}${email}`;

    button.append(title, meta);
    button.addEventListener("click", () => setUserForm(user));
    row.appendChild(button);
    list.appendChild(row);
  });
}

function renderClasses(classes) {
  const list = $("class-list");
  list.innerHTML = "";

  if (!classroomStorageAvailable) {
    const item = document.createElement("p");
    item.className = "project-status";
    item.textContent = "Start the Go server to manage classes.";
    list.appendChild(item);
    return;
  }

  if (!classes.length) {
    const item = document.createElement("p");
    item.className = "project-status";
    item.textContent = "No classes";
    list.appendChild(item);
    return;
  }

  classes.forEach((classProfile) => {
    const row = document.createElement("div");
    row.setAttribute("role", "listitem");

    const button = document.createElement("button");
    button.type = "button";
    button.className = "project-item";
    if (classProfile.id === currentClass?.id) button.classList.add("active");

    const title = document.createElement("span");
    title.className = "project-item-title";
    title.textContent = classProfile.name || "Unnamed class";

    const meta = document.createElement("span");
    meta.className = "project-item-meta";
    meta.textContent = classProfile.term || "No term";

    button.append(title, meta);
    button.addEventListener("click", () => setClassForm(classProfile));
    row.appendChild(button);
    list.appendChild(row);
  });
}

async function loadUsers() {
  try {
    const users = await appRequest(APP_API.users, { method: "GET" });
    classroomStorageAvailable = true;
    renderUsers(users);
    updateClassroomControls();
    if (!currentUser) setUserStatus("Ready");
  } catch {
    classroomStorageAvailable = false;
    renderUsers([]);
    updateClassroomControls();
    setUserStatus("Profile storage unavailable");
  }
}

async function loadClasses() {
  try {
    const classes = await appRequest(APP_API.classes, { method: "GET" });
    classroomStorageAvailable = true;
    renderClasses(classes);
    updateClassroomControls();
    if (!currentClass) setClassStatus("Ready");
  } catch {
    classroomStorageAvailable = false;
    renderClasses([]);
    updateClassroomControls();
    setClassStatus("Class storage unavailable");
  }
}

async function loadClassroomAdmin() {
  await Promise.all([loadUsers(), loadClasses()]);
}

function clearUserForm() {
  setUserForm(null);
  loadUsers();
}

function clearClassForm() {
  setClassForm(null);
  loadClasses();
}

async function saveUser() {
  if (!classroomStorageAvailable) {
    setUserStatus("Profile storage unavailable");
    return;
  }

  const payload = {
    displayName: $("user-display-name").value.trim(),
    role: $("user-role").value,
    email: $("user-email").value.trim(),
    active: true,
  };

  if (!payload.displayName) {
    setUserStatus("Profile name is required");
    return;
  }

  try {
    const path = currentUser?.id ? `${APP_API.users}/${encodeURIComponent(currentUser.id)}` : APP_API.users;
    const method = currentUser?.id ? "PUT" : "POST";
    const saved = await appRequest(path, {
      method,
      body: JSON.stringify(payload),
    });
    setUserForm(saved);
    await loadUsers();
    setUserStatus(`${saved.displayName} saved`);
  } catch (error) {
    setUserStatus(error.message || "Could not save profile");
  }
}

async function deleteUser() {
  if (!currentUser?.id || currentUser.id === "station-admin") return;
  if (!window.confirm(`Delete profile "${currentUser.displayName}"?`)) return;

  try {
    await appRequest(`${APP_API.users}/${encodeURIComponent(currentUser.id)}`, { method: "DELETE" });
    setUserForm(null);
    await loadUsers();
    setUserStatus("Profile deleted");
  } catch (error) {
    setUserStatus(error.message || "Could not delete profile");
  }
}

async function saveClassProfile() {
  if (!classroomStorageAvailable) {
    setClassStatus("Class storage unavailable");
    return;
  }

  const payload = {
    name: $("class-name").value.trim(),
    term: $("class-term").value.trim(),
  };

  if (!payload.name) {
    setClassStatus("Class name is required");
    return;
  }

  try {
    const path = currentClass?.id ? `${APP_API.classes}/${encodeURIComponent(currentClass.id)}` : APP_API.classes;
    const method = currentClass?.id ? "PUT" : "POST";
    const saved = await appRequest(path, {
      method,
      body: JSON.stringify(payload),
    });
    setClassForm(saved);
    await loadClasses();
    setClassStatus(`${saved.name} saved`);
  } catch (error) {
    setClassStatus(error.message || "Could not save class");
  }
}

async function deleteClassProfile() {
  if (!currentClass?.id) return;
  if (!window.confirm(`Delete class "${currentClass.name}"?`)) return;

  try {
    await appRequest(`${APP_API.classes}/${encodeURIComponent(currentClass.id)}`, { method: "DELETE" });
    setClassForm(null);
    await loadClasses();
    setClassStatus("Class deleted");
  } catch (error) {
    setClassStatus(error.message || "Could not delete class");
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

  $("new-project-btn").addEventListener("click", newProject);
  $("save-project-btn").addEventListener("click", saveProject);
  $("duplicate-project-btn").addEventListener("click", duplicateProject);
  $("export-project-btn").addEventListener("click", exportProject);
  $("import-project-btn").addEventListener("click", importProject);
  $("delete-project-btn").addEventListener("click", deleteProject);
  $("refresh-projects-btn").addEventListener("click", loadProjects);
  $("project-import-file").addEventListener("change", handleProjectImport);

  $("save-user-btn").addEventListener("click", saveUser);
  $("clear-user-btn").addEventListener("click", clearUserForm);
  $("delete-user-btn").addEventListener("click", deleteUser);
  $("refresh-users-btn").addEventListener("click", loadUsers);
  $("save-class-btn").addEventListener("click", saveClassProfile);
  $("clear-class-btn").addEventListener("click", clearClassForm);
  $("delete-class-btn").addEventListener("click", deleteClassProfile);
  $("refresh-classes-btn").addEventListener("click", loadClasses);

  bindHoldButton("z-up-btn", 1);
  bindHoldButton("z-down-btn", -1);

  drawArmPreview(0, 0);
  setMotionControlsEnabled(false);
  updateProjectControls();
  updateClassroomControls();
  initJoystick();
  loadProjects();
  loadClassroomAdmin();
  checkStatus();
  setInterval(checkStatus, 5000);
}

window.addEventListener("DOMContentLoaded", init);
