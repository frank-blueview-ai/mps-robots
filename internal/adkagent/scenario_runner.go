package adkagent

import (
	"context"
	"fmt"
	"math"

	"google.golang.org/adk/session"
	"google.golang.org/genai"
	"mps-robots/internal/robotsim"
)

type AgentScenarioResult struct {
	ScenarioName        string           `json:"scenarioName"`
	UserInstruction     string           `json:"userInstruction"`
	Mode                string           `json:"mode"`
	AssistantText       string           `json:"assistantText"`
	ToolCalls           []ToolCallDetail `json:"toolCalls"`
	ConfirmationCallIDs []string         `json:"confirmationCallIds"`
	FinalStateSummary   string           `json:"finalStateSummary"`
	Pass                bool             `json:"pass"`
	ErrorMessage        string           `json:"errorMessage,omitempty"`
}

func RunAgentScenario(ctx context.Context, sc *robotsim.Scenario, m *Manager, sim *robotsim.Simulator) (*AgentScenarioResult, error) {
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

	sessionID := generateSessionID()

	// Create ADK session
	_, _ = m.sessionService.Create(ctx, &session.CreateRequest{AppName: "mps-robots", UserID: "user", SessionID: sessionID})

	var result AgentScenarioResult
	result.ScenarioName = sc.Name
	result.UserInstruction = sc.UserInstruction
	result.Mode = m.cfg.ArmMode

	// Send user instruction to the ADK agent manager
	userMessage := genai.NewContentFromText(sc.UserInstruction, genai.RoleUser)

	var currentResponse ChatResponse
	var err error

	currentResponse, err = m.RunTurnInternal(ctx, sessionID, userMessage)
	if err != nil {
		result.Pass = false
		result.ErrorMessage = fmt.Sprintf("first turn error: %v", err)
		return &result, nil
	}

	result.AssistantText = currentResponse.AssistantText
	result.ToolCalls = append(result.ToolCalls, currentResponse.ToolCalls...)

	for turn := 0; turn < 10; turn++ {
		if currentResponse.Error != "" {
			result.ErrorMessage = currentResponse.Error
			break
		}

		if currentResponse.ConfirmationRequired {
			result.ConfirmationCallIDs = append(result.ConfirmationCallIDs, currentResponse.ConfirmationCallID)

			if sc.ConfirmationRequired {
				// Auto-confirm
				currentResponse, err = m.ConfirmInternal(ctx, sessionID, currentResponse.ConfirmationCallID, true)
				if err != nil {
					result.Pass = false
					result.ErrorMessage = fmt.Sprintf("confirmation error: %v", err)
					return &result, nil
				}
				result.ToolCalls = append(result.ToolCalls, currentResponse.ToolCalls...)
				if currentResponse.AssistantText != "" {
					result.AssistantText += " | " + currentResponse.AssistantText
				}
			} else {
				// Deny confirmation
				currentResponse, err = m.ConfirmInternal(ctx, sessionID, currentResponse.ConfirmationCallID, false)
				if err != nil {
					result.Pass = false
					result.ErrorMessage = fmt.Sprintf("confirmation deny error: %v", err)
					return &result, nil
				}
				break
			}
		} else {
			break
		}
	}

	// Verify final simulator state using public GetRawState
	x, y, z, gripper, errorCode, _, objects := sim.GetRawState()
	result.FinalStateSummary = fmt.Sprintf("Pose: [%.1f, %.1f, %.1f], Gripper: %s, ErrorCode: %d, Objects: %d",
		x, y, z, gripper, errorCode, len(objects))

	// Assertions
	if len(sc.ExpectedFinalState.Pose) >= 3 {
		pose := []float64{x, y, z}
		for i := 0; i < 3; i++ {
			if math.Abs(pose[i]-sc.ExpectedFinalState.Pose[i]) > 2.0 {
				result.Pass = false
				result.ErrorMessage = fmt.Sprintf("final state mismatch: pose axis %d expected %.1f, got %.1f",
					i, sc.ExpectedFinalState.Pose[i], pose[i])
				return &result, nil
			}
		}
	}

	if sc.ExpectedFinalState.Gripper != "" {
		if gripper != sc.ExpectedFinalState.Gripper {
			result.Pass = false
			result.ErrorMessage = fmt.Sprintf("final state mismatch: gripper expected %q, got %q",
				sc.ExpectedFinalState.Gripper, gripper)
			return &result, nil
		}
	}

	if errorCode != sc.ExpectedFinalState.ErrorCode {
		result.Pass = false
		result.ErrorMessage = fmt.Sprintf("final state mismatch: error code expected %d, got %d",
			sc.ExpectedFinalState.ErrorCode, errorCode)
		return &result, nil
	}

	for _, expObj := range sc.ExpectedFinalState.Objects {
		found := false
		for _, simObj := range objects {
			if simObj.Name == expObj.Name {
				found = true
				if simObj.Grasped != expObj.Grasped {
					result.Pass = false
					result.ErrorMessage = fmt.Sprintf("final state mismatch: object %q grasped expected %t, got %t",
						expObj.Name, expObj.Grasped, simObj.Grasped)
					return &result, nil
				}
				for i := 0; i < 3; i++ {
					if math.Abs(simObj.Position[i]-expObj.Position[i]) > 8.0 {
						result.Pass = false
						result.ErrorMessage = fmt.Sprintf("final state mismatch: object %q axis %d expected %.1f, got %.1f",
							expObj.Name, i, expObj.Position[i], simObj.Position[i])
						return &result, nil
					}
				}
				break
			}
		}
		if !found {
			result.Pass = false
			result.ErrorMessage = fmt.Sprintf("final state mismatch: expected object %q not found", expObj.Name)
			return &result, nil
		}
	}

	result.Pass = true
	return &result, nil
}
