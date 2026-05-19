# ORA Local-First Platform Detailed Implementation Phases

Audience: Marblehead Public Schools, High School

Status: approved direction, implementation plan draft

Date: 2026-05-18

## Approved Direction

The platform should support:

- Offline editing and saving.
- Offline physical ORA execution without public internet.
- One ORA station for the first pilot.
- Network access using the ORA device name and password, stored server-side only.
- Teacher and selected-student physical control.
- Blockly and editable Python.
- Save, export, and email submission.
- Adoption of the safety, privacy, local-first, and vendor-validation recommendations from the proposal.

## Core Principle

Separate education features from physical robot execution.

The editor, projects, Python, lessons, save/export, and teacher review can be built as a normal local web application. Physical ORA execution must be isolated behind a server-side robot runtime with safety limits, logs, and permissions.

No student browser should directly hold ORA credentials or send raw robot commands.

## Phase 0: Governance, Safety, and Offline Transport Proof

Goal: prove that no-internet physical execution is supportable before we promise classroom operation.

Work items:

- Confirm district approval for a local robotics web application.
- Confirm whether the ORA station is isolated, supervised, or accessible from student devices.
- Define teacher, selected-student, and admin roles.
- Define the physical safety envelope for the arm.
- Confirm emergency stop expectations, including the hardware emergency stop.
- Contact Ozobot for a supported local/offline ORA execution path.
- Document whether offline execution is vendor-supported, vendor-approved, or prototype-only.
- Build a small offline transport proof in a controlled setting before any UI depends on it.

Deliverables:

- Safety requirements document.
- Vendor/offline transport decision note.
- Physical test checklist.
- Go/no-go decision for offline execution.

Acceptance criteria:

- The ORA can execute at least one safe, read-only query and one controlled no-risk state command without public internet.
- The path does not expose credentials to student browsers.
- Teacher emergency stop remains available.
- The project team understands whether the transport is officially supported or a maintained local adapter.

Primary risk:

If no supported offline transport exists, physical offline execution becomes a custom adapter project and carries maintenance risk.

## Phase 1: Local Server Foundation

Goal: create the local application base that can run from one station and serve student browsers on the LAN.

Work items:

- Choose the runtime stack for the local server.
- Replace ad hoc process startup with a repeatable start/stop flow.
- Serve the app locally over HTTP for development and HTTPS for student LAN access if required.
- Add basic health checks.
- Add local configuration for one ORA station.
- Store ORA name/password outside client-side files.
- Add local log directories and retention rules.

Deliverables:

- Local server skeleton.
- Station configuration file or admin setup screen.
- Health endpoint.
- Startup/shutdown instructions.
- Local logging plan.

Acceptance criteria:

- A teacher can start the server on the classroom machine.
- Students can reach the app from approved devices on the local network.
- No ORA credential appears in browser JavaScript, project files, screenshots, or logs.

## Phase 2: Identity, Roles, and Permissions

Goal: support teacher and selected-student control without requiring a full district SSO integration for the first pilot.

Work items:

- Add local class roster support.
- Add teacher/admin login.
- Add selected-student permission for physical execution.
- Add role-based UI and API enforcement.
- Add session timeout.
- Add audit logging for login, project edits, submissions, approvals, and robot runs.

Recommended pilot roles:

- Admin: station setup, credentials, safety profiles, logs.
- Teacher: class roster, project review, run approval, emergency stop.
- Selected student operator: can request and run approved physical programs under limits.
- Student author: can create, edit, save, submit, simulate.

Deliverables:

- Local roster format.
- Role and permission matrix.
- Login/session flow.
- Audit log schema.

Acceptance criteria:

- A student cannot enable physical robot control unless assigned the selected-student role or explicitly approved by a teacher.
- Teacher actions and robot runs are logged.
- Lost or shared student sessions can be ended by the teacher.

## Phase 3: Project Storage and Submission

Goal: make student work durable, portable, and reviewable without internet.

Work items:

- Define the project file format.
- Save Blockly workspace data.
- Save editable Python code.
- Save generated Python from blocks.
- Save metadata: title, student, class, timestamp, version, safety profile.
- Add open, duplicate, rename, archive, and delete flows.
- Add project export/import.
- Add teacher submission queue.
- Add local backup routine.

Recommended project structure:

```json
{
  "formatVersion": 1,
  "title": "Pick and Place Lesson 1",
  "owner": "student-id",
  "course": "robotics",
  "updatedAt": "2026-05-18T00:00:00Z",
  "mode": "blocks-and-python",
  "blockly": {},
  "python": "",
  "generatedPython": "",
  "safetyProfileId": "pilot-default",
  "runHistory": []
}
```

Deliverables:

- Project database or local file store.
- Project dashboard.
- Import/export format.
- Submission queue.
- Backup instructions.

Acceptance criteria:

- A student can create, save, close, reopen, and export a project while offline.
- A teacher can see submitted projects locally.
- A project can be moved to another machine and reopened.

## Phase 4: Blockly and Python Editor

Goal: provide a usable robotics programming environment, not just manual controls.

Work items:

- Add local Blockly assets.
- Define ORA block categories.
- Generate readable Python from blocks.
- Add editable Python mode.
- Add syntax highlighting.
- Add block/Python synchronization policy.
- Add starter templates.
- Add local documentation snippets.
- Add validation messages close to the relevant block or code line.

Block categories:

- Movement.
- End tool.
- Timing.
- Logic and loops.
- Variables.
- Sensing/status.
- Safety.
- Teacher-approved advanced controls.

Python mode requirements:

- Python should run through a restricted execution path.
- Direct raw command access should be blocked or teacher-gated.
- Generated command plans should be validated before execution.
- Student code should have timeouts and resource limits.

Deliverables:

- Blockly editor.
- Python editor.
- Generated Python preview.
- Lesson starter templates.
- Validation messages.

Acceptance criteria:

- Students can build a block program and see the generated Python.
- Students with permission can edit Python directly.
- Invalid or unsafe values are flagged before execution.

## Phase 5: Simulation and Dry Run

Goal: make students test intent before moving the real robot.

Work items:

- Convert blocks/Python into a normalized command plan.
- Show a command timeline.
- Display estimated target positions and gripper actions.
- Validate speed, acceleration, workspace, and timeout rules.
- Add a simulated workspace view.
- Add dry-run mode that checks the full plan without sending physical motion.

Deliverables:

- Command plan model.
- Validator.
- Dry-run result panel.
- Simple simulation workspace.

Acceptance criteria:

- Every physical run has a successful validation result first.
- Students can explain what the robot will do before it moves.
- Teacher can inspect the planned command sequence.

## Phase 6: Offline ORA Runtime Adapter

Goal: execute validated programs on the real ORA without public internet.

Work items:

- Implement or integrate the approved offline ORA transport.
- Connect to one configured ORA station by device name/password.
- Track ORA state, pose, errors, warnings, readiness, and connection status.
- Expose only approved server-side robot actions to the web app.
- Add a command queue.
- Add cancellation and emergency stop.
- Add reconnect behavior.
- Add run logs.

Runtime API should support:

- Status.
- Home.
- Stop.
- Pause/resume if supported.
- Move line.
- Move joint.
- Gripper open/close.
- Read pose.
- Clear safe warnings/errors only where appropriate.
- Teacher-approved advanced commands.

Deliverables:

- ORA runtime service.
- Local-only robot API.
- Command queue.
- Run logger.
- Offline connectivity test.

Acceptance criteria:

- The ORA can be controlled from the local server without public internet.
- The runtime rejects commands outside the active safety profile.
- Only one active physical run can control the arm at a time.
- Emergency stop works even if a student session disconnects.

## Phase 7: Safety Profiles and Teacher Run Approval

Goal: make physical execution appropriate for a public high school classroom.

Work items:

- Define default pilot safety profile.
- Add teacher-editable safety profiles.
- Add teacher approval queue for physical runs.
- Add selected-student execution permissions.
- Add speed and acceleration caps.
- Add workspace boundaries.
- Add timeout limits.
- Add tool/gripper restrictions.
- Add run pre-check checklist.
- Add post-run log and student reflection prompt.

Recommended default pilot limits:

- Low maximum speed.
- Low acceleration.
- Small workspace envelope.
- No advanced tool/GPIO access.
- Teacher approval required.
- One active run at a time.
- Short run timeout.
- Emergency stop visible on all active sessions.

Deliverables:

- Safety profile model.
- Teacher approval workflow.
- Selected-student run workflow.
- Run log and reflection capture.

Acceptance criteria:

- A normal student can submit but not directly run physical motion.
- A teacher can approve a run.
- A selected student can run only within assigned permissions.
- Any unsafe plan is rejected before it reaches the ORA runtime.

## Phase 8: Email, Export, and Classroom Operations

Goal: support normal classroom hand-in and recordkeeping.

Work items:

- Add project export as a file.
- Add teacher export of submitted work.
- Add email queue.
- Add configurable email provider.
- Add offline email queue behavior.
- Add send retry and failure logs.
- Add local backup task.
- Add restore instructions.

Email provider options:

- District SMTP relay.
- Outlook/Exchange.
- Gmail/Google Workspace.
- Export-only fallback.

Deliverables:

- Export workflow.
- Email queue.
- Provider configuration guide.
- Backup and restore guide.

Acceptance criteria:

- Student work can be submitted without internet.
- If email is offline, messages are queued rather than lost.
- Teacher can export projects even if email is not configured.

## Phase 9: Pilot, Training, and Hardening

Goal: validate the platform with one ORA station before broader classroom use.

Work items:

- Run teacher-only tests.
- Run selected-student supervised tests.
- Collect classroom workflow feedback.
- Fix confusing UI and safety friction.
- Add teacher quick-start guide.
- Add student quick-start guide.
- Add troubleshooting guide.
- Add recovery procedures.
- Add E2E tests for the critical workflows.

Deliverables:

- Pilot checklist.
- Teacher guide.
- Student guide.
- Troubleshooting guide.
- Test suite.

Acceptance criteria:

- Teacher can run a complete lesson workflow.
- Student can create, save, submit, and revise a program.
- Selected student can run an approved physical program under supervision.
- The system recovers from app restart, bridge/runtime restart, and ORA disconnect.

## Go/No-Go Gates

Gate 1: Offline transport

- Required before claiming no-internet physical execution.
- Blocks Phase 6 classroom use.

Gate 2: Safety review

- Required before selected-student physical control.
- Blocks Phase 7 classroom use.

Gate 3: Privacy review

- Required before class rosters, email, or any student data is used beyond a local pilot.
- Blocks Phase 8 broader use.

Gate 4: Pilot readiness

- Required before students use the real ORA.
- Blocks Phase 9 classroom pilot.

## Open Items

- Which email provider should be configured first?
- Should the first pilot use local roster login or station-mode student names?
- Should district IT provide HTTPS certificates for student LAN access?
- What workspace boundaries match the physical ORA station?
- Who from MPS signs off on the safety profile?
- Are we allowed to contact Ozobot directly for offline/local ORA execution guidance?
