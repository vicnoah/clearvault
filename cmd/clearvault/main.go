package main

import (
	"crypto/rand"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"

	"clearvault/internal/config"
	"clearvault/internal/metadata"
	"clearvault/internal/proxy"
	"clearvault/internal/remote"
	dav "clearvault/internal/webdav"
	"golang.org/x/net/webdav"
)

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
	serverHelp := serverCmd.Bool("help", false, "显示帮助信息")

	// 5. 检查是否有命令参数
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	// 6. 根据命令类型分发
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
		meta, err := metadata.NewLocalStorage(cfg.Storage.MetadataPath)
		if err != nil {
			log.Fatalf("Failed to initialize metadata storage: %v", err)
		}
		defer meta.Close()
		handleServer(cfg, meta)

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

// handleServer - WebDAV 服务器
func handleServer(cfg *config.Config, meta metadata.Storage) {
	// 使用工厂模式创建远程存储客户端
	remoteStorage, err := remote.NewRemoteStorage(cfg.Remote)
	if err != nil {
		log.Fatalf("Failed to create remote storage: %v", err)
	}
	defer remoteStorage.Close()

	p, err := proxy.NewProxy(meta, remoteStorage, cfg.Security.MasterKey)
	if err != nil {
		log.Fatalf("Failed to initialize proxy: %v", err)
	}

	fs := proxy.NewFileSystem(p)
	ls := webdav.NewMemLS()
	pattern := cfg.Server.BaseURL
	if pattern != "" && pattern[len(pattern)-1] != '/' {
		pattern += "/"
	}

	server := dav.NewLocalServer(cfg.Server.BaseURL, fs, ls, cfg.Server.Auth.User, cfg.Server.Auth.Pass)

	log.Printf("Clearvault listening on %s at %s (webdav prefix: %s)", cfg.Server.Listen, pattern, cfg.Server.BaseURL)
	http.Handle(pattern, server)
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
