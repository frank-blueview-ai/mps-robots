package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"mps-robots/internal/adkagent"
	"mps-robots/internal/robotsim"

	_ "modernc.org/sqlite"
)

type Config struct {
	Addr                   string
	ProjectRoot            string
	WebRoot                string
	DataRoot               string
	BridgeURL              string
	OraTarget              string
	OraName                string
	OraIP                  string
	AdkEnableMotion        bool
	AdkRequireConfirmation bool
	AdkGeminiModel         string
	GeminiAPIKey           string
	AdkArmMode             string
	AdkAllowRawCartesian   bool
	AdkTestFakeAgent       bool
}

type App struct {
	cfg       Config
	bridgeURL *url.URL
	oraURL    *url.URL
	projects  *ProjectStore
	client    *http.Client
	agentMgr  *adkagent.Manager
	sim       *robotsim.Simulator
}

type Project struct {
	ID              string                 `json:"id"`
	FormatVersion   int                    `json:"formatVersion"`
	Title           string                 `json:"title"`
	Owner           string                 `json:"owner,omitempty"`
	Course          string                 `json:"course,omitempty"`
	Mode            string                 `json:"mode,omitempty"`
	Blockly         json.RawMessage        `json:"blockly,omitempty"`
	Python          string                 `json:"python,omitempty"`
	GeneratedPython string                 `json:"generatedPython,omitempty"`
	SafetyProfileID string                 `json:"safetyProfileId,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt       time.Time              `json:"createdAt"`
	UpdatedAt       time.Time              `json:"updatedAt"`
}

type ProjectSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Owner     string    `json:"owner,omitempty"`
	Course    string    `json:"course,omitempty"`
	Mode      string    `json:"mode,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ProjectStore struct {
	db *sql.DB
}

func main() {
	cfg := loadConfig()

	if err := mime.AddExtensionType(".js", "application/javascript"); err != nil {
		log.Printf("warning: could not register JavaScript MIME type: %v", err)
	}

	app, err := NewApp(cfg)
	if err != nil {
		log.Fatal(err)
	}

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("ORA Go server listening on http://%s", cfg.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("server shutdown failed: %v", err)
	}

	if err := app.projects.Close(); err != nil {
		log.Printf("database close failed: %v", err)
	}
}

func loadConfig() Config {
	workingDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	dotEnv := readDotEnv(filepath.Join(workingDir, ".env"))
	lookup := func(key string) string {
		if value := os.Getenv(key); value != "" {
			return value
		}
		return dotEnv[key]
	}

	adkEnableStr := lookup("ADK_ARM_ENABLE_MOTION")
	adkConfirmStr := lookup("ADK_REQUIRE_CONFIRMATION")
	adkArmModeStr := lookup("ADK_ARM_MODE")
	if adkArmModeStr == "" {
		adkArmModeStr = "dry_run"
	}
	adkAllowRawStr := lookup("ADK_ALLOW_RAW_CARTESIAN")
	adkFakeAgentStr := lookup("ADK_TEST_FAKE_AGENT")

	cfg := Config{
		Addr:                   firstNonEmpty(lookup("ORA_SERVER_ADDR"), "127.0.0.1:8081"),
		ProjectRoot:            firstNonEmpty(lookup("ORA_PROJECT_ROOT"), workingDir),
		BridgeURL:              firstNonEmpty(lookup("ORA_BRIDGE_URL"), "http://127.0.0.1:8787"),
		OraIP:                  firstNonEmpty(lookup("ORA_IP"), "10.1.48.113"),
		OraName:                lookup("ORA_NAME"),
		AdkEnableMotion:        strings.ToLower(adkEnableStr) == "true" || adkEnableStr == "1",
		AdkRequireConfirmation: adkConfirmStr == "" || strings.ToLower(adkConfirmStr) == "true" || adkConfirmStr == "1",
		AdkGeminiModel:         firstNonEmpty(lookup("ADK_GEMINI_MODEL"), "gemini-2.5-flash"),
		GeminiAPIKey:           firstNonEmpty(lookup("GEMINI_API_KEY"), lookup("GOOGLE_API_KEY")),
		AdkArmMode:             adkArmModeStr,
		AdkAllowRawCartesian:   strings.ToLower(adkAllowRawStr) == "true" || adkAllowRawStr == "1",
		AdkTestFakeAgent:       strings.ToLower(adkFakeAgentStr) == "true" || adkFakeAgentStr == "1",
	}

	if cfg.AdkTestFakeAgent && cfg.GeminiAPIKey == "" {
		cfg.GeminiAPIKey = "fake_key"
	}

	cfg.WebRoot = firstNonEmpty(lookup("ORA_WEB_ROOT"), defaultWebRoot(cfg.ProjectRoot))
	cfg.DataRoot = firstNonEmpty(lookup("ORA_DATA_ROOT"), filepath.Join(cfg.ProjectRoot, "runtime", "data"))
	cfg.OraTarget = firstNonEmpty(lookup("ORA_HTTP_TARGET"), "http://"+cfg.OraIP)

	flag.StringVar(&cfg.Addr, "addr", cfg.Addr, "HTTP listen address")
	flag.StringVar(&cfg.ProjectRoot, "project-root", cfg.ProjectRoot, "project root")
	flag.StringVar(&cfg.WebRoot, "web-root", cfg.WebRoot, "web asset root")
	flag.StringVar(&cfg.DataRoot, "data-root", cfg.DataRoot, "local data root")
	flag.StringVar(&cfg.BridgeURL, "bridge-url", cfg.BridgeURL, "ORA bridge target URL")
	flag.StringVar(&cfg.OraTarget, "ora-target", cfg.OraTarget, "direct ORA diagnostic target URL")
	flag.StringVar(&cfg.OraName, "ora-name", cfg.OraName, "ORA device name")
	flag.StringVar(&cfg.OraIP, "ora-ip", cfg.OraIP, "ORA device IP")
	flag.BoolVar(&cfg.AdkEnableMotion, "adk-enable-motion", cfg.AdkEnableMotion, "Enable live ADK agent motion commands")
	flag.BoolVar(&cfg.AdkRequireConfirmation, "adk-require-confirm", cfg.AdkRequireConfirmation, "Require confirmation for physical ADK motions")
	flag.StringVar(&cfg.AdkGeminiModel, "adk-gemini-model", cfg.AdkGeminiModel, "Gemini model name for ADK agent")
	flag.StringVar(&cfg.AdkArmMode, "adk-arm-mode", cfg.AdkArmMode, "Arm operation mode (dry_run, sim, live)")
	flag.BoolVar(&cfg.AdkAllowRawCartesian, "adk-allow-raw-cartesian", cfg.AdkAllowRawCartesian, "Allow raw Cartesian movements in live mode")
	flag.BoolVar(&cfg.AdkTestFakeAgent, "adk-test-fake-agent", cfg.AdkTestFakeAgent, "Enable deterministic fake agent mode for testing")
	flag.Parse()

	return cfg
}

func NewApp(cfg Config) (*App, error) {
	bridgeURL, err := url.Parse(cfg.BridgeURL)
	if err != nil {
		return nil, fmt.Errorf("invalid bridge URL: %w", err)
	}

	oraURL, err := url.Parse(cfg.OraTarget)
	if err != nil {
		return nil, fmt.Errorf("invalid ORA target URL: %w", err)
	}

	if err := os.MkdirAll(cfg.DataRoot, 0755); err != nil {
		return nil, err
	}

	dbPath := filepath.Join(cfg.DataRoot, "ora.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	projects := &ProjectStore{db: db}
	if err := projects.Init(); err != nil {
		db.Close()
		return nil, err
	}
	if err := projects.ImportLegacyProjectFiles(filepath.Join(cfg.DataRoot, "projects")); err != nil {
		db.Close()
		return nil, err
	}

	sim := robotsim.NewSimulator()

	var agentMgr *adkagent.Manager
	if cfg.GeminiAPIKey != "" {
		agentMgr, err = adkagent.NewManager(adkagent.Config{
			GeminiModel:         cfg.AdkGeminiModel,
			EnableMotion:        cfg.AdkEnableMotion,
			RequireConfirmation: cfg.AdkRequireConfirmation,
			BridgeURL:           cfg.BridgeURL,
			GeminiAPIKey:        cfg.GeminiAPIKey,
			ArmMode:             cfg.AdkArmMode,
			Simulator:           sim,
			AllowRawCartesian:   cfg.AdkAllowRawCartesian,
		})
		if err != nil {
			log.Printf("warning: failed to initialize ADK Agent Manager: %v", err)
		} else {
			log.Printf("ADK Agent Manager initialized with model %q (motion=%v, confirm=%v)",
				cfg.AdkGeminiModel, cfg.AdkEnableMotion, cfg.AdkRequireConfirmation)
		}
	} else {
		log.Printf("warning: GEMINI_API_KEY is not configured; ADK agent endpoints will return configuration errors")
	}

	return &App{
		cfg:       cfg,
		bridgeURL: bridgeURL,
		oraURL:    oraURL,
		projects:  projects,
		client:    &http.Client{Timeout: 8 * time.Second},
		agentMgr:  agentMgr,
		sim:       sim,
	}, nil
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", a.health)
	mux.HandleFunc("/api/station", a.station)
	mux.HandleFunc("/api/telemetry", a.telemetry)
	mux.HandleFunc("/api/users", a.usersHandler)
	mux.HandleFunc("/api/users/", a.userHandler)
	mux.HandleFunc("/api/classes", a.classesHandler)
	mux.HandleFunc("/api/classes/", a.classHandler)
	mux.HandleFunc("/api/projects", a.projectsHandler)
	mux.HandleFunc("/api/projects/", a.projectHandler)
	mux.HandleFunc("/bridge", withCORS(a.reverseProxy(a.bridgeURL, "/bridge")))
	mux.HandleFunc("/bridge/", withCORS(a.reverseProxy(a.bridgeURL, "/bridge")))
	mux.HandleFunc("/ora", withCORS(a.reverseProxy(a.oraURL, "/ora")))
	mux.HandleFunc("/ora/", withCORS(a.reverseProxy(a.oraURL, "/ora")))
	mux.HandleFunc("/Ora-move.js", a.rootJavaScript)

	// ADK Agent Endpoints
	mux.HandleFunc("/api/agent/chat", withCORS(a.agentChat))
	mux.HandleFunc("/api/agent/confirm", withCORS(a.agentConfirm))
	mux.HandleFunc("/api/agent/state", withCORS(a.agentState))

	// 3D Simulator / Digital Twin Endpoints
	mux.HandleFunc("/sim/view", withCORS(a.simView))
	mux.HandleFunc("/api/sim/state", withCORS(a.simState))
	mux.HandleFunc("/api/sim/reset", withCORS(a.simReset))
	mux.HandleFunc("/api/sim/scenarios/run", withCORS(a.simRunScenario))
	mux.HandleFunc("/api/sim/scenarios/run-agent", withCORS(a.simRunAgentScenario))
	mux.HandleFunc("/sim/vendor/", withCORS(a.simVendor))

	mux.HandleFunc("/", a.static)
	return requestLog(mux)
}

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"server":    "ora-go-server",
		"bridgeUrl": a.cfg.BridgeURL,
		"storage":   "sqlite",
		"oraName":   a.cfg.OraName,
		"oraIp":     a.cfg.OraIP,
		"time":      time.Now().UTC(),
	})
}

func (a *App) station(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"oraName":    a.cfg.OraName,
		"oraIp":      a.cfg.OraIP,
		"bridgeBase": "/bridge",
		"features": []string{
			"static-controller",
			"bridge-proxy",
			"sqlite-project-storage",
			"classroom-admin",
			"telemetry-stream",
			"react-typescript-ui",
		},
	})
}

func (a *App) telemetry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming is not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		a.writeTelemetryEvent(w, flusher, r.Context())

		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *App) writeTelemetryEvent(w io.Writer, flusher http.Flusher, ctx context.Context) {
	payload := a.fetchBridgeStatus(ctx)
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(`{"connected":false,"error":"telemetry marshal failed"}`)
	}

	fmt.Fprintf(w, "event: status\n")
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

func (a *App) fetchBridgeStatus(ctx context.Context) interface{} {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(a.cfg.BridgeURL, "/")+"/status", nil)
	if err != nil {
		return bridgeOffline(err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return bridgeOffline(err)
	}
	defer resp.Body.Close()

	var body interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return bridgeOffline(err)
	}

	return map[string]interface{}{
		"bridgeStatus": resp.StatusCode,
		"payload":      body,
		"at":           time.Now().UTC(),
	}
}

func bridgeOffline(err error) map[string]interface{} {
	return map[string]interface{}{
		"bridgeStatus": 0,
		"payload": map[string]interface{}{
			"connected": false,
			"error":     err.Error(),
		},
		"at": time.Now().UTC(),
	}
}

func (a *App) projectsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		projects, err := a.projects.List()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, projects)
	case http.MethodPost:
		project, err := readProject(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		saved, err := a.projects.Create(project)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, saved)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) projectHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/projects/")
	if strings.Contains(id, "/") || !validID(id) {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		project, err := a.projects.Get(id)
		if err != nil {
			status := http.StatusInternalServerError
			if isNotFound(err) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, project)
	case http.MethodPut:
		project, err := readProject(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		saved, err := a.projects.Update(id, project)
		if err != nil {
			status := http.StatusInternalServerError
			if isNotFound(err) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, saved)
	case http.MethodDelete:
		if err := a.projects.Delete(id); err != nil {
			status := http.StatusInternalServerError
			if isNotFound(err) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) reverseProxy(target *url.URL, prefix string) http.HandlerFunc {
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		writeError(w, http.StatusBadGateway, err.Error())
	}

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalPath := req.URL.Path
		originalRawQuery := req.URL.RawQuery
		originalDirector(req)

		trimmed := strings.TrimPrefix(originalPath, prefix)
		if trimmed == "" {
			trimmed = "/"
		}
		if !strings.HasPrefix(trimmed, "/") {
			trimmed = "/" + trimmed
		}

		req.URL.Path = joinURLPath(target.Path, trimmed)
		req.URL.RawPath = ""
		req.URL.RawQuery = originalRawQuery
		req.Host = target.Host
	}

	return func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	}
}

func (a *App) rootJavaScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	http.ServeFile(w, r, filepath.Join(a.cfg.ProjectRoot, "Ora-move.js"))
}

func (a *App) static(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cleanPath := path.Clean("/" + r.URL.Path)
	if cleanPath == "/" {
		http.ServeFile(w, r, filepath.Join(a.cfg.WebRoot, "index.html"))
		return
	}

	relative := strings.TrimPrefix(cleanPath, "/")
	fullPath := filepath.Join(a.cfg.WebRoot, filepath.FromSlash(relative))
	if !insideDir(a.cfg.WebRoot, fullPath) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	info, err := os.Stat(fullPath)
	if err == nil && !info.IsDir() {
		http.ServeFile(w, r, fullPath)
		return
	}

	http.ServeFile(w, r, filepath.Join(a.cfg.WebRoot, "index.html"))
}

func (s *ProjectStore) Init() error {
	if _, err := s.db.Exec(`
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;

CREATE TABLE IF NOT EXISTS users (
	id TEXT PRIMARY KEY,
	display_name TEXT NOT NULL,
	role TEXT NOT NULL CHECK (role IN ('admin', 'teacher', 'student', 'operator')),
	email TEXT NOT NULL DEFAULT '',
	active INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS classes (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	term TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS class_memberships (
	class_id TEXT NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
	user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	role TEXT NOT NULL CHECK (role IN ('teacher', 'student', 'operator')),
	created_at TEXT NOT NULL,
	PRIMARY KEY (class_id, user_id)
);

CREATE TABLE IF NOT EXISTS safety_profiles (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	config_json TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS projects (
	id TEXT PRIMARY KEY,
	format_version INTEGER NOT NULL,
	title TEXT NOT NULL,
	owner TEXT NOT NULL DEFAULT '',
	course TEXT NOT NULL DEFAULT '',
	mode TEXT NOT NULL DEFAULT 'blocks-and-python',
	blockly_json TEXT NOT NULL DEFAULT '',
	python TEXT NOT NULL DEFAULT '',
	generated_python TEXT NOT NULL DEFAULT '',
	safety_profile_id TEXT NOT NULL DEFAULT 'pilot-default',
	metadata_json TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	FOREIGN KEY (safety_profile_id) REFERENCES safety_profiles(id)
);

CREATE TABLE IF NOT EXISTS project_versions (
	id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	version_number INTEGER NOT NULL,
	snapshot_json TEXT NOT NULL,
	created_at TEXT NOT NULL,
	UNIQUE (project_id, version_number)
);

CREATE TABLE IF NOT EXISTS submissions (
	id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	submitted_by TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL CHECK (status IN ('draft', 'submitted', 'reviewed', 'returned', 'approved')),
	notes TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS approvals (
	id TEXT PRIMARY KEY,
	submission_id TEXT NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
	approved_by TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL CHECK (status IN ('approved', 'rejected', 'revoked')),
	notes TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS robot_runs (
	id TEXT PRIMARY KEY,
	project_id TEXT REFERENCES projects(id) ON DELETE SET NULL,
	requested_by TEXT NOT NULL DEFAULT '',
	approved_by TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'completed', 'failed', 'stopped', 'cancelled')),
	command_plan_json TEXT NOT NULL DEFAULT '{}',
	result_json TEXT NOT NULL DEFAULT '{}',
	started_at TEXT,
	ended_at TEXT,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_events (
	id TEXT PRIMARY KEY,
	actor_id TEXT NOT NULL DEFAULT '',
	event_type TEXT NOT NULL,
	target_type TEXT NOT NULL DEFAULT '',
	target_id TEXT NOT NULL DEFAULT '',
	details_json TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_projects_updated_at ON projects(updated_at);
CREATE INDEX IF NOT EXISTS idx_projects_owner ON projects(owner);
CREATE INDEX IF NOT EXISTS idx_audit_events_created_at ON audit_events(created_at);
`); err != nil {
		return err
	}

	now := formatDBTime(time.Now().UTC())
	if _, err := s.db.Exec(`
INSERT OR IGNORE INTO users (id, display_name, role, active, created_at, updated_at)
VALUES ('station-admin', 'Station Admin', 'admin', 1, ?, ?);

INSERT OR IGNORE INTO safety_profiles (id, name, description, config_json, created_at, updated_at)
VALUES (
	'pilot-default',
	'Pilot Default',
	'Low-risk default limits for the first ORA classroom pilot.',
	'{"maxSpeedPercent":20,"maxRunSeconds":60,"teacherApprovalRequired":true}',
	?,
	?
);
`, now, now, now, now); err != nil {
		return err
	}

	return nil
}

func (s *ProjectStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *ProjectStore) ImportLegacyProjectFiles(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		file, err := os.Open(filepath.Join(root, entry.Name()))
		if err != nil {
			return err
		}

		var project Project
		decodeErr := json.NewDecoder(file).Decode(&project)
		closeErr := file.Close()
		if decodeErr != nil {
			return decodeErr
		}
		if closeErr != nil {
			return closeErr
		}

		if !validID(project.ID) {
			project.ID = strings.TrimSuffix(entry.Name(), ".json")
		}
		if !validID(project.ID) {
			project.ID = newProjectID()
		}

		if _, err := s.Get(project.ID); err == nil {
			continue
		} else if !isNotFound(err) {
			return err
		}

		now := time.Now().UTC()
		project.FormatVersion = defaultInt(project.FormatVersion, 1)
		project.Title = firstNonEmpty(strings.TrimSpace(project.Title), "Untitled Project")
		project.Mode = firstNonEmpty(project.Mode, "blocks-and-python")
		project.SafetyProfileID = firstNonEmpty(project.SafetyProfileID, "pilot-default")
		if project.CreatedAt.IsZero() {
			project.CreatedAt = now
		}
		if project.UpdatedAt.IsZero() {
			project.UpdatedAt = now
		}

		if err := s.insertProject(project); err != nil {
			return err
		}
		if err := s.addProjectVersion(project); err != nil {
			return err
		}
		if err := s.audit("project.migrated", "project", project.ID, map[string]string{"source": "json-file"}); err != nil {
			return err
		}
	}

	return nil
}

func (s *ProjectStore) List() ([]ProjectSummary, error) {
	rows, err := s.db.Query(`
SELECT id, title, owner, course, mode, created_at, updated_at
FROM projects
ORDER BY updated_at DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	projects := []ProjectSummary{}
	for rows.Next() {
		var project ProjectSummary
		var createdAt string
		var updatedAt string

		if err := rows.Scan(&project.ID, &project.Title, &project.Owner, &project.Course, &project.Mode, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		project.CreatedAt = parseDBTime(createdAt)
		project.UpdatedAt = parseDBTime(updatedAt)
		projects = append(projects, project)
	}

	return projects, rows.Err()
}

func (s *ProjectStore) Create(project Project) (Project, error) {
	now := time.Now().UTC()
	project.ID = newProjectID()
	project.FormatVersion = defaultInt(project.FormatVersion, 1)
	project.Title = firstNonEmpty(strings.TrimSpace(project.Title), "Untitled Project")
	project.Mode = firstNonEmpty(project.Mode, "blocks-and-python")
	project.SafetyProfileID = firstNonEmpty(project.SafetyProfileID, "pilot-default")
	project.CreatedAt = now
	project.UpdatedAt = now

	if err := s.insertProject(project); err != nil {
		return Project{}, err
	}
	if err := s.addProjectVersion(project); err != nil {
		return Project{}, err
	}
	if err := s.audit("project.created", "project", project.ID, map[string]string{"title": project.Title}); err != nil {
		return Project{}, err
	}

	return project, nil
}

func (s *ProjectStore) Get(id string) (Project, error) {
	return s.read(id)
}

func (s *ProjectStore) Update(id string, project Project) (Project, error) {
	existing, err := s.read(id)
	if err != nil {
		return Project{}, err
	}

	project.ID = id
	project.FormatVersion = defaultInt(project.FormatVersion, existing.FormatVersion)
	project.Title = firstNonEmpty(strings.TrimSpace(project.Title), existing.Title)
	project.CreatedAt = existing.CreatedAt
	project.UpdatedAt = time.Now().UTC()

	if err := s.updateProject(project); err != nil {
		return Project{}, err
	}
	if err := s.addProjectVersion(project); err != nil {
		return Project{}, err
	}
	if err := s.audit("project.updated", "project", project.ID, map[string]string{"title": project.Title}); err != nil {
		return Project{}, err
	}

	return project, nil
}

func (s *ProjectStore) Delete(id string) error {
	result, err := s.db.Exec(`DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return os.ErrNotExist
	}

	return s.audit("project.deleted", "project", id, map[string]string{})
}

func (s *ProjectStore) read(id string) (Project, error) {
	var project Project
	var blocklyJSON string
	var metadataJSON string
	var createdAt string
	var updatedAt string

	err := s.db.QueryRow(`
SELECT id, format_version, title, owner, course, mode, blockly_json, python,
	generated_python, safety_profile_id, metadata_json, created_at, updated_at
FROM projects
WHERE id = ?
`, id).Scan(
		&project.ID,
		&project.FormatVersion,
		&project.Title,
		&project.Owner,
		&project.Course,
		&project.Mode,
		&blocklyJSON,
		&project.Python,
		&project.GeneratedPython,
		&project.SafetyProfileID,
		&metadataJSON,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return Project{}, err
	}

	project.Blockly = rawJSONOrEmpty(blocklyJSON)
	project.Metadata = mapFromJSON(metadataJSON)
	project.CreatedAt = parseDBTime(createdAt)
	project.UpdatedAt = parseDBTime(updatedAt)

	return project, nil
}

func (s *ProjectStore) insertProject(project Project) error {
	_, err := s.db.Exec(`
INSERT INTO projects (
	id, format_version, title, owner, course, mode, blockly_json, python,
	generated_python, safety_profile_id, metadata_json, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`,
		project.ID,
		project.FormatVersion,
		project.Title,
		project.Owner,
		project.Course,
		firstNonEmpty(project.Mode, "blocks-and-python"),
		string(project.Blockly),
		project.Python,
		project.GeneratedPython,
		firstNonEmpty(project.SafetyProfileID, "pilot-default"),
		jsonFromMap(project.Metadata),
		formatDBTime(project.CreatedAt),
		formatDBTime(project.UpdatedAt),
	)
	return err
}

func (s *ProjectStore) updateProject(project Project) error {
	_, err := s.db.Exec(`
UPDATE projects
SET format_version = ?, title = ?, owner = ?, course = ?, mode = ?, blockly_json = ?,
	python = ?, generated_python = ?, safety_profile_id = ?, metadata_json = ?, updated_at = ?
WHERE id = ?
`,
		project.FormatVersion,
		project.Title,
		project.Owner,
		project.Course,
		firstNonEmpty(project.Mode, "blocks-and-python"),
		string(project.Blockly),
		project.Python,
		project.GeneratedPython,
		firstNonEmpty(project.SafetyProfileID, "pilot-default"),
		jsonFromMap(project.Metadata),
		formatDBTime(project.UpdatedAt),
		project.ID,
	)
	return err
}

func (s *ProjectStore) addProjectVersion(project Project) error {
	version, err := s.nextProjectVersion(project.ID)
	if err != nil {
		return err
	}

	snapshot, err := json.Marshal(project)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
INSERT INTO project_versions (id, project_id, version_number, snapshot_json, created_at)
VALUES (?, ?, ?, ?, ?)
`, newProjectID(), project.ID, version, string(snapshot), formatDBTime(time.Now().UTC()))
	return err
}

func (s *ProjectStore) nextProjectVersion(projectID string) (int, error) {
	var version int
	err := s.db.QueryRow(`SELECT COALESCE(MAX(version_number), 0) + 1 FROM project_versions WHERE project_id = ?`, projectID).Scan(&version)
	return version, err
}

func (s *ProjectStore) audit(eventType string, targetType string, targetID string, details map[string]string) error {
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
INSERT INTO audit_events (id, event_type, target_type, target_id, details_json, created_at)
VALUES (?, ?, ?, ?, ?, ?)
`, newProjectID(), eventType, targetType, targetID, string(detailsJSON), formatDBTime(time.Now().UTC()))
	return err
}

func readProject(reader io.Reader) (Project, error) {
	var project Project
	limited := io.LimitReader(reader, 2*1024*1024)
	if err := json.NewDecoder(limited).Decode(&project); err != nil {
		return Project{}, err
	}
	return project, nil
}

func isNotFound(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, sql.ErrNoRows)
}

func rawJSONOrEmpty(value string) json.RawMessage {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return json.RawMessage(value)
}

func mapFromJSON(value string) map[string]interface{} {
	result := map[string]interface{}{}
	if strings.TrimSpace(value) == "" {
		return result
	}
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		return map[string]interface{}{}
	}
	return result
}

func jsonFromMap(value map[string]interface{}) string {
	if value == nil {
		return "{}"
	}

	payload, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(payload)
}

func formatDBTime(value time.Time) string {
	if value.IsZero() {
		value = time.Now().UTC()
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseDBTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func readDotEnv(path string) map[string]string {
	values := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return values
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key != "" {
			values[key] = value
		}
	}

	return values
}

func defaultWebRoot(projectRoot string) string {
	reactDist := filepath.Join(projectRoot, "frontend", "dist")
	if _, err := os.Stat(filepath.Join(reactDist, "index.html")); err == nil {
		return reactDist
	}
	return filepath.Join(projectRoot, "web")
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	payload, err := json.Marshal(body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "response marshal failed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next(w, r)
	}
}

func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func joinURLPath(basePath, requestPath string) string {
	if basePath == "" || basePath == "/" {
		return requestPath
	}
	return strings.TrimRight(basePath, "/") + "/" + strings.TrimLeft(requestPath, "/")
}

func insideDir(root, child string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	childAbs, err := filepath.Abs(child)
	if err != nil {
		return false
	}

	relative, err := filepath.Rel(rootAbs, childAbs)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func validID(id string) bool {
	if len(id) == 0 || len(id) > 80 {
		return false
	}
	for _, char := range id {
		if char >= 'a' && char <= 'z' {
			continue
		}
		if char >= 'A' && char <= 'Z' {
			continue
		}
		if char >= '0' && char <= '9' {
			continue
		}
		if char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func newProjectID() string {
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		return "p_" + time.Now().UTC().Format("20060102_150405")
	}
	return "p_" + time.Now().UTC().Format("20060102_150405") + "_" + hex.EncodeToString(random)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func defaultInt(value int, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func (a *App) agentChat(w http.ResponseWriter, r *http.Request) {
	if a.agentMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "ADK agent is not configured (missing GEMINI_API_KEY)")
		return
	}
	a.agentMgr.ChatHandler(w, r)
}

func (a *App) agentConfirm(w http.ResponseWriter, r *http.Request) {
	if a.agentMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "ADK agent is not configured (missing GEMINI_API_KEY)")
		return
	}
	a.agentMgr.ConfirmHandler(w, r)
}

func (a *App) agentState(w http.ResponseWriter, r *http.Request) {
	if a.agentMgr == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"enableMotion":        a.cfg.AdkEnableMotion,
			"requireConfirmation": a.cfg.AdkRequireConfirmation,
			"geminiModel":         a.cfg.AdkGeminiModel,
			"error":               "ADK agent is not configured (missing GEMINI_API_KEY)",
		})
		return
	}
	a.agentMgr.StateHandler(w, r)
}

func (a *App) simView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	http.ServeFile(w, r, filepath.Join(a.cfg.ProjectRoot, "web", "sim.html"))
}

func (a *App) simState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	state, err := a.sim.GetState(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	_, _, _, _, _, _, objects := a.sim.GetRawState()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"mode":              a.cfg.AdkArmMode,
		"isReady":           state.IsReady,
		"errorCode":         state.ErrorCode,
		"errorMessage":      state.ErrorMessage,
		"tcpPose":           state.TCPPose,
		"jointPose":         state.JointPose,
		"gripperState":      state.GripperState,
		"objects":           objects,
		"slots":             a.sim.Slots,
		"allowRawCartesian": a.cfg.AdkAllowRawCartesian,
	})
}

func (a *App) simReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	a.sim.Reset()
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset success"})
}

type RunScenarioRequest struct {
	Scenario string `json:"scenario"`
}

func (a *App) simRunScenario(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req RunScenarioRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.Scenario == "" {
		writeError(w, http.StatusBadRequest, "scenario filename is required")
		return
	}

	scenarioPath := filepath.Join(a.cfg.ProjectRoot, "scenarios", filepath.Clean(req.Scenario))
	sc, err := robotsim.LoadScenario(scenarioPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to load scenario: "+err.Error())
		return
	}

	err = robotsim.RunScenario(r.Context(), sc, a.sim)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "passed"})
}

func (a *App) simRunAgentScenario(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if a.agentMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "ADK agent is not configured")
		return
	}

	var req RunScenarioRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.Scenario == "" {
		writeError(w, http.StatusBadRequest, "scenario filename is required")
		return
	}

	scenarioPath := filepath.Join(a.cfg.ProjectRoot, "scenarios", filepath.Clean(req.Scenario))
	sc, err := robotsim.LoadScenario(scenarioPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to load scenario: "+err.Error())
		return
	}

	res, err := adkagent.RunAgentScenario(r.Context(), sc, a.agentMgr, a.sim)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "scenario run error: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, res)
}

func (a *App) simVendor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	file := strings.TrimPrefix(r.URL.Path, "/sim/vendor/")
	fullPath := filepath.Join(a.cfg.ProjectRoot, "web", "vendor", filepath.Clean(file))
	http.ServeFile(w, r, fullPath)
}
