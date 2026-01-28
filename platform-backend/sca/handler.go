package sca

import (
	"demo/db/mysqldb"
	"demo/db/redisdb"
	"demo/models"
	"net/http"
	"os"
	"path/filepath"

	"fmt"
	"io"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func Init() {
	mysqldb.DB.AutoMigrate(&models.ScaTask{}, &models.ScaFinding{})
	go Worker()
}

// InitUpload initializes a chunked upload
func InitUpload() gin.HandlerFunc {
	return func(c *gin.Context) {
		uploadId := uuid.New().String()
		tempDir := filepath.Join("uploads", "temp", uploadId)
		if err := os.MkdirAll(tempDir, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create temp directory"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"uploadId": uploadId})
	}
}

// UploadChunk handles a single chunk upload
func UploadChunk() gin.HandlerFunc {
	return func(c *gin.Context) {
		uploadId := c.PostForm("uploadId")
		chunkIndex := c.PostForm("chunkIndex")
		file, err := c.FormFile("file")

		if uploadId == "" || chunkIndex == "" || err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid parameters"})
			return
		}

		savePath := filepath.Join("uploads", "temp", uploadId, chunkIndex)
		if err := c.SaveUploadedFile(file, savePath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save chunk"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Chunk uploaded"})
	}
}

// MergeChunks merges all chunks into the final file
func MergeChunks() gin.HandlerFunc {
	return func(c *gin.Context) {
		uploadId := c.PostForm("uploadId")
		fileName := c.PostForm("fileName")
		totalChunksStr := c.PostForm("totalChunks")

		if uploadId == "" || fileName == "" || totalChunksStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid parameters"})
			return
		}

		totalChunks, _ := strconv.Atoi(totalChunksStr)
		tempDir := filepath.Join("uploads", "temp", uploadId)

		// Validate extension
		ext := filepath.Ext(fileName)
		if ext != ".jar" && ext != ".war" && ext != ".zip" {
			os.RemoveAll(tempDir)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Only .jar, .war, and .zip files are supported"})
			return
		}

		// Create final directory
		saveDir := "uploads/sca"
		if _, err := os.Stat(saveDir); os.IsNotExist(err) {
			os.MkdirAll(saveDir, 0755)
		}

		taskId := uuid.New().String()
		finalPath := filepath.Join(saveDir, taskId+ext)
		outFile, err := os.Create(finalPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create final file"})
			return
		}
		defer outFile.Close()

		// Merge chunks
		for i := 0; i < totalChunks; i++ {
			chunkPath := filepath.Join(tempDir, strconv.Itoa(i))
			chunkFile, err := os.Open(chunkPath)
			if err != nil {
				outFile.Close()
				os.Remove(finalPath)
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Missing chunk %d", i)})
				return
			}
			_, err = io.Copy(outFile, chunkFile)
			chunkFile.Close()
			if err != nil {
				outFile.Close()
				os.Remove(finalPath)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to merge chunk"})
				return
			}
		}

		// Cleanup temp dir
		os.RemoveAll(tempDir)

		// Create Task
		task := models.ScaTask{
			ID:     taskId,
			Type:   "Upload",
			Target: finalPath,
			Status: "pending",
		}

		if err := mysqldb.DB.Create(&task).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
			return
		}

		// Push to Redis queue
		if err := redisdb.Client.LPush(redisdb.Ctx, "sca_task_queue", taskId).Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enqueue task"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"taskId": taskId, "message": "Task created successfully"})
	}
}

func Create() gin.HandlerFunc {
	return func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "File is required"})
			return
		}

		taskId := uuid.New().String()

		// Validate extension
		ext := filepath.Ext(file.Filename)
		if ext != ".jar" && ext != ".war" && ext != ".zip" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Only .jar, .war, and .zip files are supported"})
			return
		}

		// Save file
		saveDir := "uploads/sca"
		if _, err := os.Stat(saveDir); os.IsNotExist(err) {
			os.MkdirAll(saveDir, 0755)
		}

		savePath := filepath.Join(saveDir, taskId+ext)
		if err := c.SaveUploadedFile(file, savePath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
			return
		}

		task := models.ScaTask{
			ID:     taskId,
			Type:   "Upload",
			Target: savePath,
			Status: "pending",
		}

		if err := mysqldb.DB.Create(&task).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
			return
		}

		// Push to Redis queue
		if err := redisdb.Client.LPush(redisdb.Ctx, "sca_task_queue", taskId).Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enqueue task"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"taskId": taskId, "message": "Task created successfully"})
	}
}

func List() gin.HandlerFunc {
	return func(c *gin.Context) {
		var tasks []models.ScaTask
		mysqldb.DB.Order("created_at desc").Find(&tasks)
		c.JSON(http.StatusOK, tasks)
	}
}

func Result() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskId := c.Param("id")
		var findings []models.ScaFinding
		mysqldb.DB.Where("task_id = ?", taskId).Find(&findings)
		c.JSON(http.StatusOK, findings)
	}
}

func Delete() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskId := c.Param("id")
		if taskId == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing taskId"})
			return
		}

		if err := mysqldb.DB.Where("task_id = ?", taskId).Delete(&models.ScaFinding{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete findings"})
			return
		}

		if err := mysqldb.DB.Where("id = ?", taskId).Delete(&models.ScaTask{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete task"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Deleted successfully"})
	}
}
