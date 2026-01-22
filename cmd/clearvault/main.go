package main

import (
	"crypto/rand"
	"flag"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"clearvault/internal/api"
	"clearvault/internal/config"
	"clearvault/internal/metadata"
	"clearvault/internal/proxy"
	"clearvault/internal/remote"
	dav "clearvault/internal/webdav"

	"golang.org/x/net/webdav"
)

var commands = map[string]func([]string){}

func main() {
	// 1. encrypt 子命令参数（本地文件加密）
	encryptCmd := flag.NewFlagSet("encrypt", flag.ExitOnError)
	encryptConfigPath := encryptCmd.String("config", "config.yaml", "配置文件路径")
	encryptInput := encryptCmd.String("in", "", "要加密的本地文件/目录路径")
	encryptOutput := encryptCmd.String("out", "", "加密文件输出目录")
	encryptHelp := encryptCmd.Bool("help", false, "显示帮助信息")

	// 2. export 子命令参数（元数据导出）
	exportCmd := flag.NewFlagSet("export", flag.ExitOnError)
	exportConfigPath := exportCmd.String("config", "config.yaml", "配置文件路径")
	exportPaths := exportCmd.String("paths", "", "虚拟路径列表（逗号分隔）")
	exportOutput := exportCmd.String("output", "", "输出目录")
	exportShareKey := exportCmd.String("share-key", "", "分享密钥（可选，不指定则自动生成）")
	exportHelp := exportCmd.Bool("help", false, "显示帮助信息")

	// 3. import 子命令参数（元数据导入）
	importCmd := flag.NewFlagSet("import", flag.ExitOnError)
	importConfigPath := importCmd.String("config", "config.yaml", "配置文件路径")
	importInput := importCmd.String("input", "", "输入 tar 文件路径")
	importShareKey := importCmd.String("share-key", "", "分享密钥")
	importHelp := importCmd.Bool("help", false, "显示帮助信息")

	// 4. server 子命令参数（WebDAV 服务器）
	serverCmd := flag.NewFlagSet("server", flag.ExitOnError)
	serverConfigPath := serverCmd.String("config", "config.yaml", "配置文件路径")
	serverInit := serverCmd.Bool("init", true, "如果密钥不存在，是否自动生成并初始化 (默认 true)")
	serverUIPath := serverCmd.String("ui", "", "UI 静态资源目录路径（用于管理面板）")
	serverHelp := serverCmd.Bool("help", false, "显示帮助信息")

	// 5. 检查是否有命令参数
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	// 6. 根据命令类型分发
	if cmdFunc, ok := commands[os.Args[1]]; ok {
		cmdFunc(os.Args[2:])
		return
	}

	switch os.Args[1] {
	case "encrypt":
		encryptCmd.Parse(os.Args[2:])
		if *encryptHelp {
			printEncryptUsage()
			return
		}
		cfg, err := config.LoadConfig(*encryptConfigPath)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
		meta, err := metadata.NewLocalStorage(cfg.Storage.MetadataPath)
		if err != nil {
			log.Fatalf("Failed to initialize metadata storage: %v", err)
		}
		defer meta.Close()
		handleEncrypt(encryptCmd, cfg, meta, encryptInput, encryptOutput)

	case "export":
		exportCmd.Parse(os.Args[2:])
		if *exportHelp {
			printExportUsage()
			return
		}
		cfg, err := config.LoadConfig(*exportConfigPath)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
		meta, err := metadata.NewLocalStorage(cfg.Storage.MetadataPath)
		if err != nil {
			log.Fatalf("Failed to initialize metadata storage: %v", err)
		}
		defer meta.Close()
		handleExport(exportCmd, cfg, meta, exportPaths, exportOutput, exportShareKey)

	case "import":
		importCmd.Parse(os.Args[2:])
		if *importHelp {
			printImportUsage()
			return
		}
		cfg, err := config.LoadConfig(*importConfigPath)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
		meta, err := metadata.NewLocalStorage(cfg.Storage.MetadataPath)
		if err != nil {
			log.Fatalf("Failed to initialize metadata storage: %v", err)
		}
		defer meta.Close()
		handleImport(importCmd, cfg, meta, importInput, importShareKey)

	case "config":
		configPath, rest := extractConfigPath(os.Args[2:])
		if len(rest) < 1 {
			log.Fatalf("Usage: clearvault config [--config <path>] <subcommand> [args]")
		}
		switch rest[0] {
		case "set-token":
			handleConfigSetToken(configPath, rest[1:])
		default:
			log.Fatalf("Unknown config subcommand: %s", rest[0])
		}

	case "server":
		serverCmd.Parse(os.Args[2:])
		if *serverHelp {
			printServerUsage()
			return
		}
		cfg, err := config.LoadConfig(*serverConfigPath)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}

		// 检查是否需要初始化
		if cfg.Security.MasterKey == "" || cfg.Security.MasterKey == "CHANGE-THIS-TO-A-SECURE-32BYTE-KEY" {
			if *serverInit {
				log.Println("Initializing Master Key...")
				if err := config.GenerateMasterKey(*serverConfigPath, cfg); err != nil {
					log.Fatalf("Failed to generate master key: %v", err)
				}
			} else {
				log.Println("Starting in UNINITIALIZED mode (waiting for setup)...")
			}
		}

		// 如果未初始化，meta 和 remoteStorage 将无法正常工作，需要在 handleServer 中处理
		// 我们将初始化逻辑移交给 handleServer
		handleServer(cfg, *serverConfigPath, *serverUIPath)

	default:
		printUsage()
		return
	}
}

func printUsage() {
	log.Println("ClearVault - Encrypted WebDAV Storage")
	log.Println("")
	log.Println("Usage:")
	log.Println("  clearvault <command> [command options]")
	log.Println("")
	log.Println("Commands:")
	log.Println("  encrypt   Encrypt local files/directories (offline)")
	log.Println("  export    Export metadata to encrypted share package")
	log.Println("  import    Import metadata from encrypted share package")
	log.Println("  mount     Mount encrypted storage via FUSE")
	log.Println("  server    Start WebDAV server")
	log.Println("")
	log.Println("Examples:")
	log.Println("  clearvault encrypt -in /path/to/file -out /output/dir")
	log.Println("  clearvault export --paths \"/documents\" --output /tmp/export")
	log.Println("  clearvault import --input /tmp/share.tar --share-key \"password\"")
	log.Println("  clearvault server --config config.yaml")
	log.Println("")
	log.Println("Use 'clearvault <command> --help' for more information about a command.")
}

func printEncryptUsage() {
	log.Println("Usage: clearvault encrypt [options]")
	log.Println("")
	log.Println("Encrypt local files/directories (offline operation)")
	log.Println("Uses master_key from config to encrypt files directly")
	log.Println("")
	log.Println("Options:")
	log.Println("  --config string     配置文件路径 (default \"config.yaml\")")
	log.Println("  -in string          要加密的本地文件/目录路径")
	log.Println("  -out string         加密文件输出目录")
	log.Println("  --help              显示帮助信息")
	log.Println("")
	log.Println("Examples:")
	log.Println("  clearvault encrypt -in /path/to/file -out /output/dir")
	log.Println("  clearvault encrypt -in /data -out /encrypted --config config-s3.yaml")
}

func printExportUsage() {
	log.Println("Usage: clearvault export [options]")
	log.Println("")
	log.Println("Export metadata to encrypted share package")
	log.Println("Generates a password-protected tar archive")
	log.Println("")
	log.Println("Options:")
	log.Println("  --config string     配置文件路径 (default \"config.yaml\")")
	log.Println("  --paths string      虚拟路径列表（逗号分隔）")
	log.Println("  --output string     输出目录")
	log.Println("  --share-key string  分享密钥（可选，不指定则自动生成）")
	log.Println("  --help              显示帮助信息")
	log.Println("")
	log.Println("Examples:")
	log.Println("  clearvault export --paths \"/documents,/photos\" --output /tmp/export")
	log.Println("  clearvault export --paths \"/\" --output output --share-key \"mypassword\"")
}

func printImportUsage() {
	log.Println("Usage: clearvault import [options]")
	log.Println("")
	log.Println("Import metadata from encrypted share package")
	log.Println("Restores metadata to local storage")
	log.Println("")
	log.Println("Options:")
	log.Println("  --config string     配置文件路径 (default \"config.yaml\")")
	log.Println("  --input string      输入 tar 文件路径")
	log.Println("  --share-key string  分享密钥")
	log.Println("  --help              显示帮助信息")
	log.Println("")
	log.Println("Examples:")
	log.Println("  clearvault import --input /tmp/share.tar --share-key \"password\"")
	log.Println("  clearvault import --config config-s3.yaml --input /tmp/share.tar --share-key \"password\"")
}

func printServerUsage() {
	log.Println("Usage: clearvault server [options]")
	log.Println("")
	log.Println("Start WebDAV server for encrypted storage access")
	log.Println("")
	log.Println("Options:")
	log.Println("  --config string     配置文件路径 (default \"config.yaml\")")
	log.Println("  --ui string         UI 静态资源目录路径（可选）")
	log.Println("  --init bool         如果密钥不存在，是否自动生成并初始化 (default true)")
	log.Println("  --help              显示帮助信息")
	log.Println("")
	log.Println("Examples:")
	log.Println("  clearvault server --config config.yaml")
	log.Println("  clearvault server --config config-s3.yaml")
}

// handleEncrypt - 本地文件加密
func handleEncrypt(cmd *flag.FlagSet, cfg *config.Config, meta metadata.Storage, encryptInput, encryptOutput *string) {
	// 验证必需参数
	if *encryptInput == "" {
		log.Fatalf("Error: -in parameter is required")
	}
	if *encryptOutput == "" {
		log.Fatalf("Error: -out parameter is required")
	}

	// 初始化代理（不需要远程连接）
	p, err := proxy.NewProxy(meta, nil, cfg.Security.MasterKey)
	if err != nil {
		log.Fatalf("Failed to initialize proxy: %v", err)
	}

	// 调用 ExportLocal 进行本地文件加密
	if err := p.ExportLocal(*encryptInput, *encryptOutput); err != nil {
		log.Fatalf("Export failed: %v", err)
	}

	log.Printf("✅ Local encryption completed: %s -> %s", *encryptInput, *encryptOutput)
	log.Println("⚠️  Note: Encrypted files in output directory must be uploaded to your configured remote storage (e.g., S3, WebDAV) for 'server' command to access them.")
}

func handleExport(cmd *flag.FlagSet, cfg *config.Config, meta metadata.Storage, exportPaths, exportOutput, exportShareKey *string) {
	// 验证必需参数
	if *exportPaths == "" {
		log.Fatalf("Error: --paths parameter is required")
	}
	if *exportOutput == "" {
		log.Fatalf("Error: --output parameter is required")
	}

	// 解析路径
	paths := strings.Split(*exportPaths, ",")

	// 初始化代理（不需要远程连接）
	p, err := proxy.NewProxy(meta, nil, cfg.Security.MasterKey)
	if err != nil {
		log.Fatalf("Failed to initialize proxy: %v", err)
	}

	// 生成随机密码（如果未指定）
	shareKey := *exportShareKey
	if shareKey == "" {
		shareKey = generateRandomPassword()
		log.Printf("Generated random share key: %s", shareKey)
	}

	// 创建分享包
	tarPath, err := p.CreateSharePackage(paths, *exportOutput, shareKey)
	if err != nil {
		log.Fatalf("Failed to create share package: %v", err)
	}

	log.Printf("✅ Share package created: %s", tarPath)
	log.Printf("🔑 Share Key: %s", shareKey)
}

func handleImport(cmd *flag.FlagSet, cfg *config.Config, meta metadata.Storage, importInput, importShareKey *string) {
	// 验证必需参数
	if *importInput == "" {
		log.Fatalf("Error: --input parameter is required")
	}
	if *importShareKey == "" {
		log.Fatalf("Error: --share-key parameter is required")
	}

	// 初始化代理（不需要远程连接）
	p, err := proxy.NewProxy(meta, nil, cfg.Security.MasterKey)
	if err != nil {
		log.Fatalf("Failed to initialize proxy: %v", err)
	}

	// 接收分享包
	err = p.ReceiveSharePackage(*importInput, *importShareKey)
	if err != nil {
		log.Fatalf("Failed to receive share package: %v", err)
	}

	log.Printf("✅ Share package imported successfully")
}

func extractConfigPath(args []string) (string, []string) {
	configPath := "config.yaml"
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--config" || a == "-config" {
			if i+1 >= len(args) {
				log.Fatalf("Missing value for %s", a)
			}
			configPath = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(a, "--config=") {
			configPath = strings.TrimPrefix(a, "--config=")
			continue
		}
		rest = append(rest, a)
	}
	return configPath, rest
}

func handleConfigSetToken(configPath string, args []string) {
	if len(args) < 1 {
		log.Fatalf("Usage: clearvault config [--config <path>] set-token <token>")
	}
	token := args[0]

	// Load existing config or create new if not exists
	var cfg *config.Config
	var err error

	// Check if file exists
	if _, errStat := os.Stat(configPath); os.IsNotExist(errStat) {
		// Create default config
		cfg = &config.Config{
			Server: config.ServerConfig{
				Listen: ":8080",
				Auth:   config.Auth{User: "", Pass: ""},
			},
			Remote: config.RemoteConfig{
				Type:      "local", // 默认为本地存储，避免首次启动 webdav URL 报错
				LocalPath: "./data",
			},
			Security: config.SecurityConfig{
				MasterKey: "CHANGE-THIS-TO-A-SECURE-32BYTE-KEY", // Will be auto-generated later if needed
			},
		}
	} else {
		cfg, err = config.LoadConfig(configPath)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
	}

	// Update token
	cfg.Access.Token = token

	// Save
	if err := config.SaveConfig(configPath, cfg); err != nil {
		log.Fatalf("Failed to save config: %v", err)
	}

	log.Printf("✅ Access token updated in %s", configPath)
}

// handleServer - WebDAV 服务器
func handleServer(cfg *config.Config, configPath string, uiPath string) {
	// 检查是否已初始化
	isInitialized := cfg.Security.MasterKey != "" && cfg.Security.MasterKey != "CHANGE-THIS-TO-A-SECURE-32BYTE-KEY"

	var meta metadata.Storage
	var remoteStorage remote.RemoteStorage
	var p *proxy.Proxy
	var err error

	// 仅在已初始化时加载组件
	if isInitialized {
		meta, err = metadata.NewLocalStorage(cfg.Storage.MetadataPath)
		if err != nil {
			log.Fatalf("Failed to initialize metadata storage: %v", err)
		}
		defer meta.Close()

		// 使用工厂模式创建远程存储客户端
		remoteStorage, err = remote.NewRemoteStorage(cfg.Remote)
		if err != nil {
			log.Fatalf("Failed to create remote storage: %v", err)
		}
		defer remoteStorage.Close()

		p, err = proxy.NewProxy(meta, remoteStorage, cfg.Security.MasterKey)
		if err != nil {
			log.Fatalf("Failed to initialize proxy: %v", err)
		}
	}

	// 即使未初始化，也启动 HTTP 服务以便进行 Setup
	// 这里的 fs 和 ls 在未初始化时可能为 nil，需要 apiHandler 处理
	var fs webdav.FileSystem
	var ls webdav.LockSystem

	if isInitialized {
		fs = proxy.NewFileSystem(p)
		ls = webdav.NewMemLS()
	}

	pattern := cfg.Server.BaseURL
	if pattern != "" && pattern[len(pattern)-1] != '/' {
		pattern += "/"
	}

	// API Handler 需要感知初始化状态
	apiHandler := api.NewAPIHandler(configPath) // 内部不再自动生成 Key

	// 注册 API 路由
	http.HandleFunc("/api/v1/status", apiHandler.AuthMiddleware(apiHandler.HandleStatus))
	http.HandleFunc("/api/v1/config", apiHandler.AuthMiddleware(apiHandler.HandleConfig))
	http.HandleFunc("/api/v1/paths", apiHandler.AuthMiddleware(apiHandler.HandlePaths))
	http.HandleFunc("/api/v1/mount/status", apiHandler.AuthMiddleware(apiHandler.HandleMountStatus))
	http.HandleFunc("/api/v1/mount", apiHandler.AuthMiddleware(apiHandler.HandleMount))
	http.HandleFunc("/api/v1/unmount", apiHandler.AuthMiddleware(apiHandler.HandleUnmount))
	http.HandleFunc("/api/v1/tools/encrypt", apiHandler.AuthMiddleware(apiHandler.HandleToolEncrypt))
	http.HandleFunc("/api/v1/tools/export", apiHandler.AuthMiddleware(apiHandler.HandleToolExport))
	http.HandleFunc("/api/v1/tools/import", apiHandler.AuthMiddleware(apiHandler.HandleToolImport))

	if strings.TrimSpace(uiPath) != "" {
		absUI, err := filepath.Abs(uiPath)
		if err == nil {
			uiPath = absUI
		}
		if st, err := os.Stat(uiPath); err == nil && st.IsDir() {
			fileServer := http.FileServer(http.Dir(uiPath))
			http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/api/v1/") {
					http.NotFound(w, r)
					return
				}
				if cfg.Server.BaseURL != "" && strings.HasPrefix(r.URL.Path, cfg.Server.BaseURL) {
					http.NotFound(w, r)
					return
				}

				if r.URL.Path == "/" {
					http.ServeFile(w, r, filepath.Join(uiPath, "index.html"))
					return
				}

				rel := strings.TrimPrefix(r.URL.Path, "/")
				rel = path.Clean(rel)
				rel = strings.TrimPrefix(rel, "../")
				full := filepath.Clean(filepath.Join(uiPath, rel))
				uiRoot := filepath.Clean(uiPath)
				if full != uiRoot && !strings.HasPrefix(full, uiRoot+string(os.PathSeparator)) {
					http.NotFound(w, r)
					return
				}
				if _, err := os.Stat(full); err == nil {
					fileServer.ServeHTTP(w, r)
					return
				}
				ext := strings.ToLower(path.Ext(r.URL.Path))
				accept := strings.ToLower(r.Header.Get("Accept"))
				if ext == "" || ext == ".html" || strings.Contains(accept, "text/html") {
					http.ServeFile(w, r, filepath.Join(uiPath, "index.html"))
					return
				}
				http.NotFound(w, r)
			})
		} else {
			log.Printf("UI path is not a directory: %s", uiPath)
		}
	}

	log.Printf("Clearvault listening on %s at %s (webdav prefix: %s)", cfg.Server.Listen, pattern, cfg.Server.BaseURL)

	if isInitialized {
		// 只有初始化后才挂载 WebDAV
		server := dav.NewLocalServer(cfg.Server.BaseURL, fs, ls, cfg.Server.Auth.User, cfg.Server.Auth.Pass)
		http.Handle(pattern, server)
	} else {
		// 未初始化时，WebDAV 路径返回 503
		http.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Service Uninitialized. Please configure via Web UI.", http.StatusServiceUnavailable)
		})
	}

	// 启动 HTTP 服务
	// 注意：如果我们在运行时初始化（通过 API），目前架构很难热加载 WebDAV Handler。
	// 简单做法：初始化后要求重启服务。
	// 或者：API Handler 处理初始化后，退出进程（由 supervisor 重启）？
	// 既然 fnOS 有回调脚本，我们可以让 API 保存配置后，由前端提示用户或自动重启。
	// 这里我们保持简单：未初始化只能访问 Config/Status API。

	if err := http.ListenAndServe(cfg.Server.Listen, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// generateRandomPassword 生成随机密码（16位）
func generateRandomPassword() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 16

	password := make([]byte, length)
	if _, err := rand.Read(password); err != nil {
		log.Fatalf("Failed to generate random password: %v", err)
	}

	for i := range password {
		password[i] = charset[int(password[i])%len(charset)]
	}

	return string(password)
}
