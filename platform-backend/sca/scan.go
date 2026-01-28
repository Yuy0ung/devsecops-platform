package sca

import (
	"demo/models"
	"fmt"
)

type ScaEngine struct {
	Plugins []ScannerPlugin
}

func NewScaEngine() *ScaEngine {
	return &ScaEngine{
		Plugins: []ScannerPlugin{
			&JavaScanner{},
			&PythonScanner{},
			&GoScanner{},
			&FrontendScanner{},
		},
	}
}

func (e *ScaEngine) Scan(task *models.ScaTask) ([]models.ScaFinding, []string, error) {
	var allDeps []Dependency

	// 1. Run all plugins
	for _, plugin := range e.Plugins {
		deps, err := plugin.Scan(task.Target)
		if err != nil {
			// Log error but continue
			fmt.Printf("Plugin %s failed: %v\n", plugin.Name(), err)
			continue
		}
		allDeps = append(allDeps, deps...)
	}

	// Collect unique languages
	langMap := make(map[string]bool)
	for _, dep := range allDeps {
		if dep.Language != "" {
			langMap[dep.Language] = true
		}
	}
	var languages []string
	for lang := range langMap {
		languages = append(languages, lang)
	}

	// 2. Check Vulns (OSV)
	vulnMap, err := CheckVulns(allDeps)
	if err != nil {
		return nil, nil, err
	}

	// 3. Convert to models.ScaFinding
	var findings []models.ScaFinding
	for _, dep := range allDeps {
		key := dep.Name + "@" + dep.Version
		vulns, ok := vulnMap[key]
		if !ok {
			// fmt.Printf("[SCA] No vulns for %s\n", key)
			continue
		}

		fmt.Printf("[SCA] Processing %d vulns for %s\n", len(vulns), key)

		for _, v := range vulns {
			fixed := ""
			for _, aff := range v.Affected {
				for _, r := range aff.Ranges {
					for _, ev := range r.Events {
						if ev.Fixed != "" {
							fixed = ev.Fixed
							break
						}
					}
					if fixed != "" {
						break
					}
				}
				if fixed != "" {
					break
				}
			}

			ref := ""
			if len(v.References) > 0 {
				ref = v.References[0].URL
			}

			// Simple severity mapping if not present (OSV often omits CVSS in summary list)
			// Ideally we should fetch details or parse schema.
			severity := "High"

			findings = append(findings, models.ScaFinding{
				TaskID:       task.ID,
				PackageName:  dep.Name,
				Version:      dep.Version,
				Language:     dep.Language,
				VulnID:       v.ID,
				Severity:     severity,
				Description:  v.Summary,
				FixedVersion: fixed,
				Reference:    ref,
				FilePath:     dep.FilePath,
			})
		}
	}

	return findings, languages, nil
}
