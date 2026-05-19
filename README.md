# ORA Arm Web Controller

This project serves a local ORA arm control page through project-local NGINX.

## Run

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-nginx.ps1
```

Open:

```text
http://localhost:8080
```

## Run With ORA Bridge

The local controller needs the ORA bridge to send real commands. The bridge opens the official Ozobot Editor in Playwright, connects to ORA using the same WebRTC data channel mechanism as the editor, and exposes local endpoints under `/bridge/...`.

Start NGINX first, then start the bridge:

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

The ORA device name and password are read from `.env` for this station.

Check bridge status:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-bridge.ps1 status
```

## Stop

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\stop-nginx.ps1
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

Run the official Ozobot Editor connection E2E without storing the password in files:

```powershell
$env:ORA_NAME = "ORA-FEA252"
$env:ORA_PASSWORD = "<password>"
npx playwright test tests/official-editor-connection.spec.js
```

The official-editor test verifies the same connection flow used by `https://editor.ozobot.com/en/blockly`: it opens the editor, enters the ORA credentials, and waits for the connected-state `Disconnect` button. It does not press `RUN` or send motion commands.
