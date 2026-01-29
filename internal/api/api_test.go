package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clearvault/internal/config"
)

// setupTestAPI creates a test API handler with temporary config
func setupTestAPI(t *testing.T) (*APIHandler, string, func()) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test_config.yaml")

	// Create a minimal test config
	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen:  ":8080",
		},
		Security: config.SecurityConfig{
			MasterKey: "dGhpcy1pcy1hLTMyLWJ5dGUtbG9uZy1tYXN0ZXJrZXkxMjM0NTY=",
		},
		Access: config.AccessConfig{
			Token: "test-token-12345",
		},
		Remote: config.RemoteConfig{
			Type: "local",
		},
		Storage: config.StorageConfig{
			MetadataPath: filepath.Join(tempDir, "metadata"),
			CacheDir:     filepath.Join(tempDir, "cache"),
		},
	}

	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	handler := NewAPIHandler(configPath)

	cleanup := func() {
		os.RemoveAll(tempDir)
	}

	return handler, configPath, cleanup
}

// TestNewAPIHandler tests creating a new API handler
func TestNewAPIHandler(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test_config.yaml")

	cfg := &config.Config{
		Access: config.AccessConfig{
			Token: "test-token",
		},
	}
	config.SaveConfig(configPath, cfg)

	handler := NewAPIHandler(configPath)
	if handler == nil {
		t.Fatal("NewAPIHandler() returned nil")
	}
	if handler.configPath != configPath {
		t.Error("Config path not set correctly")
	}
	if handler.token != "test-token" {
		t.Error("Token not loaded from config")
	}
}

// TestAPIHandler_HandleStatus tests the status endpoint
func TestAPIHandler_HandleStatus(t *testing.T) {
	handler, _, cleanup := setupTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	rec := httptest.NewRecorder()

	handler.HandleStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["status"] != "running" {
		t.Errorf("Expected status 'running', got %v", response["status"])
	}
	if response["initialized"] != true {
		t.Errorf("Expected initialized true, got %v", response["initialized"])
	}
}

// TestAPIHandler_HandleStatus_MethodNotAllowed tests status with wrong method
func TestAPIHandler_HandleStatus_MethodNotAllowed(t *testing.T) {
	handler, _, cleanup := setupTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/status", nil)
	rec := httptest.NewRecorder()

	handler.HandleStatus(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

// TestAPIHandler_AuthMiddleware tests authentication middleware
func TestAPIHandler_AuthMiddleware(t *testing.T) {
	handler, _, cleanup := setupTestAPI(t)
	defer cleanup()

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{
			name:       "Valid token",
			authHeader: "Bearer test-token-12345",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Invalid token",
			authHeader: "Bearer wrong-token",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Missing token",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Wrong format",
			authHeader: "test-token-12345",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()

			handler.AuthMiddleware(handler.HandleConfig)(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("Expected status %d, got %d", tt.wantStatus, rec.Code)
			}
		})
	}
}

// TestAPIHandler_HandleConfig_Get tests GET config endpoint
func TestAPIHandler_HandleConfig_Get(t *testing.T) {
	handler, _, cleanup := setupTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	req.Header.Set("Authorization", "Bearer test-token-12345")
	rec := httptest.NewRecorder()

	handler.AuthMiddleware(handler.HandleConfig)(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response config.Config
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Verify sensitive fields are masked
	if response.Security.MasterKey != maskedValue {
		t.Error("Master key should be masked")
	}
	if response.Access.Token != maskedValue {
		t.Error("Token should be masked")
	}
}

// TestAPIHandler_HandleConfig_Post tests POST config endpoint
func TestAPIHandler_HandleConfig_Post(t *testing.T) {
	handler, _, cleanup := setupTestAPI(t)
	defer cleanup()

	update := map[string]interface{}{
		"server": map[string]interface{}{
			"listen": ":9090",
		},
	}

	body, _ := json.Marshal(update)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/config", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token-12345")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.AuthMiddleware(handler.HandleConfig)(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["status"] != "ok" {
		t.Errorf("Expected status 'ok', got %v", response["status"])
	}
}

// TestAPIHandler_HandlePaths tests the paths endpoint
func TestAPIHandler_HandlePaths(t *testing.T) {
	handler, _, cleanup := setupTestAPI(t)
	defer cleanup()

	// Set accessible paths
	os.Setenv("ACCESSIBLE_PATHS", "/tmp:/var/data")
	defer os.Unsetenv("ACCESSIBLE_PATHS")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/paths", nil)
	rec := httptest.NewRecorder()

	handler.HandlePaths(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	paths, ok := response["paths"].([]interface{})
	if !ok {
		t.Fatal("Expected paths array in response")
	}
	if len(paths) != 2 {
		t.Errorf("Expected 2 paths, got %d", len(paths))
	}
}

// TestAPIHandler_IsInitialized tests initialization check
func TestAPIHandler_IsInitialized(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test_config.yaml")

	tests := []struct {
		name      string
		masterKey string
		want      bool
	}{
		{
			name:      "Valid master key",
			masterKey: "dGhpcy1pcy1hLTMyLWJ5dGUtbG9uZy1tYXN0ZXJrZXkxMjM0NTY=",
			want:      true,
		},
		{
			name:      "Empty master key",
			masterKey: "",
			want:      false,
		},
		{
			name:      "Default placeholder",
			masterKey: "CHANGE-THIS-TO-A-SECURE-32BYTE-KEY",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Security: config.SecurityConfig{
					MasterKey: tt.masterKey,
				},
			}
			config.SaveConfig(configPath, cfg)

			handler := NewAPIHandler(configPath)
			got := handler.IsInitialized()
			if got != tt.want {
				t.Errorf("IsInitialized() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestHelperFunctions tests utility functions
func TestHelperFunctions(t *testing.T) {
	t.Run("isMaskedOrEmpty", func(t *testing.T) {
		tests := []struct {
			input string
			want  bool
		}{
			{"", true},
			{"   ", true},
			{"******", true},
			{"value", false},
			{"  value  ", false},
		}
		for _, tt := range tests {
			got := isMaskedOrEmpty(tt.input)
			if got != tt.want {
				t.Errorf("isMaskedOrEmpty(%q) = %v, want %v", tt.input, got, tt.want)
			}
		}
	})

	t.Run("asStringMap", func(t *testing.T) {
		m := map[string]any{"key": "value"}
		result, ok := asStringMap(m)
		if !ok {
			t.Error("Expected map to be converted")
		}
		if result["key"] != "value" {
			t.Error("Value mismatch")
		}

		_, ok = asStringMap("not a map")
		if ok {
			t.Error("Non-map should return false")
		}
	})

	t.Run("getString", func(t *testing.T) {
		m := map[string]any{"key": "value", "num": 123}
		val, ok := getString(m, "key")
		if !ok || val != "value" {
			t.Error("Expected to get string value")
		}

		_, ok = getString(m, "num")
		if ok {
			t.Error("Non-string value should return false")
		}

		_, ok = getString(m, "missing")
		if ok {
			t.Error("Missing key should return false")
		}
	})

	t.Run("getBool", func(t *testing.T) {
		m := map[string]any{"flag": true, "str": "true"}
		val, ok := getBool(m, "flag")
		if !ok || !val {
			t.Error("Expected to get bool value")
		}

		_, ok = getBool(m, "str")
		if ok {
			t.Error("Non-bool value should return false")
		}
	})
}

// TestDirEmpty tests directory empty check
func TestDirEmpty(t *testing.T) {
	tempDir := t.TempDir()

	// Test empty directory
	empty, err := dirEmpty(tempDir)
	if err != nil {
		t.Fatalf("dirEmpty failed: %v", err)
	}
	if !empty {
		t.Error("Expected empty directory to be empty")
	}

	// Create a file
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	empty, err = dirEmpty(tempDir)
	if err != nil {
		t.Fatalf("dirEmpty failed: %v", err)
	}
	if empty {
		t.Error("Expected non-empty directory to not be empty")
	}
}

// TestIsUnderAllowedPath tests path validation
func TestIsUnderAllowedPath(t *testing.T) {
	tests := []struct {
		target string
		allowed []string
		want   bool
	}{
		{"/tmp/test", []string{"/tmp"}, true},
		{"/tmp", []string{"/tmp"}, true},
		{"/var/data", []string{"/tmp", "/var"}, true},
		{"/etc/passwd", []string{"/tmp"}, false},
		{"/tmp/../etc", []string{"/tmp"}, false},
	}

	for _, tt := range tests {
		got := isUnderAllowedPath(tt.target, tt.allowed)
		if got != tt.want {
			t.Errorf("isUnderAllowedPath(%q, %v) = %v, want %v", tt.target, tt.allowed, got, tt.want)
		}
	}
}

// TestProcessAlive tests process alive check
func TestProcessAlive(t *testing.T) {
	// Test invalid PIDs
	if processAlive(0) {
		t.Error("PID 0 should not be alive")
	}
	if processAlive(-1) {
		t.Error("Negative PID should not be alive")
	}

	// Test current process (should be alive)
	if !processAlive(os.Getpid()) {
		t.Error("Current process should be alive")
	}

	// Test non-existent process (very high PID)
	if processAlive(999999) {
		t.Error("Non-existent process should not be alive")
	}
}

// TestGetAccessiblePaths tests accessible paths parsing
func TestGetAccessiblePaths(t *testing.T) {
	// Test with ACCESSIBLE_PATHS
	os.Setenv("ACCESSIBLE_PATHS", "/tmp:/var/data:/home")
	defer os.Unsetenv("ACCESSIBLE_PATHS")

	paths := getAccessiblePaths()
	if len(paths) != 3 {
		t.Errorf("Expected 3 paths, got %d", len(paths))
	}

	// Test with TRIM_DATA_ACCESSIBLE_PATHS fallback
	os.Unsetenv("ACCESSIBLE_PATHS")
	os.Setenv("TRIM_DATA_ACCESSIBLE_PATHS", "/fallback")
	defer os.Unsetenv("TRIM_DATA_ACCESSIBLE_PATHS")

	paths = getAccessiblePaths()
	if len(paths) != 1 || paths[0] != "/fallback" {
		t.Errorf("Expected fallback path, got %v", paths)
	}

	// Test empty
	os.Unsetenv("TRIM_DATA_ACCESSIBLE_PATHS")
	paths = getAccessiblePaths()
	if len(paths) != 0 {
		t.Errorf("Expected empty paths, got %v", paths)
	}
}

// TestGenerateRandomPassword tests password generation
func TestGenerateRandomPassword(t *testing.T) {
	password := generateRandomPassword()
	if len(password) != 16 {
		t.Errorf("Expected password length 16, got %d", len(password))
	}

	// Verify charset
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	for _, c := range password {
		if !strings.ContainsRune(charset, c) {
			t.Errorf("Password contains invalid character: %c", c)
		}
	}
}

// TestMountStateFunctions tests mount state persistence
func TestMountStateFunctions(t *testing.T) {
	handler, _, cleanup := setupTestAPI(t)
	defer cleanup()

	// Test write and read
	state := mountState{
		Pid:        12345,
		Mountpoint: "/tmp/test-mount",
	}

	if err := handler.writeMountState(state); err != nil {
		t.Fatalf("writeMountState failed: %v", err)
	}

	readState, alive := handler.readMountState()
	// alive should be false because PID 12345 is not actually running
	if alive {
		t.Error("Expected process to not be alive")
	}
	if readState.Pid != 0 {
		t.Error("Expected state to be cleared for non-running process")
	}

	// Test clear
	if err := handler.writeMountState(state); err != nil {
		t.Fatalf("writeMountState failed: %v", err)
	}
	if err := handler.clearMountState(); err != nil {
		t.Fatalf("clearMountState failed: %v", err)
	}
}

// TestMountConfigFunctions tests mount config persistence
func TestMountConfigFunctions(t *testing.T) {
	handler, _, cleanup := setupTestAPI(t)
	defer cleanup()

	cfg := mountConfig{
		Mountpoint:   "/tmp/test",
		Auto:         true,
		DelaySeconds: 5,
	}

	if err := handler.writeMountConfig(cfg); err != nil {
		t.Fatalf("writeMountConfig failed: %v", err)
	}

	readCfg, ok := handler.readMountConfig()
	if !ok {
		t.Fatal("Failed to read mount config")
	}
	if readCfg.Mountpoint != cfg.Mountpoint {
		t.Error("Mountpoint mismatch")
	}
	if readCfg.Auto != cfg.Auto {
		t.Error("Auto mismatch")
	}
	if readCfg.DelaySeconds != cfg.DelaySeconds {
		t.Error("DelaySeconds mismatch")
	}
}

// TestMountPidFunctions tests mount PID persistence
func TestMountPidFunctions(t *testing.T) {
	handler, _, cleanup := setupTestAPI(t)
	defer cleanup()

	pid := os.Getpid()
	if err := handler.writeMountPid(pid); err != nil {
		t.Fatalf("writeMountPid failed: %v", err)
	}

	readPid, ok := handler.readMountPid()
	if !ok {
		t.Fatal("Failed to read mount PID")
	}
	if readPid != pid {
		t.Errorf("PID mismatch: expected %d, got %d", pid, readPid)
	}
}

// TestWriteToolJSON tests tool response writer
func TestWriteToolJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	details := map[string]interface{}{"key": "value"}
	writeToolJSON(rec, http.StatusOK, "success", details)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	var response ToolResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.OK {
		t.Error("Expected OK to be true")
	}
	if response.Message != "success" {
		t.Errorf("Expected message 'success', got %s", response.Message)
	}
}

// TestGetPkgVar tests package var directory
func TestGetPkgVar(t *testing.T) {
	// Test default (TempDir)
	os.Unsetenv("TRIM_PKGVAR")
	pkgVar := getPkgVar()
	if pkgVar != os.TempDir() {
		t.Errorf("Expected %s, got %s", os.TempDir(), pkgVar)
	}

	// Test with environment variable
	os.Setenv("TRIM_PKGVAR", "/custom/path")
	defer os.Unsetenv("TRIM_PKGVAR")

	pkgVar = getPkgVar()
	if pkgVar != "/custom/path" {
		t.Errorf("Expected /custom/path, got %s", pkgVar)
	}
}
