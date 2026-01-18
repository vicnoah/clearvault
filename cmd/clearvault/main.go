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
	inShort := flag.String("in", "", "Path to file or directory to export")
	outShort := flag.String("out", "", "Directory to write encrypted files")
	exportInputLong := flag.String("export-input", "", "")
	exportOutputLong := flag.String("export-output", "", "")
	flag.Parse()

	exportInput := *inShort
	if exportInput == "" {
		exportInput = *exportInputLong
	}
	exportOutput := *outShort
	if exportOutput == "" {
		exportOutput = *exportOutputLong
	}

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
