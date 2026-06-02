# Google Agent Development Kit (ADK) Integration for ORA Arm

This documentation explains how the Google Agent Development Kit (ADK) agent layer is configured, run, and verified on this ORA arm station controller.

## Environment Configurations

The ADK agent expects the following environment variables (which can be defined in Windows environment or the `.env` file at the root of this repository):

| Variable Name | Default Value | Description |
|---|---|---|
| `GEMINI_API_KEY` or `GOOGLE_API_KEY` | *Required* | API key for Gemini access. If missing, agent features are gracefully disabled. |
| `ADK_ARM_ENABLE_MOTION` | `false` | When `false`, all robot motions are intercepted and simulated in **dry-run** mode. When `true`, motions are forwarded to the ORA physical controller. |
| `ADK_REQUIRE_CONFIRMATION` | `true` | When `true`, sensitive physical movements (skills and coordinates moves) require a Human-in-the-Loop approval step. |
| `ADK_GEMINI_MODEL` | `gemini-2.5-flash` | The Gemini API model used for natural language understanding and function tool calling. |

---

## Safety Guidelines & Warnings

> [!CAUTION]
> **Safety Overrides (ADK_ARM_ENABLE_MOTION)**
> Keep `ADK_ARM_ENABLE_MOTION=false` during testing to prevent unexpected arm movements. Enable it only when the physical workspace is clear and you are ready to monitor live execution.

> [!IMPORTANT]
> **Human-in-the-Loop Confirmation**
> We recommend keeping `ADK_REQUIRE_CONFIRMATION=true` to review and approve every motion before it runs. Emergency stop `emergency_stop` always executes immediately and bypasses any confirmation check.

---

## Exposed Agent Tools

The agent has exclusive access to the following safe wrappers:

1. **`get_robot_state`**: Retrieves bridge/robot status, current joints angles (`J1` to `J6`), Cartesian tool Center Point (TCP) coordinates (`X`, `Y`, `Z`, `Roll`, `Pitch`, `Yaw`), errors, and motion status.
2. **`list_robot_capabilities`**: Exposes the allowable pre-defined skills, physical workspace limits, gripper features, camera state, and current motion mode.
3. **`plan_robot_action`**: High-level planner tool that returns step-by-step description of an action sequence without executing it.
4. **`execute_named_skill`**: Dispatches pre-defined skills from an allowlist (`home`, `open_gripper`, `close_gripper`, `move_to_observation_pose`, `pick_from_known_slot`, `place_at_known_slot`). Reject unrecognized skill requests.
5. **`move_to_safe_pose`**: Moves the arm to a named pose (`home`, `observation`) or validates custom `X`, `Y`, `Z` coordinates against safe boundaries (`X`: [100, 300] mm, `Y`: [-250, 250] mm, `Z`: [20, 250] mm).
6. **`emergency_stop`**: Instantly halts arm movement.

---

## HTTP Integration Endpoints

The Go server exposes the following REST interface for the ADK agent:

### 1. Chat Turn (`POST /api/agent/chat`)
Send natural-language prompts.
**Request Body:**
```json
{
  "message": "Move to the observation pose.",
  "sessionId": "optional_session_uuid"
}
```
**Response Body (If Confirmation Required):**
```json
{
  "sessionId": "example-session-id",
  "assistantText": "I'm going to move the arm to the observation pose. Can you please confirm that it is safe to proceed?",
  "toolCalls": [
    {
      "name": "execute_named_skill",
      "args": {
        "skill_name": "move_to_observation_pose"
      }
    }
  ],
  "confirmationRequired": true,
  "dryRun": true
}
```

### 2. Action Confirmation (`POST /api/agent/confirm`)
Approve or reject a pending tool execution.
**Request Body:**
```json
{
  "sessionId": "example-session-id",
  "confirmed": true
}
```
**Response Body:**
```json
{
  "sessionId": "example-session-id",
  "assistantText": "I've successfully moved the arm to the observation pose.",
  "toolCalls": [],
  "confirmationRequired": false,
  "dryRun": true
}
```

### 3. Agent Configuration State (`GET /api/agent/state`)
Retrieves the current controller configuration status.
**Response Body:**
```json
{
  "enableMotion": false,
  "requireConfirmation": true,
  "geminiModel": "gemini-2.5-flash"
}
```

---

## Limitations

- **Camera & Scene Calibration:** This station does not have physical camera or vision calibration configured. Therefore, natural-language commands requesting to "pick up the red block from the workspace" based on images will be rejected unless mapping to pre-defined slot coordinates (`pick_from_known_slot`).
- **Workspace Bounds:** Movement coordinates outside the conservative bounds are rejected immediately at the tool level to prevent joint limits errors or collisions.
