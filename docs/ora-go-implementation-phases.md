# ORA Go Platform Implementation Phases

Audience: Marblehead Public Schools, High School

Status: approved direction, Go implementation track

Date: 2026-05-19

## Implementation Decision

Use Go for the local classroom server and robot runtime boundary. Keep the browser UI in JavaScript because Blockly, live canvas/SVG visualization, and browser interaction are web-native. Keep the current Playwright/Ozobot Editor bridge as the first ORA transport adapter until an offline/native ORA transport is proven.

## Target Architecture

```text
Student/teacher browser
  - manual controller
  - Blockly editor
  - Python preview/editor
  - live ORA visualization

Go local server
  - serves the app
  - stores projects locally
  - streams telemetry
  - validates command plans
  - owns safety and permissions
  - proxies approved ORA actions

ORA bridge adapter
  - current Playwright/WebRTC bridge first
  - native/offline adapter later if proven

ORA arm
```

## Phase G0: Go Server Foundation

Goal: replace NGINX for the local app path without breaking the current controller.

Tasks:

- Add a Go HTTP server.
- Serve `web/` and the existing `Ora-move.js`.
- Proxy `/bridge/...` to the current Playwright bridge.
- Preserve `/ora/...` proxy support for diagnostic direct-device checks.
- Add `/api/health`.
- Add `/api/station` without exposing the ORA password.
- Add repeatable PowerShell start, stop, restart, and status commands.
- Keep NGINX scripts as a fallback while Go is validated.

Acceptance criteria:

- `http://localhost:8080` loads the existing controller from Go.
- `/bridge/status` behaves the same as the NGINX proxy.
- `/api/health` returns server status.
- ORA credentials stay in `.env` or process environment only.

## Phase G1: Project Storage API

Goal: support offline student work before the Blockly UI is built.

Initial implementation status, May 19, 2026: complete for SQLite-backed create, list, read, update, delete, browser save/open, duplicate, export, and import. The first local classroom schema also includes users, classes, memberships, safety profiles, project versions, submissions, approvals, robot runs, and audit events. The next refinement is to connect this storage to real Blockly workspace data once Blockly is added.

Tasks:

- Add a local SQLite database under `runtime/data/ora.db`.
- Add create, list, read, update, and delete project endpoints.
- Store Blockly workspace JSON, generated Python, editable Python, title, owner, and timestamps.
- Add import/export-ready project document format.
- Add server-side validation of project IDs and file paths.
- Add initial user/profile/safety/audit tables for the classroom administration model.
- Auto-import legacy JSON projects from `runtime/data/projects` if present.

Acceptance criteria:

- A project can be saved and reopened with no internet.
- Project and classroom data are not mixed with source code.
- The API does not allow path traversal or access outside the project data directory.

## Phase G1.5: Classroom Admin Profiles

Goal: let the teacher define local student/operator/teacher/admin profiles and classes before adding submissions and approvals.

Initial implementation status, May 19, 2026: complete for local SQLite-backed user and class create, list, read, update, and delete APIs. The browser UI can add profiles and classes, and E2E verifies persistence after reload.

Tasks:

- Add `/api/users` and `/api/users/{id}`.
- Add `/api/classes` and `/api/classes/{id}`.
- Add local UI for profile creation.
- Add local UI for class creation.
- Keep the default `station-admin` profile.
- Audit profile and class create/update/delete events.

Acceptance criteria:

- A teacher can add a selected-student operator profile.
- A teacher can add a class/term.
- Profiles and classes persist after app reload.
- Test data can be deleted cleanly through the API.

## Phase G2: Realtime Telemetry Stream

Goal: let students see arm state and motion as it changes.

Initial implementation status, May 19, 2026: complete for Server-Sent Events consumption in the React UI, robot safety status, TCP pose, joint pose, stopped/error guidance, and station event logging. E2E verifies ready, stopped, and ORA Error 22 self-collision telemetry states using mocked bridge payloads so tests do not move the physical arm.

Tasks:

- Poll the ORA bridge status endpoint server-side.
- Stream status to browsers with Server-Sent Events first.
- Include pose, joint values, state, readiness, and error code when available.
- Keep stream payloads safe and free of credentials.
- Add reconnect behavior in the browser.
- Render readable safety guidance and event history in the browser.

Acceptance criteria:

- A browser can subscribe to `/api/telemetry`.
- The stream continues across temporary bridge errors.
- Movement state can be rendered without giving students raw transport access.

## Phase G2.5: 3D Joystick and Arm Visualization

Goal: give students a near-real-time 3D view of joystick intent, ORA arm pose, and joint movement before they start composing Blockly programs.

Initial implementation status, May 19, 2026: complete for the cockpit redesign and Three.js procedural arm model. Manual Control now keeps the teach pendant, station actions, telemetry, event log, and 3D model on one practical screen. The joystick/teach pendant is separate from the arm model, the control/model columns are resizable, and the 3D model can pop out into a detached window. The scene includes an ORA/xArm-style base, links, joints, wrist, gripper, workspace grid, lighting, shadows, reach/safety rings, telemetry-driven TCP marker, telemetry-driven joint rotations, joystick-driven ghost target, target line, warning ring, freeze/follow controls, reset camera, and lock/unlock camera. E2E verifies the WebGL canvas renders nonblank by sampling pixels.

Implemented approach:

- Use Three.js in the React frontend.
- Build a realistic procedural arm model first, using joint cylinders, links, gripper geometry, workspace grid, lighting, shadows, and camera controls.
- Drive the model from `/api/telemetry` TCP and joint pose values.
- Keep joystick controls separate from the arm model and show joystick intent beside the teach pendant.
- Show a ghost target and intent line in the model so students can compare actual pose and commanded direction before movement.
- Show blocked/error states directly in the 3D scene with restrained overlays.
- Put the 3D scene beside the controls in the same Manual Control cockpit so students do not need to switch tabs during supervised motion.
- Add a draggable divider for classroom screens where the teacher wants more control area or more model area.
- Add a detachable model window for demonstrations or dual-display use while preserving the same telemetry stream.
- Allow teachers/students to freeze the 3D model while telemetry continues updating elsewhere.
- Add camera reset and camera lock/unlock controls for classroom use.
- Later, replace the procedural model with a `glb`/CAD-style model if a suitable ORA or xArm model is available and licensing permits school use.

Acceptance criteria:

- The scene renders nonblank on desktop and classroom laptop viewports.
- Pose and joint values visibly update from telemetry.
- Joystick intent is visible before a command is sent.
- Users can freeze and resume live model movement.
- Users can reset and lock/unlock the 3D camera.
- The visualization remains read-only; Go server safety checks still control physical motion.
- Playwright verifies that the canvas renders, updates, and does not overlap controls.

## Phase G3: Blockly and Command Plan

Goal: add a Blockly-style editor that produces a safe, inspectable command plan.

Tasks:

- Add local Blockly assets.
- Define ORA movement, end-tool, timing, logic, loops, variables, and sensing blocks.
- Generate a normalized command plan from blocks.
- Generate readable Python preview from the same command plan.
- Save block workspace and command plan in the project file.

Acceptance criteria:

- A student can assemble a simple movement program offline.
- The generated Python is readable enough for instruction.
- The physical robot runtime receives command plans, not arbitrary browser commands.

## Phase G4: Safety Validator and Dry Run

Goal: block unsafe programs before the ORA moves.

Tasks:

- Validate workspace limits, speed, acceleration, Z height, gripper actions, run duration, and command count.
- Add a dry-run endpoint.
- Add a command timeline.
- Add teacher-readable validation results.
- Require successful validation before physical execution.

Acceptance criteria:

- Unsafe command plans are rejected server-side.
- Students can see what failed and revise their program.
- Teachers can inspect the full planned motion before approving a run.

## Phase G5: Roles, Approval, and Run Queue

Goal: make physical execution appropriate for a public high school classroom.

Tasks:

- Add local teacher/admin session.
- Add student author and selected-student operator modes.
- Add teacher approval for physical runs.
- Add one-active-run lock.
- Add emergency stop that bypasses normal queueing.
- Add audit logs for approvals and physical runs.

Acceptance criteria:

- Normal students can create and submit programs but cannot directly move the arm.
- Selected students can run only approved, validated programs.
- Teacher emergency stop is available during every run.

## Phase G6: Offline ORA Transport Replacement

Goal: remove dependence on the public Ozobot Editor session if a reliable local/offline transport path is available.

Tasks:

- Document the current WebRTC control/admin channel behavior.
- Ask Ozobot for supported local/offline transport guidance.
- Build a native or vendor-supported adapter only after the protocol is understood.
- Keep the Go runtime API stable so the frontend does not change.

Acceptance criteria:

- ORA physical execution works without public internet.
- The transport is supportable by the school or vendor.
- The Go API remains the safety boundary regardless of transport implementation.
