package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// inputFile is a file the user gave with a question, as the tools read it.
type inputFile struct {
	ID   int
	Name string
	Kind string
	path string
}

// prepareInputs takes the paths of the files a question is about: a price
// table, a rule, a bill.
func prepareInputs(paths []string) ([]inputFile, error) {
	files := make([]inputFile, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		key := strings.ToLower(path)
		if _, exists := seen[key]; exists {
			continue
		}
		kind := strings.ToLower(filepath.Ext(path))
		switch kind {
		case ".csv", ".xlsx", ".pdf", ".docx":
			kind = kind[1:]
		default:
			return nil, fmt.Errorf("只认 CSV、Excel、PDF 或 Word 文件：%s", filepath.Base(path))
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("不是一个文件：%s", path)
		}
		seen[key] = struct{}{}
		files = append(files, inputFile{ID: len(files) + 1, Name: filepath.Base(path), Kind: kind, path: path})
	}
	return files, nil
}
