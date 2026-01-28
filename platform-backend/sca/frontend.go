package sca

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type FrontendScanner struct{}

func (s *FrontendScanner) Name() string {
	return "Frontend"
}

func (s *FrontendScanner) Scan(dir string) ([]Dependency, error) {
	var deps []Dependency

	err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		if filepath.Base(path) == "package.json" {
			// Skip node_modules to avoid massive duplicate scanning
			if strings.Contains(path, "node_modules") {
				return nil
			}
			d, err := parsePackageJson(path)
			if err == nil {
				deps = append(deps, d...)
			}
		}
		return nil
	})

	return deps, err
}

type PackageJSON struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

func parsePackageJson(path string) ([]Dependency, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var pkg PackageJSON
	if err := json.NewDecoder(file).Decode(&pkg); err != nil {
		return nil, err
	}

	var deps []Dependency
	for name, ver := range pkg.Dependencies {
		deps = append(deps, Dependency{
			Name:      name,
			Version:   cleanVersion(ver),
			Language:  "javascript",
			FilePath:  path,
			Locations: []string{"package.json"},
		})
	}
	for name, ver := range pkg.DevDependencies {
		deps = append(deps, Dependency{
			Name:      name,
			Version:   cleanVersion(ver),
			Language:  "javascript",
			FilePath:  path,
			Locations: []string{"package.json"},
		})
	}
	return deps, nil
}

func cleanVersion(v string) string {
	// Remove ^, ~
	v = strings.TrimPrefix(v, "^")
	v = strings.TrimPrefix(v, "~")
	return v
}
