package adkagent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"

	"mps-robots/internal/robot"
	"mps-robots/internal/robotsim"
)

type Config struct {
	GeminiModel         string
	EnableMotion        bool
	RequireConfirmation bool
	BridgeURL           string
	GeminiAPIKey        string
	ArmMode             string              // "dry_run", "sim", "live"
	Simulator           robot.ArmController // Shared simulator reference if instantiated externally
	AllowRawCartesian   bool
}

type PendingConfirm struct {
	SessionID          string         `json:"sessionId"`
	ConfirmationCallID string         `json:"confirmationCallId"`
	ToolName           string         `json:"toolName"`
	ToolArgs           map[string]any `json:"toolArgs"`
	RiskLevel          string         `json:"riskLevel"`
	ExpiresAt          time.Time      `json:"expiresAt"`
	Consumed           bool           `json:"consumed"`
}

type Manager struct {
	cfg             Config
	runner          *runner.Runner
	sessionService  session.Service
	client          *http.Client
	mu              sync.Mutex
	pendingConfirms map[string]*PendingConfirm
	controller      robot.ArmController
	fakeAgentStates map[string]int
	fakeAgentTasks  map[string]string
}

func (m *Manager) Controller() robot.ArmController {
	return m.controller
}

func NewManager(cfg Config) (*Manager, error) {
	if cfg.GeminiAPIKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY is not set")
	}
	if cfg.GeminiModel == "" {
		cfg.GeminiModel = "gemini-2.5-flash"
	}
	if cfg.BridgeURL == "" {
		cfg.BridgeURL = "http://127.0.0.1:8787"
	}
	if cfg.ArmMode == "" {
		cfg.ArmMode = "dry_run"
	}

	// Mode validation
	var ctrl robot.ArmController
	switch cfg.ArmMode {
	case "sim":
		if cfg.Simulator != nil {
			ctrl = cfg.Simulator
		} else {
			ctrl = robotsim.NewSimulator()
		}
	case "live":
		if !cfg.EnableMotion {
			return nil, fmt.Errorf("live mode is rejected unless motion is explicitly enabled via ADK_ARM_ENABLE_MOTION")
		}
		ctrl = NewLiveController(cfg.BridgeURL, cfg.AllowRawCartesian)
	case "dry_run":
		ctrl = NewDryRunController()
	default:
		return nil, fmt.Errorf("invalid ADK_ARM_MODE: %q", cfg.ArmMode)
	}

	sessionService := session.InMemoryService()
	m := &Manager{
		cfg:             cfg,
		sessionService:  sessionService,
		client:          &http.Client{Timeout: 30 * time.Second},
		pendingConfirms: make(map[string]*PendingConfirm),
		controller:      ctrl,
		fakeAgentStates: make(map[string]int),
		fakeAgentTasks:  make(map[string]string),
	}

	// 1. Initialize Gemini Model
	ctx := context.Background()
	model, err := gemini.NewModel(ctx, cfg.GeminiModel, &genai.ClientConfig{
		APIKey: cfg.GeminiAPIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize gemini model: %w", err)
	}

	// 2. Define tools
	getRobotStateTool, err := functiontool.New(functiontool.Config{
		Name:        "get_robot_state",
		Description: "Returns current robot status, pose/joints if available, gripper state, motion state, error state, enabled/disabled state, and whether physical motion is currently allowed.",
	}, m.getRobotStateTool)
	if err != nil {
		return nil, fmt.Errorf("failed to create get_robot_state tool: %w", err)
	}

	listRobotCapabilitiesTool, err := functiontool.New(functiontool.Config{
		Name:        "list_robot_capabilities",
		Description: "Returns the actions this build supports, workspace limits, whether gripper exists, whether camera/vision exists, and whether motion is dry-run, sim, or live.",
	}, m.listRobotCapabilitiesTool)
	if err != nil {
		return nil, fmt.Errorf("failed to create list_robot_capabilities tool: %w", err)
	}

	planRobotActionTool, err := functiontool.New(functiontool.Config{
		Name:        "plan_robot_action",
		Description: "Takes a natural-language task and returns a structured, non-executing plan. This must never move the robot.",
	}, m.planRobotActionTool)
	if err != nil {
		return nil, fmt.Errorf("failed to create plan_robot_action tool: %w", err)
	}

	executeNamedSkillTool, err := functiontool.New(functiontool.Config{
		Name:        "execute_named_skill",
		Description: "Executes only pre-existing named safe skills from an allowlist. Allowlist: home, open_gripper, close_gripper, move_to_observation_pose, pick_from_known_slot, place_at_known_slot. Reject unknown skills.",
		RequireConfirmationProvider: func(args robot.ExecuteSkillArgs) bool {
			return cfg.RequireConfirmation
		},
	}, m.executeNamedSkillTool)
	if err != nil {
		return nil, fmt.Errorf("failed to create execute_named_skill tool: %w", err)
	}

	emergencyStopTool, err := functiontool.New(functiontool.Config{
		Name:        "emergency_stop",
		Description: "Calls the existing emergency-stop / halt / disable motion path immediately. Bypasses confirmation.",
	}, m.emergencyStopTool)
	if err != nil {
		return nil, fmt.Errorf("failed to create emergency_stop tool: %w", err)
	}

	moveToSafePoseTool, err := functiontool.New(functiontool.Config{
		Name:        "move_to_safe_pose",
		Description: "Accepts a named pose ('home', 'observation') or validated coordinates X, Y, Z. Validates coordinates against safe Cartesian bounds before moving.",
		RequireConfirmationProvider: func(args robot.MovePoseArgs) bool {
			return cfg.RequireConfirmation
		},
	}, m.moveToSafePoseTool)
	if err != nil {
		return nil, fmt.Errorf("failed to create move_to_safe_pose tool: %w", err)
	}

	// 3. Create llmagent
	instructions := "You are a cautious robot-arm assistant. You may only operate the arm through the provided tools. Prefer get_robot_state and list_robot_capabilities before proposing action. For any physical action, explain the intended motion, ask for confirmation, and call only safe allowlisted tools. Never invent coordinates, robot capabilities, calibration data, or safety limits. If the request is ambiguous or unsafe, refuse the motion and explain what information is missing."
	a, err := llmagent.New(llmagent.Config{
		Name:        "ora_robot_agent",
		Model:       model,
		Description: "Cautious assistant for the Ozobot ORA arm.",
		Instruction: instructions,
		Tools: []tool.Tool{
			getRobotStateTool,
			listRobotCapabilitiesTool,
			planRobotActionTool,
			executeNamedSkillTool,
			emergencyStopTool,
			moveToSafePoseTool,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create llm agent: %w", err)
	}

	// 4. Create runner
	r, err := runner.New(runner.Config{
		AppName:        "mps-robots",
		Agent:          a,
		SessionService: sessionService,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create agent runner: %w", err)
	}

	m.runner = r
	return m, nil
}

// REST Interface Request/Response DTOs

type ChatRequest struct {
	Message   string `json:"message"`
	SessionID string `json:"sessionId,omitempty"`
}

type ChatResponse struct {
	SessionID            string           `json:"sessionId"`
	AssistantText        string           `json:"assistantText"`
	ToolCalls            []ToolCallDetail `json:"toolCalls"`
	ConfirmationRequired bool             `json:"confirmationRequired"`
	ConfirmationCallID   string           `json:"confirmationCallId,omitempty"`
	ToolName             string           `json:"toolName,omitempty"`
	ToolArgs             map[string]any   `json:"toolArgs,omitempty"`
	RiskLevel            string           `json:"riskLevel,omitempty"`
	ExpiresAt            string           `json:"expiresAt,omitempty"`
	DryRun               bool             `json:"dryRun"`
	Error                string           `json:"error,omitempty"`
}

type ToolCallDetail struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type ConfirmRequest struct {
	SessionID          string `json:"sessionId"`
	ConfirmationCallID string `json:"confirmationCallId"`
	Confirmed          bool   `json:"confirmed"`
}

type ConfirmResponse struct {
	SessionID     string           `json:"sessionId"`
	AssistantText string           `json:"assistantText"`
	ToolCalls     []ToolCallDetail `json:"toolCalls"`
	Error         string           `json:"error,omitempty"`
}

type StateResponse struct {
	EnableMotion        bool   `json:"enableMotion"`
	RequireConfirmation bool   `json:"requireConfirmation"`
	GeminiModel         string `json:"geminiModel"`
	ArmMode             string `json:"armMode"`
	AllowRawCartesian   bool   `json:"allowRawCartesian"`
}

// Handlers

func (m *Manager) ChatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = generateSessionID()
	}

	ctx := r.Context()
	_, err := m.sessionService.Get(ctx, &session.GetRequest{SessionID: sessionID})
	if err != nil {
		_, err = m.sessionService.Create(ctx, &session.CreateRequest{AppName: "mps-robots", UserID: "user", SessionID: sessionID})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create session: "+err.Error())
			return
		}
	}

	userMessage := genai.NewContentFromText(req.Message, genai.RoleUser)
	m.runTurnAndResponse(w, ctx, sessionID, userMessage)
}

func (m *Manager) ConfirmHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req ConfirmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.SessionID == "" {
		writeError(w, http.StatusBadRequest, "sessionId is required")
		return
	}
	if req.ConfirmationCallID == "" {
		writeError(w, http.StatusBadRequest, "confirmationCallId is required")
		return
	}

	resp, err := m.ConfirmInternal(r.Context(), req.SessionID, req.ConfirmationCallID, req.Confirmed)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (m *Manager) StateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, StateResponse{
		EnableMotion:        m.cfg.EnableMotion,
		RequireConfirmation: m.cfg.RequireConfirmation,
		GeminiModel:         m.cfg.GeminiModel,
		ArmMode:             m.cfg.ArmMode,
		AllowRawCartesian:   m.cfg.AllowRawCartesian,
	})
}

// Shared runner execution logic
type fakeToolContext struct {
	agent.ToolContext
	ctx context.Context
}

func (f *fakeToolContext) Deadline() (deadline time.Time, ok bool) { return f.ctx.Deadline() }
func (f *fakeToolContext) Done() <-chan struct{}                   { return f.ctx.Done() }
func (f *fakeToolContext) Err() error                              { return f.ctx.Err() }
func (f *fakeToolContext) Value(key any) any                       { return f.ctx.Value(key) }

func (m *Manager) runFakeAgentTurn(ctx context.Context, sessionID string, content *genai.Content) (ChatResponse, error) {
	m.mu.Lock()
	if m.fakeAgentStates == nil {
		m.fakeAgentStates = make(map[string]int)
	}
	if m.fakeAgentTasks == nil {
		m.fakeAgentTasks = make(map[string]string)
	}
	task := m.fakeAgentTasks[sessionID]
	m.mu.Unlock()

	var isConfirmResponse bool
	var confirmed bool
	var confirmCallID string

	if content != nil {
		for _, part := range content.Parts {
			if part.Text != "" {
				txt := strings.ToLower(part.Text)
				if strings.Contains(txt, "pick") || strings.Contains(txt, "place") || strings.Contains(txt, "slot") {
					task = "pick_place"
				} else if strings.Contains(txt, "gripper") {
					task = "gripper"
				} else if strings.Contains(txt, "bounds") || strings.Contains(txt, "x=50") {
					task = "unsafe"
				} else if strings.Contains(txt, "home") || strings.Contains(txt, "recover") {
					task = "home"
				}
				m.mu.Lock()
				m.fakeAgentTasks[sessionID] = task
				m.fakeAgentStates[sessionID] = 0
				m.mu.Unlock()
			}
			if fr := part.FunctionResponse; fr != nil {
				if fr.Name == toolconfirmation.FunctionCallName {
					isConfirmResponse = true
					confirmCallID = fr.ID
					if fr.Response != nil {
						confirmed, _ = fr.Response["confirmed"].(bool)
					}
				}
			}
		}
	}

	tctx := &fakeToolContext{ctx: ctx}
	resp := ChatResponse{
		SessionID: sessionID,
		DryRun:    m.cfg.ArmMode == "dry_run",
	}

	for {
		m.mu.Lock()
		curState := m.fakeAgentStates[sessionID]
		m.mu.Unlock()

		var executed bool
		var execErr error

		executeOrConfirm := func(toolName string, argsMap map[string]any, executeFn func() (string, error)) {
			if m.cfg.RequireConfirmation {
				m.mu.Lock()
				pending, ok := m.pendingConfirms[sessionID+"_"+confirmCallID]
				m.mu.Unlock()

				isConfirmForThisTool := isConfirmResponse && ok && pending.ToolName == toolName

				if !isConfirmForThisTool {
					callID := "fc_" + generateSessionID()
					risk := "high"
					expires := time.Now().Add(2 * time.Minute)

					m.mu.Lock()
					pending := &PendingConfirm{
						SessionID:          sessionID,
						ConfirmationCallID: callID,
						ToolName:           toolName,
						ToolArgs:           argsMap,
						RiskLevel:          risk,
						ExpiresAt:          expires,
						Consumed:           false,
					}
					m.pendingConfirms[sessionID+"_"+callID] = pending
					m.mu.Unlock()

					resp.ConfirmationRequired = true
					resp.ConfirmationCallID = callID
					resp.ToolName = toolName
					resp.ToolArgs = argsMap
					resp.RiskLevel = risk
					resp.ExpiresAt = expires.Format(time.RFC3339)
					resp.ToolCalls = []ToolCallDetail{{Name: toolName, Args: argsMap}}
					return
				} else {
					if confirmCallID != "" {
						m.mu.Lock()
						key := sessionID + "_" + confirmCallID
						if pending, ok := m.pendingConfirms[key]; ok {
							pending.Consumed = true
						}
						m.mu.Unlock()
					}
					isConfirmResponse = false // Consume it!
					if !confirmed {
						resp.AssistantText = "Confirmation denied. Motion aborted."
						resp.ConfirmationRequired = false
						execErr = fmt.Errorf("confirmation denied")
						return
					}
				}
			}

			status, err := executeFn()
			if err != nil {
				execErr = err
			} else {
				resp.AssistantText = status
			}

			m.mu.Lock()
			m.fakeAgentStates[sessionID]++
			m.mu.Unlock()
			executed = true
		}

		switch task {
		case "pick_place":
			if curState == 0 {
				args := map[string]any{"skill_name": "pick_from_known_slot"}
				executeOrConfirm("execute_named_skill", args, func() (string, error) {
					res, err := m.executeNamedSkillTool(tctx, robot.ExecuteSkillArgs{SkillName: "pick_from_known_slot"})
					if err != nil {
						return "", err
					}
					return res.Status, nil
				})
			} else if curState == 1 {
				args := map[string]any{"skill_name": "place_at_known_slot"}
				executeOrConfirm("execute_named_skill", args, func() (string, error) {
					res, err := m.executeNamedSkillTool(tctx, robot.ExecuteSkillArgs{SkillName: "place_at_known_slot"})
					if err != nil {
						return "", err
					}
					return res.Status, nil
				})
			} else {
				resp.AssistantText = "Done"
				return resp, nil
			}

		case "gripper":
			if curState == 0 {
				args := map[string]any{"skill_name": "close_gripper"}
				executeOrConfirm("execute_named_skill", args, func() (string, error) {
					res, err := m.executeNamedSkillTool(tctx, robot.ExecuteSkillArgs{SkillName: "close_gripper"})
					if err != nil {
						return "", err
					}
					return res.Status, nil
				})
			} else if curState == 1 {
				args := map[string]any{"skill_name": "open_gripper"}
				executeOrConfirm("execute_named_skill", args, func() (string, error) {
					res, err := m.executeNamedSkillTool(tctx, robot.ExecuteSkillArgs{SkillName: "open_gripper"})
					if err != nil {
						return "", err
					}
					return res.Status, nil
				})
			} else {
				resp.AssistantText = "Done"
				return resp, nil
			}

		case "unsafe":
			if curState == 0 {
				x, y, z := 50.0, 0.0, 100.0
				args := map[string]any{"x": x, "y": y, "z": z}
				executeOrConfirm("move_to_safe_pose", args, func() (string, error) {
					res, err := m.moveToSafePoseTool(tctx, robot.MovePoseArgs{X: &x, Y: &y, Z: &z})
					if err != nil {
						return "", err
					}
					return res.Status, nil
				})
			} else if curState == 1 {
				x, y, z := 200.0, 300.0, 100.0
				args := map[string]any{"x": x, "y": y, "z": z}
				executeOrConfirm("move_to_safe_pose", args, func() (string, error) {
					res, err := m.moveToSafePoseTool(tctx, robot.MovePoseArgs{X: &x, Y: &y, Z: &z})
					if err != nil {
						return "", err
					}
					return res.Status, nil
				})
			} else {
				resp.AssistantText = "Done"
				return resp, nil
			}

		case "home":
			if curState == 0 {
				args := map[string]any{"skill_name": "home"}
				executeOrConfirm("execute_named_skill", args, func() (string, error) {
					res, err := m.executeNamedSkillTool(tctx, robot.ExecuteSkillArgs{SkillName: "home"})
					if err != nil {
						return "", err
					}
					return res.Status, nil
				})
			} else if curState == 1 {
				args := map[string]any{"skill_name": "home"}
				executeOrConfirm("execute_named_skill", args, func() (string, error) {
					res, err := m.executeNamedSkillTool(tctx, robot.ExecuteSkillArgs{SkillName: "home"})
					if err != nil {
						return "", err
					}
					return res.Status, nil
				})
			} else {
				resp.AssistantText = "Done"
				return resp, nil
			}
		default:
			return resp, fmt.Errorf("unknown fake agent task")
		}

		if resp.ConfirmationRequired {
			return resp, nil
		}

		if execErr != nil {
			resp.Error = execErr.Error()
			return resp, nil
		}

		if !executed {
			return resp, nil
		}
	}
}

func (m *Manager) RunTurnInternal(ctx context.Context, sessionID string, content *genai.Content) (ChatResponse, error) {
	if m.cfg.GeminiAPIKey == "fake_key" || os.Getenv("ADK_TEST_FAKE_AGENT") == "true" {
		return m.runFakeAgentTurn(ctx, sessionID, content)
	}

	var assistantText string
	var toolCalls []ToolCallDetail
	var confirmationRequired bool
	var firstErr error
	var lastConfirm *PendingConfirm

	for event, err := range m.runner.Run(ctx, "user", sessionID, content, agent.RunConfig{
		StreamingMode: agent.StreamingModeNone,
	}) {
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		if event.Content != nil {
			for _, part := range event.Content.Parts {
				if part.Text != "" {
					assistantText += part.Text
				}
				if fc := part.FunctionCall; fc != nil {
					argsMap := make(map[string]any)
					if fc.Args != nil {
						argsMap = fc.Args
					}

					if fc.Name == toolconfirmation.FunctionCallName {
						confirmationRequired = true
						originalCall, err := toolconfirmation.OriginalCallFrom(fc)
						if err == nil && originalCall != nil {
							risk := "low"
							if originalCall.Name == "execute_named_skill" || originalCall.Name == "move_to_safe_pose" {
								risk = "high"
							}
							expires := time.Now().Add(2 * time.Minute)

							m.mu.Lock()
							key := sessionID + "_" + fc.ID
							pending := &PendingConfirm{
								SessionID:          sessionID,
								ConfirmationCallID: fc.ID,
								ToolName:           originalCall.Name,
								ToolArgs:           originalCall.Args,
								RiskLevel:          risk,
								ExpiresAt:          expires,
								Consumed:           false,
							}
							m.pendingConfirms[key] = pending
							m.mu.Unlock()

							lastConfirm = pending

							toolCalls = append(toolCalls, ToolCallDetail{
								Name: originalCall.Name,
								Args: originalCall.Args,
							})
						}
					} else {
						toolCalls = append(toolCalls, ToolCallDetail{
							Name: fc.Name,
							Args: argsMap,
						})
					}
				}
			}
		}
	}

	if firstErr != nil && len(toolCalls) == 0 && assistantText == "" {
		return ChatResponse{}, fmt.Errorf("agent run error: %w", firstErr)
	}

	resp := ChatResponse{
		SessionID:            sessionID,
		AssistantText:        assistantText,
		ToolCalls:            toolCalls,
		ConfirmationRequired: confirmationRequired,
		DryRun:               m.cfg.ArmMode == "dry_run",
	}

	if confirmationRequired && lastConfirm != nil {
		resp.ConfirmationCallID = lastConfirm.ConfirmationCallID
		resp.ToolName = lastConfirm.ToolName
		resp.ToolArgs = lastConfirm.ToolArgs
		resp.RiskLevel = lastConfirm.RiskLevel
		resp.ExpiresAt = lastConfirm.ExpiresAt.Format(time.RFC3339)
	}

	return resp, nil
}

func (m *Manager) ConfirmInternal(ctx context.Context, sessionID string, confirmationCallID string, confirmed bool) (ChatResponse, error) {
	m.mu.Lock()
	key := sessionID + "_" + confirmationCallID
	pending, ok := m.pendingConfirms[key]
	if ok {
		if pending.Consumed {
			m.mu.Unlock()
			return ChatResponse{}, fmt.Errorf("confirmation was already consumed")
		}
		if time.Now().After(pending.ExpiresAt) {
			m.mu.Unlock()
			return ChatResponse{}, fmt.Errorf("pending confirmation expired")
		}
		// Consumed status is updated in executeOrConfirm for fake agent, but we can also set it here
		pending.Consumed = true
	}
	m.mu.Unlock()

	if !ok {
		return ChatResponse{}, fmt.Errorf("no matching pending confirmation found")
	}

	funcResponse := &genai.FunctionResponse{
		Name: toolconfirmation.FunctionCallName,
		ID:   pending.ConfirmationCallID,
		Response: map[string]any{
			"confirmed": confirmed,
			"payload":   nil,
		},
	}

	appResponse := &genai.Content{
		Role:  string(genai.RoleUser),
		Parts: []*genai.Part{{FunctionResponse: funcResponse}},
	}

	return m.RunTurnInternal(ctx, sessionID, appResponse)
}

func (m *Manager) runTurnAndResponse(w http.ResponseWriter, ctx context.Context, sessionID string, content *genai.Content) {
	resp, err := m.RunTurnInternal(ctx, sessionID, content)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func generateSessionID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("s_%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	payload, err := json.Marshal(body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"response marshal failed"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
