package mysqldb

import (
	"log"
	"time"
)

// 此文件在 package mysqldb 下，和你现有的 mysqldb.go 同目录。
// 它会在包初始化（import 时）启动一个后台协程，等待 mysqldb.DB 被 Init() 设置后自动执行建表 SQL。
// 因为我们不修改你的 main.go，所以采用轮询方式检测 DB 就绪，等待时间与重试次数可调整。

func init() {
	go func() {
		// 等待 mysqldb.DB 被初始化（由你的 mysqldb.Init() 设置）
		// 超时时间：60 秒（可改）
		maxWait := 60 * time.Second
		interval := 500 * time.Millisecond
		deadline := time.Now().Add(maxWait)

		for {
			if DB != nil {
				// 找到 DB，执行建表
				if err := ensureSchema(); err != nil {
					log.Printf("[mysqldb/schema] ensureSchema failed: %v", err)
				} else {
					log.Println("[mysqldb/schema] ensureSchema succeeded")
				}
				return
			}
			if time.Now().After(deadline) {
				log.Printf("[mysqldb/schema] DB not ready after %v; abort schema init", maxWait)
				return
			}
			time.Sleep(interval)
		}
	}()
}

func ensureSchema() error {
	// 使用 GORM 的 Exec 来运行幂等 SQL（CREATE TABLE IF NOT EXISTS）
	// 表名采用复数（tasks, targets, findings, task_logs），
	// tasks.id 为 VARCHAR(64) 以匹配你 generateTaskID() 生成的 hex id。
	// targets 使用自增 id 且 task_id 为 VARCHAR(64)，并添加外键 ON DELETE CASCADE。

	sqls := []string{
		// tasks 表（持久化任务元信息）
		`CREATE TABLE IF NOT EXISTS tasks (
			id VARCHAR(64) NOT NULL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			status VARCHAR(32) NOT NULL DEFAULT 'pending',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

		// targets 表（每个目标一行，关联 tasks.id）
		`CREATE TABLE IF NOT EXISTS targets (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			task_id VARCHAR(64) NOT NULL,
			target VARCHAR(512) NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			INDEX idx_task_id (task_id),
			CONSTRAINT fk_targets_task FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

		// findings 表（扫描结果摘要；raw_ref 可存对象存储路径）
		`CREATE TABLE IF NOT EXISTS findings (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			task_id VARCHAR(64) NOT NULL,
			target VARCHAR(512) NOT NULL,
			template_id VARCHAR(128),
			severity VARCHAR(32),
			title VARCHAR(512),
			details JSON,
			raw_ref VARCHAR(1024),
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_task_id (task_id),
			INDEX idx_template_id (template_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,

		// task_logs 表（审计 / 操作日志）
		`CREATE TABLE IF NOT EXISTS task_logs (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			task_id VARCHAR(64),
			action VARCHAR(64) NOT NULL,
			actor VARCHAR(128),
			message TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_task_action (task_id, action)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
	}

	for _, q := range sqls {
		if err := DB.Exec(q).Error; err != nil {
			return err
		}
	}

	// 确保外键约束启用（通常 InnoDB 默认为启用），若需额外设置可在此处执行
	return nil
}
