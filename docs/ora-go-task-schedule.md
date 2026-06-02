# ORA Go Platform Task Schedule

Audience: Marblehead Public Schools, High School

Status: implementation schedule

Date: 2026-05-19

## Immediate Sprint

Dates: May 19, 2026 to May 22, 2026

Goal: establish the Go server as a working local app foundation while keeping the current ORA bridge intact.

Additional implementation pulled forward on May 19, 2026: the browser app is now being migrated to a React + TypeScript interface served by the Go server from `frontend/dist` when available. This keeps the one-station pilot simple while giving students a clearer tabbed layout for Dashboard, Manual Control, Projects, Classroom, and Settings.

Tasks:

- Add Go module and local server entrypoint.
- Serve the existing controller from Go.
- Proxy `/bridge/...` to the current Playwright bridge.
- Add `/api/health`, `/api/station`, and `/api/telemetry`.
- Add a local project storage API.
- Add `start-go-server.ps1` and `stop-go-server.ps1`.
- Add React + TypeScript frontend build.
- Serve React build from Go with the old static controller as fallback.
- Update E2E tests for the tabbed React interface.
- Update README with Go and NGINX run paths.
- Run local E2E tests against the Go server path.

Deliverables:

- Go local server source.
- Go server scripts.
- Go implementation phase document.
- Go task schedule document.
- React + TypeScript classroom interface.
- Updated operating instructions.

Exit criteria:

- Existing NGINX workflow still works.
- Go workflow is ready to run after Go is installed.
- The Go source keeps ORA credentials out of browser-visible files and API responses.
- React build, Go tests, and Playwright local E2E pass.

Verification evidence, May 19, 2026:

- `npm run typecheck`: passed.
- `npm run build:frontend`: passed, producing `frontend/dist/index.html` and bundled assets.
- `go test -count=1 ./...`: passed.
- `npm run test:local`: passed, 3 Playwright tests against the Go/React app.
- `GET http://127.0.0.1:8081/api/health`: returned `server:"ora-go-server"` and `storage:"sqlite"`.

## Week 1: Go Server Validation

Dates: May 25, 2026 to May 29, 2026

Tasks:

- Install Go on the ORA station.
- Run the Go server on `http://localhost:8081`.
- Confirm the React controller loads from Go.
- Confirm `/bridge/status` works through Go when the bridge is connected.
- Confirm `/api/telemetry` streams bridge status.
- Confirm project create/list/read/update/delete works locally.
- Add Go unit tests after the toolchain is installed.

Exit criteria:

- Teacher can start the Go server and bridge with documented scripts.
- Local browser control behavior matches the current NGINX setup.
- Go server logs and runtime files stay under `runtime/`.

## Week 2: Student Project Workflow

Dates: June 1, 2026 to June 5, 2026

Initial implementation status, May 19, 2026: local project create, save, reopen, duplicate, delete, export, and import are now available through the Go server and the browser UI. Storage now uses SQLite at `runtime/data/ora.db`, with starter tables for users, classes, memberships, safety profiles, versions, submissions, approvals, robot runs, and audit events. Blockly-specific project data remains a placeholder until the Blockly phase.

Tasks:

- Add frontend project save/open UI.
- Save Blockly placeholder data, Python text, and metadata through the Go API.
- Add project duplicate, rename, and export.
- Add project import validation.
- Add simple station-mode student name entry.
- Add SQLite-backed classroom data model.
- Keep JSON export/import for portability.

Exit criteria:

- A student can create, save, reopen, rename, and export a project with no internet.
- A teacher can inspect project files locally.

## Week 3: Live Visualization

Dates: June 8, 2026 to June 12, 2026

Pre-work completed May 19, 2026: classroom admin profile/class APIs and UI are now implemented. This was pulled forward because SQLite created the user/class schema and the next blocker was administration of student/operator profiles.

Implementation status, May 19, 2026: the React UI now subscribes to `/api/telemetry`, renders robot safety status, TCP pose, joint pose, stopped/error guidance, and a station event log. Playwright E2E now mocks ready, stopped, and self-collision telemetry states without moving the physical arm.

Tasks:

- Connect browser UI to `/api/telemetry`. Completed May 19, 2026.
- Render live pose and readiness state. Completed May 19, 2026.
- Show joint values and current tool position. Completed May 19, 2026.
- Add error and stopped-state guidance. Completed May 19, 2026.
- Keep manual controls gated behind `Enable Live Control`. Completed May 19, 2026.
- Add E2E coverage for ready, stopped, and ORA Error 22 self-collision states. Completed May 19, 2026.
- Add a clear/reset control for the station event log. Completed May 19, 2026.

Exit criteria:

- Students can see the arm state update in realtime while the arm moves.
- Errors are visible without requiring browser DevTools.

Verification evidence, May 19, 2026:

- `npm run typecheck`: passed.
- `npm run build:frontend`: passed, producing `frontend/dist/index.html` and bundled assets.
- `go test -count=1 ./...`: passed.
- `npm run test:local`: passed, 6 Playwright tests against the Go/React app, including the station event log reset check.
- `GET http://127.0.0.1:8081/api/health`: returned `server:"ora-go-server"` and `storage:"sqlite"`.

## Week 3.5: 3D Visualization

Dates: June 12, 2026 to June 15, 2026

Implementation status, May 19, 2026: complete for the cockpit Manual Control redesign and Three.js procedural arm model. Manual Control now keeps the teach pendant, station actions, telemetry, event log, and 3D model on one practical screen. The joystick/teach pendant is separate from the arm model, the control/model panels are resizable, and the model can pop out into a detached telemetry-driven window. The model is telemetry-driven and read-only; Go safety checks and bridge state remain the motion authority.

Tasks:

- Add Three.js to the React frontend. Completed May 19, 2026.
- Build a realistic procedural ORA/xArm-style arm model with links, joints, gripper, lighting, shadows, and workspace grid. Completed May 19, 2026.
- Separate joystick/control UI from the arm model. Completed May 19, 2026.
- Replace local Manual Control tabs with a one-screen cockpit layout. Completed May 19, 2026.
- Add a draggable divider between the controls and 3D model. Completed May 19, 2026.
- Add a detached model pop-out window. Completed May 19, 2026.
- Add a joystick intent view that mirrors manual input in the `Control` tab. Completed May 19, 2026.
- Add a joystick-driven ghost target and target line in the 3D model. Completed May 19, 2026.
- Drive arm pose from `/api/telemetry`. Completed May 19, 2026.
- Surface ready, stopped, and error states in the scene without hiding the controls. Completed May 19, 2026.
- Add `Freeze Model`, `Follow Live Pose`, `Reset Camera`, `Lock Camera`, and `Unlock Camera`. Completed May 19, 2026.
- Add Playwright canvas checks for nonblank rendering and layout. Completed May 19, 2026.

Exit criteria:

- Students can see a lifelike 3D arm representation move from telemetry.
- Students can keep practical controls visible without the 3D scene taking over the screen.
- Teachers can still see and use safety controls.
- Users can freeze the 3D model while telemetry continues updating in the Telemetry tab.
- The 3D view does not become the authority for movement; Go safety and bridge state remain the authority.

Verification evidence, May 19, 2026:

- `npm run typecheck`: passed.
- `npm run build:frontend`: passed, producing `frontend/dist/index.html` and bundled assets.
- `go test -count=1 ./...`: passed.
- `npm run test:local`: passed, 6 Playwright tests against the Go/React app.
- E2E verifies the 3D canvas is visible and painted by reading WebGL pixels.
- E2E verifies the practical one-screen cockpit layout, separate joystick intent view, resizable panels, detached model pop-out, freeze/follow model control, reset camera, and lock/unlock camera.

## Week 4: Blockly Prototype

Dates: June 15, 2026 to June 19, 2026

Tasks:

- Add local Blockly dependency.
- Build first ORA toolbox: movement, gripper, timing, loops.
- Generate a normalized command plan.
- Generate Python preview from the command plan.
- Save and load Blockly workspace through the Go project API.

Exit criteria:

- A student can build and save a simple block program.
- The generated Python preview matches the block sequence.

## Week 5: Validation and Dry Run

Dates: June 22, 2026 to June 26, 2026

Tasks:

- Add command plan validation endpoint.
- Define pilot safety limits.
- Add dry-run timeline.
- Block physical run until validation passes.
- Add teacher-readable validation messages.

Exit criteria:

- Unsafe motion is rejected server-side.
- Students can revise a program based on clear validation feedback.

## Week 6: Teacher Approval and Controlled Physical Runs

Dates: June 29, 2026 to July 3, 2026

Tasks:

- Add teacher approval queue.
- Add selected-student operator permission.
- Add one-active-run lock.
- Add run audit log.
- Add global emergency stop route and UI visibility.

Exit criteria:

- Normal students cannot run the arm directly.
- Approved selected students can run validated plans under teacher supervision.
- Emergency stop remains available independent of project state.

## Week 7+: Offline Transport Track

Dates: July 6, 2026 onward

Tasks:

- Continue investigating whether ORA physical execution can work without public internet.
- Request Ozobot guidance for local/offline ORA execution.
- Prototype native/local transport only behind the Go runtime API.
- Keep Playwright bridge as the working adapter until replacement is proven.

Exit criteria:

- Offline physical execution is either proven and documented or formally listed as blocked by vendor transport.
