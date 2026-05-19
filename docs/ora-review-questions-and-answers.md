# ORA Local Platform Review Questions and Answers

Status: reviewed. Approved direction captured on 2026-05-18.

## Approved Answers

1. Offline means editing, saving, and physical arm execution must work without public internet.

2. The first pilot supports one ORA station, accessed on the local network using the device name and password.

3. Physical arm use is allowed for both teachers and selected students, with safety controls and role-based permissions.

4. Blockly and editable Python are both in scope.

5. Save, export, and email submission are in scope. The first implementation still needs the school email provider details.

6. The project adopts the recommended local-first, safety-gated, privacy-conscious architecture.

## Short Answers

### Can we make an offline version of the Ozobot Editor?

We should not plan to clone the official Ozobot Editor directly. That could create licensing, maintenance, and support problems.

We can build our own local classroom editor using Blockly, local project storage, generated Python, validation, and teacher controls. For real ORA execution without internet, we need either a vendor-supported offline transport/runtime or explicit approval to use the command mechanism we discovered.

### Can students use it without the internet?

Yes for editing, saving, loading, simulation, generated code preview, documentation, and teacher review.

Physical execution on the ORA is the unresolved part. Official Ozobot requirements currently include internet access and specific outbound ports. If Ozobot provides an offline runtime or confirms a supported local command path, then physical execution can also work offline.

### Can we expose all ORA functionality?

Technically, yes, but educationally and safety-wise it should be layered.

Students should start with safe blocks and constrained movement. Advanced controls such as custom frames, tools, circular motion, GPIO, and raw command access should be teacher-gated or course-level gated.

### Should the system let every student move the arm directly?

No. Students should be able to write programs, but physical robot execution should go through a queue or teacher approval flow.

Recommended default:

- Students write and test in simulation/dry-run.
- Teacher approves physical runs.
- Only one program controls the arm at a time.
- Emergency stop is always available.

### Can it send projects by email?

Yes, but email requires either internet access or a school mail relay.

Offline-safe design:

- Save projects locally first.
- Queue email submissions when offline.
- Send later when the network is available.
- Also support export/download so email is not required.

### Should student work be stored in the cloud?

Not by default.

For a public school, local storage is simpler and safer. Cloud sync, external email, AI tools, camera features, or accounts should go through district privacy/security review.

### Is our current bridge production-ready?

No. It is a useful prototype and proof of connectivity, but it depends on the official online editor flow. It needs hardening, safety review, logging, credential handling, and vendor validation before classroom use.

### What is the best first pilot?

A teacher-controlled pilot at one ORA station:

- Local controller served from the teacher machine.
- Teacher starts/stops the bridge.
- Students create programs locally.
- Teacher controls physical execution.
- Save/export projects locally.
- No student access to ORA credentials.

## Proposed Product Decisions

### Decision 1: Offline meaning

Choose what "offline" must mean.

Option A: Students can edit/save/simulate offline, but physical execution may require internet.

Option B: Everything, including physical ORA execution, must work with no public internet.

Recommended: Start with Option A while we validate whether Option B is officially supportable.

### Decision 2: User model

Choose how students sign in.

Option A: No accounts; projects are saved by class station and project name.

Option B: Local class roster managed by teacher.

Option C: School SSO integration.

Recommended: Start with Option B for a pilot, then move to SSO if the district wants broader deployment.

### Decision 3: Execution permissions

Choose who can run physical robot programs.

Option A: Any student connected to the app.

Option B: Students submit to teacher; teacher runs.

Option C: Advanced students can run under strict limits.

Recommended: Option B for the pilot, with Option C only after safety procedures are proven.

### Decision 4: Programming modes

Choose the initial editor modes.

Option A: Blocks only.

Option B: Blocks plus generated Python preview.

Option C: Blocks plus editable Python.

Recommended: Option B first. Add editable Python after validation, because Python execution needs stronger sandboxing and safety checks.

### Decision 5: Email workflow

Choose submission method.

Option A: Save locally only.

Option B: Save locally plus export/download.

Option C: Save locally plus email queue.

Recommended: Option B first. Add Option C if the school confirms SMTP/email requirements.

### Decision 6: Vendor involvement

Choose whether to contact Ozobot before building offline execution.

Option A: Build only with what we can discover.

Option B: Ask Ozobot for a supported local/offline ORA control path.

Recommended: Option B. It reduces long-term risk for a school deployment.

## Questions For You

1. Is the goal true no-internet physical ORA execution, or is offline editing/saving enough if running the arm requires internet?

2. How many ORA arms will this need to support in the first pilot?

3. Will students use their own laptops/Chromebooks, or only a shared lab/teacher computer?

4. Does Marblehead Public Schools require SSO, or can the pilot use local class rosters?

5. Should student projects be stored only on the classroom machine, or backed up to a district location?

6. What email system should submissions use: Outlook/Exchange, Gmail, SMTP relay, or export-only?

7. Should students be allowed to edit Python directly, or should the first version be Blockly with Python preview?

8. Who is allowed to physically run the arm: teacher only, selected students, or any student after login?

9. Are AI/LLM and camera features allowed by district policy, or should we exclude them completely?

10. Does the classroom need a simulation mode before physical execution?

11. What safety limits should be non-negotiable: max speed, workspace boundaries, gripper restrictions, timeouts, teacher approval, or all of these?

12. Should the app support grading/feedback, or only project creation and export?

13. Do you want the UI branded for Marblehead High School/MPS, or kept generic?

14. Are we allowed to contact Ozobot support about offline/local ORA execution?

15. What is the target timeline: quick pilot, semester project, or production classroom deployment?

## My Recommended Answers

For a first school-safe pilot, I recommend:

- Offline editing, saving, generated Python preview, and local project export.
- Teacher-controlled physical execution only.
- One ORA station first.
- Blockly first, Python preview included, editable Python later.
- Local class roster or no-login station mode for the pilot.
- No AI/camera features in the first version.
- Local storage first, district backup later.
- Export/download first, email queue later.
- Vendor confirmation before claiming full offline physical execution.

## Remaining Details Needed Before Coding Each Phase

These points still need phase-level confirmation:

- Exact school email provider.
- Local roster versus station-mode names for the pilot.
- District HTTPS requirement for student LAN access.
- Safety profile owner/sign-off.
- Permission to contact Ozobot directly for offline/local ORA execution guidance.
- School-calendar dates for the pilot.
