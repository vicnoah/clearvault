//go:build fuse

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"clearvault/internal/config"
	cvfuse "clearvault/internal/fuse"
	"clearvault/internal/metadata"
	"clearvault/internal/proxy"
	"clearvault/internal/remote"

	"github.com/winfsp/cgofuse/fuse"
)

func init() {
	commands["mount"] = handleMount
}

func handleMount(args []string) {
	cmd := flag.NewFlagSet("mount", flag.ExitOnError)
	configPath := cmd.String("config", "config.yaml", "配置文件路径")
	mountpoint := cmd.String("mountpoint", "", "挂载点路径")
	help := cmd.Bool("help", false, "显示帮助信息")

	cmd.Parse(args)

	if *help {
		printMountUsage()
		return
	}

	if *mountpoint == "" {
		log.Fatal("Error: --mountpoint parameter is required")
	}

	// 自动推断并设置 FUSE_UID/FUSE_GID (如果未设置)
	// 这确保挂载的文件系统归属于挂载点目录的拥有者，而不是 root
	setupFuseOwner(*mountpoint)

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 检查初始化状态
	if cfg.Security.MasterKey == "" || cfg.Security.MasterKey == "CHANGE-THIS-TO-A-SECURE-32BYTE-KEY" {
		log.Fatal("Error: Master Key not initialized. Please run server and configure via Web UI first.")
	}

	// 初始化组件
	meta, err := metadata.NewLocalStorage(cfg.Storage.MetadataPath)
	if err != nil {
		log.Fatalf("Failed to initialize metadata storage: %v", err)
	}
	defer meta.Close()

	remoteStorage, err := remote.NewRemoteStorage(cfg.Remote)
	if err != nil {
		log.Fatalf("Failed to create remote storage: %v", err)
	}
	defer remoteStorage.Close()

	p, err := proxy.NewProxy(meta, remoteStorage, cfg.Security.MasterKey)
	if err != nil {
		log.Fatalf("Failed to initialize proxy: %v", err)
	}

	// 创建 FUSE 文件系统
	// NewClearVaultFS 内部会读取 FUSE_UID/FUSE_GID 环境变量
	fs := cvfuse.NewClearVaultFS(p)
	host := fuse.NewFileSystemHost(fs)

	// 挂载选项
	// allow_other: 允许其他用户访问 (必须，否则 NAS/Samba 无法访问)
	// default_permissions: 让内核检查权限 (配合 allow_other 使用，增加安全性)
	// auto_unmount: 进程退出时自动卸载
	options := []string{
		"-o", "allow_other",
		"-o", "default_permissions",
		"-o", "auto_unmount",
	}

	// 监听信号以优雅退出
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("Received signal, unmounting...")
		host.Unmount()
	}()

	log.Printf("Mounting FUSE at %s", *mountpoint)
	if !host.Mount(*mountpoint, options) {
		log.Fatal("Mount failed")
	}
}

func setupFuseOwner(mountpoint string) {
	// 如果环境变量已设置，则跳过
	if os.Getenv("FUSE_UID") != "" && os.Getenv("FUSE_GID") != "" {
		return
	}

	// 获取挂载点的文件信息
	info, err := os.Stat(mountpoint)
	if err != nil {
		log.Printf("Warning: Failed to stat mountpoint: %v. Using default UID/GID.", err)
		return
	}

	// 从 Sys() 中获取 UID/GID (Linux specific)
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		uid := stat.Uid
		gid := stat.Gid

		if os.Getenv("FUSE_UID") == "" {
			os.Setenv("FUSE_UID", fmt.Sprintf("%d", uid))
			log.Printf("Auto-detected mountpoint owner: UID=%d", uid)
		}
		if os.Getenv("FUSE_GID") == "" {
			os.Setenv("FUSE_GID", fmt.Sprintf("%d", gid))
			log.Printf("Auto-detected mountpoint group: GID=%d", gid)
		}
	}
}

func printMountUsage() {
	log.Println("Usage: clearvault mount [options]")
	log.Println("")
	log.Println("Mount encrypted storage via FUSE")
	log.Println("")
	log.Println("Options:")
	log.Println("  --config string     配置文件路径 (default \"config.yaml\")")
	log.Println("  --mountpoint string 挂载点路径 (required)")
	log.Println("  --help              显示帮助信息")
}
