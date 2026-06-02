package adkagent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
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
}

type PendingConfirm struct {
	SessionID          string
	ConfirmationCallID string
	ToolName           string
}

type Manager struct {
	cfg             Config
	runner          *runner.Runner
	sessionService  session.Service
	client          *http.Client
	mu              sync.Mutex
	pendingConfirms map[string]*PendingConfirm
	controller      robot.ArmController
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
		ctrl = NewLiveController(cfg.BridgeURL)
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
	DryRun               bool             `json:"dryRun"`
	Error                string           `json:"error,omitempty"`
}

type ToolCallDetail struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type ConfirmRequest struct {
	SessionID string `json:"sessionId"`
	Confirmed bool   `json:"confirmed"`
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

	m.mu.Lock()
	pending, ok := m.pendingConfirms[req.SessionID]
	if ok {
		delete(m.pendingConfirms, req.SessionID)
	}
	m.mu.Unlock()

	if !ok {
		writeError(w, http.StatusBadRequest, "no pending confirmation found for this session")
		return
	}

	funcResponse := &genai.FunctionResponse{
		Name: toolconfirmation.FunctionCallName,
		ID:   pending.ConfirmationCallID,
		Response: map[string]any{
			"confirmed": req.Confirmed,
			"payload":   nil,
		},
	}

	appResponse := &genai.Content{
		Role:  string(genai.RoleUser),
		Parts: []*genai.Part{{FunctionResponse: funcResponse}},
	}

	ctx := r.Context()
	m.runTurnAndResponse(w, ctx, req.SessionID, appResponse)
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
	})
}

// Shared runner execution logic
func (m *Manager) runTurnAndResponse(w http.ResponseWriter, ctx context.Context, sessionID string, content *genai.Content) {
	var assistantText string
	var toolCalls []ToolCallDetail
	var confirmationRequired bool
	var firstErr error

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
							m.mu.Lock()
							m.pendingConfirms[sessionID] = &PendingConfirm{
								SessionID:          sessionID,
								ConfirmationCallID: fc.ID,
								ToolName:           originalCall.Name,
							}
							m.mu.Unlock()
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
		writeError(w, http.StatusInternalServerError, "agent run error: "+firstErr.Error())
		return
	}

	writeJSON(w, http.StatusOK, ChatResponse{
		SessionID:            sessionID,
		AssistantText:        assistantText,
		ToolCalls:            toolCalls,
		ConfirmationRequired: confirmationRequired,
		DryRun:               m.cfg.ArmMode == "dry_run",
	})
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
