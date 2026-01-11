package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"dast-engine/db/redisdb"
	"dast-engine/scanner"
)

type ScanTask struct {
	TaskId  string   `json:"taskId"`
	Targets []string `json:"targets"`
	InfoKey string   `json:"infoKey"`
}

func main() {
	// Initialize Redis
	redisdb.Init("127.0.0.1:6379", "", 0)

	fmt.Println("[DAST Engine] Worker started, waiting for tasks...")

	ctx := context.Background()

	// Start cancellation listener
	go func() {
		pubsub := redisdb.Client.Subscribe(ctx, "dast:control")
		defer pubsub.Close()

		ch := pubsub.Channel()
		for msg := range ch {
			var cmd struct {
				Action string `json:"action"`
				TaskId string `json:"taskId"`
			}
			if err := json.Unmarshal([]byte(msg.Payload), &cmd); err != nil {
				log.Printf("[DAST Engine] Invalid control msg: %v", err)
				continue
			}

			if cmd.Action == "cancel" {
				log.Printf("[DAST Engine] Cancelling task: %s", cmd.TaskId)
				scanner.Cancel(cmd.TaskId)
			}
		}
	}()

	queueKey := "dast:queue:jobs"

	for {
		// Blocking pop from Redis queue
		res, err := redisdb.Client.BLPop(ctx, 5*time.Second, queueKey).Result()
		if err != nil {
			// Timeout or error, just loop again
			continue
		}

		if len(res) < 2 {
			continue
		}

		payload := res[1]
		var task ScanTask
		if err := json.Unmarshal([]byte(payload), &task); err != nil {
			log.Printf("[DAST Engine] Invalid task payload: %v", err)
			continue
		}

		log.Printf("[DAST Engine] Starting task: %s", task.TaskId)

		// Run scanner in goroutine to allow processing multiple tasks if needed,
		// though typically a worker might want to limit concurrency.
		go scanner.Run(task.TaskId, task.Targets, task.InfoKey)
	}
}
