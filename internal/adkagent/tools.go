package adkagent

import (
	"fmt"
	"log"
	"strings"

	"google.golang.org/adk/agent"
	"mps-robots/internal/robot"
)

type EmptyArgs struct{}

type PlanActionArgs struct {
	Task string `json:"task"`
}

type PlannedToolStep struct {
	Tool            string `json:"tool"`
	SkillName       string `json:"skillName,omitempty"`
	PoseName        string `json:"poseName,omitempty"`
	Description     string `json:"description"`
	MotionAffecting bool   `json:"motionAffecting"`
}

type PlanActionResults struct {
	Allowed              bool              `json:"allowed"`
	Mode                 string            `json:"mode"`
	RequiresConfirmation bool              `json:"requiresConfirmation"`
	Task                 string            `json:"task"`
	CurrentState         string            `json:"currentState"`
	Capabilities         string            `json:"capabilities"`
	Preconditions        []string          `json:"preconditions"`
	ProposedSteps        []PlannedToolStep `json:"proposedSteps"`
	RejectedReasons      []string          `json:"rejectedReasons"`
	SafetyNotes          []string          `json:"safetyNotes"`
}

// getRobotStateTool implementation
func (m *Manager) getRobotStateTool(ctx agent.ToolContext, args EmptyArgs) (*robot.RobotState, error) {
	log.Printf("[AUDIT] [TOOL] get_robot_state | mode=%s", m.cfg.ArmMode)
	res, err := m.controller.GetState(ctx)
	log.Printf("[AUDIT] [TOOL] get_robot_state finished | error=%v", err)
	return res, err
}

// listRobotCapabilitiesTool implementation
func (m *Manager) listRobotCapabilitiesTool(ctx agent.ToolContext, args EmptyArgs) (*robot.Capabilities, error) {
	log.Printf("[AUDIT] [TOOL] list_robot_capabilities | mode=%s", m.cfg.ArmMode)
	res, err := m.controller.ListCapabilities(ctx)
	log.Printf("[AUDIT] [TOOL] list_robot_capabilities finished | error=%v", err)
	return res, err
}

// planRobotActionTool implementation
func (m *Manager) planRobotActionTool(ctx agent.ToolContext, args PlanActionArgs) (*PlanActionResults, error) {
	log.Printf("[AUDIT] [TOOL] plan_robot_action | task=%q | mode=%s", args.Task, m.cfg.ArmMode)

	state, err := m.controller.GetState(ctx)
	stateStr := "Unknown"
	if err == nil {
		stateStr = fmt.Sprintf("Ready: %t, Gripper: %s, ErrorCode: %d, TCP: %v", state.IsReady, state.GripperState, state.ErrorCode, state.TCPPose)
	}

	caps, err := m.controller.ListCapabilities(ctx)
	capsStr := "Unknown"
	modeStr := "unknown"
	if err == nil {
		capsStr = fmt.Sprintf("Actions: %v, Limits: %s", caps.SupportedActions, caps.WorkspaceLimits)
		modeStr = caps.MotionMode
	}

	res := &PlanActionResults{
		Allowed:              true,
		Mode:                 modeStr,
		RequiresConfirmation: m.cfg.RequireConfirmation,
		Task:                 args.Task,
		CurrentState:         stateStr,
		Capabilities:         capsStr,
		Preconditions:        []string{"Robot must be ready and error-free"},
		SafetyNotes:          []string{"Verify clear workspace area before confirming physical motions"},
	}

	task := strings.ToLower(args.Task)
	if task == "" {
		res.Allowed = false
		res.RejectedReasons = []string{"Task description is empty"}
		log.Printf("[AUDIT] [TOOL] plan_robot_action finished | allowed=%t", res.Allowed)
		return res, nil
	}

	// Unsafe keywords check
	if strings.Contains(task, "break") || strings.Contains(task, "smash") || strings.Contains(task, "crash") || strings.Contains(task, "explode") {
		res.Allowed = false
		res.RejectedReasons = []string{"Task involves potentially destructive or unsafe words"}
		log.Printf("[AUDIT] [TOOL] plan_robot_action finished | allowed=%t", res.Allowed)
		return res, nil
	}

	// Pick & place logic
	if strings.Contains(task, "pick") || strings.Contains(task, "place") || strings.Contains(task, "slot") {
		res.ProposedSteps = []PlannedToolStep{
			{
				Tool:            "execute_named_skill",
				SkillName:       "pick_from_known_slot",
				Description:     "Pick the object from slot 1",
				MotionAffecting: true,
			},
			{
				Tool:            "execute_named_skill",
				SkillName:       "place_at_known_slot",
				Description:     "Place the object at slot 2",
				MotionAffecting: true,
			},
		}
	} else if strings.Contains(task, "home") {
		res.ProposedSteps = []PlannedToolStep{
			{
				Tool:            "execute_named_skill",
				SkillName:       "home",
				Description:     "Home the arm to default pose",
				MotionAffecting: true,
			},
		}
	} else if strings.Contains(task, "open") && strings.Contains(task, "gripper") {
		res.ProposedSteps = []PlannedToolStep{
			{
				Tool:            "execute_named_skill",
				SkillName:       "open_gripper",
				Description:     "Open the gripper fingers",
				MotionAffecting: true,
			},
		}
	} else if strings.Contains(task, "close") && strings.Contains(task, "gripper") {
		res.ProposedSteps = []PlannedToolStep{
			{
				Tool:            "execute_named_skill",
				SkillName:       "close_gripper",
				Description:     "Close the gripper fingers",
				MotionAffecting: true,
			},
		}
	} else if strings.Contains(task, "move") || strings.Contains(task, "go to") {
		if modeStr == "live" && !m.cfg.AllowRawCartesian {
			res.Allowed = false
			res.RejectedReasons = []string{"Live raw Cartesian movement is disabled. Use a named pose or named skill, or explicitly set ADK_ALLOW_RAW_CARTESIAN=true after calibration."}
		} else {
			res.ProposedSteps = []PlannedToolStep{
				{
					Tool:            "move_to_safe_pose",
					Description:     "Move to safety-checked Cartesian coordinates",
					MotionAffecting: true,
				},
			}
		}
	} else {
		res.Allowed = false
		res.RejectedReasons = []string{"Unsupported or unrecognized task request"}
	}

	log.Printf("[AUDIT] [TOOL] plan_robot_action finished | allowed=%t", res.Allowed)
	return res, nil
}

// executeNamedSkillTool implementation
func (m *Manager) executeNamedSkillTool(ctx agent.ToolContext, args robot.ExecuteSkillArgs) (*robot.ExecuteSkillResults, error) {
	log.Printf("[AUDIT] [TOOL] execute_named_skill | skill=%s | confirm=%t | mode=%s", args.SkillName, m.cfg.RequireConfirmation, m.cfg.ArmMode)
	res, err := m.controller.ExecuteNamedSkill(ctx, args.SkillName)
	log.Printf("[AUDIT] [TOOL] execute_named_skill finished | error=%v", err)
	return res, err
}

// emergencyStopTool implementation
func (m *Manager) emergencyStopTool(ctx agent.ToolContext, args EmptyArgs) (*robot.ExecuteSkillResults, error) {
	log.Printf("[AUDIT] [TOOL] emergency_stop | mode=%s", m.cfg.ArmMode)
	res, err := m.controller.EmergencyStop(ctx)
	log.Printf("[AUDIT] [TOOL] emergency_stop finished | error=%v", err)
	return res, err
}

// moveToSafePoseTool implementation
func (m *Manager) moveToSafePoseTool(ctx agent.ToolContext, args robot.MovePoseArgs) (*robot.MovePoseResults, error) {
	log.Printf("[AUDIT] [TOOL] move_to_safe_pose | name=%s | X=%v | Y=%v | Z=%v | confirm=%t | mode=%s", args.PoseName, args.X, args.Y, args.Z, m.cfg.RequireConfirmation, m.cfg.ArmMode)
	res, err := m.controller.MoveToSafePose(ctx, args)
	log.Printf("[AUDIT] [TOOL] move_to_safe_pose finished | error=%v", err)
	return res, err
}
