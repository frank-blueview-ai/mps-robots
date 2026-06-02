package adkagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"mps-robots/internal/robot"
)

// LiveController connects to the physical bridge REST endpoints.
type LiveController struct {
	bridgeURL         string
	allowRawCartesian bool
	client            *http.Client
	mu                sync.Mutex
}

func NewLiveController(bridgeURL string, allowRawCartesian bool) *LiveController {
	return &LiveController{
		bridgeURL:         bridgeURL,
		allowRawCartesian: allowRawCartesian,
		client:            &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *LiveController) callBridge(ctx context.Context, method, path string, reqBody any, respTarget any) error {
	url := fmt.Sprintf("%s%s", c.bridgeURL, path)
	var bodyReader *bytes.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader([]byte("{}"))
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach bridge: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusCreated {
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Error != "" {
			return fmt.Errorf("bridge returned status %d: %s", resp.StatusCode, errResp.Error)
		}
		return fmt.Errorf("bridge returned HTTP status %d", resp.StatusCode)
	}

	if respTarget != nil {
		if err := json.NewDecoder(resp.Body).Decode(respTarget); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}
	return nil
}

func (c *LiveController) GetState(ctx context.Context) (*robot.RobotState, error) {
	var status struct {
		Connected bool `json:"connected"`
		Latest    *struct {
			XArmConnected bool `json:"xarm_connected"`
			XArmIsReady   bool `json:"xarm_is_ready"`
			XArmState     int  `json:"xarm_state"`
			XArmErrorCode int  `json:"xarm_error_code"`
			XArmError     *struct {
				Title struct {
					En string `json:"en"`
				} `json:"title"`
			} `json:"xarm_error"`
			XArmTCPPose   []float64 `json:"xarm_tcp_pose"`
			XArmJointPose []float64 `json:"xarm_joint_pose"`
		} `json:"latest"`
	}

	err := c.callBridge(ctx, http.MethodGet, "/status", nil, &status)
	if err != nil {
		return &robot.RobotState{
			BridgeConnected:       false,
			RobotConnected:        false,
			PhysicalMotionAllowed: false,
			GripperState:          "unknown",
		}, nil
	}

	if !status.Connected || status.Latest == nil {
		return &robot.RobotState{
			BridgeConnected:       status.Connected,
			RobotConnected:        false,
			PhysicalMotionAllowed: false,
			GripperState:          "unknown",
		}, nil
	}

	latest := status.Latest
	errMsg := ""
	if latest.XArmError != nil {
		errMsg = latest.XArmError.Title.En
	}

	motionAllowed := status.Connected && latest.XArmConnected && latest.XArmIsReady && latest.XArmErrorCode == 0

	return &robot.RobotState{
		BridgeConnected:       status.Connected,
		RobotConnected:        latest.XArmConnected,
		IsReady:               latest.XArmIsReady,
		State:                 latest.XArmState,
		ErrorCode:             latest.XArmErrorCode,
		ErrorMessage:          errMsg,
		TCPPose:               latest.XArmTCPPose,
		JointPose:             latest.XArmJointPose,
		PhysicalMotionAllowed: motionAllowed,
		GripperState:          "unknown",
	}, nil
}

func (c *LiveController) ListCapabilities(ctx context.Context) (*robot.Capabilities, error) {
	var actions []string
	for k := range robot.AllowedSkills {
		actions = append(actions, k)
	}

	limitsDesc := fmt.Sprintf("Cartesian Limits - X: [%.1f, %.1f] mm, Y: [%.1f, %.1f] mm, Z: [%.1f, %.1f] mm",
		robot.MinX, robot.MaxX, robot.MinY, robot.MaxY, robot.MinZ, robot.MaxZ)

	return &robot.Capabilities{
		SupportedActions: actions,
		WorkspaceLimits:  limitsDesc,
		GripperExists:    true,
		CameraExists:     false,
		MotionMode:       "live",
	}, nil
}

func (c *LiveController) ensureLiveReady(ctx context.Context, action string) error {
	state, err := c.GetState(ctx)
	if err != nil {
		return fmt.Errorf("readiness check failed: %w", err)
	}
	if !state.BridgeConnected {
		return fmt.Errorf("readiness check failed: bridge is disconnected")
	}
	if !state.RobotConnected {
		return fmt.Errorf("readiness check failed: robot is disconnected")
	}
	if !state.IsReady {
		return fmt.Errorf("readiness check failed: robot is not ready")
	}
	if state.ErrorCode != 0 {
		return fmt.Errorf("readiness check failed: robot has error code %d (%s)", state.ErrorCode, state.ErrorMessage)
	}
	if !state.PhysicalMotionAllowed {
		return fmt.Errorf("readiness check failed: PhysicalMotionAllowed is false")
	}
	return nil
}

func (c *LiveController) ExecuteNamedSkill(ctx context.Context, skillName string) (*robot.ExecuteSkillResults, error) {
	if !robot.AllowedSkills[skillName] {
		return nil, fmt.Errorf("skill %q is not in the allowed skills list", skillName)
	}

	if err := c.ensureLiveReady(ctx, "execute_named_skill: "+skillName); err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	switch skillName {
	case "home":
		err := c.callBridge(ctx, http.MethodPost, "/home", nil, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to home: %w", err)
		}
	case "open_gripper":
		err := c.callBridge(ctx, http.MethodPost, "/gripper", map[string]bool{"open": true}, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to open gripper: %w", err)
		}
	case "close_gripper":
		err := c.callBridge(ctx, http.MethodPost, "/gripper", map[string]bool{"open": false}, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to close gripper: %w", err)
		}
	case "move_to_observation_pose":
		pose := map[string]any{
			"x":         180.0,
			"y":         0.0,
			"z":         120.0,
			"a":         180.0,
			"b":         0.0,
			"c":         0.0,
			"wait":      true,
			"timeoutMs": 15000,
		}
		err := c.callBridge(ctx, http.MethodPost, "/move-line", pose, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to move to observation pose: %w", err)
		}
	case "pick_from_known_slot":
		if err := c.callBridge(ctx, http.MethodPost, "/gripper", map[string]bool{"open": true}, nil); err != nil {
			return nil, fmt.Errorf("pick: failed to open gripper: %w", err)
		}
		approach := map[string]any{"x": 200.0, "y": -100.0, "z": 80.0, "a": 180.0, "b": 0.0, "c": 0.0, "wait": true}
		if err := c.callBridge(ctx, http.MethodPost, "/move-line", approach, nil); err != nil {
			return nil, fmt.Errorf("pick: failed to move to approach pose: %w", err)
		}
		down := map[string]any{"x": 200.0, "y": -100.0, "z": 20.0, "a": 180.0, "b": 0.0, "c": 0.0, "wait": true}
		if err := c.callBridge(ctx, http.MethodPost, "/move-line", down, nil); err != nil {
			return nil, fmt.Errorf("pick: failed to move down to slot: %w", err)
		}
		if err := c.callBridge(ctx, http.MethodPost, "/gripper", map[string]bool{"open": false}, nil); err != nil {
			return nil, fmt.Errorf("pick: failed to close gripper: %w", err)
		}
		if err := c.callBridge(ctx, http.MethodPost, "/move-line", approach, nil); err != nil {
			return nil, fmt.Errorf("pick: failed to lift up: %w", err)
		}
	case "place_at_known_slot":
		approach := map[string]any{"x": 200.0, "y": 100.0, "z": 80.0, "a": 180.0, "b": 0.0, "c": 0.0, "wait": true}
		if err := c.callBridge(ctx, http.MethodPost, "/move-line", approach, nil); err != nil {
			return nil, fmt.Errorf("place: failed to move to approach pose: %w", err)
		}
		down := map[string]any{"x": 200.0, "y": 100.0, "z": 20.0, "a": 180.0, "b": 0.0, "c": 0.0, "wait": true}
		if err := c.callBridge(ctx, http.MethodPost, "/move-line", down, nil); err != nil {
			return nil, fmt.Errorf("place: failed to move down to slot: %w", err)
		}
		if err := c.callBridge(ctx, http.MethodPost, "/gripper", map[string]bool{"open": true}, nil); err != nil {
			return nil, fmt.Errorf("place: failed to open gripper: %w", err)
		}
		if err := c.callBridge(ctx, http.MethodPost, "/move-line", approach, nil); err != nil {
			return nil, fmt.Errorf("place: failed to lift up: %w", err)
		}
	}

	status := fmt.Sprintf("live: successfully executed skill %q", skillName)
	return &robot.ExecuteSkillResults{Status: status}, nil
}

func (c *LiveController) MoveToSafePose(ctx context.Context, pose robot.MovePoseArgs) (*robot.MovePoseResults, error) {
	if pose.PoseName != "" {
		var skillName string
		switch pose.PoseName {
		case "home":
			skillName = "home"
		case "observation":
			skillName = "move_to_observation_pose"
		default:
			return nil, fmt.Errorf("unknown named pose %q", pose.PoseName)
		}
		res, err := c.ExecuteNamedSkill(ctx, skillName)
		if err != nil {
			return nil, err
		}
		return &robot.MovePoseResults{Status: res.Status}, nil
	}

	if pose.X == nil || pose.Y == nil || pose.Z == nil {
		return nil, fmt.Errorf("must specify either a named pose or all coordinate axes (x, y, z)")
	}

	if !c.allowRawCartesian {
		return nil, fmt.Errorf("Live raw Cartesian movement is disabled. Use a named pose or named skill, or explicitly set ADK_ALLOW_RAW_CARTESIAN=true after calibration.")
	}

	if err := c.ensureLiveReady(ctx, "move_to_safe_pose"); err != nil {
		return nil, err
	}

	x, y, z := *pose.X, *pose.Y, *pose.Z
	if err := robot.ValidatePose(x, y, z); err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	reqPose := map[string]any{
		"x":         x,
		"y":         y,
		"z":         z,
		"a":         180.0,
		"b":         0.0,
		"c":         0.0,
		"wait":      true,
		"timeoutMs": 20000,
	}

	err := c.callBridge(ctx, http.MethodPost, "/move-line", reqPose, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to execute Cartesian move: %w", err)
	}

	status := fmt.Sprintf("live: successfully moved to Cartesian (%.1f, %.1f, %.1f)", x, y, z)
	return &robot.MovePoseResults{Status: status}, nil
}

func (c *LiveController) EmergencyStop(ctx context.Context) (*robot.ExecuteSkillResults, error) {
	err := c.callBridge(ctx, http.MethodPost, "/stop", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to trigger emergency stop: %w", err)
	}
	return &robot.ExecuteSkillResults{Status: "Emergency stop successfully triggered."}, nil
}

// DryRunController executes simulated commands without actual motion.
type DryRunController struct {
	mu           sync.Mutex
	tcpPose      []float64
	jointPose    []float64
	gripperState string
}

func NewDryRunController() *DryRunController {
	return &DryRunController{
		tcpPose:      []float64{180.0, 0.0, 120.0, 180.0, 0.0, 0.0},
		jointPose:    []float64{0, 10, -10, 0, 80, 0},
		gripperState: "unknown",
	}
}

func (c *DryRunController) GetState(ctx context.Context) (*robot.RobotState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return &robot.RobotState{
		BridgeConnected:       true,
		RobotConnected:        true,
		IsReady:               true,
		State:                 0,
		ErrorCode:             0,
		TCPPose:               append([]float64(nil), c.tcpPose...),
		JointPose:             append([]float64(nil), c.jointPose...),
		PhysicalMotionAllowed: false,
		GripperState:          c.gripperState,
	}, nil
}

func (c *DryRunController) ListCapabilities(ctx context.Context) (*robot.Capabilities, error) {
	var actions []string
	for k := range robot.AllowedSkills {
		actions = append(actions, k)
	}

	limitsDesc := fmt.Sprintf("Cartesian Limits - X: [%.1f, %.1f] mm, Y: [%.1f, %.1f] mm, Z: [%.1f, %.1f] mm",
		robot.MinX, robot.MaxX, robot.MinY, robot.MaxY, robot.MinZ, robot.MaxZ)

	return &robot.Capabilities{
		SupportedActions: actions,
		WorkspaceLimits:  limitsDesc,
		GripperExists:    true,
		CameraExists:     false,
		MotionMode:       "dry-run",
	}, nil
}

func (c *DryRunController) ExecuteNamedSkill(ctx context.Context, skillName string) (*robot.ExecuteSkillResults, error) {
	if !robot.AllowedSkills[skillName] {
		return nil, fmt.Errorf("skill %q is not in the allowed skills list", skillName)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if strings.Contains(skillName, "open") {
		c.gripperState = "open"
	} else if strings.Contains(skillName, "close") {
		c.gripperState = "closed"
	}

	status := fmt.Sprintf("dry-run: successfully simulated skill %q", skillName)
	return &robot.ExecuteSkillResults{Status: status}, nil
}

func (c *DryRunController) MoveToSafePose(ctx context.Context, pose robot.MovePoseArgs) (*robot.MovePoseResults, error) {
	if pose.PoseName != "" {
		var skillName string
		switch pose.PoseName {
		case "home":
			skillName = "home"
		case "observation":
			skillName = "move_to_observation_pose"
		default:
			return nil, fmt.Errorf("unknown named pose %q", pose.PoseName)
		}
		res, err := c.ExecuteNamedSkill(ctx, skillName)
		if err != nil {
			return nil, err
		}
		return &robot.MovePoseResults{Status: res.Status}, nil
	}

	if pose.X == nil || pose.Y == nil || pose.Z == nil {
		return nil, fmt.Errorf("must specify either a named pose or all coordinate axes (x, y, z)")
	}

	x, y, z := *pose.X, *pose.Y, *pose.Z
	if err := robot.ValidatePose(x, y, z); err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.tcpPose[0] = x
	c.tcpPose[1] = y
	c.tcpPose[2] = z

	status := fmt.Sprintf("dry-run: successfully simulated move to Cartesian (%.1f, %.1f, %.1f)", x, y, z)
	return &robot.MovePoseResults{Status: status}, nil
}

func (c *DryRunController) EmergencyStop(ctx context.Context) (*robot.ExecuteSkillResults, error) {
	return &robot.ExecuteSkillResults{Status: "Emergency stop successfully triggered."}, nil
}
