package target

import (
	"context"
	"demo/db/mysqldb"
	"demo/db/redisdb"
	"demo/models"
	"encoding/json"
	"log"
	"time"
)

// 补偿 worker：消费 "task:sync:targets:queue"，将 MySQL 中该 task 的 targets 补回 Redis。
// deleteWorker: 消费 "target:delete:queue"
func Init() {
	go compensatorWorker()
	go deleteWorker()
}

// deleteWorker 消费 "target:delete:queue"
// 支持两种入队格式（灵活适配不同调用处）：
// 1) 仅推入 taskId 字符串 -> 表示删除该任务下的所有 targets（MySQL+Redis）
// 2) 推入 JSON 对象 {"taskId":"...", "targets": ["a","b", ...]} -> 表示删除指定 targets（批量）
// 失败时会把原始 payload 推到 failedKey 以便人工或后续重试处理。
func deleteWorker() {
	ctx := context.Background()
	const queueKey = "target:delete:queue"
	const failedKey = "target:delete:failed"
	backoffBase := time.Second * 2

	type payload struct {
		TaskId  string   `json:"taskId"`
		Targets []string `json:"targets,omitempty"`
	}

	for {
		res, err := redisdb.Client.BLPop(ctx, 5*time.Second, queueKey).Result()
		if err != nil {
			// 超时或错误，稍后重试
			time.Sleep(time.Second)
			continue
		}
		if len(res) < 2 {
			continue
		}
		raw := res[1]
		log.Printf("[target.deleteWorker] popped payload=%s", raw)

		var p payload
		isJSON := json.Unmarshal([]byte(raw), &p) == nil && p.TaskId != ""
		if !isJSON {
			// treat raw as plain taskId
			p = payload{TaskId: raw}
		}

		// 防御：taskId 必须存在
		if p.TaskId == "" {
			log.Printf("[target.deleteWorker] invalid payload(no taskId): %s; pushing to failed", raw)
			_ = redisdb.Client.RPush(ctx, failedKey, raw).Err()
			continue
		}

		// 如果指定了 targets 列表 -> 删除这些 targets（按项删除 MySQL 再清 Redis）
		if len(p.Targets) > 0 {
			// 使用事务删除 MySQL 中对应的 rows
			tx := mysqldb.DB.Begin()
			if tx.Error != nil {
				log.Printf("[target.deleteWorker] db begin failed task=%s err=%v; requeue", p.TaskId, tx.Error)
				_ = redisdb.Client.RPush(ctx, queueKey, raw).Err()
				time.Sleep(backoffBase)
				continue
			}

			if err := tx.Where("task_id = ? AND target IN ?", p.TaskId, p.Targets).Delete(&models.Target{}).Error; err != nil {
				_ = tx.Rollback()
				log.Printf("[target.deleteWorker] db delete targets failed task=%s err=%v; move to failed", p.TaskId, err)
				_ = redisdb.Client.RPush(ctx, failedKey, raw).Err()
				time.Sleep(backoffBase)
				continue
			}
			if err := tx.Commit().Error; err != nil {
				_ = tx.Rollback()
				log.Printf("[target.deleteWorker] db commit failed task=%s err=%v; requeue", p.TaskId, err)
				_ = redisdb.Client.RPush(ctx, queueKey, raw).Err()
				time.Sleep(backoffBase)
				continue
			}

			// Redis: 逐项 LREM
			redisKey := GetTaskTargetsKey(p.TaskId)
			var redisErrs []string
			for _, t := range p.Targets {
				if _, err := redisdb.Client.LRem(ctx, redisKey, 0, t).Result(); err != nil {
					redisErrs = append(redisErrs, err.Error())
				}
			}
			if len(redisErrs) > 0 {
				log.Printf("[target.deleteWorker] redis LRem had errors task=%s errs=%v; pushing to failed", p.TaskId, redisErrs)
				_ = redisdb.Client.RPush(ctx, failedKey, raw).Err()
				continue
			}

			log.Printf("[target.deleteWorker] partial delete succeeded task=%s targets=%d", p.TaskId, len(p.Targets))
			// done for this payload
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// 否则：删除该 taskId 下的所有 targets（MySQL + Redis）
		// 使用事务删除 MySQL 中所有 targets for task
		tx := mysqldb.DB.Begin()
		if tx.Error != nil {
			log.Printf("[target.deleteWorker] db begin failed task=%s err=%v; requeue", p.TaskId, tx.Error)
			_ = redisdb.Client.RPush(ctx, queueKey, raw).Err()
			time.Sleep(backoffBase)
			continue
		}

		if err := tx.Where("task_id = ?", p.TaskId).Delete(&models.Target{}).Error; err != nil {
			_ = tx.Rollback()
			log.Printf("[target.deleteWorker] db delete all targets failed task=%s err=%v; move to failed", p.TaskId, err)
			_ = redisdb.Client.RPush(ctx, failedKey, raw).Err()
			time.Sleep(backoffBase)
			continue
		}

		if err := tx.Commit().Error; err != nil {
			_ = tx.Rollback()
			log.Printf("[target.deleteWorker] db commit failed task=%s err=%v; requeue", p.TaskId, err)
			_ = redisdb.Client.RPush(ctx, queueKey, raw).Err()
			time.Sleep(backoffBase)
			continue
		}

		// 删除 Redis 整个 targets 列表
		redisKey := GetTaskTargetsKey(p.TaskId)
		if err := redisdb.Client.Del(ctx, redisKey).Err(); err != nil {
			log.Printf("[target.deleteWorker] redis del failed task=%s err=%v; push to failed", p.TaskId, err)
			_ = redisdb.Client.RPush(ctx, failedKey, raw).Err()
			time.Sleep(backoffBase)
			continue
		}
		// 同时尝试从任务 list 中移除（若存在）
		if err := redisdb.Client.LRem(ctx, "tasks:list", 0, p.TaskId).Err(); err != nil {
			log.Printf("[target.deleteWorker] redis LRem tasks:list failed task=%s err=%v; continue", p.TaskId, err)
			// 不一定要视为致命错误
		}

		log.Printf("[target.deleteWorker] deleted all targets for task=%s", p.TaskId)
		time.Sleep(100 * time.Millisecond)
	}
}
func compensatorWorker() {
	ctx := context.Background()
	const queueKey = "task:sync:targets:queue"
	const failedKey = "task:sync:targets:failed"
	backoffBase := time.Second * 2

	for {
		// BLPop 超时，避免永久阻塞导致无法优雅退出（我们简单循环）
		res, err := redisdb.Client.BLPop(ctx, 5*time.Second, queueKey).Result()
		if err != nil {
			// 超时会返回 redis.Nil 或 context deadline; 在 go-redis v8, timeout 无元素时返回 redis.Nil
			// 这里统一 sleep 后继续
			time.Sleep(time.Second)
			continue
		}
		if len(res) < 2 {
			// 非法数据，继续
			continue
		}
		taskId := res[1]
		log.Printf("[compensator] popped task=%s from queue", taskId)

		// 查询 MySQL 中该任务的 targets
		var dbTargets []models.Target
		if err := mysqldb.DB.Where("task_id = ?", taskId).Find(&dbTargets).Error; err != nil {
			log.Printf("[compensator] mysql query failed task=%s err=%v; requeue after backoff", taskId, err)
			// 回退到队列末尾并稍作延迟（avoid hot loop）
			_ = redisdb.Client.RPush(ctx, queueKey, taskId).Err()
			time.Sleep(backoffBase)
			continue
		}

		if len(dbTargets) == 0 {
			log.Printf("[compensator] no targets found in mysql for task=%s, skipping", taskId)
			continue
		}

		// 将 MySQL 中的 targets 写回 Redis（去重不是这里的职责，按原逻辑写入）
		redisKey := GetTaskTargetsKey(taskId)
		targetsInterface := make([]interface{}, 0, len(dbTargets))
		for _, t := range dbTargets {
			targetsInterface = append(targetsInterface, t.Target)
		}

		if err := redisdb.Client.RPush(ctx, redisKey, targetsInterface...).Err(); err != nil {
			log.Printf("[compensator] redis push failed task=%s err=%v; move to failed list", taskId, err)
			// 写入失败队列，供人工介入或后续批量处理
			_ = redisdb.Client.RPush(ctx, failedKey, taskId).Err()
			continue
		}

		// 成功同步后，把任务状态尝试恢复为 pending（best-effort）
		if err := mysqldb.DB.Model(&models.Task{}).Where("id = ?", taskId).Update("status", "pending").Error; err != nil {
			log.Printf("[compensator] mysql update status failed task=%s err=%v", taskId, err)
		}

		log.Printf("[compensator] sync succeeded task=%s count=%d", taskId, len(dbTargets))
		// 短暂停，避免对 Redis/MySQL 造成突发压力
		time.Sleep(200 * time.Millisecond)
	}
}
