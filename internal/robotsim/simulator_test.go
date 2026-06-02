package robotsim

import (
	"context"
	"path/filepath"
	"testing"

	"mps-robots/internal/robot"
)

func TestSimulatorDefaultsAndState(t *testing.T) {
	sim := NewSimulator()
	ctx := context.Background()

	state, err := sim.GetState(ctx)
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}

	if !state.IsReady {
		t.Error("expected simulator to be ready")
	}
	if state.ErrorCode != 0 {
		t.Errorf("expected error code 0, got %d", state.ErrorCode)
	}
	if state.GripperState != "open" {
		t.Errorf("expected open gripper, got %q", state.GripperState)
	}

	// Home positions
	if state.TCPPose[0] != 180.0 || state.TCPPose[1] != 0.0 || state.TCPPose[2] != 120.0 {
		t.Errorf("unexpected initial TCP pose: %v", state.TCPPose)
	}
}

func TestSimSkillsAndObjects(t *testing.T) {
	sim := NewSimulator()
	sim.StepDelay = 0 // Instant motion
	ctx := context.Background()

	// Initial object checks
	if len(sim.Objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(sim.Objects))
	}
	obj := sim.Objects[0]
	if obj.Name != "red_cube" || obj.Grasped {
		t.Errorf("unexpected initial object state: %+v", obj)
	}

	// 1. Pick skill
	_, err := sim.ExecuteNamedSkill(ctx, "pick_from_known_slot")
	if err != nil {
		t.Fatalf("pick failed: %v", err)
	}

	state, _ := sim.GetState(ctx)
	if state.GripperState != "closed" {
		t.Errorf("expected gripper to be closed, got %q", state.GripperState)
	}
	if !sim.Objects[0].Grasped {
		t.Error("expected object to be grasped")
	}

	// 2. Place skill
	_, err = sim.ExecuteNamedSkill(ctx, "place_at_known_slot")
	if err != nil {
		t.Fatalf("place failed: %v", err)
	}

	state, _ = sim.GetState(ctx)
	if state.GripperState != "open" {
		t.Errorf("expected gripper to be open, got %q", state.GripperState)
	}
	if sim.Objects[0].Grasped {
		t.Error("expected object to be released")
	}

	// Verify object has moved to slot 2 (X: 200, Y: 100)
	finalPos := sim.Objects[0].Position
	if finalPos[0] != 200.0 || finalPos[1] != 100.0 {
		t.Errorf("expected object at slot 2 (200, 100), got position: %v", finalPos)
	}
}

func TestUnsafeMotionRejection(t *testing.T) {
	sim := NewSimulator()
	sim.StepDelay = 0
	ctx := context.Background()

	// Try out of bounds motion
	tooFar := 500.0
	yVal := 0.0
	zVal := 100.0
	_, err := sim.MoveToSafePose(ctx, robot.MovePoseArgs{X: &tooFar, Y: &yVal, Z: &zVal})
	if err == nil {
		t.Error("expected out of bounds motion to be rejected")
	}

	state, _ := sim.GetState(ctx)
	if state.ErrorCode != 101 {
		t.Errorf("expected error code 101 for safety violation, got %d", state.ErrorCode)
	}
}

func TestEmergencyStop(t *testing.T) {
	sim := NewSimulator()
	sim.StepDelay = 0
	ctx := context.Background()

	_, err := sim.EmergencyStop(ctx)
	if err != nil {
		t.Fatalf("emergency stop failed: %v", err)
	}

	state, _ := sim.GetState(ctx)
	if state.ErrorCode != 99 || state.IsReady {
		t.Errorf("unexpected error state after emergency stop: %+v", state)
	}

	// Any subsequent motion must fail
	_, err = sim.ExecuteNamedSkill(ctx, "home")
	if err == nil {
		t.Error("expected motion to fail when in error state")
	}
}

func TestScenarioRunner(t *testing.T) {
	sim := NewSimulator()
	ctx := context.Background()

	// Load and run basic pick place scenario
	path1 := filepath.Join("..", "..", "scenarios", "basic_pick_place.yaml")
	sc1, err := LoadScenario(path1)
	if err != nil {
		t.Fatalf("failed to load scenario 1: %v", err)
	}
	if err := RunScenario(ctx, sc1, sim); err != nil {
		t.Errorf("scenario 1 failed: %v", err)
	}

	// Load and run gripper test
	path2 := filepath.Join("..", "..", "scenarios", "gripper_test.yaml")
	sc2, err := LoadScenario(path2)
	if err != nil {
		t.Fatalf("failed to load scenario 2: %v", err)
	}
	if err := RunScenario(ctx, sc2, sim); err != nil {
		t.Errorf("scenario 2 failed: %v", err)
	}

	// Load and run unsafe motion rejection
	path3 := filepath.Join("..", "..", "scenarios", "unsafe_motion_rejection.yaml")
	sc3, err := LoadScenario(path3)
	if err != nil {
		t.Fatalf("failed to load scenario 3: %v", err)
	}
	if err := RunScenario(ctx, sc3, sim); err != nil {
		t.Errorf("scenario 3 failed: %v", err)
	}

	// Load and run recovery from error
	path4 := filepath.Join("..", "..", "scenarios", "recovery_from_error.yaml")
	sc4, err := LoadScenario(path4)
	if err != nil {
		t.Fatalf("failed to load scenario 4: %v", err)
	}
	if err := RunScenario(ctx, sc4, sim); err != nil {
		t.Errorf("scenario 4 failed: %v", err)
	}
}

func TestScenarioRunnerDeterministicFails(t *testing.T) {
	sim := NewSimulator()
	ctx := context.Background()

	// Construct an expected fail scenario (e.g. step expected success but fails due to error state)
	sc := &Scenario{
		Name: "fail_scenario",
		InitialState: InitialState{
			ErrorCode: 5,
		},
		Steps: []ScenarioStep{
			{
				Tool:           "execute_named_skill",
				Args:           map[string]any{"skill_name": "home"},
				ExpectedStatus: "success", // Will fail because robot starts in error state
			},
		},
	}

	err := RunScenario(ctx, sc, sim)
	if err == nil {
		t.Error("expected scenario run to fail")
	}
}
