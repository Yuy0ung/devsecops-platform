package sast

import (
	"demo/db/mysqldb"
	"demo/models"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

var TaskChannel = make(chan models.SastTask, 100)

type SarifReport struct {
	Runs []struct {
		Results []struct {
			RuleID  string `json:"ruleId"`
			Message struct {
				Text string `json:"text"`
			} `json:"message"`
			Locations []struct {
				PhysicalLocation struct {
					ArtifactLocation struct {
						URI string `json:"uri"`
					} `json:"artifactLocation"`
					Region struct {
						StartLine int `json:"startLine"`
					} `json:"region"`
				} `json:"physicalLocation"`
			} `json:"locations"`
			CodeFlows []struct {
				ThreadFlows []struct {
					Locations []struct {
						Location struct {
							PhysicalLocation struct {
								ArtifactLocation struct {
									URI string `json:"uri"`
								} `json:"artifactLocation"`
								Region struct {
									StartLine int `json:"startLine"`
								} `json:"region"`
							} `json:"physicalLocation"`
							Message struct {
								Text string `json:"text"`
							} `json:"message"`
						} `json:"location"`
					} `json:"locations"`
				} `json:"threadFlows"`
			} `json:"codeFlows"`
		} `json:"results"`
	} `json:"runs"`
}

type CodeFlowLocation struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

func Worker() {
	for task := range TaskChannel {
		processTask(task)
	}
}

func processTask(task models.SastTask) {
	// Update status to running
	task.Status = "running"
	mysqldb.DB.Save(&task)

	workDir := filepath.Join("temp_sast", task.ID)
	os.MkdirAll(workDir, 0755)
	// defer os.RemoveAll(workDir) // Cleanup disabled for code preview

	var dbPath string
	var err error

	if task.Type == "Git" {
		// Clone repo
		repoPath := filepath.Join(workDir, "source")
		cmd := exec.Command("git", "clone", "-b", task.Branch, task.Target, repoPath)
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("Git clone failed: %s, output: %s", err, output)
			failTask(task, "Git clone failed")
			return
		}

		// Build DB
		dbPath = filepath.Join(workDir, "db")
		// codeql database create <db> --language=java --command="mvn clean install -Dmaven.test.skip=true" --source-root=<src>
		// Note: User said "prioritize java8". We assume 'mvn' is available and configured.
		// If explicit java version is needed, we might need to set JAVA_HOME.
		// For now, use default environment.

		// Note: "mvn clean install" might take long and fail if dependencies are missing.
		// Sometimes just --language=java without command works if it's simple, but for Java it usually needs build.
		// User specific command: --command="mvn clean install -Dmaven.test.skip=true"

		cmd = exec.Command("codeql", "database", "create", dbPath,
			"--language=java",
			"--command=mvn clean install -Dmaven.test.skip=true",
			"--source-root="+repoPath,
			"--overwrite",
		)
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("CodeQL create db failed: %s, output: %s", err, output)
			failTask(task, "CodeQL database creation failed: "+string(output))
			return
		}
	} else {
		// Upload mode
		zipPath := filepath.Join("uploads", task.ID+".zip")
		// Unzip
		// Assuming 'unzip' command exists or use archive/zip
		// Using exec for simplicity
		cmd := exec.Command("unzip", "-q", zipPath, "-d", workDir)
		if err := cmd.Run(); err != nil {
			log.Printf("Unzip failed: %s", err)
			failTask(task, "Failed to unzip database")
			return
		}

		// Find the database directory. It might be nested.
		// CodeQL DB usually has a 'codeql-database.yml' file.
		foundDB := false
		filepath.Walk(workDir, func(path string, info os.FileInfo, err error) error {
			if !info.IsDir() && info.Name() == "codeql-database.yml" {
				dbPath = filepath.Dir(path)
				foundDB = true
				return filepath.SkipDir
			}
			return nil
		})

		if !foundDB {
			failTask(task, "Invalid CodeQL database structure")
			return
		}
	}

	// Analyze
	// codeql database analyze <db> codeql/java-queries:Security/CWE/CWE-089 --format=sarif-latest --output=sql.sarif
	// We need to construct the queries based on selected rules.
	// User rules: CWE-078, CWE-502, etc.
	// Map rules to CodeQL queries.
	// Example: CWE-089 -> codeql/java-queries:Security/CWE/CWE-089
	// But actually, 'codeql/java-queries:Security/CWE/CWE-089' might be a suite or path.
	// If the user has the standard codeql-repo checked out or installed.
	// Assuming `codeql/java-queries` is available.
	// If we want to use the standard suites, we might pass multiple queries.

	// Parse rules from task.Rules (JSON string)
	var rules []string
	if err := json.Unmarshal([]byte(task.Rules), &rules); err != nil {
		log.Printf("Failed to parse rules: %v", err)
		// Fallback if parsing fails or empty
		rules = []string{}
	}

	var queries []string
	for _, rule := range rules {
		if rule == "" {
			continue
		}
		// Construct query path
		// E.g. CWE-089 -> codeql/java-queries:Security/CWE/CWE-089
		queries = append(queries, fmt.Sprintf("codeql/java-queries:Security/CWE/%s", rule))
	}

	// If no rules selected, maybe run default?
	if len(queries) == 0 {
		queries = append(queries, "codeql/java-queries:Security/CWE/") // Fallback
	}

	sarifFile := filepath.Join(workDir, "results.sarif")
	args := []string{"database", "analyze", dbPath}
	args = append(args, queries...)
	args = append(args, "--format=sarif-latest", "--output="+sarifFile)

	// Ensure we use enough threads/RAM if needed
	// args = append(args, "--threads=4", "--ram=4096")

	cmd := exec.Command("codeql", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("CodeQL analyze failed: %s, output: %s", err, output)
		failTask(task, "CodeQL analysis failed: "+string(output))
		return
	}

	// Parse SARIF
	sarifContent, err := os.ReadFile(sarifFile)
	if err != nil {
		failTask(task, "Failed to read SARIF output")
		return
	}

	var report SarifReport
	if err := json.Unmarshal(sarifContent, &report); err != nil {
		failTask(task, "Failed to parse SARIF output")
		return
	}

	// Save findings
	count := 0
	for _, run := range report.Runs {
		for _, res := range run.Results {
			file := ""
			line := 0
			if len(res.Locations) > 0 {
				file = res.Locations[0].PhysicalLocation.ArtifactLocation.URI
				line = res.Locations[0].PhysicalLocation.Region.StartLine
			}

			// Extract code flow
			var codeFlows []CodeFlowLocation
			if len(res.CodeFlows) > 0 && len(res.CodeFlows[0].ThreadFlows) > 0 {
				for _, loc := range res.CodeFlows[0].ThreadFlows[0].Locations {
					codeFlows = append(codeFlows, CodeFlowLocation{
						File:    loc.Location.PhysicalLocation.ArtifactLocation.URI,
						Line:    loc.Location.PhysicalLocation.Region.StartLine,
						Message: loc.Location.Message.Text,
					})
				}
			}
			codeFlowJson, _ := json.Marshal(codeFlows)

			finding := models.SastFinding{
				TaskID:      task.ID,
				RuleID:      res.RuleID,
				Description: res.Message.Text, // Simplified
				Severity:    "High",           // CodeQL SARIF might have level in 'level' or 'properties'
				File:        file,
				Line:        line,
				Message:     res.Message.Text,
				CodeFlow:    string(codeFlowJson),
			}
			mysqldb.DB.Create(&finding)
			count++
		}
	}

	task.Status = "completed"
	task.Result = fmt.Sprintf("Found %d vulnerabilities", count)
	mysqldb.DB.Save(&task)
}

func failTask(task models.SastTask, message string) {
	task.Status = "failed"
	task.Result = message
	mysqldb.DB.Save(&task)
}
