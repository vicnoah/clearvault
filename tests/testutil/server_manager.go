// Package testutil provides utilities for integration testing
package testutil

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestServerManager manages the lifecycle of zs3 and sweb test servers
type TestServerManager struct {
	zs3Cmd      *exec.Cmd
	swebCmd     *exec.Cmd
	zs3DataDir  string
	swebDataDir string
	zs3LogFile  string
	swebLogFile string
	projectRoot string
	toolsDir    string
	mu          sync.Mutex
	started     bool
}

// ServerConfig holds configuration for test servers
type ServerConfig struct {
	ZS3Port     int
	SWEBPort    int
	ZS3DataDir  string
	SWEBDataDir string
	AccessKey   string
	SecretKey   string
	WebDAVUser  string
	WebDAVPass  string
}

// DefaultServerConfig returns default server configuration
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		ZS3Port:     9000,
		SWEBPort:    8081,
		ZS3DataDir:  "data/test-s3",
		SWEBDataDir: "data/test-webdav",
		AccessKey:   "minioadmin",
		SecretKey:   "minioadmin",
		WebDAVUser:  "admin",
		WebDAVPass:  "admin123",
	}
}

// NewTestServerManager creates a new test server manager
func NewTestServerManager() (*TestServerManager, error) {
	projectRoot, err := findProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to find project root: %w", err)
	}

	return &TestServerManager{
		projectRoot: projectRoot,
		toolsDir:    filepath.Join(projectRoot, "tools"),
		zs3DataDir:  filepath.Join(projectRoot, "data", "test-s3"),
		swebDataDir: filepath.Join(projectRoot, "data", "test-webdav"),
		zs3LogFile:  filepath.Join(projectRoot, "tests", "testdata", "zs3.log"),
		swebLogFile: filepath.Join(projectRoot, "tests", "testdata", "sweb.log"),
	}, nil
}

// findProjectRoot finds the project root directory by looking for go.mod
func findProjectRoot() (string, error) {
	// Start from current directory
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Walk up the tree looking for go.mod
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Fallback: use runtime caller
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Abs(filepath.Join(filepath.Dir(filename), "..", ".."))
}

// IsZS3Running checks if zs3 server is already running
func (m *TestServerManager) IsZS3Running() bool {
	resp, err := http.Get("http://localhost:9000/minio/health/live")
	if err == nil {
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusForbidden
	}
	// Try alternative endpoint
	resp, err = http.Get("http://localhost:9000/")
	if err == nil {
		resp.Body.Close()
		return true
	}
	return false
}

// IsSWEBRunning checks if sweb server is already running
func (m *TestServerManager) IsSWEBRunning() bool {
	resp, err := http.Get("http://localhost:8081/webdav/")
	if err == nil {
		resp.Body.Close()
		return true
	}
	return false
}

// StartZS3 starts the zs3 S3 server
func (m *TestServerManager) StartZS3() error {
	if m.IsZS3Running() {
		fmt.Println("ZS3 server is already running, skipping start")
		return nil
	}

	// Create data directory
	if err := os.MkdirAll(m.zs3DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create zs3 data dir: %w", err)
	}

	// Create log directory
	if err := os.MkdirAll(filepath.Dir(m.zs3LogFile), 0755); err != nil {
		return fmt.Errorf("failed to create log dir: %w", err)
	}

	// Open log file
	logFile, err := os.OpenFile(m.zs3LogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open zs3 log file: %w", err)
	}

	// Find zs3 binary
	zs3Bin := m.findZS3Binary()

	// Start zs3
	m.zs3Cmd = exec.Command(zs3Bin,
		"--port", "9000",
		"--dir", m.zs3DataDir,
		"--access-key", "minioadmin",
		"--secret-key", "minioadmin",
	)
	m.zs3Cmd.Stdout = logFile
	m.zs3Cmd.Stderr = logFile
	m.zs3Cmd.Dir = m.zs3DataDir

	if err := m.zs3Cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to start zs3: %w", err)
	}

	fmt.Printf("ZS3 started with PID: %d\n", m.zs3Cmd.Process.Pid)

	// Wait for server to be ready
	if err := m.waitForZS3(30 * time.Second); err != nil {
		m.StopZS3()
		return fmt.Errorf("zs3 failed to start: %w", err)
	}

	fmt.Println("ZS3 is ready!")
	return nil
}

// StartSWEB starts the sweb WebDAV server
func (m *TestServerManager) StartSWEB() error {
	if m.IsSWEBRunning() {
		fmt.Println("SWEB server is already running, skipping start")
		return nil
	}

	// Create data directory
	if err := os.MkdirAll(m.swebDataDir, 0755); err != nil {
		return fmt.Errorf("failed to create sweb data dir: %w", err)
	}

	// Create log directory
	if err := os.MkdirAll(filepath.Dir(m.swebLogFile), 0755); err != nil {
		return fmt.Errorf("failed to create log dir: %w", err)
	}

	// Open log file
	logFile, err := os.OpenFile(m.swebLogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open sweb log file: %w", err)
	}

	// Find sweb binary
	swebBin := m.findSWEBBinary()

	// Start sweb with correct arguments
	// sweb -webdav -webdav-dir <dir> -webdav-port 8081 -p <http-port>
	m.swebCmd = exec.Command(swebBin,
		"-webdav",
		"-webdav-dir", m.swebDataDir,
		"-webdav-port", "8081",
		"-p", "18080", // Use different HTTP port to avoid conflict
	)
	m.swebCmd.Stdout = logFile
	m.swebCmd.Stderr = logFile
	m.swebCmd.Dir = m.swebDataDir

	if err := m.swebCmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to start sweb: %w", err)
	}

	fmt.Printf("SWEB started with PID: %d\n", m.swebCmd.Process.Pid)

	// Wait for server to be ready
	if err := m.waitForSWEB(30 * time.Second); err != nil {
		m.StopSWEB()
		return fmt.Errorf("sweb failed to start: %w", err)
	}

	fmt.Println("SWEB is ready!")
	return nil
}

// findZS3Binary finds the zs3 binary
func (m *TestServerManager) findZS3Binary() string {
	// Check tools directory
	zs3Bin := filepath.Join(m.toolsDir, "zs3")
	if _, err := os.Stat(zs3Bin); err == nil {
		return zs3Bin
	}

	// Check system PATH
	if path, err := exec.LookPath("zs3"); err == nil {
		return path
	}

	return "zs3" // Fallback
}

// findSWEBBinary finds the sweb binary
func (m *TestServerManager) findSWEBBinary() string {
	// Check tools directory
	swebBin := filepath.Join(m.toolsDir, "sweb")
	if _, err := os.Stat(swebBin); err == nil {
		return swebBin
	}

	// Check system PATH
	if path, err := exec.LookPath("sweb"); err == nil {
		return path
	}

	return "sweb" // Fallback
}

// waitForZS3 waits for zs3 to be ready
func (m *TestServerManager) waitForZS3(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if m.IsZS3Running() {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for zs3")
}

// waitForSWEB waits for sweb to be ready
func (m *TestServerManager) waitForSWEB(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if m.IsSWEBRunning() {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for sweb")
}

// StopZS3 stops the zs3 server
func (m *TestServerManager) StopZS3() error {
	if m.zs3Cmd != nil && m.zs3Cmd.Process != nil {
		fmt.Printf("Stopping ZS3 (PID: %d)...\n", m.zs3Cmd.Process.Pid)
		if err := m.zs3Cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill zs3: %w", err)
		}
		m.zs3Cmd.Wait()
		m.zs3Cmd = nil
		fmt.Println("ZS3 stopped")
	}
	return nil
}

// StopSWEB stops the sweb server
func (m *TestServerManager) StopSWEB() error {
	if m.swebCmd != nil && m.swebCmd.Process != nil {
		fmt.Printf("Stopping SWEB (PID: %d)...\n", m.swebCmd.Process.Pid)
		if err := m.swebCmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill sweb: %w", err)
		}
		m.swebCmd.Wait()
		m.swebCmd = nil
		fmt.Println("SWEB stopped")
	}
	return nil
}

// StartAll starts both servers
func (m *TestServerManager) StartAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return nil
	}

	fmt.Println("=== Starting Test Servers ===")

	if err := m.StartZS3(); err != nil {
		return fmt.Errorf("failed to start zs3: %w", err)
	}

	if err := m.StartSWEB(); err != nil {
		m.StopZS3()
		return fmt.Errorf("failed to start sweb: %w", err)
	}

	m.started = true
	fmt.Println("=== Test Servers Started ===")
	return nil
}

// StopAll stops both servers
func (m *TestServerManager) StopAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return nil
	}

	fmt.Println("=== Stopping Test Servers ===")

	if err := m.StopSWEB(); err != nil {
		fmt.Printf("Warning: failed to stop sweb: %v\n", err)
	}

	if err := m.StopZS3(); err != nil {
		fmt.Printf("Warning: failed to stop zs3: %v\n", err)
	}

	m.started = false
	fmt.Println("=== Test Servers Stopped ===")
	return nil
}

// Cleanup cleans up test data directories
func (m *TestServerManager) Cleanup() error {
	fmt.Println("Cleaning up test data...")
	if err := os.RemoveAll(m.zs3DataDir); err != nil {
		return fmt.Errorf("failed to clean zs3 data: %w", err)
	}
	if err := os.RemoveAll(m.swebDataDir); err != nil {
		return fmt.Errorf("failed to clean sweb data: %w", err)
	}
	return nil
}

// GetZS3Endpoint returns the S3 endpoint URL
func (m *TestServerManager) GetZS3Endpoint() string {
	return "localhost:9000"
}

// GetWebDAVURL returns the WebDAV URL
func (m *TestServerManager) GetWebDAVURL() string {
	return "http://localhost:8081/webdav"
}

// GetS3Credentials returns S3 credentials
func (m *TestServerManager) GetS3Credentials() (accessKey, secretKey string) {
	return "minioadmin", "minioadmin"
}

// GetWebDAVCredentials returns WebDAV credentials
func (m *TestServerManager) GetWebDAVCredentials() (user, pass string) {
	return "admin", "admin123"
}

// EnsureServersForTest ensures servers are running for a test
// This should be called at the beginning of integration tests
func EnsureServersForTest(t *testing.T) *TestServerManager {
	manager, err := NewTestServerManager()
	if err != nil {
		t.Fatalf("Failed to create server manager: %v", err)
	}

	if err := manager.StartAll(); err != nil {
		t.Fatalf("Failed to start test servers: %v", err)
	}

	// Register cleanup
	t.Cleanup(func() {
		manager.StopAll()
	})

	return manager
}

// EnsureServersForTestSkipCleanup ensures servers are running but doesn't cleanup
// Useful when you want servers to persist across multiple tests
func EnsureServersForTestSkipCleanup(t *testing.T) *TestServerManager {
	manager, err := NewTestServerManager()
	if err != nil {
		t.Fatalf("Failed to create server manager: %v", err)
	}

	if err := manager.StartAll(); err != nil {
		t.Fatalf("Failed to start test servers: %v", err)
	}

	return manager
}

// SkipIfServersNotRunning skips the test if servers are not available
func SkipIfServersNotRunning(t *testing.T) {
	manager, err := NewTestServerManager()
	if err != nil {
		t.Skipf("Failed to create server manager: %v", err)
	}

	if !manager.IsZS3Running() {
		t.Skip("ZS3 server is not running")
	}

	if !manager.IsSWEBRunning() {
		t.Skip("SWEB server is not running")
	}
}

// CreateS3Bucket creates a test bucket in S3
func (m *TestServerManager) CreateS3Bucket(bucketName string) error {
	// Use mc or aws CLI, or minio-go to create bucket
	// For simplicity, use HTTP PUT request
	url := fmt.Sprintf("http://localhost:9000/%s", bucketName)

	req, err := http.NewRequest(http.MethodPut, url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "AWS minioadmin:minioadmin")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create bucket: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create bucket: %s - %s", resp.Status, string(body))
	}

	return nil
}

// Context-aware server management for long-running tests

// ServerPool manages a pool of test servers that can be reused across tests
type ServerPool struct {
	manager *TestServerManager
	mu      sync.Mutex
	inUse   bool
}

// Global server pool instance
var globalPool *ServerPool
var poolOnce sync.Once

// GetGlobalServerPool returns the global server pool
func GetGlobalServerPool() *ServerPool {
	poolOnce.Do(func() {
		manager, err := NewTestServerManager()
		if err != nil {
			panic(fmt.Sprintf("Failed to create server manager: %v", err))
		}
		globalPool = &ServerPool{manager: manager}
	})
	return globalPool
}

// Acquire starts servers and marks them as in use
func (p *ServerPool) Acquire(ctx context.Context) (*TestServerManager, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.manager.StartAll(); err != nil {
		return nil, err
	}

	p.inUse = true
	return p.manager, nil
}

// Release marks servers as no longer in use
func (p *ServerPool) Release() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inUse = false
}

// Shutdown stops all servers in the pool
func (p *ServerPool) Shutdown() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.manager.StopAll()
}

// RunWithServers runs a test function with servers available
func RunWithServers(t *testing.T, testFunc func(*testing.T, *TestServerManager)) {
	manager := EnsureServersForTest(t)
	testFunc(t, manager)
}

// RunWithExistingServers runs a test only if servers are already running
func RunWithExistingServers(t *testing.T, testFunc func(*testing.T, *TestServerManager)) {
	SkipIfServersNotRunning(t)

	manager, err := NewTestServerManager()
	if err != nil {
		t.Fatalf("Failed to create server manager: %v", err)
	}

	testFunc(t, manager)
}

// MustHaveServers is a test helper that ensures servers are running
// It tries to start them if they're not running
func MustHaveServers(t *testing.T) *TestServerManager {
	return EnsureServersForTest(t)
}

// SetupTestEnvironment sets up the complete test environment
// This is useful for integration test suites
func SetupTestEnvironment(t *testing.T) (*TestServerManager, func()) {
	manager := MustHaveServers(t)

	cleanup := func() {
		// Cleanup is handled by t.Cleanup, but this allows explicit cleanup if needed
		manager.StopAll()
	}

	return manager, cleanup
}

// CheckServerLogs returns the recent log output from servers
func (m *TestServerManager) CheckServerLogs(server string) (string, error) {
	var logFile string
	switch server {
	case "zs3":
		logFile = m.zs3LogFile
	case "sweb":
		logFile = m.swebLogFile
	default:
		return "", fmt.Errorf("unknown server: %s", server)
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		return "", fmt.Errorf("failed to read log file: %w", err)
	}

	// Return last 50 lines
	lines := strings.Split(string(data), "\n")
	if len(lines) > 50 {
		lines = lines[len(lines)-50:]
	}

	return strings.Join(lines, "\n"), nil
}
