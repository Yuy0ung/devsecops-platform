package task

import (
	"crypto/rand"
	"demo/db/mysqldb"
	"demo/db/redisdb"
	"demo/models"
	"demo/scanner"
	"demo/target"

	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type Task struct {
	TaskID    string   `json:"taskId"`
	TaskName  string   `json:"taskName"` // 新增任务名称
	Targets   []string `json:"targets"`
	Status    string   `json:"status"` // pending, running, stopped, finished
	CreatedAt string   `json:"createdAt"`
	UpdatedAt string   `json:"updatedAt"`
}

// 生成任务 ID
func generateTaskID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func Create() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			TaskName string   `json:"taskName"`
			Targets  []string `json:"targets"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
			return
		}
		if len(req.Targets) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing targets"})
			return
		}
		if strings.TrimSpace(req.TaskName) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing taskName"})
			return
		}

		taskId := generateTaskID()
		now := time.Now()

		// 1) 持久化到 MySQL（事务内先写 tasks + targets）
		taskModel := &models.Task{
			ID:        taskId,
			Name:      req.TaskName,
			Status:    "pending",
			CreatedAt: now,
			UpdatedAt: now,
		}

		// 使用事务保证 tasks 与 targets 一致性
		tx := mysqldb.DB.Begin()
		if err := tx.Create(taskModel).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db create task failed: " + err.Error()})
			return
		}

		// 批量插入 targets
		targetModels := make([]models.Target, 0, len(req.Targets))
		for _, t := range req.Targets {
			targetModels = append(targetModels, models.Target{
				TaskID: taskId,
				Target: t,
			})
		}
		if len(targetModels) > 0 {
			if err := tx.CreateInBatches(&targetModels, 100).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "db insert targets failed: " + err.Error()})
				return
			}
		}
		tx.Commit()

		// 2) 仍按原逻辑写入 Redis（用于队列/扫描）
		key := target.GetTaskTargetsKey(taskId)
		targetsInterface := make([]interface{}, len(req.Targets))
		for i, t := range req.Targets {
			targetsInterface[i] = t
		}
		if err := redisdb.Client.RPush(redisdb.Ctx, key, targetsInterface...).Err(); err != nil {
			// Redis 写失败不回滚 MySQL，但要记录错误
			c.JSON(http.StatusInternalServerError, gin.H{"error": "redis push failed: " + err.Error()})
			return
		}

		// 3) 写任务 info 到 Redis（保持原来用于 API 显示）
		taskInfo := map[string]interface{}{
			"taskId":     taskId,
			"taskName":   req.TaskName,
			"created_at": now.Format("2006-01-02 15:04:05"),
			"updated_at": now.Format("2006-01-02 15:04:05"),
			"status":     "pending",
		}
		if err := redisdb.Client.HSet(redisdb.Ctx, "task:"+taskId+":info", taskInfo).Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if err := redisdb.Client.RPush(redisdb.Ctx, "tasks:list", taskId).Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":  "任务创建成功",
			"taskId":   taskId,
			"taskName": req.TaskName,
			"created":  now.Format("2006-01-02 15:04:05"),
			"targets":  req.Targets,
		})
	}
}

// 获取任务列表
func List() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 优先从 MySQL 查询 tasks 元信息
		var dbTasks []models.Task
		if err := mysqldb.DB.Order("created_at desc").Find(&dbTasks).Error; err == nil && len(dbTasks) > 0 {
			resp := make([]gin.H, 0, len(dbTasks))
			for _, t := range dbTasks {
				// 为了显示最新运行状态，可优先读取 Redis 中的 task:{id}:info.status（如果存在）
				status := t.Status
				if info, _ := redisdb.Client.HGetAll(redisdb.Ctx, "task:"+t.ID+":info").Result(); len(info) > 0 {
					if s, ok := info["status"]; ok {
						status = s
					}
				}
				resp = append(resp, gin.H{
					"taskId":     t.ID,
					"taskName":   t.Name,
					"status":     status,
					"created_at": t.CreatedAt.Format("2006-01-02 15:04:05"),
					"updated_at": t.UpdatedAt.Format("2006-01-02 15:04:05"),
				})
			}
			c.JSON(http.StatusOK, gin.H{"tasks": resp})
			return
		}

		// 如果 MySQL 没数据，再退回到 Redis（向后兼容）
		taskIds, err := redisdb.Client.LRange(redisdb.Ctx, "tasks:list", 0, -1).Result()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		tasks := []gin.H{}
		for _, id := range taskIds {
			info, _ := redisdb.Client.HGetAll(redisdb.Ctx, "task:"+id+":info").Result()
			if len(info) == 0 {
				continue
			}
			tasks = append(tasks, gin.H{
				"taskId":     info["taskId"],
				"taskName":   info["taskName"],
				"status":     info["status"],
				"created_at": info["created_at"],
				"updated_at": info["updated_at"],
			})
		}
		c.JSON(http.StatusOK, gin.H{"tasks": tasks})
	}
}

// Start 启动任务扫描（防止重复启动）
// - 使用 Redis 短期锁避免并发竞争。
// - 使用 MySQL 原子更新（WHERE id=? AND status != 'running'）保证只有一个请求把状态改为 running。
func Start() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := redisdb.Ctx
		taskId, _ := c.GetQuery("taskId")
		if taskId == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing taskId"})
			return
		}

		taskKey := "task:" + taskId + ":targets"
		infoKey := "task:" + taskId + ":info"

		// 1) 先检查是否有 targets（保持原有行为）
		targets, err := redisdb.Client.LRange(ctx, taskKey, 0, -1).Result()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "redis error: " + err.Error()})
			return
		}
		if len(targets) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found or empty"})
			return
		}

		// 2) 尝试抢一个短期启动锁，避免高并发下多个请求同时进入 DB 更新路径。
		lockKey := "task:lock:" + taskId
		// TTL 设为 30s（启动过程里很快释放；若启动器异常，锁会自动过期）
		const lockTTL = 30 * time.Second
		acquired, err := redisdb.Client.SetNX(ctx, lockKey, "1", lockTTL).Result()
		if err != nil {
			// Redis 出问题时，不阻断——但告知错误（可根据你想要的策略改为 500）
			c.JSON(http.StatusInternalServerError, gin.H{"error": "redis lock error: " + err.Error()})
			return
		}
		if !acquired {
			// 已有别的请求在竞争或刚刚启动，拒绝重复启动
			c.JSON(http.StatusBadRequest, gin.H{"error": "task start already in progress"})
			return
		}
		// 确保在函数返回前释放锁（如果我们还持有它）
		releaseLock := func() {
			_ = redisdb.Client.Del(ctx, lockKey).Err()
		}
		defer releaseLock()

		// 3) 使用 MySQL 原子更新：只有当当前状态不是 running 才改为 running
		res := mysqldb.DB.Model(&models.Task{}).
			Where("id = ? AND status != ?", taskId, "running").
			Update("status", "running")
		if res.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db update failed: " + res.Error.Error()})
			return
		}
		if res.RowsAffected == 0 {
			// 没有更新任何行，说明已经是 running 或不存在
			// 为了精确判断原因，可以再查一次状态（可选）
			var t models.Task
			if err := mysqldb.DB.First(&t, "id = ?", taskId).Error; err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			} else if t.Status == "running" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "task already running"})
			} else {
				c.JSON(http.StatusBadRequest, gin.H{"error": "task cannot be started (status=" + t.Status + ")"})
			}
			return
		}

		// 到此：我们成功把数据库状态改为 running（唯一一次改写）
		// 释放锁（不需要持有锁到扫描完成）。若你希望锁覆盖整个扫描过程可改为更长 TTL 并在扫描结束时删除。
		releaseLock()

		// 4) 更新 Redis 中的任务 info，保持前后兼容
		_, _ = redisdb.Client.HSet(ctx, infoKey, "status", "running", "updated_at", time.Now().Format("2006-01-02 15:04:05")).Result()

		// 5) 启动扫描（异步），并返回
		go scanner.Run(taskId, targets, infoKey)

		c.JSON(http.StatusOK, gin.H{
			"message": "任务已开始扫描",
			"taskId":  taskId,
		})
	}
}

// 停止任务扫描
// Stop - 仅停止指定 taskId 的扫描（不会影响其他任务）
// 设计原则：以 taskId 为粒度、幂等、安全并发。
func Stop() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskId, _ := c.GetQuery("taskId")
		if taskId == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing taskId"})
			return
		}

		// 1) 读取任务，确认存在
		var t models.Task
		if err := mysqldb.DB.First(&t, "id = ?", taskId).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}

		// 2) 如果不是 running，直接返回适当信息（幂等）
		if t.Status != "running" {
			// 同步 Redis 显示状态（best-effort）
			_, _ = redisdb.Client.HSet(redisdb.Ctx, "task:"+taskId+":info", "status", t.Status, "updated_at", time.Now().Format("2006-01-02 15:04:05")).Result()
			c.JSON(http.StatusBadRequest, gin.H{"error": "task not running", "status": t.Status})
			return
		}

		// 3) 原子把 MySQL 状态从 running -> stopped（避免并发冲突）
		res := mysqldb.DB.Model(&models.Task{}).
			Where("id = ? AND status = ?", taskId, "running").
			Updates(map[string]interface{}{"status": "stopped", "updated_at": time.Now()})
		if res.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db update failed: " + res.Error.Error()})
			return
		}
		// 若没更新到行，可能被其它进程改走了：通知前端并保持幂等
		if res.RowsAffected == 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "task state changed concurrently, please retry"})
			return
		}

		// 4) 更新 Redis 中该任务的状态（仅针对该 taskId 的 key）
		infoKey := "task:" + taskId + ":info"
		_, _ = redisdb.Client.HSet(redisdb.Ctx, infoKey,
			"status", "stopped",
			"updated_at", time.Now().Format("2006-01-02 15:04:05"),
		).Result()

		// 5) 尝试取消 scanner（幂等调用；注意 scanner 需要只取消指定 taskId）
		// 直接调用，不当作有返回值的表达式
		scanner.Cancel(taskId)

		// 6) 返回结果
		c.JSON(http.StatusOK, gin.H{
			"message": "任务停止成功",
			"taskId":  taskId,
			"status":  "stopped",
		})
	}
}

// 删除任务
// Delete 删除任务接口（软删除+异步物理删除）
func Delete() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskId, _ := c.GetQuery("taskId")
		if taskId == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 taskId"})
			return
		}

		// 1) 检查 MySQL 是否存在并状态
		var t models.Task
		if err := mysqldb.DB.First(&t, "id = ?", taskId).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
			return
		}

		if t.Status == "running" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "任务正在运行中，无法删除"})
			return
		}

		// 2) 软删除：标记在 MySQL (status=deleted)
		if err := mysqldb.DB.Model(&models.Task{}).Where("id = ?", taskId).
			Updates(map[string]interface{}{"status": "deleted", "updated_at": time.Now()}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// 3) 取消 scanner（幂等）
		scanner.Cancel(taskId)

		// 4) 删除 Redis keys（尝试）
		keys := []string{
			"task:" + taskId + ":info",
			"task:" + taskId + ":targets",
			"task:" + taskId + ":result",
			"task:" + taskId + ":log",
		}
		_ = redisdb.Client.Del(redisdb.Ctx, keys...).Err()
		_ = redisdb.Client.LRem(redisdb.Ctx, "tasks:list", 0, taskId).Err()

		// 5) 将任务推入后台删除队列，由 worker 异步物理删除
		if err := redisdb.Client.RPush(redisdb.Ctx, "task:delete:queue", taskId).Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "加入删除队列失败: " + err.Error()})
			return
		}

		// 返回前端已标记删除
		c.JSON(http.StatusOK, gin.H{
			"message": "任务已标记删除（后台正在清理）",
			"taskId":  taskId,
		})
	}
}
