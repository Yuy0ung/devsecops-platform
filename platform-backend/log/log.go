package log

import (
	"demo/db/redisdb"

	"github.com/gin-gonic/gin"
)

func GetLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskId := c.Query("taskId")
		if taskId == "" {
			c.JSON(400, gin.H{"error": "missing taskId"})
			return
		}

		logKey := "task:" + taskId + ":log"

		// 读取最近 N 条日志（例如最后 100 条）
		logs, err := redisdb.Client.LRange(redisdb.Ctx, logKey, -100, -1).Result()
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		c.JSON(200, gin.H{
			"taskId": taskId,
			"logs":   logs,
		})
	}
}
