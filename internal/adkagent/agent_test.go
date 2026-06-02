package adkagent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/adk/agent"
	"mps-robots/internal/robot"
	"mps-robots/internal/robotsim"
)

// mockToolContext stubs out the ADK tool.Context interface for direct unit testing of functions.
type mockToolContext struct {
	agent.ToolContext
	ctx context.Context
}

func (m *mockToolContext) Deadline() (deadline time.Time, ok bool) { return m.ctx.Deadline() }
func (m *mockToolContext) Done() <-chan struct{}                   { return m.ctx.Done() }
func (m *mockToolContext) Err() error                              { return m.ctx.Err() }
func (m *mockToolContext) Value(key any) any                       { return m.ctx.Value(key) }

func TestManagerModeDefaultsAndValidation(t *testing.T) {
	// 1. ADK_ARM_MODE defaults to dry_run if empty.
	m, err := NewManager(Config{
		GeminiAPIKey: "fake_key",
		ArmMode:      "",
	})
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	if m.cfg.ArmMode != "dry_run" {
		t.Errorf("expected ArmMode to default to dry_run, got %q", m.cfg.ArmMode)
	}
	if _, ok := m.controller.(*DryRunController); !ok {
		t.Errorf("expected controller to be DryRunController, got %T", m.controller)
	}

	// 2. Live mode is rejected unless explicitly enabled (EnableMotion must be true).
	_, err = NewManager(Config{
		GeminiAPIKey: "fake_key",
		ArmMode:      "live",
		EnableMotion: false,
	})
	if err == nil {
		t.Error("expected NewManager to return error for live mode when EnableMotion is false")
	}

	// 3. Live mode is accepted when EnableMotion is true.
	mLive, err := NewManager(Config{
		GeminiAPIKey: "fake_key",
		ArmMode:      "live",
		EnableMotion: true,
	})
	if err != nil {
		t.Fatalf("failed to create manager in live mode: %v", err)
	}
	if _, ok := mLive.controller.(*LiveController); !ok {
		t.Errorf("expected controller to be LiveController, got %T", mLive.controller)
	}
}

func TestSimModeNeverCallsBridge(t *testing.T) {
	var called int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&called, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Instantiate manager in sim mode with the bridge URL set to the mock server.
	m, err := NewManager(Config{
		GeminiAPIKey: "fake_key",
		ArmMode:      "sim",
		BridgeURL:    server.URL,
	})
	if err != nil {
		t.Fatalf("failed to create manager in sim mode: %v", err)
	}

	ctx := &mockToolContext{ctx: context.Background()}

	// Query state and execute a skill in sim mode.
	_, err = m.getRobotStateTool(ctx, EmptyArgs{})
	if err != nil {
		t.Fatalf("getRobotStateTool failed: %v", err)
	}

	_, err = m.executeNamedSkillTool(ctx, robot.ExecuteSkillArgs{SkillName: "home"})
	if err != nil {
		t.Fatalf("executeNamedSkillTool failed: %v", err)
	}

	// Verify that the mock bridge server was never called.
	if atomic.LoadInt32(&called) > 0 {
		t.Error("expected sim mode to never call the physical robot bridge")
	}
}

func TestGetRobotStateToolLive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"connected": true,
			"latest": {
				"xarm_connected": true,
				"xarm_is_ready": true,
				"xarm_state": 0,
				"xarm_error_code": 0,
				"xarm_tcp_pose": [123.4, -45.6, 210.0, 1, 2, 3],
				"xarm_joint_pose": [10, 20, 30, 40, 50, 60]
			}
		}`))
	}))
	defer server.Close()

	m := &Manager{
		cfg: Config{
			BridgeURL:    server.URL,
			EnableMotion: true,
			ArmMode:      "live",
		},
		controller: NewLiveController(server.URL, false),
	}

	ctx := &mockToolContext{ctx: context.Background()}
	state, err := m.getRobotStateTool(ctx, EmptyArgs{})
	if err != nil {
		t.Fatalf("getRobotStateTool failed: %v", err)
	}

	if !state.BridgeConnected {
		t.Error("expected BridgeConnected to be true")
	}
	if !state.RobotConnected {
		t.Error("expected RobotConnected to be true")
	}
	if state.TCPPose[0] != 123.4 {
		t.Errorf("expected X coordinate 123.4, got %.1f", state.TCPPose[0])
	}
}

func TestListRobotCapabilities(t *testing.T) {
	m := &Manager{
		controller: NewDryRunController(),
	}

	ctx := &mockToolContext{ctx: context.Background()}
	caps, err := m.listRobotCapabilitiesTool(ctx, EmptyArgs{})
	if err != nil {
		t.Fatalf("listRobotCapabilitiesTool failed: %v", err)
	}

	if len(caps.SupportedActions) == 0 {
		t.Error("expected non-empty supported actions")
	}
	if caps.MotionMode != "dry-run" {
		t.Errorf("expected motion mode dry-run, got %s", caps.MotionMode)
	}
}

func TestExecuteNamedSkillDryRunAndLive(t *testing.T) {
	var called int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"connected": true,
				"latest": {
					"xarm_connected": true,
					"xarm_is_ready": true,
					"xarm_state": 0,
					"xarm_error_code": 0
				}
			}`))
			return
		}
		atomic.AddInt32(&called, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mDry := &Manager{
		controller: NewDryRunController(),
	}

	ctx := &mockToolContext{ctx: context.Background()}
	resDry, err := mDry.executeNamedSkillTool(ctx, robot.ExecuteSkillArgs{SkillName: "home"})
	if err != nil {
		t.Fatalf("executeNamedSkill dry-run failed: %v", err)
	}
	if resDry.Status != `dry-run: successfully simulated skill "home"` {
		t.Errorf("unexpected status: %s", resDry.Status)
	}
	if atomic.LoadInt32(&called) > 0 {
		t.Error("expected no bridge calls in dry-run mode")
	}

	mLive := &Manager{
		controller: NewLiveController(server.URL, false),
	}

	resLive, err := mLive.executeNamedSkillTool(ctx, robot.ExecuteSkillArgs{SkillName: "home"})
	if err != nil {
		t.Fatalf("executeNamedSkill live-run failed: %v", err)
	}
	if resLive.Status != `live: successfully executed skill "home"` {
		t.Errorf("unexpected status: %s", resLive.Status)
	}
	if atomic.LoadInt32(&called) != 1 {
		t.Errorf("expected 1 bridge call in live mode, got %d", atomic.LoadInt32(&called))
	}
}

func TestExecuteNamedSkillUnknownRejected(t *testing.T) {
	m := &Manager{
		controller: NewDryRunController(),
	}

	ctx := &mockToolContext{ctx: context.Background()}
	_, err := m.executeNamedSkillTool(ctx, robot.ExecuteSkillArgs{SkillName: "invalid_skill"})
	if err == nil {
		t.Fatal("expected error for invalid skill name")
	}
}

func TestMoveToSafePoseLimitsValidation(t *testing.T) {
	m := &Manager{
		controller: NewDryRunController(),
	}

	ctx := &mockToolContext{ctx: context.Background()}

	xVal, yVal, zVal := 200.0, 0.0, 100.0
	_, err := m.moveToSafePoseTool(ctx, robot.MovePoseArgs{X: &xVal, Y: &yVal, Z: &zVal})
	if err != nil {
		t.Fatalf("expected valid coordinates to succeed, got: %v", err)
	}

	unsafeX := 50.0
	_, err = m.moveToSafePoseTool(ctx, robot.MovePoseArgs{X: &unsafeX, Y: &yVal, Z: &zVal})
	if err == nil {
		t.Fatal("expected coordinates out of safe bounds to be rejected")
	}
}

func TestEmergencyStopBypasses(t *testing.T) {
	var called int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&called, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	m := &Manager{
		controller: NewLiveController(server.URL, false),
	}

	ctx := &mockToolContext{ctx: context.Background()}
	_, err := m.emergencyStopTool(ctx, EmptyArgs{})
	if err != nil {
		t.Fatalf("emergencyStopTool failed: %v", err)
	}

	if atomic.LoadInt32(&called) != 1 {
		t.Error("expected emergency stop to dispatch HTTP stop command to bridge")
	}
}

func TestConcurrentCallsSerialization(t *testing.T) {
	var activeCalls int32
	var maxConcurrent int32
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"connected": true,
				"latest": {
					"xarm_connected": true,
					"xarm_is_ready": true,
					"xarm_state": 0,
					"xarm_error_code": 0
				}
			}`))
			return
		}
		current := atomic.AddInt32(&activeCalls, 1)
		mu.Lock()
		if current > maxConcurrent {
			maxConcurrent = current
		}
		mu.Unlock()

		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&activeCalls, -1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	m := &Manager{
		controller: NewLiveController(server.URL, false),
	}

	ctx := &mockToolContext{ctx: context.Background()}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		m.executeNamedSkillTool(ctx, robot.ExecuteSkillArgs{SkillName: "home"})
	}()

	go func() {
		defer wg.Done()
		m.executeNamedSkillTool(ctx, robot.ExecuteSkillArgs{SkillName: "open_gripper"})
	}()

	wg.Wait()

	if atomic.LoadInt32(&maxConcurrent) > 1 {
		t.Errorf("expected maximum concurrent physical calls to be 1, got %d", maxConcurrent)
	}
}

func TestSimModeSimulatorStateTransitions(t *testing.T) {
	sim := robotsim.NewSimulator()
	sim.StepDelay = 0

	m := &Manager{
		controller: sim,
	}

	ctx := &mockToolContext{ctx: context.Background()}

	// Test that simulated skills update the shared simulator state correctly
	res, err := m.executeNamedSkillTool(ctx, robot.ExecuteSkillArgs{SkillName: "pick_from_known_slot"})
	if err != nil {
		t.Fatalf("failed to pick: %v", err)
	}
	if res.Status != `sim: successfully executed skill "pick_from_known_slot"` {
		t.Errorf("unexpected status: %s", res.Status)
	}

	// Verify that objects state was updated in the simulator
	_, _, _, gripper, _, _, objects := sim.GetRawState()
	if gripper != "closed" {
		t.Errorf("expected gripper to be closed in simulator state, got %s", gripper)
	}
	if len(objects) == 0 || !objects[0].Grasped {
		t.Error("expected object to be grasped in simulator state")
	}
}

func TestAgentScenarioRunner(t *testing.T) {
	sim := robotsim.NewSimulator()
	sim.StepDelay = 0

	m, err := NewManager(Config{
		GeminiAPIKey:        "fake_key",
		ArmMode:             "sim",
		RequireConfirmation: true,
		Simulator:           sim,
	})
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx := context.Background()

	path1 := filepath.Join("..", "..", "scenarios", "basic_pick_place.yaml")
	sc1, err := robotsim.LoadScenario(path1)
	if err != nil {
		t.Fatalf("failed to load scenario 1: %v", err)
	}

	res, err := RunAgentScenario(ctx, sc1, m, sim)
	if err != nil {
		t.Fatalf("agent scenario run failed: %v", err)
	}
	if !res.Pass {
		t.Errorf("expected agent scenario to pass, got error: %s", res.ErrorMessage)
	}
}
