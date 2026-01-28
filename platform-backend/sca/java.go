package sca

import (
	"archive/zip"
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
)

type JavaScanner struct{}

func (s *JavaScanner) Name() string {
	return "Java"
}

func (s *JavaScanner) Scan(dir string) ([]Dependency, error) {
	var deps []Dependency
	fmt.Printf("[SCA] Starting Java Scan in directory: %s\n", dir)

	err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Scan Jar/War (Deep Scan)
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".jar" || ext == ".war" {
			fmt.Printf("[SCA] Scanning Jar/War: %s\n", path)
			d, err := scanJar(path)
			if err == nil {
				if len(d) > 0 {
					fmt.Printf("[SCA] Found %d dependencies in %s\n", len(d), filepath.Base(path))
				}
				deps = append(deps, d...)
			} else {
				fmt.Printf("[SCA] Error scanning jar %s: %v\n", path, err)
			}
		}
		return nil
	})

	fmt.Printf("[SCA] Total Java dependencies found: %d\n", len(deps))
	return deps, err
}

func scanJar(path string) ([]Dependency, error) {
	var deps []Dependency
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	for _, f := range r.File {
		// Method 1: Look for META-INF/maven/.../pom.properties (Standard Maven)
		if strings.Contains(f.Name, "META-INF/maven/") && strings.HasSuffix(f.Name, "pom.properties") {
			rc, err := f.Open()
			if err != nil {
				continue
			}

			scanner := bufio.NewScanner(rc)
			var groupID, artifactID, version string
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if strings.HasPrefix(line, "groupId=") {
					groupID = strings.TrimPrefix(line, "groupId=")
				} else if strings.HasPrefix(line, "artifactId=") {
					artifactID = strings.TrimPrefix(line, "artifactId=")
				} else if strings.HasPrefix(line, "version=") {
					version = strings.TrimPrefix(line, "version=")
				}
			}
			rc.Close()

			if groupID != "" && artifactID != "" && version != "" {
				depName := groupID + ":" + artifactID
				fmt.Printf("[SCA] Extracted Dependency (pom.properties): %s @ %s\n", depName, version)
				deps = append(deps, Dependency{
					Name:      depName,
					Version:   version,
					Language:  "java",
					FilePath:  path,
					Locations: []string{f.Name},
				})
			}
		}

		// Method 2: Look for JARs inside (Nested JARs/WARs) - e.g. WEB-INF/lib/*.jar or BOOT-INF/lib/*.jar
		if strings.HasSuffix(strings.ToLower(f.Name), ".jar") {
			// Try to read nested jar to find pom.properties
			// This is expensive but necessary for accurate OSV queries (need GroupID)

			foundPom := false
			var nestedGroupID, nestedArtifactID, nestedVersion string

			rc, err := f.Open()
			if err == nil {
				// Read entire nested jar into memory
				// Limit size to avoid OOM? jars are usually small (1-10MB)
				content, err := io.ReadAll(rc)
				rc.Close()

				if err == nil {
					zipReader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
					if err == nil {
						for _, nf := range zipReader.File {
							if strings.Contains(nf.Name, "META-INF/maven/") && strings.HasSuffix(nf.Name, "pom.properties") {
								nrc, err := nf.Open()
								if err != nil {
									continue
								}
								scanner := bufio.NewScanner(nrc)
								for scanner.Scan() {
									line := strings.TrimSpace(scanner.Text())
									if strings.HasPrefix(line, "groupId=") {
										nestedGroupID = strings.TrimPrefix(line, "groupId=")
									} else if strings.HasPrefix(line, "artifactId=") {
										nestedArtifactID = strings.TrimPrefix(line, "artifactId=")
									} else if strings.HasPrefix(line, "version=") {
										nestedVersion = strings.TrimPrefix(line, "version=")
									}
								}
								nrc.Close()
								if nestedGroupID != "" && nestedArtifactID != "" && nestedVersion != "" {
									foundPom = true
									break // Found valid pom, stop searching this nested jar
								}
							}
						}
					}
				}
			}

			if foundPom {
				depName := nestedGroupID + ":" + nestedArtifactID
				// Check duplicate
				exists := false
				for _, d := range deps {
					if d.Name == depName && d.Version == nestedVersion {
						exists = true
						break
					}
				}
				if !exists {
					fmt.Printf("[SCA] Extracted Nested Dependency (Deep): %s @ %s\n", depName, nestedVersion)
					deps = append(deps, Dependency{
						Name:      depName,
						Version:   nestedVersion,
						Language:  "java",
						FilePath:  path,
						Locations: []string{f.Name},
					})
				}
				continue // Skip filename heuristic if we found pom
			}

			// Fallback: Extract filename to guess artifactId and version
			// Common pattern: artifactId-version.jar
			baseName := filepath.Base(f.Name)
			nameWithoutExt := strings.TrimSuffix(baseName, filepath.Ext(baseName))

			// Simple heuristic to split version
			// Find the first digit that looks like a version start
			parts := strings.Split(nameWithoutExt, "-")
			if len(parts) > 1 {
				// Try to find where version starts
				versionIndex := -1
				for i, part := range parts {
					if len(part) > 0 && part[0] >= '0' && part[0] <= '9' {
						versionIndex = i
						break
					}
				}

				if versionIndex > 0 {
					artifactID := strings.Join(parts[:versionIndex], "-")
					version := strings.Join(parts[versionIndex:], "-")

					// Skip if we already found this via pom.properties (naive check)
					alreadyFound := false
					for _, d := range deps {
						if strings.HasSuffix(d.Name, ":"+artifactID) && d.Version == version {
							alreadyFound = true
							break
						}
					}

					if !alreadyFound {
						// Note: We don't have groupId here, so we use artifactID as Name for now.
						fmt.Printf("[SCA] Found nested JAR (filename only): %s -> %s @ %s\n", f.Name, artifactID, version)

						deps = append(deps, Dependency{
							Name:      artifactID, // Missing GroupID!
							Version:   version,
							Language:  "java",
							FilePath:  path,
							Locations: []string{f.Name},
						})
					}
				}
			}
		}
	}
	return deps, nil
}
