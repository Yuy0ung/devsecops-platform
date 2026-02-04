package sca

import (
	"demo/db/mysqldb"
	"demo/models"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// InitLocalDB initializes the local vulnerability database from JSON files
func InitLocalDB() {
	// AutoMigrate the LocalVuln table
	if err := mysqldb.DB.AutoMigrate(&models.LocalVuln{}); err != nil {
		fmt.Printf("[SCA] Failed to migrate LocalVuln table: %v\n", err)
		return
	}

	vulnDir := "sca/vuln_db"
	files, err := os.ReadDir(vulnDir)
	if err != nil {
		// If directory doesn't exist, it's fine, just skip seeding
		if os.IsNotExist(err) {
			fmt.Println("[SCA] vuln_db directory not found, skipping local DB seeding.")
			return
		}
		fmt.Printf("[SCA] Failed to read vuln_db directory: %v\n", err)
		return
	}

	var newVulns []models.LocalVuln
	var processedCount int

	// Optimization: Get all existing VulnIDs to avoid duplicates
	existingVulnIDs := make(map[string]bool)
	var existing []string
	mysqldb.DB.Model(&models.LocalVuln{}).Pluck("vuln_id", &existing)
	for _, id := range existing {
		existingVulnIDs[id] = true
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}

		content, err := os.ReadFile(filepath.Join(vulnDir, file.Name()))
		if err != nil {
			fmt.Printf("[SCA] Failed to read file %s: %v\n", file.Name(), err)
			continue
		}

		var vuln models.LocalVuln
		if err := json.Unmarshal(content, &vuln); err != nil {
			fmt.Printf("[SCA] Failed to parse JSON %s: %v\n", file.Name(), err)
			continue
		}

		// Deduplication check
		if existingVulnIDs[vuln.VulnID] {
			continue
		}

		newVulns = append(newVulns, vuln)
		processedCount++
	}

	if len(newVulns) > 0 {
		if err := mysqldb.DB.Create(&newVulns).Error; err != nil {
			fmt.Printf("[SCA] Failed to seed local DB: %v\n", err)
		} else {
			fmt.Printf("[SCA] Successfully added %d new vulnerabilities to local DB.\n", len(newVulns))
		}
	} else {
		fmt.Println("[SCA] Local DB is up to date (no new vulnerabilities found in vuln_db).")
	}
}

// CheckLocalVulns replaces the OSV API check with a local database lookup
func CheckLocalVulns(deps []Dependency) (map[string][]models.LocalVuln, error) {
	vulnMap := make(map[string][]models.LocalVuln)

	// Optimization: Collect all package names to query in one go
	var packageNames []string
	for i, dep := range deps {
		// Resolve Java GroupId if missing (same logic as OSV)
		if dep.Language == "java" && !strings.Contains(dep.Name, ":") && dep.Version != "" {
			groupID, err := ResolveMavenGroupId(dep.Name, dep.Version)
			if err == nil && groupID != "" {
				fmt.Printf("[SCA] Resolved (LocalDB): %s -> %s:%s\n", dep.Name, groupID, dep.Name)
				deps[i].Name = groupID + ":" + dep.Name
			}
		}
		packageNames = append(packageNames, deps[i].Name)
	}

	var rules []models.LocalVuln
	// Search for rules matching any of the package names
	if err := mysqldb.DB.Where("package_name IN ?", packageNames).Find(&rules).Error; err != nil {
		return nil, err
	}

	// Group rules by package name for faster lookup
	rulesMap := make(map[string][]models.LocalVuln)
	for _, rule := range rules {
		rulesMap[rule.PackageName] = append(rulesMap[rule.PackageName], rule)
	}

	for _, dep := range deps {
		potentialVulns, exists := rulesMap[dep.Name]
		if !exists {
			continue
		}

		var matchedVulns []models.LocalVuln
		for _, v := range potentialVulns {
			if isVersionAffected(dep.Version, v.AffectedVersion) {
				matchedVulns = append(matchedVulns, v)
			}
		}

		if len(matchedVulns) > 0 {
			key := dep.Name + "@" + dep.Version
			vulnMap[key] = matchedVulns
		}
	}

	return vulnMap, nil
}

// Simple version comparator: supports "< version", "<= version", "= version"
func isVersionAffected(current, affectedSpec string) bool {
	parts := strings.Split(strings.TrimSpace(affectedSpec), " ")
	if len(parts) != 2 {
		// Fallback: exact match if no operator
		return current == affectedSpec
	}

	operator := parts[0]
	targetVer := parts[1]

	return compareVersions(current, targetVer, operator)
}

// Compare versions v1 and v2 with operator
// Returns true if v1 operator v2 is true (e.g. 1.2.0 < 1.2.83)
func compareVersions(v1, v2, operator string) bool {
	// Naive split by dot, handle potential non-numeric parts
	s1 := normalizeVersion(v1)
	s2 := normalizeVersion(v2)

	len1 := len(s1)
	len2 := len(s2)
	maxLen := len1
	if len2 > maxLen {
		maxLen = len2
	}

	for i := 0; i < maxLen; i++ {
		n1 := 0
		if i < len1 {
			n1 = s1[i]
		}

		n2 := 0
		if i < len2 {
			n2 = s2[i]
		}

		if n1 > n2 {
			// v1 > v2
			if operator == ">" || operator == ">=" {
				return true
			}
			return false
		}
		if n1 < n2 {
			// v1 < v2
			if operator == "<" || operator == "<=" {
				return true
			}
			return false
		}
	}

	// Equal
	if operator == "=" || operator == "<=" || operator == ">=" {
		return true
	}

	return false
}

// Helper to convert version string into slice of integers
func normalizeVersion(v string) []int {
	// Remove common suffixes/prefixes if needed, but for now simple split
	// Handle things like "1.2.3-RELEASE" -> "1.2.3"
	if idx := strings.Index(v, "-"); idx != -1 {
		v = v[:idx]
	}

	parts := strings.Split(v, ".")
	var nums []int
	for _, p := range parts {
		// Extract digits only from start of string
		d := ""
		for _, char := range p {
			if char >= '0' && char <= '9' {
				d += string(char)
			} else {
				break
			}
		}
		if d != "" {
			n, _ := strconv.Atoi(d)
			nums = append(nums, n)
		} else {
			nums = append(nums, 0)
		}
	}
	return nums
}
