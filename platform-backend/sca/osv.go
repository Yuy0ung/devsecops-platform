package sca

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const OSVQueryBatchURL = "https://api.osv.dev/v1/querybatch"

type OSVRequest struct {
	Queries []OSVQuery `json:"queries"`
}

type OSVQuery struct {
	Package OSVPackage `json:"package"`
	Version string     `json:"version"`
}

type OSVPackage struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

type OSVResponse struct {
	Results []OSVResult `json:"results"`
}

type OSVResult struct {
	Vulns []OSVVuln `json:"vulns"`
}

type OSVVuln struct {
	ID         string         `json:"id"`
	Summary    string         `json:"summary"`
	Details    string         `json:"details"`
	Affected   []OSVAffected  `json:"affected"`
	References []OSVReference `json:"references"`
}

type OSVAffected struct {
	Ranges []OSVRange `json:"ranges"`
}

type OSVRange struct {
	Type   string     `json:"type"`
	Events []OSVEvent `json:"events"`
}

type OSVEvent struct {
	Fixed string `json:"fixed"`
}

type OSVReference struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

func CheckVulns(deps []Dependency) (map[string][]OSVVuln, error) {
	var queries []OSVQuery
	var validDeps []Dependency

	for _, dep := range deps {
		ecosystem := ""
		switch dep.Language {
		case "java":
			ecosystem = "Maven"
			// Auto-complete GroupId if missing (only ArtifactId present)
			if !strings.Contains(dep.Name, ":") {
				// Avoid resolving if version is ambiguous or empty
				if dep.Version != "" {
					fmt.Printf("[SCA] Missing GroupId for %s@%s, resolving via Maven Central...\n", dep.Name, dep.Version)
					groupID, err := resolveMavenGroupId(dep.Name, dep.Version)
					if err == nil && groupID != "" {
						fmt.Printf("[SCA] Resolved: %s -> %s:%s\n", dep.Name, groupID, dep.Name)
						dep.Name = groupID + ":" + dep.Name
					} else {
						fmt.Printf("[SCA] Failed to resolve GroupId for %s: %v. Skipping OSV check to avoid noise.\n", dep.Name, err)
						continue
					}
				} else {
					continue
				}
			}
		case "python":
			ecosystem = "PyPI"
		case "go":
			ecosystem = "Go"
		case "javascript":
			ecosystem = "npm"
		}

		if ecosystem != "" {
			queries = append(queries, OSVQuery{
				Package: OSVPackage{Name: dep.Name, Ecosystem: ecosystem},
				Version: dep.Version,
			})
			validDeps = append(validDeps, dep)
		}
	}

	if len(queries) == 0 {
		return nil, nil
	}

	fmt.Printf("[SCA] Querying OSV for %d dependencies...\n", len(queries))
	// Simple batching (OSV limit is around 1000, usually safe for single project scan)
	// TODO: Implement chunking if queries > 1000

	reqBody, _ := json.Marshal(OSVRequest{Queries: queries})
	resp, err := http.Post(OSVQueryBatchURL, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		fmt.Printf("[SCA] OSV Query Failed: %v\n", err)
		return nil, err
	}
	defer resp.Body.Close()

	var osvResp OSVResponse
	if err := json.NewDecoder(resp.Body).Decode(&osvResp); err != nil {
		fmt.Printf("[SCA] Failed to decode OSV response: %v\n", err)
		return nil, err
	}

	vulnMap := make(map[string][]OSVVuln)
	totalVulns := 0
	for i, result := range osvResp.Results {
		if len(result.Vulns) > 0 {
			dep := validDeps[i]
			fmt.Printf("[SCA] Found %d vulns for %s@%s\n", len(result.Vulns), dep.Name, dep.Version)
			key := dep.Name + "@" + dep.Version
			vulnMap[key] = result.Vulns
			totalVulns += len(result.Vulns)
		}
	}

	fmt.Printf("[SCA] OSV Scan Complete. Total vulnerabilities found: %d\n", totalVulns)
	return vulnMap, nil
}

// Maven Central Search Response
type MavenSearchResponse struct {
	Response struct {
		Docs []struct {
			G string `json:"g"`
			A string `json:"a"`
			V string `json:"v"`
		} `json:"docs"`
	} `json:"response"`
}

// resolveMavenGroupId queries Maven Central to find the GroupId for a given ArtifactId and Version
func resolveMavenGroupId(artifactId, version string) (string, error) {
	// API: https://search.maven.org/solrsearch/select?q=a:"artifactId"+AND+v:"version"&rows=1&wt=json
	baseURL := "https://search.maven.org/solrsearch/select"
	params := url.Values{}
	params.Add("q", fmt.Sprintf(`a:"%s" AND v:"%s"`, artifactId, version))
	params.Add("rows", "1")
	params.Add("wt", "json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(baseURL + "?" + params.Encode())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("maven central returned status: %d", resp.StatusCode)
	}

	var searchResp MavenSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return "", err
	}

	if len(searchResp.Response.Docs) > 0 {
		return searchResp.Response.Docs[0].G, nil
	}

	return "", fmt.Errorf("no results found")
}
