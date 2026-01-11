package redisdb

import (
	"context"
	"log"

	"github.com/redis/go-redis/v9"
)

var (
	Ctx    = context.Background()
	Client *redis.Client
)

func Init(addr, password string, db int) {
	Client = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	opt := Client.Options()
	log.Printf("redis init: addr=%s db=%d (password set=%v)", opt.Addr, opt.DB, password != "")
	if err := Client.Ping(Ctx).Err(); err != nil {
		log.Fatalf("redis ping failed: %v", err)
	}
	log.Printf("redis ping ok")
}
