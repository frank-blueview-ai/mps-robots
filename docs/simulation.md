# 3D Digital Twin Simulation Mode

The Ozobot ORA Robot Agent application supports a **3D Digital Twin Simulator** that enables training, testing, and demonstrating agent behavior in a virtual environment before driving motion on the physical arm.

---

## Kinematic & Motion Modes

The application dynamically shifts between three arm modes via configuration:

1. **`dry_run`**: No physical or simulated arm motions. Command tools run safety limit validations and log output but do not affect or update coordinate states.
2. **`sim`**: Motions run inside a local 3D kinematics simulator. Simulates coordinate interpolation over time, gripper grasp physics, slot collisions, and error states.
3. **`live`**: Motions drive real physical joints on the Ozobot ORA arm via the Playwright-bound Node REST bridge.

---

## Configuration Variables

Configure these settings in your `.env` file or export them as environment variables:

| Variable | Default | Valid Values | Description |
| :--- | :--- | :--- | :--- |
| `ADK_ARM_MODE` | `dry_run` | `dry_run`, `sim`, `live` | Selects the active backend controller driver. |
| `ADK_ARM_ENABLE_MOTION` | `false` | `true`, `false` | Stricter safety lock. `live` mode is rejected unless explicitly set to `true`. |
| `ADK_REQUIRE_CONFIRMATION` | `true` | `true`, `false` | Mandates Human-in-the-Loop confirmations prior to executing named skills or Cartesian movements. |

---

## Starting the Application in Simulation Mode

1. Set variables in your `.env` file:
   ```env
   ADK_ARM_MODE=sim
   ADK_ARM_ENABLE_MOTION=false
   ADK_REQUIRE_CONFIRMATION=true
   GEMINI_API_KEY=your_gemini_api_key
   ```

2. Run the server:
   ```bash
   go run ./cmd/ora-server
   ```

3. Open the **3D Simulator Visualizer** in your web browser:
   [http://localhost:8081/sim/view](http://localhost:8081/sim/view)

---

## 3D Simulation Features

The visualizer dashboard renders a real-time digital twin of the ORA workspace:
* **Arm Kinematics**: Displays base cylinder, 3 main arm links, joint axes, and gripper fingers rotating based on joint state.
* **Workspace Surface**: Includes a grid floor and a table.
* **Slots/Bins**: Renders slot targets (Slot 1 and Slot 2) as colored rings.
* **Rigid Objects**: Simulates solid colored blocks inside the workspace. A red cube starts at Slot 1.
* **Object Grasping**: Closing the gripper near the cube triggers grasping. The object will stick to the gripper tool center point (TCP) and follow its path until dropped or placed in a slot.
* **Smooth Interpolation**: Movement commands animate joints from source to destination over time at a speed of 150 mm/s.
* **Emergency Halt**: Triggering emergency stop instantly freezes the arm, enters a recovery error state (ErrorCode 99), and rejects subsequent commands until a simulation reset is dispatched.

---

## Scripted Evaluation Scenarios

Scenarios define deterministic test suites to validate skills under various environments.

### Supported Scenarios:
1. **Basic Pick & Place** (`scenarios/basic_pick_place.yaml`): Tests picking the red cube from Slot 1 and placing it at Slot 2.
2. **Gripper Function Test** (`scenarios/gripper_test.yaml`): Exercises gripper opening, closing, and object contact verification.
3. **Unsafe Motion Rejection** (`scenarios/unsafe_motion_rejection.yaml`): Ensures movement attempts out-of-bounds (e.g., X=50mm) fail validation.
4. **Recovery From Error State** (`scenarios/recovery_from_error.yaml`): Starts in error state, verifies motion is rejected, executes a recovery reset, and finishes homing.

### Running Scenarios:
* **Via Web Dashboard**: Open [http://localhost:8081/sim/view](http://localhost:8081/sim/view), select a scenario from the dropdown list, and click **Execute Scenario Test**. Step-by-step logs and outcomes will print directly in the Evaluation Logs console.
* **Via HTTP REST API**:
  ```bash
  curl -X POST http://localhost:8081/api/sim/scenarios/run \
       -H "Content-Type: application/json" \
       -d '{"scenario": "basic_pick_place.yaml"}'
  ```

---

## Limitations & Safety Warnings

> [!WARNING]
> **SIMULATION DOES NOT PROVE REAL-WORLD SAFETY**
> * **Kinematic Approximation**: The simulator uses a simplified geometric link model to animate positions in the browser. It does not calculate exact link torque, inertia, cable tolerances, or dynamic payload parameters.
> * **No Collision Validation**: Success in simulation does not guarantee collision-free execution on the physical ORA arm. Real-world obstacles, environment changes, and physical misalignment are not modeled.
> * **Safety Gates are Mandatory**: Simulated tests must never be used to bypass physical limits, teacher approvals, or verification gates. Physical deployment in `live` mode must always maintain strict Human-in-the-Loop confirmations (`ADK_REQUIRE_CONFIRMATION=true`).
