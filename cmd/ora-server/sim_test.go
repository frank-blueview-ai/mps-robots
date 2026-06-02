package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func newTestAppWithRealRoot(t *testing.T) *App {
	t.Helper()

	// Since tests run in cmd/ora-server directory, the project root is "../../"
	realRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("failed to resolve project root: %v", err)
	}

	tempDir := t.TempDir()
	dataRoot := filepath.Join(tempDir, "data")

	app, err := NewApp(Config{
		Addr:        "127.0.0.1:0",
		ProjectRoot: realRoot,
		WebRoot:     filepath.Join(realRoot, "web"),
		DataRoot:    dataRoot,
		BridgeURL:   "http://127.0.0.1:8787",
		OraTarget:   "http://127.0.0.1:65535",
		AdkArmMode:  "sim",
	})
	if err != nil {
		t.Fatalf("NewApp failed: %v", err)
	}
	t.Cleanup(func() {
		if err := app.projects.Close(); err != nil {
			t.Fatalf("close test database: %v", err)
		}
	})

	return app
}

func TestSimulatorViewEndpoint(t *testing.T) {
	app := newTestAppWithRealRoot(t)
	server := httptest.NewServer(app.routes())
	defer server.Close()

	res, err := http.Get(server.URL + "/sim/view")
	if err != nil {
		t.Fatalf("GET /sim/view failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", res.StatusCode)
	}

	contentType := res.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("expected text/html content-type, got %q", contentType)
	}
}

func TestSimulatorStateEndpoint(t *testing.T) {
	app := newTestAppWithRealRoot(t)
	server := httptest.NewServer(app.routes())
	defer server.Close()

	res, err := http.Get(server.URL + "/api/sim/state")
	if err != nil {
		t.Fatalf("GET /api/sim/state failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	var state map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&state); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if state["mode"] != "sim" {
		t.Errorf("expected mode 'sim', got %q", state["mode"])
	}
	if state["gripperState"] != "open" {
		t.Errorf("expected gripper state 'open', got %q", state["gripperState"])
	}
	objects, ok := state["objects"].([]interface{})
	if !ok || len(objects) == 0 {
		t.Error("expected objects list to be non-empty")
	}
}

func TestSimulatorResetEndpoint(t *testing.T) {
	app := newTestAppWithRealRoot(t)
	server := httptest.NewServer(app.routes())
	defer server.Close()

	res, err := http.Post(server.URL+"/api/sim/reset", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/sim/reset failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body["status"] != "reset success" {
		t.Errorf("unexpected status: %q", body["status"])
	}
}

func TestSimulatorScenarioEndpoint(t *testing.T) {
	app := newTestAppWithRealRoot(t)
	server := httptest.NewServer(app.routes())
	defer server.Close()

	reqBody := `{"scenario": "gripper_test.yaml"}`
	res, err := http.Post(server.URL+"/api/sim/scenarios/run", "application/json", bytes.NewBufferString(reqBody))
	if err != nil {
		t.Fatalf("POST /api/sim/scenarios/run failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body["status"] != "passed" {
		t.Errorf("expected scenario to pass, got: %+v", body)
	}
}
