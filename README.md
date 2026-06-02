# ORA Arm Web Controller

This project serves a local ORA arm control page. The recommended path is the Go local server with a React + TypeScript frontend, a local SQLite database, and a proxy to the current ORA bridge. Project-local NGINX remains as a fallback for the older static controller.

## Run

### Option A: Go Local Server

Install Go once on the ORA station:

```powershell
winget install GoLang.Go
```

Start the Go server:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-go-server.ps1 start
```

The script builds the React frontend and the Go server before starting the app. For frontend-only development, run:

```powershell
npm run dev:frontend
```

Open:

```text
http://localhost:8080
```

Manage the Go server:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-go-server.ps1 status
powershell -ExecutionPolicy Bypass -File .\scripts\start-go-server.ps1 restart
powershell -ExecutionPolicy Bypass -File .\scripts\start-go-server.ps1 stop
```

The Go server stores local classroom data in SQLite at `runtime\data\ora.db`.

To build without starting the server:

```powershell
npm run build:frontend
go build -o .\runtime\ora-server.exe .\cmd\ora-server
```

### Option B: NGINX Fallback

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-nginx.ps1
```

Open:

```text
http://localhost:8080
```

## Run With ORA Bridge

The local controller needs the ORA bridge to send real commands. The bridge opens the official Ozobot Editor in Playwright, connects to ORA using the same WebRTC data channel mechanism as the editor, and exposes local endpoints under `/bridge/...`.

Start either the Go server or NGINX first, then start the bridge:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-go-server.ps1 start
powershell -ExecutionPolicy Bypass -File .\scripts\start-bridge.ps1 start
```

Fallback NGINX path:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-nginx.ps1
powershell -ExecutionPolicy Bypass -File .\scripts\start-bridge.ps1 start
```

The bridge reads ORA credentials from `.env`:

```text
ORA_NAME=ORA-FEA252
ORA_PASSWORD=<password>
```

Then open:

```text
http://localhost:8080
```

To open the controller in a clean browser profile with extensions disabled:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\open-controller.ps1
```

The page shows status immediately, but movement and gripper commands are locked until you click `Enable Live Control`. `Emergency Stop` remains available.

Only one editor session should control ORA at a time. Disconnect the official Ozobot Editor before using the local controller. The local bridge itself opens a hidden Ozobot Editor session to reach ORA.

## Restart

If only the bridge loses the WebRTC control channel:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-bridge.ps1 restart
```

If NGINX or the ORA network changes, restart the local app:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\restart-app.ps1
```

To restart using the Go server instead of NGINX:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\restart-app.ps1 -Server go
```

The ORA device name and password are read from `.env` for this station.

Check bridge status:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-bridge.ps1 status
```

## Stop

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\stop-nginx.ps1
powershell -ExecutionPolicy Bypass -File .\scripts\start-go-server.ps1 stop
powershell -ExecutionPolicy Bypass -File .\scripts\start-bridge.ps1 stop
```

The older `scripts\stop-bridge.ps1` command still works as a wrapper.

## ORA Network

The controller proxies browser requests from `/ora/...` to:

```text
http://10.1.48.113/
```

Connect Windows to the ORA device Wi-Fi network before using the controls. The device password is intentionally not stored in this repo.

## Current ORA Transport Status

The official Ozobot Editor can connect to `ORA-FEA252`, but that connection is not a simple local HTTP API at `10.1.48.113`.
The editor sends ORA arm commands such as `xarm_move_line`, `xarm_set_state`, and `xarm_set_lite6_gripper` over WebRTC data channels named `control` and `admin`.

The local bridge connects to the official editor and exposes:

```text
GET  /bridge/status
POST /bridge/command
POST /bridge/stop
POST /bridge/ready
POST /bridge/gripper
POST /bridge/home
POST /bridge/move-step
POST /bridge/move-step-over
POST /bridge/move-line
```

## Go Server APIs

The Go server adds local-first APIs that do not expose the ORA password:

```text
GET    /api/health
GET    /api/station
GET    /api/telemetry
GET    /api/users
POST   /api/users
GET    /api/users/{id}
PUT    /api/users/{id}
DELETE /api/users/{id}
GET    /api/classes
POST   /api/classes
GET    /api/classes/{id}
PUT    /api/classes/{id}
DELETE /api/classes/{id}
GET    /api/projects
POST   /api/projects
GET    /api/projects/{id}
PUT    /api/projects/{id}
DELETE /api/projects/{id}
```

`/api/telemetry` is a Server-Sent Events stream. It polls the current bridge status server-side and gives the browser a realtime feed for live arm visualization.

Student work, project versions, starter role/profile tables, safety profiles, run records, and audit events are stored in:

```text
runtime/data/ora.db
```

Older JSON projects under `runtime/data/projects` are imported into SQLite when the Go server starts. Export/import still uses JSON files so work can be moved between stations.

## Student Project Workflow

When the Go server is running, the controller includes local project storage:

- `New` clears the editor fields.
- `Save` creates or updates a local project.
- `Duplicate` creates a copy of the current project.
- `Export` downloads the current project as JSON.
- `Import` loads an exported project JSON file back into local storage.

This workflow works without public internet. It requires the Go server because NGINX only serves static files and proxy routes.

The current React interface is organized into these sections:

- `Dashboard`: station status, recent work, and quick navigation.
- `Manual Control`: teacher-gated direct controls, live pose readouts, safety feedback, station event log, and emergency stop.
- `Projects`: student work save/open/import/export.
- `Classroom`: local profiles and classes.
- `Settings`: station diagnostics and enabled features.

## Realtime Visualization

The React controller subscribes to `/api/telemetry` and renders:

- bridge online/offline state;
- ORA arm connected/disconnected state;
- xArm ready/stopped/error state;
- TCP pose values for `X`, `Y`, `Z`, `Roll`, `Pitch`, and `Yaw`;
- joint pose values for `J1` through `J6`;
- readable blocked-motion guidance such as `Stopped: click Set Ready` or `Error 22: Self-Collision Error`;
- a station event log for telemetry transitions and command outcomes, with a clear/reset control.

Movement remains locked unless the bridge is connected, ORA reports a safe ready state, and `Enable Live Control` is active. `Emergency Stop` and `Set Ready` remain reachable for recovery workflows.

`Manual Control` is organized as a compact cockpit for one-screen classroom use:

- `Teach Pendant`: separate XY joystick, Z controls, step size, speed limit, and joystick intent preview.
- `Station Actions`: emergency stop, set ready, initial position, gripper, and compact pose summary.
- `3D Model`: live arm model beside the controls, with freeze/follow, camera reset, camera lock, and pop-out controls.
- `Live Pose`: compact TCP and joint readouts.
- `Station Event Log`: telemetry and command history with clear/reset control.

The 3D arm model is implemented with Three.js:

- procedural ORA/xArm-style base, links, joints, wrist, and gripper;
- workspace grid, table surface, reach/safety rings, lighting, shadows, and warning ring;
- TCP pose marker driven from `/api/telemetry`;
- joint rotations driven from telemetry joint values;
- joystick intent is shown separately beside the teach pendant;
- joystick intent also drives a ghost target and target line in the 3D model;
- draggable control/model divider for classroom screens;
- detachable model pop-out window for demonstrations;
- `Freeze Model` keeps the arm model stationary while telemetry continues updating elsewhere;
- `Follow Live Pose` resumes telemetry-driven movement;
- `Reset Camera`, `Lock Camera`, and `Unlock Camera` support classroom use;
- canvas E2E check that verifies the WebGL scene is visibly painted.

A later refinement can replace the procedural model with an imported `glb`/CAD-style model if a suitable ORA or xArm model is available and licensing permits school use.

## Classroom Admin

The Go server also enables a local classroom admin panel:

- Add student, selected-student operator, teacher, and admin profiles.
- Add classes and terms.
- Store profile/class records in SQLite.
- Keep a default `Station Admin` profile.

This is the first administration slice. Authentication, class membership assignment, submissions, and teacher approval workflows are next phases.

## SQLite Data Model

The first local database schema includes:

```text
users
classes
class_memberships
safety_profiles
projects
project_versions
submissions
approvals
robot_runs
audit_events
```

For the one-station pilot, SQLite keeps setup simple. PostgreSQL remains a later option if the project moves to a central server or multiple stations.

Back up the classroom database by copying:

```text
runtime/data/ora.db
runtime/data/ora.db-wal
runtime/data/ora.db-shm
```

The `Could not establish connection. Receiving end does not exist.` console messages in Chrome come from a Chrome extension context and are not produced by this app.
The `A listener indicated an asynchronous response...` / `runtime.lastError` console messages are also browser-extension noise. Use `scripts\open-controller.ps1` to open a clean profile with extensions disabled.

## E2E Tests

Install the Node dependencies once:

```powershell
npm install
npx playwright install chromium
```

Run the local controller E2E:

```powershell
npm run test:local
```

The default E2E target is the Go server. To force the Go target explicitly:

```powershell
$env:ORA_WEB_SERVER = "go"
npm run test:local
Remove-Item Env:\ORA_WEB_SERVER
```

The automated local controller suite now targets the Go/React app. The NGINX fallback can still be started for manual checks of the older static controller.

Run the official Ozobot Editor connection E2E without storing the password in files:

```powershell
$env:ORA_NAME = "ORA-FEA252"
$env:ORA_PASSWORD = "<password>"
npx playwright test tests/official-editor-connection.spec.js
```

The official-editor test verifies the same connection flow used by `https://editor.ozobot.com/en/blockly`: it opens the editor, enters the ORA credentials, and waits for the connected-state `Disconnect` button. It does not press `RUN` or send motion commands.
