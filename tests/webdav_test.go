package tests

import (
	"fmt"
	"testing"

	"github.com/studio-b12/gowebdav"
)

func TestWebDAVConnect(t *testing.T) {
	url := "https://pan.vicno.cc/dav/115/clearvault"
	user := "admin"
	pass := "9776586516"

	c := gowebdav.NewClient(url, user, pass)
	err := c.Connect()
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	files, err := c.ReadDir("/")
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}

	fmt.Printf("Found %d files\n", len(files))
	for _, f := range files {
		fmt.Printf(" - %s\n", f.Name())
	}
}
