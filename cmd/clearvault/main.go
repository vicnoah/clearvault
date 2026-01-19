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
	dav "clearvault/internal/webdav"
	"golang.org/x/net/webdav"
)

func main() {
	// 基础配置
	configPath := flag.String("config", "config.yaml", "Path to config file")

	// 导出命令
	exportCmd := flag.NewFlagSet("export", flag.ExitOnError)
	exportPaths := exportCmd.String("paths", "", "虚拟路径列表（逗号分隔）")
	exportOutput := exportCmd.String("output", "", "输出目录")
	exportShareKey := exportCmd.String("share-key", "", "分享密钥（可选，不指定则自动生成）")

	// 导入命令
	importCmd := flag.NewFlagSet("import", flag.ExitOnError)
	importInput := importCmd.String("input", "", "输入 tar 文件路径")
	importShareKey := importCmd.String("share-key", "", "分享密钥")

	// 旧版导出命令（兼容）
	inShort := flag.String("in", "", "Path to file or directory to export")
	outShort := flag.String("out", "", "Directory to write encrypted files")
	exportInputLong := flag.String("export-input", "", "")
	exportOutputLong := flag.String("export-output", "", "")

	flag.Parse()

	if len(os.Args) < 2 {
		printUsage()
		return
	}

	// 加载配置
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 初始化元数据存储
	meta, err := metadata.NewLocalStorage(cfg.Storage.MetadataPath)
	if err != nil {
		log.Fatalf("Failed to initialize metadata storage: %v", err)
	}
	defer meta.Close()

	switch os.Args[1] {
	case "export":
		exportCmd.Parse(os.Args[2:])
		handleExport(exportCmd, cfg, meta, exportPaths, exportOutput, exportShareKey)

	case "import":
		importCmd.Parse(os.Args[2:])
		handleImport(importCmd, cfg, meta, importInput, importShareKey)

	default:
		// 旧版命令兼容
		handleLegacyExport(cfg, meta, inShort, outShort, exportInputLong, exportOutputLong)
	}
}

func printUsage() {
	log.Println("Usage:")
	log.Println("  clearvault export --paths \"/documents/report.pdf\" --output /tmp/export [--share-key \"password\"]")
	log.Println("  clearvault import --input /tmp/share_abc123.tar --share-key \"password\"")
	log.Println("  clearvault -in /path/to/file -out /output/dir  (legacy)")
	log.Println("  clearvault  (start webdav server)")
}

func handleExport(cmd *flag.FlagSet, cfg *config.Config, meta metadata.Storage, exportPaths, exportOutput, exportShareKey *string) {
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

func handleLegacyExport(cfg *config.Config, meta metadata.Storage, inShort, outShort, exportInputLong, exportOutputLong *string) {
	exportInput := *inShort
	if exportInput == "" {
		exportInput = *exportInputLong
	}
	exportOutput := *outShort
	if exportOutput == "" {
		exportOutput = *exportOutputLong
	}

	if exportInput != "" || exportOutput != "" {
		if exportInput == "" || exportOutput == "" {
			log.Fatalf("Both -in and -out (or -export-input and -export-output) must be specified")
		}
		p, err := proxy.NewProxy(meta, nil, cfg.Security.MasterKey)
		if err != nil {
			log.Fatalf("Failed to initialize export proxy: %v", err)
		}
		if err := p.ExportLocal(exportInput, exportOutput); err != nil {
			log.Fatalf("Export failed: %v", err)
		}
		return
	}

	// 启动 WebDAV 服务器
	remote := dav.NewRemoteClient(cfg.Remote.URL, cfg.Remote.User, cfg.Remote.Pass)

	p, err := proxy.NewProxy(meta, remote, cfg.Security.MasterKey)
	if err != nil {
		log.Fatalf("Failed to initialize proxy: %v", err)
	}

	fs := proxy.NewFileSystem(p)
	ls := webdav.NewMemLS()
	pattern := cfg.Server.BaseURL
	if pattern != "" && pattern[len(pattern)-1] != '/' {
		pattern += "/"
	}

	server := dav.NewLocalServer(cfg.Server.BaseURL, fs, ls)

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
