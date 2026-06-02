package robot

import (
	"context"
	"fmt"
)

// Safety Limits
const (
	MinX = 100.0
	MaxX = 300.0
	MinY = -250.0
	MaxY = 250.0
	MinZ = 20.0
	MaxZ = 250.0
)

// Allowed Skills Allowlist
var AllowedSkills = map[string]bool{
	"home":                     true,
	"open_gripper":             true,
	"close_gripper":            true,
	"move_to_observation_pose": true,
	"pick_from_known_slot":     true,
	"place_at_known_slot":      true,
}

type RobotState struct {
	BridgeConnected       bool      `json:"bridge_connected"`
	RobotConnected        bool      `json:"robot_connected"`
	IsReady               bool      `json:"is_ready"`
	State                 int       `json:"state"`
	ErrorCode             int       `json:"error_code"`
	ErrorMessage          string    `json:"error_message,omitempty"`
	TCPPose               []float64 `json:"tcp_pose,omitempty"`   // [X, Y, Z, Roll, Pitch, Yaw]
	JointPose             []float64 `json:"joint_pose,omitempty"` // [J1, J2, J3, J4, J5, J6]
	PhysicalMotionAllowed bool      `json:"physical_motion_allowed"`
	GripperState          string    `json:"gripper_state"` // "open", "closed", "unknown"
}

type Capabilities struct {
	SupportedActions []string `json:"supported_actions"`
	WorkspaceLimits  string   `json:"workspace_limits"`
	GripperExists    bool     `json:"gripper_exists"`
	CameraExists     bool     `json:"camera_exists"`
	MotionMode       string   `json:"motion_mode"` // "dry-run" or "live" or "sim"
}

type ExecuteSkillArgs struct {
	SkillName string `json:"skill_name"`
}

type ExecuteSkillResults struct {
	Status string `json:"status"`
}

type MovePoseArgs struct {
	PoseName string   `json:"pose_name,omitempty"`
	X        *float64 `json:"x,omitempty"`
	Y        *float64 `json:"y,omitempty"`
	Z        *float64 `json:"z,omitempty"`
}

type MovePoseResults struct {
	Status string `json:"status"`
}

// ArmController is the common interface implemented by all robot backends.
type ArmController interface {
	GetState(ctx context.Context) (*RobotState, error)
	ListCapabilities(ctx context.Context) (*Capabilities, error)
	ExecuteNamedSkill(ctx context.Context, skillName string) (*ExecuteSkillResults, error)
	MoveToSafePose(ctx context.Context, pose MovePoseArgs) (*MovePoseResults, error)
	EmergencyStop(ctx context.Context) (*ExecuteSkillResults, error)
}

// ValidatePose validates coordinates against safe Cartesian bounds
func ValidatePose(x, y, z float64) error {
	if x < MinX || x > MaxX {
		return fmt.Errorf("X coordinate %.1f is out of safe bounds [%.1f, %.1f]", x, MinX, MaxX)
	}
	if y < MinY || y > MaxY {
		return fmt.Errorf("Y coordinate %.1f is out of safe bounds [%.1f, %.1f]", y, MinY, MaxY)
	}
	if z < MinZ || z > MaxZ {
		return fmt.Errorf("Z coordinate %.1f is out of safe bounds [%.1f, %.1f]", z, MinZ, MaxZ)
	}
	return nil
}
