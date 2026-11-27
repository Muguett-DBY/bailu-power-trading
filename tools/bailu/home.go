package main

// bailu is the tool of the framework: the trading calculators, the handbook,
// the published prices and the reading of price tables, for any agent to call.
// It reads the handbook and the prices from the framework it is part of,
// wherever that is, and keeps its cache there too: nothing is installed
// outside the project, and deleting the folder removes it all.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// frameworkDir gives the root of the framework: the one named by the caller,
// or the nearest directory above the working one, or above the tool's own
// place, that holds the handbook.
func frameworkDir(explicit string) (string, error) {
	if explicit == "" {
		explicit = os.Getenv("BAILU_HOME")
	}
	if explicit != "" {
		// The folder named is the framework, or the mistake is said to be one:
		// to look elsewhere would run on a framework the caller never meant.
		for dir := filepath.Clean(explicit); ; dir = filepath.Dir(dir) {
			if _, err := os.Stat(filepath.Join(dir, "knowledge", "handbook")); err == nil {
				return dir, nil
			}
			if parent := filepath.Dir(dir); parent == dir {
				break
			}
		}
		return "", fmt.Errorf("--home 或 BAILU_HOME 指明的目录（%s）和它的上层目录里没有 knowledge/handbook，不是框架所在目录；请指明 bailu-power-trading 文件夹本身", explicit)
	}
	var roots []string
	if working, err := os.Getwd(); err == nil {
		roots = append(roots, working)
	}
	if executable, err := os.Executable(); err == nil {
		roots = append(roots, filepath.Dir(executable))
	}
	for _, root := range roots {
		for dir := filepath.Clean(root); ; dir = filepath.Dir(dir) {
			if _, err := os.Stat(filepath.Join(dir, "knowledge", "handbook")); err == nil {
				return dir, nil
			}
			if parent := filepath.Dir(dir); parent == dir {
				break
			}
		}
	}
	return "", errors.New("找不到框架：当前目录和工具所在目录的上层都没有 knowledge/handbook。请在部署了框架的项目文件夹里运行，或用 --home 指明框架所在目录")
}
