package target

import (
	"demo/db/mysqldb"
	"demo/db/redisdb"
	"demo/models"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/projectdiscovery/nuclei/v3/pkg/output"
)

// GetTaskTargetsKey: 导出函数，返回任务在 Redis 中的 targets key
func GetTaskTargetsKey(taskId string) string {
	return "task:" + taskId + ":targets"
}

// Add - 批量添加 targets：先写入 MySQL（持久化），再写入 Redis（用于扫描队列）
// 若 Redis 写入失败，会将 taskId 推入补偿队列并在 MySQL 上将 task 状态置为 pending_sync（best-effort）。
// 额外记录日志，插入时显式设置 CreatedAt/UpdatedAt。
func Add() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Targets []string `json:"targets"`
			TaskId  string   `json:"taskId"`
		}
		if err := c.BindJSON(&req); err != nil || len(req.Targets) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid targets"})
			return
		}
		taskId := req.TaskId
		if taskId == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing taskId"})
			return
		}

		// 1) 持久化到 MySQL（批量插入 targets）
		targetModels := make([]models.Target, 0, len(req.Targets))
		now := time.Now()
		for _, t := range req.Targets {
			targetModels = append(targetModels, models.Target{
				TaskID:    taskId,
				Target:    t,
				CreatedAt: now,
				UpdatedAt: now,
			})
		}
		if len(targetModels) > 0 {
			if err := mysqldb.DB.CreateInBatches(&targetModels, 100).Error; err != nil {
				log.Printf("[target.Add] db insert targets failed task=%s err=%v", taskId, err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "db insert targets failed: " + err.Error()})
				return
			}
		}

		// 2) 写入 Redis（任务专属队列）——若失败则记录补偿队列
		targetsInterface := make([]interface{}, len(req.Targets))
		for i, t := range req.Targets {
			targetsInterface[i] = t
		}
		key := GetTaskTargetsKey(taskId)
		if err := redisdb.Client.RPush(redisdb.Ctx, key, targetsInterface...).Err(); err != nil {
			log.Printf("[target.Add] redis push failed task=%s err=%v; enqueue compensator", taskId, err)
			// 记录补偿项，后台 worker 会重试把 targets 推回 Redis
			_ = redisdb.Client.RPush(redisdb.Ctx, "task:sync:targets:queue", taskId).Err()
			// 同时将 MySQL 上的该任务状态标记为 pending_sync（best-effort）
			_ = mysqldb.DB.Model(&models.Task{}).Where("id = ?", taskId).
				Update("status", "pending_sync").Error

			c.JSON(http.StatusAccepted, gin.H{
				"message": "targets stored in DB, but redis push failed; queued for retry",
				"taskId":  taskId,
				"targets": req.Targets,
			})
			return
		}

		log.Printf("[target.Add] success task=%s count=%d", taskId, len(req.Targets))
		c.JSON(http.StatusOK, gin.H{
			"message": "添加成功",
			"taskId":  taskId,
			"targets": req.Targets,
		})
	}
}

// List - 列出指定 taskId 的所有目标
// 优先从 MySQL 读取（长期存储），若 MySQL 未命中则回退到 Redis（兼容旧数据）
func List() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskId, _ := c.GetQuery("taskId")
		if taskId == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing taskId"})
			return
		}

		// 优先从 MySQL 读取 targets（长期存储）
		var dbTargets []models.Target
		if err := mysqldb.DB.Where("task_id = ?", taskId).Order("id asc").Find(&dbTargets).Error; err == nil && len(dbTargets) > 0 {
			targets := make([]string, 0, len(dbTargets))
			for _, t := range dbTargets {
				targets = append(targets, t.Target)
			}
			c.JSON(http.StatusOK, gin.H{
				"taskId":  taskId,
				"targets": targets,
				"source":  "mysql",
			})
			return
		}

		// 回退到 Redis（兼容以前的数据模型）
		key := GetTaskTargetsKey(taskId)
		targets, err := redisdb.Client.LRange(redisdb.Ctx, key, 0, -1).Result()
		if err != nil {
			log.Printf("[target.List] redis lrange failed task=%s err=%v", taskId, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"taskId":  taskId,
			"targets": targets,
			"source":  "redis",
		})
	}
}

// Delete - 从 MySQL 和 Redis 中删除给定的 targets（支持批量）
// 关键：如果 MySQL 事务 Commit 失败则不会继续清理 Redis（避免不一致）。
func Delete() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Targets []string `json:"targets"`
			TaskId  string   `json:"taskId"`
		}
		if err := c.BindJSON(&req); err != nil || req.TaskId == "" || len(req.Targets) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		taskId := req.TaskId

		// 1) 在 MySQL 中删除这些 targets（如果存在）
		tx := mysqldb.DB.Begin()
		if tx.Error != nil {
			log.Printf("[target.Delete] db begin failed task=%s err=%v", taskId, tx.Error)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db begin failed: " + tx.Error.Error()})
			return
		}

		result := tx.Where("task_id = ? AND target IN ?", taskId, req.Targets).Delete(&models.Target{})
		if result.Error != nil {
			tx.Rollback()
			log.Printf("[target.Delete] db delete failed task=%s err=%v", taskId, result.Error)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db delete targets failed: " + result.Error.Error()})
			return
		}
		deletedFromDB := result.RowsAffected

		if err := tx.Commit().Error; err != nil {
			// 关键改进：若 Commit 失败，不继续删除 Redis，返回 500 并记录错误
			log.Printf("[target.Delete] tx commit failed task=%s err=%v", taskId, err)
			_ = tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db commit failed: " + err.Error()})
			return
		}

		// 2) 在 Redis 中删除（LREM）——逐项删除以保持语义一致
		redisKey := GetTaskTargetsKey(taskId)
		var redisErrs []string
		deletedFromRedis := int64(0)
		for _, t := range req.Targets {
			n, err := redisdb.Client.LRem(redisdb.Ctx, redisKey, 0, t).Result()
			if err != nil {
				redisErrs = append(redisErrs, err.Error())
				continue
			}
			deletedFromRedis += n
		}

		resp := gin.H{
			"message":          "targets delete attempted",
			"taskId":           taskId,
			"requestedTargets": len(req.Targets),
			"deletedFromDB":    deletedFromDB,
			"deletedFromRedis": deletedFromRedis,
		}
		if len(redisErrs) > 0 {
			resp["redisErrors"] = redisErrs
		}

		if deletedFromDB == 0 && deletedFromRedis == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "no matching targets found", "detail": resp})
			return
		}

		log.Printf("[target.Delete] success task=%s deletedDB=%d deletedRedis=%d", taskId, deletedFromDB, deletedFromRedis)
		c.JSON(http.StatusOK, resp)
	}
}

// Result - 获取扫描结果（保持 Redis 分页实现）
func Result() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskId, _ := c.GetQuery("taskId")
		if taskId == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing taskId"})
			return
		}
		resultKey := "task:" + taskId + ":result"

		pageStr := c.DefaultQuery("page", "1")
		pageSizeStr := c.DefaultQuery("pageSize", "20")
		page, err := strconv.Atoi(pageStr)
		if err != nil || page <= 0 {
			page = 1
		}
		pageSize, err := strconv.Atoi(pageSizeStr)
		if err != nil || pageSize <= 0 {
			pageSize = 20
		}

		total, err := redisdb.Client.LLen(redisdb.Ctx, resultKey).Result()
		if err != nil {
			log.Printf("[target.Result] redis llen failed task=%s err=%v", taskId, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if total == 0 {
			c.JSON(http.StatusOK, gin.H{
				"taskId":   taskId,
				"total":    0,
				"page":     page,
				"pageSize": pageSize,
				"count":    0,
				"results":  []output.ResultEvent{},
			})
			return
		}

		start := int64((page - 1) * pageSize)
		end := start + int64(pageSize) - 1
		if start >= total {
			c.JSON(http.StatusOK, gin.H{
				"taskId":   taskId,
				"total":    total,
				"page":     page,
				"pageSize": pageSize,
				"count":    0,
				"results":  []output.ResultEvent{},
			})
			return
		}
		rawResults, err := redisdb.Client.LRange(redisdb.Ctx, resultKey, start, end).Result()
		if err != nil {
			log.Printf("[target.Result] redis lrange failed task=%s err=%v", taskId, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		results := make([]output.ResultEvent, 0, len(rawResults))
		for _, item := range rawResults {
			var ev output.ResultEvent
			if err := json.Unmarshal([]byte(item), &ev); err != nil {
				continue
			}
			results = append(results, ev)
		}

		c.JSON(http.StatusOK, gin.H{
			"taskId":   taskId,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
			"count":    len(results),
			"results":  results,
		})
	}
}
