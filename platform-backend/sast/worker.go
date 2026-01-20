package sast

import (
	"demo/db/mysqldb"
	"demo/db/redisdb"
	"demo/models"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

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
	log.Println("SAST Worker started, waiting for tasks...")
	for {
		// Block until a task is available in Redis
		// "sast_task_queue"
		result, err := redisdb.Client.BRPop(redisdb.Ctx, 0, "sast_task_queue").Result()
		if err != nil {
			log.Printf("Redis BRPop failed: %v", err)
			time.Sleep(5 * time.Second) // Retry delay
			continue
		}
		taskId := result[1]

		log.Printf("Processing task: %s", taskId)

		// Fetch task from DB
		var task models.SastTask
		if err := mysqldb.DB.Where("id = ?", taskId).First(&task).Error; err != nil {
			log.Printf("Task %s not found in DB: %v", taskId, err)
			continue
		}

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
		cmd := exec.Command("unzip", "-q", zipPath, "-d", workDir)
		if err := cmd.Run(); err != nil {
			log.Printf("Unzip failed: %s", err)
			failTask(task, "Failed to unzip database")
			return
		}

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

	// Parse rules from task.Rules (JSON string)
	var rules []string
	if err := json.Unmarshal([]byte(task.Rules), &rules); err != nil {
		log.Printf("Failed to parse rules: %v", err)
		rules = []string{}
	}

	var queries []string
	for _, rule := range rules {
		if rule == "" {
			continue
		}
		// E.g. CWE-089 -> codeql/java-queries:Security/CWE/CWE-089
		queries = append(queries, fmt.Sprintf("codeql/java-queries:Security/CWE/%s", rule))
	}

	if len(queries) == 0 {
		queries = append(queries, "codeql/java-queries:Security/CWE/") // Fallback
	}

	sarifFile := filepath.Join(workDir, "results.sarif")
	totalCount := 0

	// Sequential Scan Loop to save memory
	for _, query := range queries {
		// Clean up previous sarif file
		os.Remove(sarifFile)

		log.Printf("Running CodeQL analysis for query: %s", query)
		args := []string{"database", "analyze", dbPath, query}
		args = append(args, "--format=sarif-latest", "--output="+sarifFile)
		// args = append(args, "--threads=4", "--ram=4096")

		cmd := exec.Command("codeql", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("CodeQL analyze failed for query %s: %s, output: %s", query, err, output)
			// Continue to next query instead of failing completely
			continue
		}

		// Parse SARIF
		sarifContent, err := os.ReadFile(sarifFile)
		if err != nil {
			log.Printf("Failed to read SARIF output for query %s: %v", query, err)
			continue
		}

		var report SarifReport
		if err := json.Unmarshal(sarifContent, &report); err != nil {
			log.Printf("Failed to parse SARIF output for query %s: %v", query, err)
			continue
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
					Description: res.Message.Text,
					Severity:    "High",
					File:        file,
					Line:        line,
					Message:     res.Message.Text,
					CodeFlow:    string(codeFlowJson),
				}

				// AI Audit Integration
				performAIAudit(task, workDir, res.RuleID, codeFlows, &finding)

				mysqldb.DB.Create(&finding)
				count++
			}
		}
		totalCount += count
	}

	task.Status = "completed"
	task.Result = fmt.Sprintf("Found %d vulnerabilities", totalCount)
	mysqldb.DB.Save(&task)
}

func failTask(task models.SastTask, message string) {
	task.Status = "failed"
	task.Result = message
	mysqldb.DB.Save(&task)
}
