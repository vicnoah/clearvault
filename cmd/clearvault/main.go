package main

import (
	"clearvault/internal/config"
	"clearvault/internal/metadata"
	"clearvault/internal/proxy"
	dav "clearvault/internal/webdav"
	"flag"
	"log"
	"net/http"

	"golang.org/x/net/webdav"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	var meta metadata.Storage
	switch cfg.Storage.MetadataType {
	case "sqlite":
		meta, err = metadata.NewSqliteStorage(cfg.Storage.MetadataPath)
	case "local", "":
		meta, err = metadata.NewLocalStorage(cfg.Storage.MetadataPath)
	default:
		log.Fatalf("Unknown metadata type: %s", cfg.Storage.MetadataType)
	}

	if err != nil {
		log.Fatalf("Failed to initialize metadata storage: %v", err)
	}
	defer meta.Close()

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
