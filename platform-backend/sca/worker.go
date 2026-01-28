package sca

import (
	"archive/zip"
	"demo/db/mysqldb"
	"demo/db/redisdb"
	"demo/models"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func Worker() {
	for {
		result, err := redisdb.Client.BRPop(redisdb.Ctx, 0, "sca_task_queue").Result()
		if err != nil {
			fmt.Println("Redis error:", err)
			time.Sleep(5 * time.Second)
			continue
		}

		taskId := result[1]
		processTask(taskId)
	}
}

func processTask(taskId string) {
	var task models.ScaTask
	if err := mysqldb.DB.First(&task, "id = ?", taskId).Error; err != nil {
		fmt.Println("Task not found:", taskId)
		return
	}

	task.Status = "running"
	mysqldb.DB.Save(&task)

	// Ensure file exists
	if _, err := os.Stat(task.Target); os.IsNotExist(err) {
		updateStatus(&task, "failed", "File not found: "+task.Target)
		return
	}

	scanTarget := task.Target
	// Handle ZIP files: Unzip and scan the directory
	if filepath.Ext(task.Target) == ".zip" {
		unzipDir := filepath.Join("uploads", "sca", "unzip", taskId)
		if err := Unzip(task.Target, unzipDir); err != nil {
			updateStatus(&task, "failed", "Unzip failed: "+err.Error())
			return
		}
		scanTarget = unzipDir
		defer os.RemoveAll(unzipDir) // Cleanup after scan
	}

	// Scan
	engine := NewScaEngine()

	findings, languages, err := engine.Scan(&models.ScaTask{
		ID:     task.ID,
		Target: scanTarget,
	})
	if err != nil {
		updateStatus(&task, "failed", "Scan failed: "+err.Error())
		return
	}

	// Save findings
	maxSeverityVal := 0
	maxSeverityStr := ""

	severityMap := map[string]int{
		"Critical": 5,
		"High":     4,
		"Medium":   3,
		"Low":      2,
		"Info":     1,
	}

	for _, f := range findings {
		f.TaskID = task.ID
		// Just use the basename for display if it's a single file scan
		f.FilePath = filepath.Base(task.Target)
		mysqldb.DB.Create(&f)

		val := severityMap[f.Severity]
		if val > maxSeverityVal {
			maxSeverityVal = val
			maxSeverityStr = f.Severity
		}
	}

	task.VulnCount = len(findings)
	task.MaxSeverity = maxSeverityStr
	task.ProjectLanguage = strings.Join(languages, ", ")

	updateStatus(&task, "completed", fmt.Sprintf("Found %d vulnerabilities", len(findings)))
}

func updateStatus(task *models.ScaTask, status, result string) {
	task.Status = status
	task.Result = result
	mysqldb.DB.Save(task)
}

func Unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)

		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("%s: illegal file path", fpath)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)

		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}
	return nil
}
