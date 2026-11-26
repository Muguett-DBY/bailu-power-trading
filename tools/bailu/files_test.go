package main

import (
	"os"
	"path/filepath"
	"testing"
)

// knowledgeFile reads a file of the framework's knowledge for a test: the
// tests run in the tool's folder, the knowledge is two levels above it.
func knowledgeFile(t *testing.T, parts ...string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(append([]string{filepath.Join("..", "..", "knowledge")}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
