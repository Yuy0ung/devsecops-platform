package sca

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type GoScanner struct{}

func (s *GoScanner) Name() string {
	return "Go"
}

func (s *GoScanner) Scan(dir string) ([]Dependency, error) {
	var deps []Dependency

	err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		if filepath.Base(path) == "go.mod" {
			d, err := parseGoMod(path)
			if err == nil {
				deps = append(deps, d...)
			}
		}
		return nil
	})

	return deps, err
}

func parseGoMod(path string) ([]Dependency, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var deps []Dependency
	scanner := bufio.NewScanner(file)
	inRequire := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		if strings.HasPrefix(line, "require (") {
			inRequire = true
			continue
		}
		if inRequire && line == ")" {
			inRequire = false
			continue
		}

		var parts []string
		if inRequire {
			parts = strings.Fields(line)
		} else if strings.HasPrefix(line, "require ") {
			parts = strings.Fields(strings.TrimPrefix(line, "require "))
		}

		if len(parts) >= 2 {
			// Handle version like v1.2.3 // indirect
			version := parts[1]
			// Clean version string? OSV expects standard semver usually.
			
			deps = append(deps, Dependency{
				Name:      parts[0],
				Version:   version,
				Language:  "go",
				FilePath:  path,
				Locations: []string{"go.mod"},
			})
		}
	}
	return deps, scanner.Err()
}
