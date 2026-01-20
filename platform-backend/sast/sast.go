package sast

import (
	"demo/db/mysqldb"
	"demo/db/redisdb"
	"demo/models"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func Init() {
	mysqldb.DB.AutoMigrate(&models.SastTask{}, &models.SastFinding{})
	os.MkdirAll("uploads", 0755)
	go Worker()
}

type CreateTaskRequest struct {
	RepoUrl string   `json:"repoUrl"`
	Branch  string   `json:"branch"`
	Rules   []string `json:"rules"`
}

func Create() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateTaskRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		taskId := uuid.New().String()
		rulesBytes, _ := json.Marshal(req.Rules)
		rulesJson := string(rulesBytes)

		task := models.SastTask{
			ID:     taskId,
			Type:   "Git",
			Target: req.RepoUrl,
			Branch: req.Branch,
			Status: "pending",
			Rules:  rulesJson,
		}

		if err := mysqldb.DB.Create(&task).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
			return
		}

		// Push to Redis queue
		if err := redisdb.Client.LPush(redisdb.Ctx, "sast_task_queue", taskId).Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enqueue task"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"taskId": taskId, "message": "Task created successfully"})
	}
}

func Upload() gin.HandlerFunc {
	return func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
			return
		}

		// Save uploaded file
		taskId := uuid.New().String()
		uploadPath := filepath.Join("uploads", taskId+".zip")
		// Ensure uploads directory exists (in main or init)
		if err := c.SaveUploadedFile(file, uploadPath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
			return
		}

		task := models.SastTask{
			ID:     taskId,
			Type:   "Upload",
			Target: file.Filename,
			Status: "pending",
			Rules:  "[]", // Default rules or passed via form
		}

		if err := mysqldb.DB.Create(&task).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
			return
		}

		// Push to Redis queue
		if err := redisdb.Client.LPush(redisdb.Ctx, "sast_task_queue", taskId).Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enqueue task"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"taskId": taskId, "message": "File uploaded and task created"})
	}
}

func List() gin.HandlerFunc {
	return func(c *gin.Context) {
		var tasks []models.SastTask
		mysqldb.DB.Order("created_at desc").Find(&tasks)
		c.JSON(http.StatusOK, tasks)
	}
}

func Result() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskId := c.Param("id")
		var vulns []models.SastFinding
		mysqldb.DB.Where("task_id = ?", taskId).Find(&vulns)
		c.JSON(http.StatusOK, vulns)
	}
}

func Delete() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskId := c.Param("id")
		if taskId == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing taskId"})
			return
		}

		// 1. Delete database records
		// Delete vulnerabilities first (foreign key usually handles this, but explicit is safe)
		if err := mysqldb.DB.Where("task_id = ?", taskId).Delete(&models.SastFinding{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete findings"})
			return
		}

		// Delete task
		if err := mysqldb.DB.Where("id = ?", taskId).Delete(&models.SastTask{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete task"})
			return
		}

		// 2. Clean up file system
		// Remove temp_sast/{taskId} directory (contains source code, db, results)
		workDir := filepath.Join("temp_sast", taskId)
		os.RemoveAll(workDir)

		// Remove uploaded zip if it exists
		uploadPath := filepath.Join("uploads", taskId+".zip")
		os.Remove(uploadPath)

		c.JSON(http.StatusOK, gin.H{"message": "Task and associated files deleted successfully"})
	}
}

func GetFileContent() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskId := c.Param("id")
		filePath := c.Query("path")

		if taskId == "" || filePath == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing taskId or path"})
			return
		}

		// Define source root
		sourceRoot := filepath.Join("temp_sast", taskId, "source")

		// Secure join
		// Note: filepath.Join cleans the path
		fullPath := filepath.Join(sourceRoot, filePath)

		// Verify the path is still inside sourceRoot (prevent ../ escape)
		// We use Abs to be sure
		absSourceRoot, _ := filepath.Abs(sourceRoot)
		absFullPath, _ := filepath.Abs(fullPath)

		if !strings.HasPrefix(absFullPath, absSourceRoot) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return
		}

		content, err := os.ReadFile(fullPath)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"content": string(content)})
	}
}
