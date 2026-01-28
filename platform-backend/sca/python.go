package sca

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type PythonScanner struct{}

func (s *PythonScanner) Name() string {
	return "Python"
}

func (s *PythonScanner) Scan(dir string) ([]Dependency, error) {
	var deps []Dependency

	err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		if filepath.Base(path) == "requirements.txt" {
			d, err := parseRequirementsTxt(path)
			if err == nil {
				deps = append(deps, d...)
			}
		}
		return nil
	})

	return deps, err
}

func parseRequirementsTxt(path string) ([]Dependency, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var deps []Dependency
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Simple parsing: package==version
		// Also handle package>=version, package~=version
		// Ignore markers for now
		
		parts := strings.Split(line, "==")
		if len(parts) == 2 {
			deps = append(deps, Dependency{
				Name:      parts[0],
				Version:   parts[1],
				Language:  "python",
				FilePath:  path,
				Locations: []string{"requirements.txt"},
			})
			continue
		}
		
		// Fallback for >=
		parts = strings.Split(line, ">=")
		if len(parts) == 2 {
			deps = append(deps, Dependency{
				Name:      parts[0],
				Version:   parts[1],
				Language:  "python",
				FilePath:  path,
				Locations: []string{"requirements.txt"},
			})
			continue
		}
	}
	return deps, scanner.Err()
}
