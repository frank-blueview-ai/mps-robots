package adkagent

import (
	"fmt"
	"log"

	"google.golang.org/adk/agent"
	"mps-robots/internal/robot"
)

type EmptyArgs struct{}

type PlanActionArgs struct {
	Task string `json:"task"`
}

type PlanActionResults struct {
	Steps []string `json:"steps"`
}

// getRobotStateTool implementation
func (m *Manager) getRobotStateTool(ctx agent.ToolContext, args EmptyArgs) (*robot.RobotState, error) {
	log.Printf("[ADKAGENT] [TOOL] get_robot_state requested")
	return m.controller.GetState(ctx)
}

// listRobotCapabilitiesTool implementation
func (m *Manager) listRobotCapabilitiesTool(ctx agent.ToolContext, args EmptyArgs) (*robot.Capabilities, error) {
	log.Printf("[ADKAGENT] [TOOL] list_robot_capabilities requested")
	return m.controller.ListCapabilities(ctx)
}

// planRobotActionTool implementation
func (m *Manager) planRobotActionTool(ctx agent.ToolContext, args PlanActionArgs) (*PlanActionResults, error) {
	log.Printf("[ADKAGENT] [TOOL] plan_robot_action requested for task: %q", args.Task)

	steps := []string{
		fmt.Sprintf("Interpret instruction: %s", args.Task),
		"Validate safety limits and collision bounds",
		"Propose physical motion commands to ORA arm controller",
	}

	return &PlanActionResults{
		Steps: steps,
	}, nil
}

// executeNamedSkillTool implementation
func (m *Manager) executeNamedSkillTool(ctx agent.ToolContext, args robot.ExecuteSkillArgs) (*robot.ExecuteSkillResults, error) {
	log.Printf("[ADKAGENT] [TOOL] execute_named_skill requested: %q", args.SkillName)
	return m.controller.ExecuteNamedSkill(ctx, args.SkillName)
}

// emergencyStopTool implementation
func (m *Manager) emergencyStopTool(ctx agent.ToolContext, args EmptyArgs) (*robot.ExecuteSkillResults, error) {
	log.Printf("[ADKAGENT] [TOOL] emergency_stop requested!")
	return m.controller.EmergencyStop(ctx)
}

// moveToSafePoseTool implementation
func (m *Manager) moveToSafePoseTool(ctx agent.ToolContext, args robot.MovePoseArgs) (*robot.MovePoseResults, error) {
	log.Printf("[ADKAGENT] [TOOL] move_to_safe_pose requested: Name=%q, X=%v, Y=%v, Z=%v", args.PoseName, args.X, args.Y, args.Z)
	return m.controller.MoveToSafePose(ctx, args)
}
