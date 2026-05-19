# ORA Local-First Learning Platform Proposal

Audience: Marblehead Public Schools, High School

Status: approved direction as of 2026-05-18. Implementation still requires phase-by-phase execution and safety gates.

## Summary Recommendation

Build a local-first classroom robotics platform instead of trying to copy the Ozobot Editor website.

The right model is a school-hosted web app that runs on a teacher workstation, lab computer, or small local server. Students open the app from a browser on the school network, create ORA programs, save their work locally, preview generated code, simulate where possible, and submit or export projects. Actual arm execution should go through a controlled robot service with teacher safety controls, not directly from each student browser.

This gives the school:

- A classroom-safe robotics environment.
- A path to work when the public internet is unavailable.
- Local storage for student work.
- Teacher review before physical robot movement.
- A curriculum-friendly bridge from blocks to Python and robot control.

The hard part is not the web page. The hard part is the robot transport. Ozobot's supported ORA flow currently expects internet access and specific outbound ports. For a true no-internet execution path, we should confirm with Ozobot whether they provide or allow a supported offline ORA transport/runtime.

## Current Facts

From official Ozobot material and our local tests:

- ORA is a 6-axis educational cobot using Ozobot Blockly as its software environment.
- Ozobot lists internet access as an ORA operating requirement.
- Ozobot lists outbound network requirements including TCP and UDP ports used by the editor/robot connection path.
- The supported browser requirement is Chrome, Edge, or another Chromium-based browser.
- The current Ozobot Editor includes Blockly and a Python runtime/API flow.
- Our current local controller can talk to ORA only by using a bridge that connects through the official online editor mechanism.

References:

- https://ozobot.com/ora/
- https://editor.ozobot.com/en/changelog
- https://editor.ozobot.com/doc-python-api/ora/index.html
- https://editor.ozobot.com/doc-python-api/ora/simple.html

## Product Goal

Students should be able to:

- Open a local classroom ORA editor without needing the public internet.
- Create programs using blocks, Python, or both.
- Change motion parameters such as speed, acceleration, target pose, joints, gripper, frame, and tool settings.
- Save and reopen projects.
- Export project files for grading or portfolios.
- Submit work to the teacher.
- Run programs on the real ORA only when safety rules and permissions allow it.
- Learn the software development cycle: design, code, simulate, test, debug, document, submit, improve.

Teachers should be able to:

- Manage class projects.
- Approve or deny physical robot execution.
- Set safety limits.
- Reset or stop the robot.
- See logs of what ran, when, and by whom.
- Export or email student work.
- Use the platform even when the school internet is down, at least for editing, saving, simulation, and local review.

## Proposed Architecture

### 1. Classroom Server

A local classroom server runs the application. This could be:

- A teacher Windows workstation.
- A dedicated lab PC.
- A small server on the robotics VLAN.
- A managed mini PC attached to the ORA station.

Responsibilities:

- Serve the student web app.
- Store projects locally.
- Manage ORA credentials securely.
- Run the robot bridge/runtime.
- Enforce safety limits.
- Keep logs.
- Queue email/export jobs when the internet is unavailable.

Students should not need the ORA password. The server owns the credential and exposes only safe, role-based actions.

### 2. Student Web App

The student app should run in Chrome or Edge and work over the school LAN.

Main screens:

- Project dashboard.
- Blockly editor.
- Python preview/editor.
- Robot state panel.
- Simulation or dry-run panel.
- Submit/export panel.
- Teacher approval state.

The first screen should be the actual editor, not a landing page.

### 3. Offline Authoring

Use local copies of open-source editor components where allowed:

- Blockly for visual programming.
- A local generator that turns blocks into Python or an internal command plan.
- Local docs and examples.
- Local validation rules.
- Local project files.

Projects should be saved in a plain, portable format such as:

- Workspace XML/JSON for blocks.
- Generated Python.
- Project metadata.
- Safety profile used during validation.
- Optional comments/reflection fields for students.

The project format should be versioned so old student projects can still open after future updates.

### 4. Robot Runtime Layer

This is the critical design decision.

Option A: Vendor-supported offline transport

- Best long-term path.
- Ask Ozobot whether an offline ORA runtime, local API, or supported WebRTC/command client is available.
- This reduces legal, maintenance, and classroom reliability risk.

Option B: Local bridge using the same command channel we discovered

- Feasible technically.
- Higher maintenance risk because it depends on current editor/device behavior.
- Should not be considered production-ready for a public school until reviewed for vendor permission, reliability, and safety.

Option C: Offline authoring plus online execution

- Students can edit, save, simulate, and submit offline.
- The real arm runs only when internet is available.
- This is the safest near-term fallback if true offline ORA execution is not supported.

Recommendation: pursue Option A first, keep Option C as the reliable fallback, and treat Option B as a prototype until approved.

### 5. Safety Layer

Physical robot movement should be gated by a safety system independent of the editor UI.

Recommended controls:

- Teacher-only arm enable.
- Student programs default to simulation/dry-run.
- Physical run requires teacher approval.
- Global emergency stop always visible.
- Hardware emergency stop remains required.
- Speed and acceleration caps by course level.
- Workspace boundaries based on the classroom setup.
- Gripper force/tool restrictions where applicable.
- Collision/error status display.
- Run timeout.
- One active operator at a time.
- Audit log for every command/program run.

Suggested permission levels:

- Level 1: view, blocks, simulation only.
- Level 2: approved blocks, slow physical execution.
- Level 3: Python editing, teacher-reviewed execution.
- Level 4: advanced frames/tools/GPIO, teacher/admin only.

### 6. Save, Submit, and Email

Local save should not depend on email.

Recommended workflow:

- Students save locally to the classroom server.
- Students submit a project to the teacher queue.
- Teacher reviews and can export/download.
- If internet or school SMTP is available, the server sends email.
- If offline, the server queues the email and sends later.

Email should be a convenience, not the source of truth.

For public school use, avoid collecting unnecessary student data. The minimum useful data is probably:

- Student display name or local class ID.
- Project title.
- Timestamp.
- Course/section.
- Project file.
- Teacher feedback.

### 7. Curriculum Features

The app should teach robotics software development, not just provide buttons.

Recommended classroom features:

- Block-to-Python preview.
- Read-only generated command timeline.
- Unit-aware inputs for millimeters, degrees, speed, acceleration.
- Warnings for unsafe values.
- Simulated workspace grid.
- Program run log.
- Reflection prompt after a run: expected behavior, observed behavior, next change.
- Starter templates by lesson.
- Teacher-created constraints per assignment.

### 8. Functional Scope

Expose ORA functionality in layers:

Basic:

- Home/reset.
- Read current pose.
- Move to named location.
- Open/close gripper.
- Emergency stop.
- Slow linear moves.

Intermediate:

- Cartesian pose moves.
- Joint moves.
- Relative moves.
- Speed and acceleration.
- Variables and loops.
- Simple input/output.
- Saved points.

Advanced:

- Frames.
- Tool configuration.
- Circular moves.
- Concurrent tasks.
- Programmatic state checks.
- Error handling.
- GPIO, only if physically safe and curriculum-approved.

Teacher/admin:

- Calibration workflow link or instructions.
- Clear warnings/errors where supported.
- Safety profile editing.
- Device credential management.
- Logs and exports.

### 9. Security and Privacy

Recommended rules:

- Do not store ORA passwords in client-side JavaScript.
- Do not put robot credentials in student-visible files.
- Keep the robot service on the school LAN.
- Use HTTPS if students connect from other machines.
- Use school identity if available, or a local class roster if not.
- Keep audit logs local unless the district approves cloud storage.
- Review FERPA/COPPA implications before adding cloud accounts, AI tools, webcam features, or external sharing.
- Disable or omit AI/camera features unless explicitly approved by the district.

### 10. Deployment Options

Small pilot:

- One teacher workstation.
- One ORA.
- Local-only access from the teacher machine.
- Students pair at the station.

Classroom LAN:

- One local server.
- Multiple student laptops.
- One ORA execution queue.
- Teacher approval for physical runs.

CTE lab:

- Multiple ORA stations.
- Device reservations.
- Assignment tracking.
- Per-station safety profiles.
- Central backups.

## Recommended Roadmap

Phase 0: Approval and vendor validation

- Confirm district goals.
- Confirm Ozobot licensing/support for offline execution.
- Confirm school network constraints.
- Confirm safety requirements.

Phase 1: Harden the current local controller

- Teacher-only live control.
- Stronger status/error handling.
- Better emergency stop behavior.
- Logs.
- No student credential exposure.

Phase 2: Local project system

- Save/load projects.
- Export/import project files.
- Student project dashboard.
- Teacher review queue.

Phase 3: Offline authoring

- Local Blockly editor.
- Generated Python preview.
- Command validation.
- Lesson templates.
- Simulation/dry-run.

Phase 4: Controlled physical execution

- Robot runtime adapter.
- Safety profiles.
- Teacher approval.
- Program queue.
- Run logs.

Phase 5: Classroom operations

- Email/export workflow.
- Local backups.
- Multi-user roles.
- Multi-robot support if needed.

## Main Risks

Vendor support risk:

True offline execution may not be supported without Ozobot cooperation.

Safety risk:

Giving students full direct control of a physical arm without teacher gates is not appropriate for a school setting.

Maintenance risk:

Depending on private editor internals can break when Ozobot updates their editor.

Privacy risk:

Adding accounts, email, AI, camera, or cloud sync introduces district review requirements.

Network risk:

School firewall/VLAN rules may block the exact traffic ORA needs.

## Approved Direction

The approved direction is a local-first platform split into two tracks:

1. Education platform: local editor, projects, lessons, save/export, review, simulation.
2. Robot execution: vendor-confirmed transport, safety controls, teacher-approved physical runs.

The required target is offline editing, offline saving, and offline physical ORA execution with no public internet. The first pilot is one ORA station accessed over the local network using the ORA device name and password stored server-side only. Both teachers and selected students may physically run the arm, with safety gates. Blockly and editable Python are both in scope. Save, export, and email workflows are in scope.
