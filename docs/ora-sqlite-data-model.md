# ORA SQLite Data Model

Status: initial local schema implemented

Date: 2026-05-19

## Decision

Use SQLite for the one-station classroom pilot. It keeps the application local, simple to start, and easy to back up. PostgreSQL remains the recommended path if the platform moves to a central server, multiple ORA stations, or district-wide hosting.

## Database Location

```text
runtime/data/ora.db
```

SQLite may also create:

```text
runtime/data/ora.db-wal
runtime/data/ora.db-shm
```

Back up all three files when the server is stopped.

## Initial Tables

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

## Current Behavior

- The browser project workflow uses `/api/projects`.
- The browser classroom admin workflow uses `/api/users` and `/api/classes`.
- Projects are stored in SQLite, not loose JSON files.
- User profiles and classes are stored in SQLite.
- Every project create and update writes a project version snapshot.
- Project, user, and class create/update/delete operations write audit events.
- A default `station-admin` user row is created.
- A default `pilot-default` safety profile is created.
- Older JSON projects under `runtime/data/projects` are imported on server start if they are not already in SQLite.
- Project export/import remains JSON-based for portability between stations.

## Next Schema Work

- Add teacher/admin UI for users and classes.
- Add project submissions and review UI.
- Add teacher approval records before physical runs.
- Add robot run logs once command-plan execution exists.
- Add backup/restore scripts for `runtime/data/ora.db`.
