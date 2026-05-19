# ORA Local-First Platform Task Schedule

Audience: Marblehead Public Schools, High School

Status: draft schedule for review

Start date assumption: Monday, May 18, 2026

Planning assumption: one ORA station, one local classroom server, teacher plus selected-student physical control, Blockly plus editable Python, offline editing/saving, and offline physical execution.

## Schedule Summary

This is a 12-week pilot schedule. The critical path is offline ORA execution. If a supported offline transport is not available, the education platform can still proceed, but physical no-internet execution becomes a separate risk item.

Target milestones:

- Week 1: requirements, safety, and vendor/offline transport request.
- Week 2: offline transport proof decision.
- Weeks 3-4: local server, roles, and project storage.
- Weeks 5-6: Blockly/Python editor.
- Week 7: validation and dry-run.
- Weeks 8-9: offline runtime adapter and safety-gated execution.
- Week 10: export/email and operations.
- Week 11: pilot hardening and tests.
- Week 12: teacher-supervised classroom pilot.

## Week 1: Requirements and Safety Baseline

Dates: May 18, 2026 to May 22, 2026

Tasks:

- Record approved product decisions.
- Define pilot users: teacher, selected students, student authors, admin.
- Draft role and permission matrix.
- Draft physical safety requirements.
- Identify emergency stop expectations.
- Identify ORA station location and network layout.
- Write Ozobot vendor questions for offline/local ORA control.
- Identify district privacy/security reviewer if student names or email are used.

Deliverables:

- Approved assumptions document.
- Safety requirements draft.
- Vendor question list.
- Initial risk register.

Exit criteria:

- Offline physical execution is documented as a required outcome.
- The team agrees that no student browser will hold ORA credentials.
- Safety review owner is identified.

## Week 2: Offline Transport Proof and Architecture Decision

Dates: May 25, 2026 to May 29, 2026

Tasks:

- Test ORA connectivity with no public internet.
- Determine whether the current discovered command path can work offline.
- Capture what ports/protocols are required locally.
- Review Ozobot response if available.
- Choose runtime path: vendor-supported adapter or school-maintained local adapter.
- Define runtime API boundary.
- Define go/no-go rule for classroom physical execution.

Deliverables:

- Offline transport proof report.
- Runtime architecture decision.
- Runtime API outline.
- Updated risk register.

Exit criteria:

- A controlled offline status query succeeds, or the blocker is clearly documented.
- A safe offline state command succeeds, or the blocker is clearly documented.
- Implementation does not proceed to student physical execution without a viable runtime path.

## Week 3: Local Server and Station Configuration

Dates: June 1, 2026 to June 5, 2026

Tasks:

- Define local server stack.
- Create repeatable start/stop procedure.
- Add one-station ORA configuration.
- Store ORA credentials server-side only.
- Add server health checks.
- Add local logging directories.
- Add configuration backup guidance.
- Plan HTTPS for student LAN access if required.

Deliverables:

- Local server foundation.
- Station configuration model.
- Health check endpoint.
- Startup guide draft.

Exit criteria:

- Teacher can start the server locally.
- Approved student devices can open the app on the LAN.
- ORA credentials are not visible in client files.

## Week 4: Users, Roles, Projects, and Submissions

Dates: June 8, 2026 to June 12, 2026

Tasks:

- Add local roster or station-mode identity.
- Add teacher/admin session.
- Add selected-student permission.
- Add project create/open/save/rename/delete.
- Define project file schema.
- Add project export/import.
- Add teacher submission queue.
- Add audit log for project and permission events.

Deliverables:

- Role model.
- Project storage.
- Submission queue.
- Export/import flow.

Exit criteria:

- Student can save and reopen a project offline.
- Teacher can view submitted projects.
- Selected-student role is enforceable.

## Week 5: Blockly Editor

Dates: June 15, 2026 to June 19, 2026

Tasks:

- Add local Blockly assets.
- Build ORA movement block category.
- Build end-tool block category.
- Build timing, loops, variables, and logic categories.
- Add speed and acceleration inputs.
- Add pose and named-location inputs.
- Add block validation messages.
- Add starter templates for the first lesson.

Deliverables:

- Local Blockly editor.
- ORA block toolbox.
- Starter templates.

Exit criteria:

- Student can build a simple ORA program offline.
- Invalid block values are visible before running.
- The project saves and restores the Blockly workspace.

## Week 6: Python Editor and Block-to-Python Preview

Dates: June 22, 2026 to June 26, 2026

Tasks:

- Add generated Python preview.
- Add editable Python mode.
- Add syntax highlighting.
- Define synchronization between blocks and Python.
- Add Python validation and lint-style messages.
- Define restricted Python execution model.
- Add teacher-gated Python run permissions.

Deliverables:

- Python editor.
- Generated Python preview.
- Python validation plan.

Exit criteria:

- Student can view generated Python from blocks.
- Approved users can edit Python.
- Python cannot bypass server-side safety validation.

## Week 7: Command Plan, Validation, and Dry Run

Dates: June 29, 2026 to July 3, 2026

Tasks:

- Convert Blockly and Python into a normalized command plan.
- Add command timeline.
- Add workspace boundary validation.
- Add speed and acceleration validation.
- Add timeout validation.
- Add simple simulated workspace.
- Add dry-run result panel.
- Add teacher inspection view.

Deliverables:

- Command plan model.
- Validator.
- Dry-run workflow.
- Simulation panel.

Exit criteria:

- Every physical run must pass validation first.
- Teacher can inspect the planned physical commands.
- Unsafe commands are rejected before runtime execution.

## Week 8: Offline Runtime Adapter

Dates: July 6, 2026 to July 10, 2026

Tasks:

- Implement approved offline ORA transport path.
- Add runtime connection lifecycle.
- Add status polling/subscription.
- Add pose and warning/error reporting.
- Add home, stop, move, gripper, and read-pose actions.
- Add runtime reconnect handling.
- Add command response timeout handling.
- Add runtime logs.

Deliverables:

- Offline ORA runtime service.
- Local robot API.
- Runtime logs.
- Offline connection test.

Exit criteria:

- Runtime connects to ORA with no public internet.
- Runtime can read status.
- Runtime can execute one approved, low-risk physical command under teacher supervision.

## Week 9: Safety-Gated Physical Execution

Dates: July 13, 2026 to July 17, 2026

Tasks:

- Add default pilot safety profile.
- Add teacher approval queue for physical runs.
- Add selected-student run workflow.
- Add one-active-run lock.
- Add global emergency stop.
- Add cancel/stop behavior for disconnected sessions.
- Add physical run audit log.
- Add post-run student reflection.

Deliverables:

- Safety profile enforcement.
- Teacher approval workflow.
- Selected-student execution workflow.
- Physical run audit log.

Exit criteria:

- Normal students cannot directly run the arm.
- Teacher can approve or reject a physical run.
- Selected students can run only approved, validated plans.
- Emergency stop is available during every run.

## Week 10: Export, Email, Backup, and Operations

Dates: July 20, 2026 to July 24, 2026

Tasks:

- Add project export package.
- Add teacher bulk export.
- Add email queue.
- Add configurable email provider.
- Add offline queue and retry behavior.
- Add failure logs.
- Add local backup routine.
- Add restore procedure.

Deliverables:

- Export workflow.
- Email queue.
- Backup/restore guide.
- Operations checklist.

Exit criteria:

- Student work can be exported without internet.
- Email submissions queue if network/email is unavailable.
- Teacher can recover projects from backup.

## Week 11: E2E Tests, Documentation, and Hardening

Dates: July 27, 2026 to July 31, 2026

Tasks:

- Add E2E tests for project save/load.
- Add E2E tests for Blockly and Python flows.
- Add E2E tests for teacher approval.
- Add E2E tests for runtime status and emergency stop.
- Add failure-mode tests for ORA disconnect.
- Write teacher quick-start guide.
- Write student quick-start guide.
- Write troubleshooting guide.
- Fix pilot-blocking UI and reliability issues.

Deliverables:

- E2E test suite.
- Teacher guide.
- Student guide.
- Troubleshooting guide.
- Pilot release candidate.

Exit criteria:

- Critical workflows pass locally.
- Teacher can follow the guide without developer help.
- Known pilot risks are documented.

## Week 12: Supervised Pilot

Dates: August 3, 2026 to August 7, 2026

Tasks:

- Run teacher-only pilot workflow.
- Run selected-student supervised workflow.
- Observe project creation, submission, approval, and physical run.
- Collect teacher feedback.
- Collect student feedback.
- Review logs.
- Fix high-severity issues.
- Decide next deployment phase.

Deliverables:

- Pilot report.
- Issue list.
- Next-phase recommendation.
- Updated schedule for classroom rollout.

Exit criteria:

- One complete lesson workflow succeeds.
- Offline editing/saving works.
- Offline physical execution works or blocker is formally documented.
- Teacher agrees whether the system is ready for broader use.

## Critical Path

The critical path is:

1. Offline ORA transport proof.
2. Runtime adapter.
3. Safety validation.
4. Teacher approval workflow.
5. Supervised pilot.

If the offline transport proof fails, continue building the education platform but pause claims about offline physical execution until Ozobot or another supported path resolves the blocker.

## Parallel Work

These tasks can happen while offline transport is being validated:

- Project storage.
- Local roster.
- Blockly editor.
- Python preview/editor.
- Export/import.
- Teacher review queue.
- Documentation.

These tasks should not proceed to student-facing physical execution until offline transport and safety are validated:

- Selected-student run permission.
- Physical run queue.
- Raw advanced ORA commands.
- Python execution against real hardware.

## Review Questions Still Open

- Which email provider should be implemented first?
- Should first pilot identity be roster-based or simple station-mode names?
- Does district IT require HTTPS on the classroom LAN?
- Who approves the safety profile?
- Can we contact Ozobot directly for offline/local ORA execution guidance?
- What exact dates should align with the school calendar?
