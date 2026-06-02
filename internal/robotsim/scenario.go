package robotsim

import (
	"context"
	"fmt"
	"math"
	"os"

	"gopkg.in/yaml.v3"
	"mps-robots/internal/robot"
)

type InitialState struct {
	Pose         []float64   `yaml:"pose"`
	Gripper      string      `yaml:"gripper"`
	ErrorCode    int         `yaml:"error_code"`
	ErrorMessage string      `yaml:"error_message"`
	Objects      []SimObject `yaml:"objects"`
	Slots        []SimSlot   `yaml:"slots"`
}

type ScenarioStep struct {
	Tool           string         `yaml:"tool"`
	Args           map[string]any `yaml:"args"`
	ExpectedStatus string         `yaml:"expected_status"` // "success" or "error" or "rejected"
}

type ExpectedFinalState struct {
	Pose      []float64   `yaml:"pose"`
	Gripper   string      `yaml:"gripper"`
	ErrorCode int         `yaml:"error_code"`
	Objects   []SimObject `yaml:"objects"`
}

type Scenario struct {
	Name                 string             `yaml:"name"`
	Description          string             `yaml:"description"`
	InitialState         InitialState       `yaml:"initial_state"`
	UserInstruction      string             `yaml:"user_instruction"`
	Steps                []ScenarioStep     `yaml:"steps"`
	ExpectedFinalState   ExpectedFinalState `yaml:"expected_final_state"`
	ConfirmationRequired bool               `yaml:"confirmation_required"`
}

// LoadScenario loads a scenario from a YAML file.
func LoadScenario(filepath string) (*Scenario, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read scenario file: %w", err)
	}

	var sc Scenario
	if err := yaml.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("failed to parse scenario YAML: %w", err)
	}

	return &sc, nil
}

// RunScenario executes a scenario against the simulator and verifies results.
func RunScenario(ctx context.Context, sc *Scenario, sim *Simulator) error {
	// Set StepDelay to 0 to execute tests instantly
	originalDelay := sim.StepDelay
	sim.StepDelay = 0
	defer func() { sim.StepDelay = originalDelay }()

	// Apply Initial State
	sim.SetInitialState(
		sc.InitialState.Pose,
		sc.InitialState.Gripper,
		sc.InitialState.ErrorCode,
		sc.InitialState.ErrorMessage,
		sc.InitialState.Objects,
		sc.InitialState.Slots,
	)

	// Execute steps
	for idx, step := range sc.Steps {
		var err error
		switch step.Tool {
		case "execute_named_skill":
			skillName, _ := step.Args["skill_name"].(string)
			_, err = sim.ExecuteNamedSkill(ctx, skillName)
		case "move_to_safe_pose":
			poseArgs := robot.MovePoseArgs{}
			if poseName, ok := step.Args["pose_name"].(string); ok {
				poseArgs.PoseName = poseName
			}
			if xVal, ok := step.Args["x"].(float64); ok {
				poseArgs.X = &xVal
			} else if xValInt, ok := step.Args["x"].(int); ok {
				xf := float64(xValInt)
				poseArgs.X = &xf
			}
			if yVal, ok := step.Args["y"].(float64); ok {
				poseArgs.Y = &yVal
			} else if yValInt, ok := step.Args["y"].(int); ok {
				yf := float64(yValInt)
				poseArgs.Y = &yf
			}
			if zVal, ok := step.Args["z"].(float64); ok {
				poseArgs.Z = &zVal
			} else if zValInt, ok := step.Args["z"].(int); ok {
				zf := float64(zValInt)
				poseArgs.Z = &zf
			}
			_, err = sim.MoveToSafePose(ctx, poseArgs)
		case "emergency_stop":
			_, err = sim.EmergencyStop(ctx)
		case "reset":
			sim.Reset()
		default:
			return fmt.Errorf("step %d: unsupported tool command %q", idx, step.Tool)
		}

		// Verify result
		if err != nil {
			if step.ExpectedStatus == "success" {
				return fmt.Errorf("step %d: expected success but got error: %v", idx, err)
			}
		} else {
			if step.ExpectedStatus == "error" || step.ExpectedStatus == "rejected" {
				return fmt.Errorf("step %d: expected failure/rejection but call succeeded", idx)
			}
		}
	}

	// Validate final state
	sim.mu.Lock()
	defer sim.mu.Unlock()

	// 1. Robot pose check
	if len(sc.ExpectedFinalState.Pose) >= 3 {
		for i := 0; i < 3; i++ {
			if math.Abs(sim.tcpPose[i]-sc.ExpectedFinalState.Pose[i]) > 1.0 {
				return fmt.Errorf("final state mismatch: robot pose axis %d expected %.1f, got %.1f",
					i, sc.ExpectedFinalState.Pose[i], sim.tcpPose[i])
			}
		}
	}

	// 2. Gripper check
	if sc.ExpectedFinalState.Gripper != "" {
		if sim.gripperState != sc.ExpectedFinalState.Gripper {
			return fmt.Errorf("final state mismatch: gripper expected %q, got %q",
				sc.ExpectedFinalState.Gripper, sim.gripperState)
		}
	}

	// 3. Error code check
	if sim.errorCode != sc.ExpectedFinalState.ErrorCode {
		return fmt.Errorf("final state mismatch: error code expected %d, got %d",
			sc.ExpectedFinalState.ErrorCode, sim.errorCode)
	}

	// 4. Objects check
	for _, expObj := range sc.ExpectedFinalState.Objects {
		found := false
		for _, simObj := range sim.Objects {
			if simObj.Name == expObj.Name {
				found = true
				// Check grasped status
				if simObj.Grasped != expObj.Grasped {
					return fmt.Errorf("final state mismatch: object %q grasped state expected %t, got %t",
						expObj.Name, expObj.Grasped, simObj.Grasped)
				}
				// Check position within tolerance
				for i := 0; i < 3; i++ {
					if math.Abs(simObj.Position[i]-expObj.Position[i]) > 5.0 {
						return fmt.Errorf("final state mismatch: object %q axis %d expected %.1f, got %.1f",
							expObj.Name, i, expObj.Position[i], simObj.Position[i])
					}
				}
				break
			}
		}
		if !found {
			return fmt.Errorf("final state mismatch: expected object %q not found in simulator", expObj.Name)
		}
	}

	return nil
}
