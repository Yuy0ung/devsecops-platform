package task

import (
	"context"
	"demo/db/mysqldb"
	"demo/db/redisdb"
	"demo/models"
	"log"
	"time"
)

// 初始化  Worker
func Init() {
	go deleteWorker()
	go taskCompensatorWorker()
}

// 补偿 worker：保证 Redis 中 tasks:list 与 task:{id}:info 与 MySQL 数据一致
func taskCompensatorWorker() {
	ctx := context.Background()
	backoffBase := time.Second * 2

	for {
		// 获取 MySQL 所有非删除任务
		var dbTasks []models.Task
		if err := mysqldb.DB.Where("status != ?", "deleted").Find(&dbTasks).Error; err != nil {
			log.Printf("[taskCompensator] mysql query failed: %v", err)
			time.Sleep(backoffBase)
			continue
		}

		for _, t := range dbTasks {
			infoKey := "task:" + t.ID + ":info"

			// 检查 Redis 中 task:{id}:info 是否存在
			exists, err := redisdb.Client.Exists(ctx, infoKey).Result()
			if err != nil {
				log.Printf("[taskCompensator] redis exists check failed for task=%s err=%v", t.ID, err)
				continue
			}
			if exists == 0 {
				// 不存在则补回 Redis
				taskInfo := map[string]interface{}{
					"taskId":     t.ID,
					"taskName":   t.Name,
					"status":     t.Status,
					"created_at": t.CreatedAt.Format("2006-01-02 15:04:05"),
					"updated_at": t.UpdatedAt.Format("2006-01-02 15:04:05"),
				}
				if err := redisdb.Client.HSet(ctx, infoKey, taskInfo).Err(); err != nil {
					log.Printf("[taskCompensator] redis HSet failed task=%s err=%v", t.ID, err)
					continue
				}
				log.Printf("[taskCompensator] restored task info to redis task=%s", t.ID)
			}

			// 检查 tasks:list 是否包含该 taskId
			taskList, err := redisdb.Client.LRange(ctx, "tasks:list", 0, -1).Result()
			if err != nil {
				log.Printf("[taskCompensator] redis LRange tasks:list failed: %v", err)
				continue
			}
			found := false
			for _, id := range taskList {
				if id == t.ID {
					found = true
					break
				}
			}
			if !found {
				if err := redisdb.Client.RPush(ctx, "tasks:list", t.ID).Err(); err != nil {
					log.Printf("[taskCompensator] redis RPush tasks:list failed task=%s err=%v", t.ID, err)
				} else {
					log.Printf("[taskCompensator] restored taskId to tasks:list task=%s", t.ID)
				}
			}
		}

		// 每分钟检查一次
		time.Sleep(time.Minute)
	}
}

func deleteWorker() {
	ctx := context.Background()
	const queueKey = "task:delete:queue"
	const failedKey = "task:delete:failed"
	backoffBase := time.Second * 2

	for {
		// 从删除队列取出 taskId，超时避免永久阻塞
		res, err := redisdb.Client.BLPop(ctx, 5*time.Second, queueKey).Result()
		if err != nil {
			// 超时或其他错误，sleep 后继续
			time.Sleep(time.Second)
			continue
		}
		if len(res) < 2 {
			continue
		}
		taskId := res[1]
		log.Printf("[deleteWorker] processing task=%s", taskId)

		// 查询任务是否存在（防止重复删除）
		var t models.Task
		if err := mysqldb.DB.First(&t, "id = ?", taskId).Error; err != nil {
			log.Printf("[deleteWorker] task not found in MySQL: %s", taskId)
			continue
		}

		// 事务删除 Task 与关联 Target
		tx := mysqldb.DB.Begin()
		if err := tx.Where("task_id = ?", taskId).Delete(&models.Target{}).Error; err != nil {
			tx.Rollback()
			log.Printf("[deleteWorker] failed to delete targets for task %s: %v", taskId, err)
			_ = redisdb.Client.RPush(ctx, failedKey, taskId).Err()
			time.Sleep(backoffBase)
			continue
		}
		if err := tx.Where("id = ?", taskId).Delete(&models.Task{}).Error; err != nil {
			tx.Rollback()
			log.Printf("[deleteWorker] failed to delete task %s: %v", taskId, err)
			_ = redisdb.Client.RPush(ctx, failedKey, taskId).Err()
			time.Sleep(backoffBase)
			continue
		}
		tx.Commit()

		// 删除 Redis 相关 key
		keys := []string{
			"task:" + taskId + ":info",
			"task:" + taskId + ":targets",
			"task:" + taskId + ":result",
			"task:" + taskId + ":log",
		}
		if err := redisdb.Client.Del(ctx, keys...).Err(); err != nil {
			log.Printf("[deleteWorker] failed to delete Redis keys for task %s: %v", taskId, err)
			_ = redisdb.Client.RPush(ctx, failedKey, taskId).Err()
			time.Sleep(backoffBase)
			continue
		}

		// 从 tasks:list 移除
		if err := redisdb.Client.LRem(ctx, "tasks:list", 0, taskId).Err(); err != nil {
			log.Printf("[deleteWorker] failed to remove task from list: %s, err: %v", taskId, err)
			_ = redisdb.Client.RPush(ctx, failedKey, taskId).Err()
			time.Sleep(backoffBase)
			continue
		}

		log.Printf("[deleteWorker] task %s deleted successfully", taskId)
		time.Sleep(100 * time.Millisecond) // 避免循环过快
	}
}
