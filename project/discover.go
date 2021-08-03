package project

import (
	"path/filepath"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/project/manifest"
	"github.com/nokia/ntt/project/suiteindex"
)

// Discover walks towards the file system root and collects
// known test suite layouts.
//
// This initial version of Discover returns a string slice, but this may change
// in future releases.
func Discover(path string) []string {

	var list []string

	fs.WalkUp(path, func(path string) bool {
		// Check source directories
		if file := filepath.Join(path, manifest.Name); fs.IsRegular(file) {
			list = append(list, path)
		}
		list = append(list, readSuites(filepath.Join(path, suiteindex.Name))...)

		// Check build directories
		for _, file := range fs.Glob(path + "/*build*/" + suiteindex.Name) {
			list = append(list, readSuites(file)...)
		}
		for _, file := range fs.Glob(path + "/build/native/*/sct/" + suiteindex.Name) {
			list = append(list, readSuites(file)...)
		}
		return true
	})

	// Remove duplicate entries
	result := make([]string, 0, len(list))
	visited := make(map[string]bool)
	for _, v := range list {
		if !visited[v] {
			visited[v] = true
			result = append(result, v)
		}
	}
	return result
}

func readSuites(file string) []string {
	var list []string

	si, err := suiteindex.ReadFile(file)
	if err != nil {
		return nil
	}

	for _, suite := range si.Suites {
		if suite.RootDir != "" {
			if file := filepath.Join(suite.RootDir, manifest.Name); fs.IsRegular(file) {
				list = append(list, suite.RootDir)
			}
		}
	}

	return list
}