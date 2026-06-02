import {
  AlertTriangle,
  BookOpen,
  Bot,
  Boxes,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  CircleStop,
  ClipboardList,
  Download,
  ExternalLink,
  Gauge,
  GraduationCap,
  HardDrive,
  Home,
  Import,
  Joystick,
  Lock,
  PanelLeftClose,
  PanelLeftOpen,
  Pause,
  Play,
  Plus,
  RefreshCw,
  Save,
  Settings,
  ShieldCheck,
  Square,
  Trash2,
  Unlock,
  Upload,
  Users,
} from "lucide-react";
import { CSSProperties, ChangeEvent, FormEvent, PointerEvent as ReactPointerEvent, ReactNode, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ArmVisualization, type JoystickIntent } from "./ArmVisualization";

type View = "dashboard" | "manual" | "projects" | "classroom" | "settings";

type Station = {
  oraName: string;
  oraIp: string;
  bridgeBase: string;
  features: string[];
};

type RobotStatus = {
  xarm_connected?: boolean;
  xarm_is_ready?: boolean;
  xarm_state?: number;
  xarm_error_code?: number;
  xarm_error?: { title?: { en?: string } };
  xarm_tcp_pose?: number[];
  xarm_joint_pose?: number[];
};

type BridgeStatus = {
  connected: boolean;
  latest?: RobotStatus;
  reason?: string;
  error?: string;
};

type TelemetryEnvelope = {
  bridgeStatus: number;
  payload?: BridgeStatus;
  at?: string;
};

type TelemetryMode = "connecting" | "streaming" | "fallback";

type StationEvent = {
  id: number;
  severity: "info" | "ok" | "warning" | "error";
  message: string;
  at: string;
};

type Project = {
  id?: string;
  formatVersion?: number;
  title: string;
  owner?: string;
  course?: string;
  mode?: string;
  blockly?: unknown;
  python?: string;
  generatedPython?: string;
  safetyProfileId?: string;
  metadata?: Record<string, unknown>;
  createdAt?: string;
  updatedAt?: string;
};

type UserProfile = {
  id?: string;
  displayName: string;
  role: "admin" | "teacher" | "student" | "operator";
  email?: string;
  active?: boolean;
  createdAt?: string;
  updatedAt?: string;
};

type ClassProfile = {
  id?: string;
  name: string;
  term?: string;
  createdAt?: string;
  updatedAt?: string;
};

const views: Array<{ id: View; label: string; icon: ReactNode }> = [
  { id: "dashboard", label: "Dashboard", icon: <Gauge size={18} /> },
  { id: "manual", label: "Manual Control", icon: <Joystick size={18} /> },
  { id: "projects", label: "Projects", icon: <BookOpen size={18} /> },
  { id: "classroom", label: "Classroom", icon: <GraduationCap size={18} /> },
  { id: "settings", label: "Settings", icon: <Settings size={18} /> },
];

const defaultStation: Station = {
  oraName: "ORA-FEA252",
  oraIp: "10.1.48.113",
  bridgeBase: "/bridge",
  features: [],
};

const emptyProject: Project = {
  title: "",
  owner: "",
  mode: "blocks-and-python",
  python: "",
  safetyProfileId: "pilot-default",
};

const emptyUser: UserProfile = {
  displayName: "",
  role: "student",
  email: "",
  active: true,
};

const emptyClass: ClassProfile = {
  name: "",
  term: "",
};

const idleJoystickIntent: JoystickIntent = {
  x: 0,
  y: 0,
  z: 0,
  label: "idle",
};

async function requestJson<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      ...(init.body ? { "Content-Type": "application/json" } : {}),
      ...(init.headers || {}),
    },
  });

  if (!response.ok) {
    let message = `HTTP ${response.status}`;
    try {
      const body = await response.json();
      message = body.error || message;
    } catch {
      // Keep the HTTP status fallback.
    }
    throw new Error(message);
  }

  return (await response.json()) as T;
}

function robotBlockedMessage(status?: RobotStatus): string {
  if (!status) return "Waiting for ORA status";
  if (!status.xarm_connected) return "ORA arm disconnected";

  const errorCode = status.xarm_error_code ?? 0;
  if (errorCode) {
    return `Error ${errorCode}: ${status.xarm_error?.title?.en || "Robot error"}`;
  }

  if (!status.xarm_is_ready) {
    if (status.xarm_state === 5) return "Stopped: click Set Ready";
    return `Movement locked: state ${status.xarm_state ?? "unknown"}`;
  }

  return "";
}

function normalizeTelemetry(envelope: TelemetryEnvelope): BridgeStatus {
  const payload = envelope.payload || { connected: false };
  return {
    connected: payload.connected === true,
    latest: payload.latest,
    reason: payload.reason || payload.error || (envelope.bridgeStatus === 0 ? "Bridge unavailable" : undefined),
    error: payload.error,
  };
}

function bridgeHeadline(bridge: BridgeStatus): string {
  if (!bridge.connected) return bridge.reason || bridge.error || "Bridge offline";
  const blocked = robotBlockedMessage(bridge.latest);
  return blocked || "Ready";
}

function bridgeSeverity(bridge: BridgeStatus): StationEvent["severity"] {
  if (!bridge.connected) return "error";
  if ((bridge.latest?.xarm_error_code ?? 0) > 0) return "error";
  if (robotBlockedMessage(bridge.latest)) return "warning";
  return "ok";
}

function bridgeSignature(bridge: BridgeStatus): string {
  const latest = bridge.latest || {};
  return [
    bridge.connected ? "connected" : "offline",
    latest.xarm_connected ? "arm-connected" : "arm-disconnected",
    latest.xarm_is_ready ? "ready" : "not-ready",
    latest.xarm_state ?? "state-unknown",
    latest.xarm_error_code ?? 0,
    bridge.reason || bridge.error || "",
  ].join("|");
}

function formatTelemetryTime(value?: string): string {
  if (!value) return "No stream update yet";
  return new Date(value).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function labeledPose(values: Array<{ label: string; value?: number }>): Array<{ label: string; value: string }> {
  return values.map((item) => ({
    label: item.label,
    value: typeof item.value === "number" ? item.value.toFixed(1) : "No data",
  }));
}

function roleLabel(role: UserProfile["role"]): string {
  return {
    admin: "Admin",
    teacher: "Teacher",
    student: "Student author",
    operator: "Selected student operator",
  }[role];
}

function formatDate(value?: string): string {
  if (!value) return "Not saved";
  return new Date(value).toLocaleString();
}

function downloadJSON(filename: string, payload: unknown) {
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

export function App() {
  const isModelPopout = new URLSearchParams(window.location.search).get("view") === "model-popout";
  return isModelPopout ? <ModelPopout /> : <StationApp />;
}

function StationApp() {
  const [activeView, setActiveView] = useState<View>("dashboard");
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => {
    const saved = window.localStorage.getItem("ora-sidebar-collapsed");
    if (saved !== null) {
      return saved === "true";
    }
    return window.innerWidth < 1200;
  });

  useEffect(() => {
    window.localStorage.setItem("ora-sidebar-collapsed", String(sidebarCollapsed));
  }, [sidebarCollapsed]);

  const [station, setStation] = useState<Station>(defaultStation);
  const [bridge, setBridge] = useState<BridgeStatus>({ connected: false });
  const [projects, setProjects] = useState<Project[]>([]);
  const [users, setUsers] = useState<UserProfile[]>([]);
  const [classes, setClasses] = useState<ClassProfile[]>([]);
  const [currentProject, setCurrentProject] = useState<Project>(emptyProject);
  const [currentUser, setCurrentUser] = useState<UserProfile>(emptyUser);
  const [currentClass, setCurrentClass] = useState<ClassProfile>(emptyClass);
  const [projectMessage, setProjectMessage] = useState("Ready");
  const [classroomMessage, setClassroomMessage] = useState("Ready");
  const [commandMessage, setCommandMessage] = useState("Idle");
  const [liveControl, setLiveControl] = useState(false);
  const [gripperOpen, setGripperOpen] = useState(true);
  const [telemetryMode, setTelemetryMode] = useState<TelemetryMode>("connecting");
  const [telemetryAt, setTelemetryAt] = useState("");
  const [events, setEvents] = useState<StationEvent[]>([]);
  const [joystickIntent, setJoystickIntent] = useState<JoystickIntent>(idleJoystickIntent);
  const eventIdRef = useRef(0);
  const bridgeSignatureRef = useRef("");

  const blocked = bridge.connected ? robotBlockedMessage(bridge.latest) : "Bridge offline";
  const safeToMove = bridge.connected && !blocked;

  const recordEvent = useCallback((severity: StationEvent["severity"], message: string) => {
    eventIdRef.current += 1;
    setEvents((current) => [
      {
        id: eventIdRef.current,
        severity,
        message,
        at: new Date().toISOString(),
      },
      ...current,
    ].slice(0, 12));
  }, []);

  const clearEvents = useCallback(() => {
    setEvents([]);
  }, []);

  const clearJoystickIntent = useCallback(() => {
    setJoystickIntent(idleJoystickIntent);
  }, []);

  const applyBridgeStatus = useCallback((nextBridge: BridgeStatus, source: "telemetry" | "refresh" = "telemetry") => {
    const signature = bridgeSignature(nextBridge);
    if (signature !== bridgeSignatureRef.current) {
      bridgeSignatureRef.current = signature;
      const sourceLabel = source === "telemetry" ? "Telemetry" : "Refresh";
      recordEvent(bridgeSeverity(nextBridge), `${sourceLabel}: ${bridgeHeadline(nextBridge)}`);
    }
    setBridge(nextBridge);
  }, [recordEvent]);

  useEffect(() => {
    void loadStation();
    void refreshAll();
  }, []);

  useEffect(() => {
    if (!("EventSource" in window)) {
      setTelemetryMode("fallback");
      recordEvent("warning", "Telemetry stream is not supported in this browser");
      void loadBridgeStatus();
      return;
    }

    let closed = false;
    const source = new EventSource("/api/telemetry");
    setTelemetryMode("connecting");

    source.addEventListener("open", () => {
      if (!closed) {
        setTelemetryMode("streaming");
        recordEvent("ok", "Telemetry stream connected");
      }
    });

    source.addEventListener("status", (event) => {
      try {
        const envelope = JSON.parse(event.data) as TelemetryEnvelope;
        setTelemetryAt(envelope.at || new Date().toISOString());
        setTelemetryMode("streaming");
        applyBridgeStatus(normalizeTelemetry(envelope), "telemetry");
      } catch {
        recordEvent("warning", "Telemetry update could not be read");
      }
    });

    source.onerror = () => {
      if (!closed) {
        setTelemetryMode("fallback");
        recordEvent("warning", "Telemetry stream interrupted; reconnecting");
      }
    };

    return () => {
      closed = true;
      source.close();
    };
  }, [applyBridgeStatus, recordEvent]);

  useEffect(() => {
    if (!safeToMove && liveControl) {
      setLiveControl(false);
    }
  }, [safeToMove, liveControl]);

  async function loadStation() {
    try {
      setStation(await requestJson<Station>("/api/station"));
    } catch {
      setStation(defaultStation);
    }
  }

  async function loadBridgeStatus() {
    try {
      applyBridgeStatus(await requestJson<BridgeStatus>("/bridge/status"), "refresh");
    } catch {
      applyBridgeStatus({ connected: false, reason: "Bridge offline" }, "refresh");
    }
  }

  async function loadProjects() {
    setProjects(await requestJson<Project[]>("/api/projects"));
  }

  async function loadUsers() {
    setUsers(await requestJson<UserProfile[]>("/api/users"));
  }

  async function loadClasses() {
    setClasses(await requestJson<ClassProfile[]>("/api/classes"));
  }

  async function refreshAll() {
    await Promise.allSettled([loadBridgeStatus(), loadProjects(), loadUsers(), loadClasses()]);
  }

  async function sendBridgeCommand(action: string, data: unknown = {}) {
    if (!liveControl && !["stop", "ready", "move-step-over"].includes(action)) {
      setCommandMessage("Controls locked");
      recordEvent("warning", "Command blocked: live control is disabled");
      return;
    }

    if (!safeToMove && !["stop", "ready", "move-step-over"].includes(action)) {
      setCommandMessage(blocked);
      recordEvent("warning", `Command blocked: ${blocked}`);
      return;
    }

    const endpoint = `${station.bridgeBase}/${action}`;
    try {
      setCommandMessage(action);
      recordEvent(action === "stop" ? "warning" : "info", `Command sent: ${action}`);
      await requestJson(endpoint, {
        method: "POST",
        body: JSON.stringify(data),
      });
      await loadBridgeStatus();
    } catch (error) {
      const message = error instanceof Error ? error.message : "Command failed";
      setCommandMessage(message);
      recordEvent("error", `Command failed: ${message}`);
    }
  }

  function newProject() {
    setCurrentProject(emptyProject);
    setProjectMessage("New project");
  }

  async function openProject(id?: string) {
    if (!id) return;
    const project = await requestJson<Project>(`/api/projects/${encodeURIComponent(id)}`);
    setCurrentProject(project);
    setProjectMessage(`${project.title} loaded`);
  }

  async function saveProject(event?: FormEvent) {
    event?.preventDefault();
    const payload: Project = {
      title: currentProject.title.trim() || "Untitled Project",
      owner: currentProject.owner?.trim() || "",
      mode: "blocks-and-python",
      blockly: currentProject.blockly || { workspaceVersion: 1, blocks: [] },
      python: currentProject.python || "",
      generatedPython: currentProject.generatedPython || "",
      safetyProfileId: currentProject.safetyProfileId || "pilot-default",
      metadata: {
        savedFrom: "react-controller",
        lastManualCommand: commandMessage,
      },
    };

    const saved = currentProject.id
      ? await requestJson<Project>(`/api/projects/${encodeURIComponent(currentProject.id)}`, {
          method: "PUT",
          body: JSON.stringify(payload),
        })
      : await requestJson<Project>("/api/projects", {
          method: "POST",
          body: JSON.stringify(payload),
        });

    setCurrentProject(saved);
    setProjectMessage(`${saved.title} saved`);
    await loadProjects();
  }

  async function duplicateProject() {
    if (!currentProject.id) return;
    const saved = await requestJson<Project>("/api/projects", {
      method: "POST",
      body: JSON.stringify({
        ...currentProject,
        id: undefined,
        title: `Copy of ${currentProject.title || "Untitled Project"}`,
      }),
    });
    setCurrentProject(saved);
    setProjectMessage(`${saved.title} saved`);
    await loadProjects();
  }

  async function deleteProject() {
    if (!currentProject.id) return;
    await requestJson(`/api/projects/${encodeURIComponent(currentProject.id)}`, { method: "DELETE" });
    newProject();
    setProjectMessage("Project deleted");
    await loadProjects();
  }

  async function importProject(event: ChangeEvent<HTMLInputElement>) {
    const [file] = event.target.files || [];
    event.target.value = "";
    if (!file) return;

    const imported = JSON.parse(await file.text()) as Project;
    const saved = await requestJson<Project>("/api/projects", {
      method: "POST",
      body: JSON.stringify({
        ...imported,
        id: undefined,
        title: imported.title ? `Imported ${imported.title}` : "Imported Project",
      }),
    });
    setCurrentProject(saved);
    setProjectMessage(`${saved.title} imported`);
    await loadProjects();
  }

  async function saveUser(event?: FormEvent) {
    event?.preventDefault();
    if (!currentUser.displayName.trim()) {
      setClassroomMessage("Profile name is required");
      return;
    }

    const payload = {
      ...currentUser,
      displayName: currentUser.displayName.trim(),
      email: currentUser.email?.trim() || "",
      active: true,
    };

    const saved = currentUser.id
      ? await requestJson<UserProfile>(`/api/users/${encodeURIComponent(currentUser.id)}`, {
          method: "PUT",
          body: JSON.stringify(payload),
        })
      : await requestJson<UserProfile>("/api/users", {
          method: "POST",
          body: JSON.stringify(payload),
        });

    setCurrentUser(saved);
    setClassroomMessage(`${saved.displayName} saved`);
    await loadUsers();
  }

  async function deleteUser() {
    if (!currentUser.id || currentUser.id === "station-admin") return;
    await requestJson(`/api/users/${encodeURIComponent(currentUser.id)}`, { method: "DELETE" });
    setCurrentUser(emptyUser);
    setClassroomMessage("Profile deleted");
    await loadUsers();
  }

  async function saveClass(event?: FormEvent) {
    event?.preventDefault();
    if (!currentClass.name.trim()) {
      setClassroomMessage("Class name is required");
      return;
    }

    const payload = {
      ...currentClass,
      name: currentClass.name.trim(),
      term: currentClass.term?.trim() || "",
    };

    const saved = currentClass.id
      ? await requestJson<ClassProfile>(`/api/classes/${encodeURIComponent(currentClass.id)}`, {
          method: "PUT",
          body: JSON.stringify(payload),
        })
      : await requestJson<ClassProfile>("/api/classes", {
          method: "POST",
          body: JSON.stringify(payload),
        });

    setCurrentClass(saved);
    setClassroomMessage(`${saved.name} saved`);
    await loadClasses();
  }

  async function deleteClass() {
    if (!currentClass.id) return;
    await requestJson(`/api/classes/${encodeURIComponent(currentClass.id)}`, { method: "DELETE" });
    setCurrentClass(emptyClass);
    setClassroomMessage("Class deleted");
    await loadClasses();
  }

  const content = useMemo(() => {
    switch (activeView) {
      case "dashboard":
        return (
          <Dashboard
            station={station}
            bridge={bridge}
            blocked={blocked}
            projects={projects}
            users={users}
            classes={classes}
            telemetryMode={telemetryMode}
            telemetryAt={telemetryAt}
            events={events}
            onClearEvents={clearEvents}
            onNavigate={setActiveView}
            onRefresh={refreshAll}
          />
        );
      case "manual":
        return (
          <ManualControl
            bridge={bridge}
            blocked={blocked}
            safeToMove={safeToMove}
            liveControl={liveControl}
            gripperOpen={gripperOpen}
            commandMessage={commandMessage}
            telemetryMode={telemetryMode}
            telemetryAt={telemetryAt}
            events={events}
            joystickIntent={joystickIntent}
            onClearEvents={clearEvents}
            onPreviewJoystickIntent={setJoystickIntent}
            onClearJoystickIntent={clearJoystickIntent}
            onToggleLive={() => setLiveControl((value) => !value)}
            onSetGripperOpen={setGripperOpen}
            onCommand={sendBridgeCommand}
          />
        );
      case "projects":
        return (
          <Projects
            projects={projects}
            currentProject={currentProject}
            message={projectMessage}
            onChangeProject={setCurrentProject}
            onNew={newProject}
            onOpen={openProject}
            onSave={saveProject}
            onDuplicate={duplicateProject}
            onDelete={deleteProject}
            onExport={() => currentProject.id && downloadJSON(`${slug(currentProject.title)}.json`, currentProject)}
            onImport={importProject}
            onRefresh={loadProjects}
          />
        );
      case "classroom":
        return (
          <Classroom
            users={users}
            classes={classes}
            currentUser={currentUser}
            currentClass={currentClass}
            message={classroomMessage}
            onChangeUser={setCurrentUser}
            onChangeClass={setCurrentClass}
            onSaveUser={saveUser}
            onDeleteUser={deleteUser}
            onClearUser={() => setCurrentUser(emptyUser)}
            onSaveClass={saveClass}
            onDeleteClass={deleteClass}
            onClearClass={() => setCurrentClass(emptyClass)}
            onRefresh={async () => {
              await Promise.all([loadUsers(), loadClasses()]);
              setClassroomMessage("Refreshed");
            }}
          />
        );
      case "settings":
        return <SettingsView station={station} bridge={bridge} telemetryMode={telemetryMode} telemetryAt={telemetryAt} onRefresh={refreshAll} />;
      default:
        return null;
    }
  }, [
    activeView,
    blocked,
    bridge,
    classes,
    classroomMessage,
    commandMessage,
    currentClass,
    currentProject,
    currentUser,
    gripperOpen,
    joystickIntent,
    liveControl,
    projectMessage,
    projects,
    safeToMove,
    station,
    telemetryAt,
    telemetryMode,
    events,
    clearEvents,
    clearJoystickIntent,
    users,
  ]);

  return (
    <div className={sidebarCollapsed ? "app-shell sidebar-collapsed" : "app-shell"}>
      <aside className={sidebarCollapsed ? "sidebar collapsed" : "sidebar"}>
        <div className="brand">
          <Bot aria-hidden="true" size={28} />
          <div>
            <p className="eyebrow">ORA Classroom</p>
            <h1>ORA Arm Control</h1>
          </div>
        </div>

        <nav className="nav" role="tablist" aria-label="Primary sections">
          {views.map((view) => (
            <button
              key={view.id}
              type="button"
              role="tab"
              aria-selected={activeView === view.id}
              className={activeView === view.id ? "nav-item active" : "nav-item"}
              onClick={() => setActiveView(view.id)}
            >
              {view.icon}
              <span>{view.label}</span>
            </button>
          ))}
        </nav>

        <button
          className="sidebar-toggle-btn"
          type="button"
          aria-label={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"}
          onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
        >
          {sidebarCollapsed ? <PanelLeftOpen size={20} /> : <PanelLeftClose size={20} />}
          {!sidebarCollapsed && <span>Collapse Sidebar</span>}
        </button>
      </aside>

      <main className="main-panel">
        <header className="topbar">
          <div>
            <p className="eyebrow">Station</p>
            <h2>{station.oraName || "ORA station"}</h2>
            <p>{station.oraIp}</p>
          </div>
          <StatusBadge bridge={bridge} blocked={blocked} />
        </header>

        {content}
      </main>
    </div>
  );
}

function ModelPopout() {
  const [bridge, setBridge] = useState<BridgeStatus>({ connected: false, reason: "Connecting to station" });
  const [telemetryMode, setTelemetryMode] = useState<TelemetryMode>("connecting");
  const [telemetryAt, setTelemetryAt] = useState("");
  const [modelFollowsLive, setModelFollowsLive] = useState(true);
  const [cameraLocked, setCameraLocked] = useState(false);
  const [resetCameraSignal, setResetCameraSignal] = useState(0);

  const loadBridgeStatus = useCallback(async () => {
    try {
      setBridge(await requestJson<BridgeStatus>("/bridge/status"));
    } catch {
      setBridge({ connected: false, reason: "Bridge offline" });
    }
  }, []);

  useEffect(() => {
    if (!("EventSource" in window)) {
      setTelemetryMode("fallback");
      void loadBridgeStatus();
      return;
    }

    let closed = false;
    const source = new EventSource("/api/telemetry");
    source.addEventListener("open", () => {
      if (!closed) setTelemetryMode("streaming");
    });
    source.addEventListener("status", (event) => {
      try {
        const envelope = JSON.parse(event.data) as TelemetryEnvelope;
        setTelemetryAt(envelope.at || new Date().toISOString());
        setTelemetryMode("streaming");
        setBridge(normalizeTelemetry(envelope));
      } catch {
        setTelemetryMode("fallback");
      }
    });
    source.onerror = () => {
      if (!closed) setTelemetryMode("fallback");
    };

    return () => {
      closed = true;
      source.close();
    };
  }, [loadBridgeStatus]);

  const blocked = bridge.connected ? robotBlockedMessage(bridge.latest) : "Bridge offline";
  const safeToMove = bridge.connected && !blocked;

  return (
    <main className="model-popout-shell" aria-label="Detached 3D arm model">
      <div className="model-popout-toolbar">
        <div>
          <p className="eyebrow">ORA visualization</p>
          <h2>3D Arm Model</h2>
        </div>
        <div className="model-toolbar">
          <button type="button" aria-pressed={!modelFollowsLive} onClick={() => setModelFollowsLive((value) => !value)}>
            {modelFollowsLive ? <Pause size={18} /> : <Play size={18} />}
            {modelFollowsLive ? "Freeze Model" : "Follow Live Pose"}
          </button>
          <button type="button" onClick={() => setResetCameraSignal((value) => value + 1)}>
            <RefreshCw size={18} /> Reset Camera
          </button>
          <button type="button" aria-pressed={cameraLocked} onClick={() => setCameraLocked((value) => !value)}>
            {cameraLocked ? <Unlock size={18} /> : <Lock size={18} />}
            {cameraLocked ? "Unlock Camera" : "Lock Camera"}
          </button>
        </div>
      </div>
      <SafetyStrip bridge={bridge} blocked={blocked} telemetryMode={telemetryMode} telemetryAt={telemetryAt} />
      <ArmVisualization
        status={bridge.latest}
        blocked={blocked}
        liveControl={false}
        safeToMove={safeToMove}
        joystickIntent={idleJoystickIntent}
        followPose={modelFollowsLive}
        cameraLocked={cameraLocked}
        resetCameraSignal={resetCameraSignal}
      />
    </main>
  );
}

function Dashboard(props: {
  station: Station;
  bridge: BridgeStatus;
  blocked: string;
  projects: Project[];
  users: UserProfile[];
  classes: ClassProfile[];
  telemetryMode: TelemetryMode;
  telemetryAt: string;
  events: StationEvent[];
  onClearEvents: () => void;
  onNavigate: (view: View) => void;
  onRefresh: () => Promise<void>;
}) {
  const latestProject = props.projects[0];
  return (
    <section className="page-section" aria-label="Dashboard">
      <div className="section-heading">
        <div>
          <h2>Dashboard</h2>
          <p>Station overview, recent work, and current robot readiness.</p>
        </div>
        <button className="icon-button" type="button" onClick={() => void props.onRefresh()} aria-label="Refresh dashboard">
          <RefreshCw size={18} />
        </button>
      </div>

      <div className="metric-grid">
        <Metric icon={<ShieldCheck size={20} />} label="Robot Status" value={props.bridge.connected ? "Bridge online" : "Bridge offline"} detail={props.blocked || "Ready"} />
        <Metric icon={<Gauge size={20} />} label="Telemetry" value={props.telemetryMode === "streaming" ? "Streaming" : props.telemetryMode === "connecting" ? "Connecting" : "Retrying"} detail={formatTelemetryTime(props.telemetryAt)} />
        <Metric icon={<BookOpen size={20} />} label="Projects" value={String(props.projects.length)} detail={latestProject ? `Latest: ${latestProject.title}` : "No saved projects"} />
        <Metric icon={<Users size={20} />} label="Profiles" value={String(props.users.length)} detail={`${props.classes.length} classes`} />
      </div>

      <SafetyFeedback bridge={props.bridge} blocked={props.blocked} telemetryMode={props.telemetryMode} telemetryAt={props.telemetryAt} />

      <div className="action-strip">
        <button type="button" onClick={() => props.onNavigate("manual")}><Joystick size={18} /> Manual Control</button>
        <button type="button" onClick={() => props.onNavigate("projects")}><BookOpen size={18} /> Open Projects</button>
        <button type="button" onClick={() => props.onNavigate("classroom")}><GraduationCap size={18} /> Manage Classroom</button>
      </div>

      <EventLog events={props.events} onClear={props.onClearEvents} />
    </section>
  );
}

function ManualControl(props: {
  bridge: BridgeStatus;
  blocked: string;
  safeToMove: boolean;
  liveControl: boolean;
  gripperOpen: boolean;
  commandMessage: string;
  telemetryMode: TelemetryMode;
  telemetryAt: string;
  events: StationEvent[];
  joystickIntent: JoystickIntent;
  onClearEvents: () => void;
  onPreviewJoystickIntent: (intent: JoystickIntent) => void;
  onClearJoystickIntent: () => void;
  onToggleLive: () => void;
  onSetGripperOpen: (open: boolean) => void;
  onCommand: (action: string, data?: unknown) => Promise<void>;
}) {
  const cockpitRef = useRef<HTMLDivElement | null>(null);
  const [controlPanelPercent, setControlPanelPercent] = useState(() => {
    const saved = window.localStorage.getItem("ora-control-panel-percent");
    if (saved === null) return 42;
    const parsed = Number(saved);
    return Number.isFinite(parsed) ? clampPercent(parsed) : 42;
  });
  const [pendantCollapsed, setPendantCollapsed] = useState(() => {
    return window.localStorage.getItem("ora-pendant-collapsed") === "true";
  });
  const [actionsCollapsed, setActionsCollapsed] = useState(() => {
    return window.localStorage.getItem("ora-actions-collapsed") === "true";
  });
  const [poseCollapsed, setPoseCollapsed] = useState(() => {
    return window.localStorage.getItem("ora-pose-collapsed") === "true";
  });

  useEffect(() => {
    window.localStorage.setItem("ora-pendant-collapsed", String(pendantCollapsed));
  }, [pendantCollapsed]);

  useEffect(() => {
    window.localStorage.setItem("ora-actions-collapsed", String(actionsCollapsed));
  }, [actionsCollapsed]);

  useEffect(() => {
    window.localStorage.setItem("ora-pose-collapsed", String(poseCollapsed));
  }, [poseCollapsed]);

  const [modelFollowsLive, setModelFollowsLive] = useState(true);
  const [cameraLocked, setCameraLocked] = useState(false);
  const [resetCameraSignal, setResetCameraSignal] = useState(0);
  const [stepSize, setStepSize] = useState("small");
  const [speedPercent, setSpeedPercent] = useState(35);
  const pose = props.bridge.latest?.xarm_tcp_pose;
  const jointPose = props.bridge.latest?.xarm_joint_pose;
  const controlsEnabled = props.safeToMove && props.liveControl;
  const cockpitStyle = { "--control-panel-width": `${controlPanelPercent}%` } as CSSProperties;
  const poseItems = labeledPose([
    { label: "X", value: pose?.[0] },
    { label: "Y", value: pose?.[1] },
    { label: "Z", value: pose?.[2] },
    { label: "Roll", value: pose?.[3] },
    { label: "Pitch", value: pose?.[4] },
    { label: "Yaw", value: pose?.[5] },
  ]);
  const jointItems = labeledPose([
    { label: "J1", value: jointPose?.[0] },
    { label: "J2", value: jointPose?.[1] },
    { label: "J3", value: jointPose?.[2] },
    { label: "J4", value: jointPose?.[3] },
    { label: "J5", value: jointPose?.[4] },
    { label: "J6", value: jointPose?.[5] },
  ]);

  const intents = {
    yIncrease: { x: 0, y: 1, z: 0, label: "Y+" },
    yDecrease: { x: 0, y: -1, z: 0, label: "Y-" },
    xIncrease: { x: 1, y: 0, z: 0, label: "X+" },
    xDecrease: { x: -1, y: 0, z: 0, label: "X-" },
    zIncrease: { x: 0, y: 0, z: 1, label: "Z up" },
    zDecrease: { x: 0, y: 0, z: -1, label: "Z down" },
  } satisfies Record<string, JoystickIntent>;

  useEffect(() => {
    window.localStorage.setItem("ora-control-panel-percent", String(controlPanelPercent));
  }, [controlPanelPercent]);

  function intentHandlers(intent: JoystickIntent) {
    return {
      onPointerEnter: () => props.onPreviewJoystickIntent(intent),
      onPointerDown: () => props.onPreviewJoystickIntent(intent),
      onPointerLeave: props.onClearJoystickIntent,
      onPointerUp: props.onClearJoystickIntent,
      onFocus: () => props.onPreviewJoystickIntent(intent),
      onBlur: props.onClearJoystickIntent,
    };
  }

  async function step(direction: string, intent: JoystickIntent) {
    props.onPreviewJoystickIntent(intent);
    try {
      await props.onCommand("move-step", { isLoop: true, direction, isMoveTool: false, stepSize, speedScale: speedPercent / 100 });
      await props.onCommand("move-step-over");
    } finally {
      props.onClearJoystickIntent();
    }
  }

  function startResize(event: ReactPointerEvent<HTMLButtonElement>) {
    const container = cockpitRef.current;
    if (!container) return;

    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);

    const update = (clientX: number) => {
      const rect = container.getBoundingClientRect();
      if (!rect.width) return;
      const nextPercent = ((clientX - rect.left) / rect.width) * 100;
      setControlPanelPercent(clampPercent(nextPercent));
    };

    const onMove = (moveEvent: PointerEvent) => update(moveEvent.clientX);
    const onUp = () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
    };

    update(event.clientX);
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
  }

  function popOutModel() {
    const url = `${window.location.origin}${window.location.pathname}?view=model-popout`;
    window.open(url, "ora-model-popout", "popup,width=1120,height=820");
  }

  return (
    <section className="page-section manual-workspace" aria-label="Manual control">
      <div className="section-heading manual-heading">
        <div>
          <h2>Manual Control</h2>
          <p>Teacher-gated direct controls for supervised positioning.</p>
        </div>
        <button className={props.liveControl ? "armed-button" : ""} type="button" disabled={!props.safeToMove} onClick={props.onToggleLive}>
          <ShieldCheck size={18} /> {props.liveControl ? "Disable Live Control" : "Enable Live Control"}
        </button>
      </div>

      <SafetyStrip bridge={props.bridge} blocked={props.blocked} telemetryMode={props.telemetryMode} telemetryAt={props.telemetryAt} />

      <div className="manual-cockpit" ref={cockpitRef} style={cockpitStyle}>
        <section className="control-rail" aria-label="Control panel">
          <div className="tool-panel teach-panel">
            <div className="panel-headline" style={{ marginBottom: pendantCollapsed ? 0 : '14px' }}>
              <button
                className="collapsible-toggle-btn"
                type="button"
                aria-expanded={!pendantCollapsed}
                onClick={() => setPendantCollapsed(!pendantCollapsed)}
              >
                {pendantCollapsed ? <ChevronRight size={16} /> : <ChevronDown size={16} />}
                <div>
                  <h3>Teach Pendant</h3>
                  {!pendantCollapsed && <p>Step motion with teacher-gated live control.</p>}
                </div>
              </button>
              <span className={controlsEnabled ? "status-pill ok" : "status-pill warning"}>{controlsEnabled ? "Armed" : "Locked"}</span>
            </div>

            {!pendantCollapsed && (
              <>
                <div className="control-tuning">
                  <label>
                    <span>Step</span>
                    <select value={stepSize} onChange={(event) => setStepSize(event.target.value)}>
                      <option value="small">Small</option>
                      <option value="medium">Medium</option>
                      <option value="large">Large</option>
                    </select>
                  </label>
                  <label>
                    <span>Speed</span>
                    <input aria-label="Speed limit" type="range" min="10" max="100" step="5" value={speedPercent} onChange={(event) => setSpeedPercent(Number(event.target.value))} />
                  </label>
                  <div className="speed-readout">
                    <span>Limit</span>
                    <strong>{speedPercent}%</strong>
                  </div>
                </div>

                <div className="movement-pad-row">
                  <div className="xy-pad" aria-label="XY movement pad">
                    <button type="button" disabled={!controlsEnabled} {...intentHandlers(intents.yIncrease)} onClick={() => void step("position-y-increase", intents.yIncrease)}>Y+</button>
                    <button type="button" disabled={!controlsEnabled} {...intentHandlers(intents.xDecrease)} onClick={() => void step("position-x-decrease", intents.xDecrease)}>X-</button>
                    <div className="pad-center"><Joystick size={26} /></div>
                    <button type="button" disabled={!controlsEnabled} {...intentHandlers(intents.xIncrease)} onClick={() => void step("position-x-increase", intents.xIncrease)}>X+</button>
                    <button type="button" disabled={!controlsEnabled} {...intentHandlers(intents.yDecrease)} onClick={() => void step("position-y-decrease", intents.yDecrease)}>Y-</button>
                  </div>
                  <div className="vertical-pad" aria-label="Z movement pad">
                    <button type="button" disabled={!controlsEnabled} {...intentHandlers(intents.zIncrease)} onClick={() => void step("position-z-increase", intents.zIncrease)}><ChevronDown className="flip" size={18} /> Z Up</button>
                    <button type="button" disabled={!controlsEnabled} {...intentHandlers(intents.zDecrease)} onClick={() => void step("position-z-decrease", intents.zDecrease)}><ChevronDown size={18} /> Z Down</button>
                  </div>
                </div>

                <JoystickIntentView intent={props.joystickIntent} enabled={controlsEnabled} />
              </>
            )}
          </div>

          <div className="tool-panel action-panel">
            <div className="panel-headline" style={{ marginBottom: actionsCollapsed ? 0 : '14px' }}>
              <button
                className="collapsible-toggle-btn"
                type="button"
                aria-expanded={!actionsCollapsed}
                onClick={() => setActionsCollapsed(!actionsCollapsed)}
              >
                {actionsCollapsed ? <ChevronRight size={16} /> : <ChevronDown size={16} />}
                <h3>Station Actions</h3>
              </button>
            </div>
            {!actionsCollapsed && (
              <>
                <div className="button-stack">
                  <button className="danger-button" type="button" onClick={() => void props.onCommand("stop")}><CircleStop size={18} /> Emergency Stop</button>
                  <button type="button" onClick={() => void props.onCommand("ready")}><CheckCircle2 size={18} /> Set Ready</button>
                  <button type="button" disabled={!controlsEnabled} onClick={() => void props.onCommand("home")}><Home size={18} /> Initial Position</button>
                  <button
                    type="button"
                    disabled={!controlsEnabled}
                    onClick={() => {
                      const open = !props.gripperOpen;
                      props.onSetGripperOpen(open);
                      void props.onCommand("gripper", { open });
                    }}
                  >
                    <Boxes size={18} /> {props.gripperOpen ? "Close Gripper" : "Open Gripper"}
                  </button>
                </div>
                <CompactPoseSummary pose={poseItems} joints={jointItems} command={props.commandMessage} />
              </>
            )}
          </div>
        </section>

        <button className="cockpit-resizer" type="button" aria-label="Resize cockpit panels" onPointerDown={startResize}>
          <span />
        </button>

        <section className="model-rail" aria-label="3D model panel">
          <div className="model-toolbar">
            <button type="button" aria-pressed={!modelFollowsLive} onClick={() => setModelFollowsLive((value) => !value)}>
              {modelFollowsLive ? <Pause size={18} /> : <Play size={18} />}
              {modelFollowsLive ? "Freeze Model" : "Follow Live Pose"}
            </button>
            <button type="button" onClick={() => setResetCameraSignal((value) => value + 1)}>
              <RefreshCw size={18} /> Reset Camera
            </button>
            <button type="button" aria-pressed={cameraLocked} onClick={() => setCameraLocked((value) => !value)}>
              {cameraLocked ? <Unlock size={18} /> : <Lock size={18} />}
              {cameraLocked ? "Unlock Camera" : "Lock Camera"}
            </button>
            <button type="button" onClick={popOutModel}>
              <ExternalLink size={18} /> Pop Out Model
            </button>
          </div>
          <ArmVisualization
            status={props.bridge.latest}
            blocked={props.blocked}
            liveControl={props.liveControl}
            safeToMove={props.safeToMove}
            joystickIntent={props.joystickIntent}
            followPose={modelFollowsLive}
            cameraLocked={cameraLocked}
            resetCameraSignal={resetCameraSignal}
          />
        </section>
      </div>

      <div className="manual-support-grid">
        <section className="tool-panel telemetry-compact" aria-label="Telemetry panel">
          <div className="panel-headline" style={{ marginBottom: poseCollapsed ? 0 : '14px' }}>
            <button
              className="collapsible-toggle-btn"
              type="button"
              aria-expanded={!poseCollapsed}
              onClick={() => setPoseCollapsed(!poseCollapsed)}
            >
              {poseCollapsed ? <ChevronRight size={16} /> : <ChevronDown size={16} />}
              <h3>Live Pose</h3>
            </button>
          </div>
          {!poseCollapsed && (
            <div className="telemetry-grid">
              <PoseGrid label="TCP Pose" items={poseItems} />
              <PoseGrid label="Joint Pose" items={jointItems} />
            </div>
          )}
        </section>
        <EventLog events={props.events} onClear={props.onClearEvents} />
      </div>
    </section>
  );
}

function Projects(props: {
  projects: Project[];
  currentProject: Project;
  message: string;
  onChangeProject: (project: Project) => void;
  onNew: () => void;
  onOpen: (id?: string) => Promise<void>;
  onSave: (event?: FormEvent) => Promise<void>;
  onDuplicate: () => Promise<void>;
  onDelete: () => Promise<void>;
  onExport: () => void;
  onImport: (event: ChangeEvent<HTMLInputElement>) => Promise<void>;
  onRefresh: () => Promise<void>;
}) {
  return (
    <section className="page-section" aria-label="Projects">
      <div className="section-heading">
        <div>
          <h2>Projects</h2>
          <p>Save, reopen, export, and import student work while offline.</p>
        </div>
        <button className="icon-button" type="button" onClick={() => void props.onRefresh()} aria-label="Refresh projects"><RefreshCw size={18} /></button>
      </div>

      <div className="split-layout">
        <form className="form-panel" onSubmit={(event) => void props.onSave(event)}>
          <div className="form-row">
            <label>
              <span>Project</span>
              <input value={props.currentProject.title} onChange={(event) => props.onChangeProject({ ...props.currentProject, title: event.target.value })} placeholder="Untitled Project" />
            </label>
            <label>
              <span>Student</span>
              <input value={props.currentProject.owner || ""} onChange={(event) => props.onChangeProject({ ...props.currentProject, owner: event.target.value })} placeholder="Student name" />
            </label>
          </div>
          <label>
            <span>Python draft</span>
            <textarea value={props.currentProject.python || ""} onChange={(event) => props.onChangeProject({ ...props.currentProject, python: event.target.value })} rows={10} placeholder="# Python preview and editable code will appear here" />
          </label>
          <p className="status-line">{props.message}</p>
          <div className="action-row">
            <button type="button" onClick={props.onNew}><Plus size={18} /> New</button>
            <button type="submit"><Save size={18} /> Save</button>
            <button type="button" disabled={!props.currentProject.id} onClick={() => void props.onDuplicate()}><Upload size={18} /> Duplicate</button>
            <button type="button" disabled={!props.currentProject.id} onClick={props.onExport}><Download size={18} /> Export</button>
            <label className="file-button">
              <Import size={18} /> Import
              <input type="file" accept="application/json,.json" hidden onChange={(event) => void props.onImport(event)} />
            </label>
            <button className="danger-button" type="button" disabled={!props.currentProject.id} onClick={() => void props.onDelete()}><Trash2 size={18} /> Delete</button>
          </div>
        </form>

        <ItemList
          title="Saved Projects"
          empty="No saved projects"
          items={props.projects.map((project) => ({
            id: project.id || "",
            title: project.title || "Untitled Project",
            meta: `${project.owner || "No student"} · ${formatDate(project.updatedAt)}`,
            active: project.id === props.currentProject.id,
            onClick: () => void props.onOpen(project.id),
          }))}
        />
      </div>
    </section>
  );
}

function Classroom(props: {
  users: UserProfile[];
  classes: ClassProfile[];
  currentUser: UserProfile;
  currentClass: ClassProfile;
  message: string;
  onChangeUser: (user: UserProfile) => void;
  onChangeClass: (classProfile: ClassProfile) => void;
  onSaveUser: (event?: FormEvent) => Promise<void>;
  onDeleteUser: () => Promise<void>;
  onClearUser: () => void;
  onSaveClass: (event?: FormEvent) => Promise<void>;
  onDeleteClass: () => Promise<void>;
  onClearClass: () => void;
  onRefresh: () => Promise<void>;
}) {
  return (
    <section className="page-section" aria-label="Classroom">
      <div className="section-heading">
        <div>
          <h2>Classroom</h2>
          <p>Local profiles and classes for the one-station pilot.</p>
        </div>
        <button className="icon-button" type="button" onClick={() => void props.onRefresh()} aria-label="Refresh classroom"><RefreshCw size={18} /></button>
      </div>

      <div className="two-column">
        <form className="form-panel" onSubmit={(event) => void props.onSaveUser(event)}>
          <h3>User Profiles</h3>
          <label>
            <span>Profile name</span>
            <input value={props.currentUser.displayName} onChange={(event) => props.onChangeUser({ ...props.currentUser, displayName: event.target.value })} placeholder="Student or teacher name" />
          </label>
          <div className="form-row">
            <label>
              <span>Role</span>
              <select value={props.currentUser.role} onChange={(event) => props.onChangeUser({ ...props.currentUser, role: event.target.value as UserProfile["role"] })}>
                <option value="student">Student author</option>
                <option value="operator">Selected student operator</option>
                <option value="teacher">Teacher</option>
                <option value="admin">Admin</option>
              </select>
            </label>
            <label>
              <span>Email</span>
              <input type="email" value={props.currentUser.email || ""} onChange={(event) => props.onChangeUser({ ...props.currentUser, email: event.target.value })} placeholder="optional" />
            </label>
          </div>
          <div className="action-row">
            <button type="submit"><Save size={18} /> {props.currentUser.id ? "Update Profile" : "Add Profile"}</button>
            <button type="button" onClick={props.onClearUser}><Square size={18} /> Clear</button>
            <button className="danger-button" type="button" disabled={!props.currentUser.id || props.currentUser.id === "station-admin"} onClick={() => void props.onDeleteUser()}><Trash2 size={18} /> Delete Profile</button>
          </div>
          <p className="status-line">{props.message}</p>

          <ItemList
            title="Profiles"
            empty="No profiles"
            items={props.users.map((user) => ({
              id: user.id || "",
              title: user.displayName,
              meta: `${roleLabel(user.role)}${user.email ? ` · ${user.email}` : ""}`,
              active: user.id === props.currentUser.id,
              onClick: () => props.onChangeUser(user),
            }))}
          />
        </form>

        <form className="form-panel" onSubmit={(event) => void props.onSaveClass(event)}>
          <h3>Classes</h3>
          <div className="form-row">
            <label>
              <span>Class name</span>
              <input value={props.currentClass.name} onChange={(event) => props.onChangeClass({ ...props.currentClass, name: event.target.value })} placeholder="Robotics 1" />
            </label>
            <label>
              <span>Term</span>
              <input value={props.currentClass.term || ""} onChange={(event) => props.onChangeClass({ ...props.currentClass, term: event.target.value })} placeholder="2026" />
            </label>
          </div>
          <div className="action-row">
            <button type="submit"><Save size={18} /> {props.currentClass.id ? "Update Class" : "Add Class"}</button>
            <button type="button" onClick={props.onClearClass}><Square size={18} /> Clear</button>
            <button className="danger-button" type="button" disabled={!props.currentClass.id} onClick={() => void props.onDeleteClass()}><Trash2 size={18} /> Delete Class</button>
          </div>
          <p className="status-line">{props.message}</p>

          <ItemList
            title="Classes"
            empty="No classes"
            items={props.classes.map((classProfile) => ({
              id: classProfile.id || "",
              title: classProfile.name,
              meta: classProfile.term || "No term",
              active: classProfile.id === props.currentClass.id,
              onClick: () => props.onChangeClass(classProfile),
            }))}
          />
        </form>
      </div>
    </section>
  );
}

function SettingsView(props: { station: Station; bridge: BridgeStatus; telemetryMode: TelemetryMode; telemetryAt: string; onRefresh: () => Promise<void> }) {
  return (
    <section className="page-section" aria-label="Settings">
      <div className="section-heading">
        <div>
          <h2>Settings</h2>
          <p>Station diagnostics and local storage details.</p>
        </div>
        <button className="icon-button" type="button" onClick={() => void props.onRefresh()} aria-label="Refresh settings"><RefreshCw size={18} /></button>
      </div>

      <div className="metric-grid">
        <Metric icon={<Bot size={20} />} label="Device" value={props.station.oraName} detail={props.station.oraIp} />
        <Metric icon={<HardDrive size={20} />} label="Storage" value="SQLite" detail="runtime/data/ora.db" />
        <Metric icon={<ClipboardList size={20} />} label="Bridge" value={props.bridge.connected ? "Connected" : "Offline"} detail="/bridge/status" />
        <Metric icon={<Gauge size={20} />} label="Telemetry" value={props.telemetryMode} detail={formatTelemetryTime(props.telemetryAt)} />
      </div>

      <div className="tool-panel">
        <h3>Enabled Features</h3>
        <div className="chip-row">
          {props.station.features.map((feature) => <span className="chip" key={feature}>{feature}</span>)}
        </div>
      </div>
    </section>
  );
}

function StatusBadge(props: { bridge: BridgeStatus; blocked: string }) {
  return (
    <div className={props.bridge.connected ? "status-badge online" : "status-badge offline"}>
      {props.bridge.connected ? <CheckCircle2 size={18} /> : <AlertTriangle size={18} />}
      <div>
        <strong>{props.bridge.connected ? "Bridge online" : "Bridge offline"}</strong>
        <span>{props.blocked || "Ready"}</span>
      </div>
    </div>
  );
}

function Metric(props: { icon: ReactNode; label: string; value: string; detail: string }) {
  return (
    <div className="metric-card">
      <div className="metric-icon">{props.icon}</div>
      <span>{props.label}</span>
      <strong>{props.value}</strong>
      <p>{props.detail}</p>
    </div>
  );
}

function SafetyStrip(props: { bridge: BridgeStatus; blocked: string; telemetryMode: TelemetryMode; telemetryAt: string }) {
  const latest = props.bridge.latest;
  const severity = bridgeSeverity(props.bridge);
  const stateText = typeof latest?.xarm_state === "number" ? `State ${latest.xarm_state}` : "State unknown";
  const errorCode = latest?.xarm_error_code ?? 0;

  return (
    <section className={`safety-strip ${severity}`} aria-label="Robot safety status">
      <div className="safety-strip-message">
        <strong>{props.blocked || "Ready"}</strong>
        <span>{severity === "ok" ? "Movement available after live control is enabled" : "Motion is blocked until this condition clears"}</span>
      </div>
      <Fact label="Bridge" value={props.bridge.connected ? "Online" : "Offline"} />
      <Fact label="Arm" value={latest?.xarm_connected ? "Connected" : "Disconnected"} />
      <Fact label="Motion" value={latest?.xarm_is_ready ? "Ready" : "Locked"} />
      <Fact label="xArm state" value={stateText} />
      <Fact label="Error" value={String(errorCode)} />
      <Fact label="Telemetry" value={props.telemetryMode === "streaming" ? "Streaming" : props.telemetryMode === "connecting" ? "Connecting" : "Retrying"} detail={formatTelemetryTime(props.telemetryAt)} />
    </section>
  );
}

function SafetyFeedback(props: { bridge: BridgeStatus; blocked: string; telemetryMode: TelemetryMode; telemetryAt: string }) {
  const latest = props.bridge.latest;
  const severity = bridgeSeverity(props.bridge);
  const stateText = typeof latest?.xarm_state === "number" ? `State ${latest.xarm_state}` : "State unknown";
  const errorCode = latest?.xarm_error_code ?? 0;

  return (
    <div className={`safety-panel ${severity}`} aria-label="Robot safety status">
      <div className="safety-head">
        <h3>Robot Safety Status</h3>
        <span className={`status-pill ${severity}`}>{severity === "ok" ? "Ready" : severity === "warning" ? "Blocked" : "Needs attention"}</span>
      </div>
      <p className="safety-message">{props.blocked || "Ready for teacher-gated motion"}</p>
      <div className="fact-grid">
        <Fact label="Bridge" value={props.bridge.connected ? "Online" : "Offline"} />
        <Fact label="Arm" value={latest?.xarm_connected ? "Connected" : "Disconnected"} />
        <Fact label="Motion" value={latest?.xarm_is_ready ? "Ready" : "Locked"} />
        <Fact label="xArm state" value={stateText} />
        <Fact label="Error code" value={String(errorCode)} />
        <Fact label="Telemetry" value={props.telemetryMode === "streaming" ? "Streaming" : props.telemetryMode === "connecting" ? "Connecting" : "Retrying"} detail={formatTelemetryTime(props.telemetryAt)} />
      </div>
    </div>
  );
}

function JoystickIntentView(props: { intent: JoystickIntent; enabled: boolean }) {
  const x = Math.round(props.intent.x * 38);
  const y = Math.round(-props.intent.y * 38);
  const z = Math.round(props.intent.z * -16);
  const label = props.intent.label || "idle";

  return (
    <div className="joystick-intent" aria-label="Joystick intent view">
      <div className="intent-pad">
        <div className="intent-shadow" />
        <div className="intent-stick" style={{ transform: `translate(${x / 3}px, ${y / 3 + z}px) rotateX(${props.intent.y * 12}deg) rotateZ(${-props.intent.x * 12}deg)` }} />
        <div className="intent-knob" style={{ transform: `translate(${x}px, ${y + z}px)` }} />
      </div>
      <div>
        <span>Joystick intent</span>
        <strong>{props.enabled ? label : "locked"}</strong>
      </div>
    </div>
  );
}

function CompactPoseSummary(props: { pose: Array<{ label: string; value: string }>; joints: Array<{ label: string; value: string }>; command: string }) {
  return (
    <div className="compact-pose">
      <div aria-label="TCP pose summary">
        <span>TCP</span>
        <strong>{props.pose.slice(0, 3).map((item) => `${item.label} ${item.value}`).join(" | ")}</strong>
      </div>
      <div aria-label="Joint pose summary">
        <span>Joints</span>
        <strong>{props.joints.slice(0, 3).map((item) => `${item.label} ${item.value}`).join(" | ")}</strong>
      </div>
      <div>
        <span>Last command</span>
        <strong>{props.command}</strong>
      </div>
    </div>
  );
}

function Fact(props: { label: string; value: string; detail?: string }) {
  return (
    <div className="fact">
      <span>{props.label}</span>
      <strong>{props.value}</strong>
      {props.detail ? <small>{props.detail}</small> : null}
    </div>
  );
}

function PoseGrid(props: { label: string; items: Array<{ label: string; value: string }> }) {
  return (
    <div className="pose-panel" aria-label={`${props.label} readout`}>
      <span>{props.label}</span>
      <div className="pose-grid">
        {props.items.map((item) => (
          <div className="pose-cell" key={item.label}>
            <small>{item.label}</small>
            <strong>{item.value}</strong>
          </div>
        ))}
      </div>
    </div>
  );
}

function EventLog(props: { events: StationEvent[]; onClear: () => void }) {
  const [collapsed, setCollapsed] = useState(() => {
    return window.localStorage.getItem("ora-events-collapsed") === "true";
  });

  useEffect(() => {
    window.localStorage.setItem("ora-events-collapsed", String(collapsed));
  }, [collapsed]);

  return (
    <section className="event-log" aria-label="Station event log">
      <div className="event-log-head" style={{ marginBottom: collapsed ? 0 : '14px' }}>
        <button
          className="collapsible-toggle-btn"
          type="button"
          aria-expanded={!collapsed}
          onClick={() => setCollapsed(!collapsed)}
        >
          {collapsed ? <ChevronRight size={16} /> : <ChevronDown size={16} />}
          <h3>Station Event Log</h3>
        </button>
        <button className="icon-button" type="button" onClick={props.onClear} aria-label="Clear station event log" disabled={props.events.length === 0}>
          <Trash2 size={16} />
        </button>
      </div>
      {!collapsed && (
        <div role="log" aria-live="polite" className="event-list">
          {props.events.length === 0 ? (
            <p className="status-line">Waiting for station events</p>
          ) : props.events.map((event) => (
            <div className={`event-row ${event.severity}`} key={event.id}>
              <time>{formatTelemetryTime(event.at)}</time>
              <span>{event.message}</span>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

function ItemList(props: { title: string; empty: string; items: Array<{ id: string; title: string; meta: string; active?: boolean; onClick: () => void }> }) {
  return (
    <div className="list-panel">
      <h3>{props.title}</h3>
      <div role="list" className="item-list" aria-label={props.title}>
        {props.items.length === 0 ? (
          <p className="status-line">{props.empty}</p>
        ) : props.items.map((item) => (
          <div key={item.id} role="listitem">
            <button type="button" className={item.active ? "list-item active" : "list-item"} onClick={item.onClick}>
              <span>{item.title}</span>
              <small>{item.meta}</small>
            </button>
          </div>
        ))}
      </div>
    </div>
  );
}

function Readout(props: { label: string; values?: number[] }) {
  return (
    <div className="readout">
      <span>{props.label}</span>
      <strong>{props.values?.length ? props.values.map((value) => Number(value).toFixed(1)).join(", ") : "No data"}</strong>
    </div>
  );
}

function slug(value: string): string {
  return value.replace(/[^a-z0-9_-]+/gi, "-").replace(/^-|-$/g, "") || "ora-project";
}

function clampPercent(value: number): number {
  return Math.min(62, Math.max(22, Math.round(value)));
}
