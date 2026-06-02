package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestApp(t *testing.T, bridgeURL string) *App {
	t.Helper()

	root := t.TempDir()
	webRoot := filepath.Join(root, "web")
	dataRoot := filepath.Join(root, "data")

	app, err := NewApp(Config{
		Addr:        "127.0.0.1:0",
		ProjectRoot: root,
		WebRoot:     webRoot,
		DataRoot:    dataRoot,
		BridgeURL:   bridgeURL,
		OraTarget:   "http://127.0.0.1:65535",
		OraName:     "ORA-FEA252",
		OraIP:       "10.1.48.113",
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

func TestHealthDoesNotExposePassword(t *testing.T) {
	app := newTestApp(t, "http://127.0.0.1:65535")
	server := httptest.NewServer(app.routes())
	defer server.Close()

	res, err := http.Get(server.URL + "/api/health")
	if err != nil {
		t.Fatalf("GET /api/health failed: %v", err)
	}
	defer res.Body.Close()

	var body map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode health response: %v", err)
	}

	if body["server"] != "ora-go-server" {
		t.Fatalf("unexpected server marker: %#v", body["server"])
	}
	if body["storage"] != "sqlite" {
		t.Fatalf("unexpected storage marker: %#v", body["storage"])
	}
	if _, exists := body["oraPassword"]; exists {
		t.Fatal("health response exposed ORA password field")
	}
}

func TestSQLiteSchemaIncludesClassroomTables(t *testing.T) {
	app := newTestApp(t, "http://127.0.0.1:65535")

	requiredTables := []string{
		"users",
		"classes",
		"class_memberships",
		"safety_profiles",
		"projects",
		"project_versions",
		"submissions",
		"approvals",
		"robot_runs",
		"audit_events",
	}

	for _, table := range requiredTables {
		var name string
		err := app.projects.db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Fatalf("missing table %s: %v", table, err)
		}
	}
}

func TestProjectCRUD(t *testing.T) {
	app := newTestApp(t, "http://127.0.0.1:65535")
	server := httptest.NewServer(app.routes())
	defer server.Close()

	createBody := `{"title":"Lesson 1","owner":"student-a","blockly":{"blocks":[]}}`
	res, err := http.Post(server.URL+"/api/projects", "application/json", strings.NewReader(createBody))
	if err != nil {
		t.Fatalf("POST /api/projects failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d", res.StatusCode)
	}

	var created Project
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatalf("decode created project: %v", err)
	}
	if created.ID == "" || created.Title != "Lesson 1" {
		t.Fatalf("unexpected created project: %#v", created)
	}

	res, err = http.Get(server.URL + "/api/projects/" + created.ID)
	if err != nil {
		t.Fatalf("GET project failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d", res.StatusCode)
	}

	updateReq, err := http.NewRequest(http.MethodPut, server.URL+"/api/projects/"+created.ID, strings.NewReader(`{"title":"Lesson 1 Revised","python":"print('hi')"}`))
	if err != nil {
		t.Fatalf("build update request: %v", err)
	}
	updateReq.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(updateReq)
	if err != nil {
		t.Fatalf("PUT project failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d", res.StatusCode)
	}

	var updated Project
	if err := json.NewDecoder(res.Body).Decode(&updated); err != nil {
		t.Fatalf("decode updated project: %v", err)
	}
	if updated.Title != "Lesson 1 Revised" || updated.Python == "" {
		t.Fatalf("unexpected updated project: %#v", updated)
	}

	deleteReq, err := http.NewRequest(http.MethodDelete, server.URL+"/api/projects/"+created.ID, nil)
	if err != nil {
		t.Fatalf("build delete request: %v", err)
	}
	res, err = http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("DELETE project failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d", res.StatusCode)
	}
}

func TestUserAndClassCRUD(t *testing.T) {
	app := newTestApp(t, "http://127.0.0.1:65535")
	server := httptest.NewServer(app.routes())
	defer server.Close()

	userResponse, err := http.Post(server.URL+"/api/users", "application/json", strings.NewReader(`{"displayName":"Ada Lovelace","role":"student","email":"ada@example.test","active":true}`))
	if err != nil {
		t.Fatalf("POST /api/users failed: %v", err)
	}
	defer userResponse.Body.Close()
	if userResponse.StatusCode != http.StatusCreated {
		t.Fatalf("user create status = %d", userResponse.StatusCode)
	}

	var user UserProfile
	if err := json.NewDecoder(userResponse.Body).Decode(&user); err != nil {
		t.Fatalf("decode user: %v", err)
	}
	if user.ID == "" || user.DisplayName != "Ada Lovelace" || user.Role != "student" {
		t.Fatalf("unexpected user: %#v", user)
	}

	usersResponse, err := http.Get(server.URL + "/api/users")
	if err != nil {
		t.Fatalf("GET /api/users failed: %v", err)
	}
	defer usersResponse.Body.Close()
	if usersResponse.StatusCode != http.StatusOK {
		t.Fatalf("users list status = %d", usersResponse.StatusCode)
	}

	deleteUserReq, err := http.NewRequest(http.MethodDelete, server.URL+"/api/users/"+user.ID, nil)
	if err != nil {
		t.Fatalf("build user delete request: %v", err)
	}
	deleteUserResponse, err := http.DefaultClient.Do(deleteUserReq)
	if err != nil {
		t.Fatalf("DELETE /api/users failed: %v", err)
	}
	defer deleteUserResponse.Body.Close()
	if deleteUserResponse.StatusCode != http.StatusOK {
		t.Fatalf("user delete status = %d", deleteUserResponse.StatusCode)
	}

	classResponse, err := http.Post(server.URL+"/api/classes", "application/json", strings.NewReader(`{"name":"Robotics 1","term":"2026"}`))
	if err != nil {
		t.Fatalf("POST /api/classes failed: %v", err)
	}
	defer classResponse.Body.Close()
	if classResponse.StatusCode != http.StatusCreated {
		t.Fatalf("class create status = %d", classResponse.StatusCode)
	}

	var class ClassProfile
	if err := json.NewDecoder(classResponse.Body).Decode(&class); err != nil {
		t.Fatalf("decode class: %v", err)
	}
	if class.ID == "" || class.Name != "Robotics 1" {
		t.Fatalf("unexpected class: %#v", class)
	}

	deleteClassReq, err := http.NewRequest(http.MethodDelete, server.URL+"/api/classes/"+class.ID, nil)
	if err != nil {
		t.Fatalf("build class delete request: %v", err)
	}
	deleteClassResponse, err := http.DefaultClient.Do(deleteClassReq)
	if err != nil {
		t.Fatalf("DELETE /api/classes failed: %v", err)
	}
	defer deleteClassResponse.Body.Close()
	if deleteClassResponse.StatusCode != http.StatusOK {
		t.Fatalf("class delete status = %d", deleteClassResponse.StatusCode)
	}
}

func TestLegacyProjectFilesAreImported(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	legacyRoot := filepath.Join(dataRoot, "projects")
	if err := os.MkdirAll(legacyRoot, 0755); err != nil {
		t.Fatalf("create legacy project dir: %v", err)
	}

	legacy := `{
  "id": "legacy_project",
  "formatVersion": 1,
  "title": "Legacy Lesson",
  "owner": "student-a",
  "mode": "blocks-and-python",
  "python": "move_home()"
}`
	if err := os.WriteFile(filepath.Join(legacyRoot, "legacy_project.json"), []byte(legacy), 0644); err != nil {
		t.Fatalf("write legacy project: %v", err)
	}

	app, err := NewApp(Config{
		Addr:        "127.0.0.1:0",
		ProjectRoot: root,
		WebRoot:     filepath.Join(root, "web"),
		DataRoot:    dataRoot,
		BridgeURL:   "http://127.0.0.1:65535",
		OraTarget:   "http://127.0.0.1:65535",
		OraName:     "ORA-FEA252",
		OraIP:       "10.1.48.113",
	})
	if err != nil {
		t.Fatalf("NewApp failed: %v", err)
	}
	defer app.projects.Close()

	project, err := app.projects.Get("legacy_project")
	if err != nil {
		t.Fatalf("legacy project was not imported: %v", err)
	}
	if project.Title != "Legacy Lesson" || project.Python != "move_home()" {
		t.Fatalf("unexpected legacy project: %#v", project)
	}
}

func TestBridgeProxy(t *testing.T) {
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			t.Fatalf("bridge saw path %q", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, map[string]bool{"connected": true})
	}))
	defer bridge.Close()

	app := newTestApp(t, bridge.URL)
	server := httptest.NewServer(app.routes())
	defer server.Close()

	res, err := http.Get(server.URL + "/bridge/status")
	if err != nil {
		t.Fatalf("GET /bridge/status failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("bridge status = %d", res.StatusCode)
	}

	var body map[string]bool
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode bridge response: %v", err)
	}
	if !body["connected"] {
		t.Fatal("bridge response did not proxy connected=true")
	}
}
