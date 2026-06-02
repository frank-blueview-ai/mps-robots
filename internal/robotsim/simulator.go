package robotsim

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"mps-robots/internal/robot"
)

type SimObject struct {
	Name     string     `json:"name"`
	Color    string     `json:"color"`
	Shape    string     `json:"shape"` // "cube", "cylinder"
	Position [3]float64 `json:"position"`
	Size     [3]float64 `json:"size"`
	Grasped  bool       `json:"grasped"`
}

type SimSlot struct {
	Name     string     `json:"name"`
	Position [3]float64 `json:"position"`
}

type Simulator struct {
	mu           sync.Mutex
	tcpPose      []float64 // [X, Y, Z, Roll, Pitch, Yaw]
	jointPose    []float64 // [J1, J2, J3, J4, J5, J6]
	gripperState string    // "open", "closed", "unknown"
	errorCode    int
	errorMessage string
	isReady      bool

	Objects   []SimObject `json:"objects"`
	Slots     []SimSlot   `json:"slots"`
	StepDelay time.Duration
	Speed     float64 // mm per second
}

func NewSimulator() *Simulator {
	sim := &Simulator{
		tcpPose:      []float64{180.0, 0.0, 120.0, 180.0, 0.0, 0.0},
		jointPose:    make([]float64, 6),
		gripperState: "open",
		isReady:      true,
		StepDelay:    50 * time.Millisecond,
		Speed:        150.0, // 150 mm/s
	}

	// Initialize slots
	sim.Slots = []SimSlot{
		{Name: "slot1", Position: [3]float64{200.0, -100.0, 20.0}},
		{Name: "slot2", Position: [3]float64{200.0, 100.0, 20.0}},
	}

	// Initialize default objects
	sim.Objects = []SimObject{
		{
			Name:     "red_cube",
			Color:    "#ef4444",
			Shape:    "cube",
			Position: [3]float64{200.0, -100.0, 15.0}, // Z=15 represents center of 30mm cube resting on slot Z=20
			Size:     [3]float64{30.0, 30.0, 30.0},
			Grasped:  false,
		},
	}

	sim.updatePosition(180.0, 0.0, 120.0)
	return sim
}

// Reset resets the simulator state to default values.
func (s *Simulator) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tcpPose = []float64{180.0, 0.0, 120.0, 180.0, 0.0, 0.0}
	s.gripperState = "open"
	s.errorCode = 0
	s.errorMessage = ""
	s.isReady = true

	s.Objects = []SimObject{
		{
			Name:     "red_cube",
			Color:    "#ef4444",
			Shape:    "cube",
			Position: [3]float64{200.0, -100.0, 15.0},
			Size:     [3]float64{30.0, 30.0, 30.0},
			Grasped:  false,
		},
	}
	s.updatePosition(180.0, 0.0, 120.0)
}

func (s *Simulator) GetRawState() (x, y, z float64, gripper string, errCode int, errMsg string, objects []SimObject) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tcpPose[0], s.tcpPose[1], s.tcpPose[2], s.gripperState, s.errorCode, s.errorMessage, append([]SimObject(nil), s.Objects...)
}

// GetState implements robot.ArmController.
func (s *Simulator) GetState(ctx context.Context) (*robot.RobotState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return &robot.RobotState{
		BridgeConnected:       true,
		RobotConnected:        true,
		IsReady:               s.isReady,
		State:                 0,
		ErrorCode:             s.errorCode,
		ErrorMessage:          s.errorMessage,
		TCPPose:               append([]float64(nil), s.tcpPose...),
		JointPose:             append([]float64(nil), s.jointPose...),
		PhysicalMotionAllowed: s.errorCode == 0,
		GripperState:          s.gripperState,
	}, nil
}

// ListCapabilities implements robot.ArmController.
func (s *Simulator) ListCapabilities(ctx context.Context) (*robot.Capabilities, error) {
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
		MotionMode:       "sim",
	}, nil
}

// ExecuteNamedSkill implements robot.ArmController.
func (s *Simulator) ExecuteNamedSkill(ctx context.Context, skillName string) (*robot.ExecuteSkillResults, error) {
	s.mu.Lock()
	if !robot.AllowedSkills[skillName] {
		s.mu.Unlock()
		return nil, fmt.Errorf("skill %q is not in the allowed skills list", skillName)
	}
	if s.errorCode != 0 {
		s.mu.Unlock()
		return nil, fmt.Errorf("robot is in error state: %s", s.errorMessage)
	}
	s.mu.Unlock()

	switch skillName {
	case "home":
		if err := s.interpolateTo(ctx, 180.0, 0.0, 120.0); err != nil {
			return nil, err
		}
	case "move_to_observation_pose":
		if err := s.interpolateTo(ctx, 180.0, 0.0, 120.0); err != nil {
			return nil, err
		}
	case "open_gripper":
		s.mu.Lock()
		s.gripperState = "open"
		for i := range s.Objects {
			if s.Objects[i].Grasped {
				s.Objects[i].Grasped = false
				// Drop it
				slotFound := false
				for _, slot := range s.Slots {
					sX, sY := slot.Position[0], slot.Position[1]
					oX, oY := s.Objects[i].Position[0], s.Objects[i].Position[1]
					dist := math.Sqrt((sX-oX)*(sX-oX) + (sY-oY)*(sY-oY))
					if dist < 30.0 {
						s.Objects[i].Position = [3]float64{sX, sY, slot.Position[2]}
						slotFound = true
						break
					}
				}
				if !slotFound {
					s.Objects[i].Position[2] = 15.0 // Table Z center
				}
			}
		}
		s.mu.Unlock()
	case "close_gripper":
		s.mu.Lock()
		s.gripperState = "closed"
		tcpX, tcpY, tcpZ := s.tcpPose[0], s.tcpPose[1], s.tcpPose[2]
		for i := range s.Objects {
			oX, oY, oZ := s.Objects[i].Position[0], s.Objects[i].Position[1], s.Objects[i].Position[2]
			dist := math.Sqrt((tcpX-oX)*(tcpX-oX) + (tcpY-oY)*(tcpY-oY) + (tcpZ-oZ)*(tcpZ-oZ))
			if dist < 35.0 { // tolerance to grasp
				s.Objects[i].Grasped = true
				s.Objects[i].Position = [3]float64{tcpX, tcpY, tcpZ - 10.0}
				break
			}
		}
		s.mu.Unlock()
	case "pick_from_known_slot":
		// 1. Open gripper
		_, _ = s.ExecuteNamedSkill(ctx, "open_gripper")
		// 2. Move to approach pose of slot 1
		if err := s.interpolateTo(ctx, 200.0, -100.0, 80.0); err != nil {
			return nil, err
		}
		// 3. Move down to slot 1
		if err := s.interpolateTo(ctx, 200.0, -100.0, 20.0); err != nil {
			return nil, err
		}
		// 4. Close gripper
		_, _ = s.ExecuteNamedSkill(ctx, "close_gripper")
		// 5. Lift up
		if err := s.interpolateTo(ctx, 200.0, -100.0, 80.0); err != nil {
			return nil, err
		}
	case "place_at_known_slot":
		// 1. Move to approach pose of slot 2
		if err := s.interpolateTo(ctx, 200.0, 100.0, 80.0); err != nil {
			return nil, err
		}
		// 2. Move down to slot 2
		if err := s.interpolateTo(ctx, 200.0, 100.0, 20.0); err != nil {
			return nil, err
		}
		// 3. Open gripper
		_, _ = s.ExecuteNamedSkill(ctx, "open_gripper")
		// 4. Lift up
		if err := s.interpolateTo(ctx, 200.0, 100.0, 80.0); err != nil {
			return nil, err
		}
	}

	return &robot.ExecuteSkillResults{
		Status: fmt.Sprintf("sim: successfully executed skill %q", skillName),
	}, nil
}

// MoveToSafePose implements robot.ArmController.
func (s *Simulator) MoveToSafePose(ctx context.Context, pose robot.MovePoseArgs) (*robot.MovePoseResults, error) {
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
		res, err := s.ExecuteNamedSkill(ctx, skillName)
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
		s.mu.Lock()
		s.errorCode = 101
		s.errorMessage = fmt.Sprintf("Safety limit violated: %v", err)
		s.mu.Unlock()
		return nil, err
	}

	if err := s.interpolateTo(ctx, x, y, z); err != nil {
		return nil, err
	}

	return &robot.MovePoseResults{
		Status: fmt.Sprintf("sim: successfully moved to Cartesian (%.1f, %.1f, %.1f)", x, y, z),
	}, nil
}

// EmergencyStop implements robot.ArmController.
func (s *Simulator) EmergencyStop(ctx context.Context) (*robot.ExecuteSkillResults, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.errorCode = 99
	s.errorMessage = "Emergency Stop Triggered"
	s.isReady = false

	return &robot.ExecuteSkillResults{
		Status: "Emergency stop successfully triggered.",
	}, nil
}

func (s *Simulator) interpolateTo(ctx context.Context, targetX, targetY, targetZ float64) error {
	s.mu.Lock()
	if s.errorCode != 0 {
		s.mu.Unlock()
		return fmt.Errorf("robot is in error state: %s", s.errorMessage)
	}
	startX, startY, startZ := s.tcpPose[0], s.tcpPose[1], s.tcpPose[2]
	s.mu.Unlock()

	dx := targetX - startX
	dy := targetY - startY
	dz := targetZ - startZ
	distance := math.Sqrt(dx*dx + dy*dy + dz*dz)

	if distance == 0 {
		return nil
	}

	stepDistance := s.Speed * (float64(s.StepDelay) / float64(time.Second))
	if stepDistance <= 0 || s.StepDelay == 0 {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.errorCode != 0 {
			return fmt.Errorf("robot is in error state: %s", s.errorMessage)
		}
		if err := robot.ValidatePose(targetX, targetY, targetZ); err != nil {
			s.errorCode = 101
			s.errorMessage = fmt.Sprintf("Safety limit violated during motion: %v", err)
			return err
		}
		s.updatePosition(targetX, targetY, targetZ)
		return nil
	}

	steps := int(distance / stepDistance)
	if steps < 1 {
		steps = 1
	}

	for i := 1; i <= steps; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		s.mu.Lock()
		if s.errorCode != 0 {
			s.mu.Unlock()
			return fmt.Errorf("movement aborted: robot entered error state")
		}
		t := float64(i) / float64(steps)
		curX := startX + dx*t
		curY := startY + dy*t
		curZ := startZ + dz*t

		if err := robot.ValidatePose(curX, curY, curZ); err != nil {
			s.errorCode = 101
			s.errorMessage = fmt.Sprintf("Safety limit violated during motion: %v", err)
			s.mu.Unlock()
			return err
		}

		s.updatePosition(curX, curY, curZ)
		s.mu.Unlock()

		time.Sleep(s.StepDelay)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.errorCode != 0 {
		return fmt.Errorf("movement aborted: robot entered error state")
	}
	s.updatePosition(targetX, targetY, targetZ)
	return nil
}

func (s *Simulator) updatePosition(x, y, z float64) {
	s.tcpPose[0] = x
	s.tcpPose[1] = y
	s.tcpPose[2] = z

	s.jointPose[0] = math.Atan2(y, x) * 180.0 / math.Pi
	s.jointPose[1] = (z - 100.0) / 2.0
	s.jointPose[2] = -s.jointPose[1] + 10.0
	s.jointPose[3] = 0.0
	s.jointPose[4] = 90.0 - s.jointPose[1] - s.jointPose[2]
	s.jointPose[5] = 0.0

	for i := range s.Objects {
		if s.Objects[i].Grasped {
			s.Objects[i].Position = [3]float64{x, y, z - 10.0}
		}
	}
}

// SetInitialState sets the simulator state to a specific initial configuration.
func (s *Simulator) SetInitialState(pose []float64, gripper string, errCode int, errMsg string, objects []SimObject, slots []SimSlot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(pose) >= 6 {
		s.tcpPose = append([]float64(nil), pose...)
	} else {
		s.tcpPose = []float64{180.0, 0.0, 120.0, 180.0, 0.0, 0.0}
	}
	s.gripperState = gripper
	if s.gripperState == "" {
		s.gripperState = "open"
	}
	s.errorCode = errCode
	s.errorMessage = errMsg
	s.isReady = (s.errorCode == 0)

	if objects != nil {
		s.Objects = append([]SimObject(nil), objects...)
	} else {
		s.Objects = nil
	}

	if slots != nil {
		s.Slots = append([]SimSlot(nil), slots...)
	} else {
		s.Slots = []SimSlot{
			{Name: "slot1", Position: [3]float64{200.0, -100.0, 20.0}},
			{Name: "slot2", Position: [3]float64{200.0, 100.0, 20.0}},
		}
	}
	s.updatePosition(s.tcpPose[0], s.tcpPose[1], s.tcpPose[2])
}
