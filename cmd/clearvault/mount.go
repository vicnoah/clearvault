//go:build !fuse

package main

import "log"

func init() {
	commands["mount"] = handleMountStub
}

func handleMountStub(args []string) {
	log.Fatal("Error: This binary was built without FUSE support.")
}
