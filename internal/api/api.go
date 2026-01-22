package api

import (
	"clearvault/internal/config"
	"clearvault/internal/metadata"
	"clearvault/internal/proxy"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type APIHandler struct {
	configPath string
	mu         sync.RWMutex
	mountMu    sync.Mutex
	startTime  time.Time
	token      string
}

type ToolResponse struct {
	OK      bool                   `json:"ok"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

const maskedValue = "******"

func isMaskedOrEmpty(s string) bool {
	return strings.TrimSpace(s) == "" || s == maskedValue
}

func asStringMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func getString(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func getBool(m map[string]any, key string) (bool, bool) {
	v, ok := m[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

func NewAPIHandler(configPath string) *APIHandler {
	// 尝试预加载 token，但主要依赖后续从 config 加载
	var token string
	if cfg, err := config.LoadConfig(configPath); err == nil {
		token = cfg.Access.Token
	}

	return &APIHandler{
		configPath: configPath,
		startTime:  time.Now(),
		token:      token,
	}
}

type StatusResponse struct {
	Status    string    `json:"status"`
	Uptime    string    `json:"uptime"`
	Version   string    `json:"version"`
	StartTime time.Time `json:"start_time"`
}

func (h *APIHandler) IsInitialized() bool {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		return false
	}
	return cfg.Security.MasterKey != "" && cfg.Security.MasterKey != "CHANGE-THIS-TO-A-SECURE-32BYTE-KEY"
}

func (h *APIHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	initialized := h.IsInitialized()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "running",
		"uptime":      time.Since(h.startTime).String(),
		"initialized": initialized,
	})
}

func (h *APIHandler) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.mu.RLock()
		token := h.token
		h.mu.RUnlock()

		// 0. Auth Logic First
		// Always enforce auth if token is set, regardless of initialization state
		if token != "" {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Unauthorized: Missing Authorization header", http.StatusUnauthorized)
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, "Unauthorized: Invalid Authorization header format", http.StatusUnauthorized)
				return
			}

			if parts[1] != token {
				http.Error(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
				return
			}
		}

		// 1. Check Initialization (MasterKey)
		// Uninitialized mode: only allow minimal endpoints for setup UI.
		if r.URL.Path != "/api/v1/status" && r.URL.Path != "/api/v1/config" && r.URL.Path != "/api/v1/paths" {
			if !h.IsInitialized() {
				http.Error(w, "Service Uninitialized", http.StatusServiceUnavailable)
				return
			}
		}

		next(w, r)
	}
}

func (h *APIHandler) HandlePaths(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pathsEnv := os.Getenv("ACCESSIBLE_PATHS")
	if pathsEnv == "" {
		pathsEnv = os.Getenv("TRIM_DATA_ACCESSIBLE_PATHS")
	}
	var paths []string
	if pathsEnv != "" {
		paths = strings.Split(pathsEnv, ":")
	} else {
		paths = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"paths": paths,
	})
}

func (h *APIHandler) HandleConfig(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if r.Method == http.MethodGet {
		cfg, err := config.LoadConfig(h.configPath)
		if err != nil {
			http.Error(w, "Failed to load config", http.StatusInternalServerError)
			return
		}

		// Update cached token
		h.token = cfg.Access.Token

		// Mask sensitive fields
		cfg.Security.MasterKey = "******"
		cfg.Access.Token = "******"
		cfg.Remote.Pass = "******"
		cfg.Remote.SecretKey = "******"

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
		return
	}

	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		var raw map[string]any
		if err := json.Unmarshal(body, &raw); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		cfgOld, err := config.LoadConfig(h.configPath)
		if err != nil {
			http.Error(w, "Failed to load config", http.StatusInternalServerError)
			return
		}
		cfgNew := *cfgOld

		// server
		if serverRaw, ok := asStringMap(raw["server"]); ok {
			if listen, ok := getString(serverRaw, "listen"); ok && strings.TrimSpace(listen) != "" {
				cfgNew.Server.Listen = listen
			}
			if baseURL, ok := getString(serverRaw, "base_url"); ok && strings.TrimSpace(baseURL) != "" {
				cfgNew.Server.BaseURL = baseURL
			}
			if authRaw, ok := asStringMap(serverRaw["auth"]); ok {
				if user, ok := getString(authRaw, "user"); ok && strings.TrimSpace(user) != "" {
					cfgNew.Server.Auth.User = user
				}
				if pass, ok := getString(authRaw, "pass"); ok && !isMaskedOrEmpty(pass) {
					cfgNew.Server.Auth.Pass = pass
				}
			}
		}

		// access
		if accessRaw, ok := asStringMap(raw["access"]); ok {
			if token, ok := getString(accessRaw, "token"); ok && !isMaskedOrEmpty(token) {
				cfgNew.Access.Token = token
			}
		}

		// security
		if securityRaw, ok := asStringMap(raw["security"]); ok {
			if masterKey, ok := getString(securityRaw, "master_key"); ok {
				if masterKey == maskedValue {
					// ignore
				} else if strings.TrimSpace(masterKey) == "" {
					// only allow empty master key in uninitialized mode (to trigger auto-generation)
					if !h.IsInitialized() {
						cfgNew.Security.MasterKey = ""
					}
				} else {
					cfgNew.Security.MasterKey = masterKey
				}
			}
		}

		// remote
		if remoteRaw, ok := asStringMap(raw["remote"]); ok {
			if rt, ok := getString(remoteRaw, "type"); ok && strings.TrimSpace(rt) != "" {
				cfgNew.Remote.Type = rt
			}
			effectiveType := strings.ToLower(strings.TrimSpace(cfgNew.Remote.Type))

			switch effectiveType {
			case "local":
				if lp, ok := getString(remoteRaw, "local_path"); ok {
					cfgNew.Remote.LocalPath = lp
				}
			case "s3":
				if endpoint, ok := getString(remoteRaw, "endpoint"); ok {
					cfgNew.Remote.Endpoint = endpoint
				}
				if region, ok := getString(remoteRaw, "region"); ok {
					cfgNew.Remote.Region = region
				}
				if bucket, ok := getString(remoteRaw, "bucket"); ok {
					cfgNew.Remote.Bucket = bucket
				}
				if ak, ok := getString(remoteRaw, "access_key"); ok {
					cfgNew.Remote.AccessKey = ak
				}
				if ssl, ok := getBool(remoteRaw, "use_ssl"); ok {
					cfgNew.Remote.UseSSL = ssl
				}
				if sk, ok := getString(remoteRaw, "secret_key"); ok && !isMaskedOrEmpty(sk) {
					cfgNew.Remote.SecretKey = sk
				}
			default: // webdav (and legacy)
				if url, ok := getString(remoteRaw, "url"); ok {
					cfgNew.Remote.URL = url
				}
				if user, ok := getString(remoteRaw, "user"); ok {
					cfgNew.Remote.User = user
				}
				if pass, ok := getString(remoteRaw, "pass"); ok && !isMaskedOrEmpty(pass) {
					cfgNew.Remote.Pass = pass
				}
			}
		}

		// storage
		if storageRaw, ok := asStringMap(raw["storage"]); ok {
			if mp, ok := getString(storageRaw, "metadata_path"); ok && strings.TrimSpace(mp) != "" {
				cfgNew.Storage.MetadataPath = mp
			}
			if cd, ok := getString(storageRaw, "cache_dir"); ok && strings.TrimSpace(cd) != "" {
				cfgNew.Storage.CacheDir = cd
			}
		}

		beforeKey := cfgNew.Security.MasterKey
		// Check if we need to generate master key
		if cfgNew.Security.MasterKey == "" || cfgNew.Security.MasterKey == "CHANGE-THIS-TO-A-SECURE-32BYTE-KEY" {
			log.Println("Initializing Master Key via API...")
			if err := config.GenerateMasterKey(h.configPath, &cfgNew); err != nil {
				http.Error(w, "Failed to generate master key: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}

		err = config.SaveConfig(h.configPath, &cfgNew)
		if err != nil {
			http.Error(w, "Failed to save config: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Update token cache if changed
		h.token = cfgNew.Access.Token

		w.WriteHeader(http.StatusOK)
		resp := map[string]string{"status": "ok"}
		if strings.TrimSpace(beforeKey) == "" || beforeKey == "CHANGE-THIS-TO-A-SECURE-32BYTE-KEY" {
			resp["master_key"] = cfgNew.Security.MasterKey
		}
		json.NewEncoder(w).Encode(resp)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

type mountState struct {
	Pid        int    `json:"pid"`
	Mountpoint string `json:"mountpoint"`
}

type mountConfig struct {
	Mountpoint   string `json:"mountpoint"`
	Auto         bool   `json:"auto"`
	DelaySeconds int    `json:"delaySeconds"`
}

func (h *APIHandler) HandleMountStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	state, alive := h.readMountState()
	w.Header().Set("Content-Type", "application/json")
	if !alive {
		json.NewEncoder(w).Encode(map[string]any{
			"mounted": false,
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]any{
		"mounted":    true,
		"pid":        state.Pid,
		"mountpoint": state.Mountpoint,
	})
}

func (h *APIHandler) HandleMount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if _, alive := h.readMountState(); alive {
		http.Error(w, "Already mounted", http.StatusConflict)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	var req struct {
		Mountpoint string `json:"mountpoint"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	mountpoint := strings.TrimSpace(req.Mountpoint)
	if mountpoint == "" {
		http.Error(w, "mountpoint is required", http.StatusBadRequest)
		return
	}

	allowed := getAccessiblePaths()
	if !isUnderAllowedPath(mountpoint, allowed) {
		http.Error(w, "mountpoint is not within authorized paths", http.StatusForbidden)
		return
	}

	if err := os.MkdirAll(mountpoint, 0755); err != nil {
		http.Error(w, "Failed to create mountpoint: "+err.Error(), http.StatusInternalServerError)
		return
	}
	empty, err := dirEmpty(mountpoint)
	if err != nil {
		http.Error(w, "Failed to check mountpoint: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !empty {
		http.Error(w, "mountpoint must be an empty directory", http.StatusBadRequest)
		return
	}

	if err := h.writeMountConfig(mountConfig{
		Mountpoint:   mountpoint,
		Auto:         true,
		DelaySeconds: 5,
	}); err != nil {
		http.Error(w, "Failed to persist mount config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	exe, err := os.Executable()
	if err != nil {
		http.Error(w, "Failed to locate executable: "+err.Error(), http.StatusInternalServerError)
		return
	}

	cmd := exec.Command(exe, "mount", "--config", h.configPath, "--mountpoint", mountpoint)
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// 0644 允许同组和其他用户读取
	logFile, err := os.OpenFile(filepath.Join(getPkgVar(), "mount.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	if err := cmd.Start(); err != nil {
		if logFile != nil {
			logFile.Close()
		}
		http.Error(w, "Failed to start mount: "+err.Error(), http.StatusInternalServerError)
		return
	}

	state := mountState{Pid: cmd.Process.Pid, Mountpoint: mountpoint}
	if err := h.writeMountState(state); err != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		http.Error(w, "Failed to persist mount state: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.writeMountPid(cmd.Process.Pid); err != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_ = h.clearMountState()
		http.Error(w, "Failed to persist mount pid: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"mounted":    true,
		"pid":        state.Pid,
		"mountpoint": state.Mountpoint,
	})
}

func (h *APIHandler) HandleUnmount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	state, alive := h.readMountState()
	if !alive {
		_ = h.clearMountState()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"mounted": false})
		return
	}

	p, err := os.FindProcess(state.Pid)
	if err != nil {
		_ = h.clearMountState()
		http.Error(w, "Failed to find mount process", http.StatusInternalServerError)
		return
	}
	_ = p.Signal(syscall.SIGTERM)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(state.Pid) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	_ = h.clearMountState()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"mounted": false})
}

type toolEncryptRequest struct {
	Input     string `json:"input"`
	OutputDir string `json:"output_dir"`
}

type toolExportRequest struct {
	OutputDir string `json:"output_dir"`
	ShareKey  string `json:"share_key"`
}

type toolImportRequest struct {
	Input    string `json:"input"`
	ShareKey string `json:"share_key"`
}

func generateRandomPassword() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 16
	password := make([]byte, length)
	_, _ = rand.Read(password)
	for i := range password {
		password[i] = charset[int(password[i])%len(charset)]
	}
	return string(password)
}

func writeToolJSON(w http.ResponseWriter, status int, msg string, details map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ToolResponse{OK: status >= 200 && status < 300, Message: msg, Details: details})
}

func (h *APIHandler) newLocalProxy() (*proxy.Proxy, metadata.Storage, *config.Config, error) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		return nil, nil, nil, err
	}
	meta, err := metadata.NewLocalStorage(cfg.Storage.MetadataPath)
	if err != nil {
		return nil, nil, nil, err
	}
	p, err := proxy.NewProxy(meta, nil, cfg.Security.MasterKey)
	if err != nil {
		return nil, nil, nil, err
	}
	return p, meta, cfg, nil
}

func (h *APIHandler) HandleToolEncrypt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req toolEncryptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeToolJSON(w, http.StatusBadRequest, "Invalid request body", nil)
		return
	}
	req.Input = filepath.Clean(strings.TrimSpace(req.Input))
	req.OutputDir = filepath.Clean(strings.TrimSpace(req.OutputDir))
	if req.Input == "" || req.OutputDir == "" {
		writeToolJSON(w, http.StatusBadRequest, "input and output_dir are required", nil)
		return
	}

	allowed := getAccessiblePaths()
	if len(allowed) == 0 {
		writeToolJSON(w, http.StatusBadRequest, "No accessible paths configured", nil)
		return
	}
	if !isUnderAllowedPath(req.Input, allowed) || !isUnderAllowedPath(req.OutputDir, allowed) {
		writeToolJSON(w, http.StatusForbidden, "Path not allowed", nil)
		return
	}
	if st, err := os.Stat(req.Input); err != nil || st == nil {
		writeToolJSON(w, http.StatusBadRequest, "Input path not found", nil)
		return
	}
	if err := os.MkdirAll(req.OutputDir, 0755); err != nil {
		writeToolJSON(w, http.StatusInternalServerError, "Failed to create output directory: "+err.Error(), nil)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	p, meta, _, err := h.newLocalProxy()
	if err != nil {
		writeToolJSON(w, http.StatusInternalServerError, "Failed to initialize: "+err.Error(), nil)
		return
	}
	defer func() { _ = meta.Close() }()
	if err := p.ExportLocal(req.Input, req.OutputDir); err != nil {
		writeToolJSON(w, http.StatusInternalServerError, "Encrypt failed: "+err.Error(), nil)
		return
	}
	writeToolJSON(w, http.StatusOK, "ok", map[string]interface{}{"output_dir": req.OutputDir})
}

func (h *APIHandler) HandleToolExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req toolExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeToolJSON(w, http.StatusBadRequest, "Invalid request body", nil)
		return
	}
	req.OutputDir = filepath.Clean(strings.TrimSpace(req.OutputDir))
	req.ShareKey = strings.TrimSpace(req.ShareKey)
	if req.OutputDir == "" {
		writeToolJSON(w, http.StatusBadRequest, "output_dir is required", nil)
		return
	}

	allowed := getAccessiblePaths()
	if len(allowed) == 0 {
		writeToolJSON(w, http.StatusBadRequest, "No accessible paths configured", nil)
		return
	}
	if !isUnderAllowedPath(req.OutputDir, allowed) {
		writeToolJSON(w, http.StatusForbidden, "Path not allowed", nil)
		return
	}
	if err := os.MkdirAll(req.OutputDir, 0755); err != nil {
		writeToolJSON(w, http.StatusInternalServerError, "Failed to create output directory: "+err.Error(), nil)
		return
	}

	shareKey := req.ShareKey
	if shareKey == "" {
		shareKey = generateRandomPassword()
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	p, meta, _, err := h.newLocalProxy()
	if err != nil {
		writeToolJSON(w, http.StatusInternalServerError, "Failed to initialize: "+err.Error(), nil)
		return
	}
	defer func() { _ = meta.Close() }()
	tarPath, err := p.CreateSharePackage([]string{"/"}, req.OutputDir, shareKey)
	if err != nil {
		writeToolJSON(w, http.StatusInternalServerError, "Export failed: "+err.Error(), nil)
		return
	}
	writeToolJSON(w, http.StatusOK, "ok", map[string]interface{}{"tar_path": tarPath, "share_key": shareKey})
}

func (h *APIHandler) HandleToolImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req toolImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeToolJSON(w, http.StatusBadRequest, "Invalid request body", nil)
		return
	}
	req.Input = filepath.Clean(strings.TrimSpace(req.Input))
	req.ShareKey = strings.TrimSpace(req.ShareKey)
	if req.Input == "" || req.ShareKey == "" {
		writeToolJSON(w, http.StatusBadRequest, "input and share_key are required", nil)
		return
	}

	allowed := getAccessiblePaths()
	if len(allowed) == 0 {
		writeToolJSON(w, http.StatusBadRequest, "No accessible paths configured", nil)
		return
	}
	if !isUnderAllowedPath(req.Input, allowed) {
		writeToolJSON(w, http.StatusForbidden, "Path not allowed", nil)
		return
	}
	st, err := os.Stat(req.Input)
	if err != nil || st == nil || st.IsDir() {
		writeToolJSON(w, http.StatusBadRequest, "Input tar not found", nil)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	p, meta, _, err := h.newLocalProxy()
	if err != nil {
		writeToolJSON(w, http.StatusInternalServerError, "Failed to initialize: "+err.Error(), nil)
		return
	}
	defer func() { _ = meta.Close() }()
	if err := p.ReceiveSharePackage(req.Input, req.ShareKey); err != nil {
		writeToolJSON(w, http.StatusInternalServerError, "Import failed: "+err.Error(), nil)
		return
	}
	writeToolJSON(w, http.StatusOK, "ok", nil)
}

func (h *APIHandler) readMountState() (mountState, bool) {
	path := filepath.Join(getPkgVar(), "mount.json")
	data, err := os.ReadFile(path)
	if err != nil {
		pid, pidOk := h.readMountPid()
		cfg, cfgOk := h.readMountConfig()
		if pidOk && cfgOk && pid > 0 && cfg.Mountpoint != "" && processAlive(pid) {
			return mountState{Pid: pid, Mountpoint: cfg.Mountpoint}, true
		}
		return mountState{}, false
	}
	var st mountState
	if err := json.Unmarshal(data, &st); err != nil {
		_ = os.Remove(path)
		return mountState{}, false
	}
	if st.Pid <= 0 || st.Mountpoint == "" {
		_ = os.Remove(path)
		return mountState{}, false
	}
	if !processAlive(st.Pid) {
		_ = os.Remove(path)
		return mountState{}, false
	}
	return st, true
}

func (h *APIHandler) writeMountState(st mountState) error {
	path := filepath.Join(getPkgVar(), "mount.json")
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func (h *APIHandler) clearMountState() error {
	path := filepath.Join(getPkgVar(), "mount.json")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_ = os.Remove(filepath.Join(getPkgVar(), "mount.pid"))
	return nil
}

func (h *APIHandler) writeMountPid(pid int) error {
	return os.WriteFile(filepath.Join(getPkgVar(), "mount.pid"), []byte(strconv.Itoa(pid)), 0600)
}

func (h *APIHandler) readMountPid() (int, bool) {
	data, err := os.ReadFile(filepath.Join(getPkgVar(), "mount.pid"))
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}
	return pid, true
}

func (h *APIHandler) writeMountConfig(cfg mountConfig) error {
	path := filepath.Join(getPkgVar(), "mount.config.json")
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func (h *APIHandler) readMountConfig() (mountConfig, bool) {
	path := filepath.Join(getPkgVar(), "mount.config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return mountConfig{}, false
	}
	var cfg mountConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return mountConfig{}, false
	}
	return cfg, true
}

func getPkgVar() string {
	if v := os.Getenv("TRIM_PKGVAR"); v != "" {
		return v
	}
	return os.TempDir()
}

func getAccessiblePaths() []string {
	pathsEnv := os.Getenv("ACCESSIBLE_PATHS")
	if pathsEnv == "" {
		pathsEnv = os.Getenv("TRIM_DATA_ACCESSIBLE_PATHS")
	}
	raw := strings.Split(pathsEnv, ":")
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func isUnderAllowedPath(target string, allowed []string) bool {
	target = filepath.Clean(target)
	for _, base := range allowed {
		base = filepath.Clean(base)
		if target == base {
			return true
		}
		if strings.HasPrefix(target, base+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func dirEmpty(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
