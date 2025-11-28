package main

import (
	"demo/db/mysqldb"
	"demo/db/redisdb"
	"demo/log"
	"demo/target"
	"demo/task"
	"demo/user"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	redisdb.Init("127.0.0.1:6379", "", 0)
	mysqldb.Init("root", "123456", "127.0.0.1", "dast")
	mysqldb.DB = mysqldb.DB.Debug()
	task.Init()
	target.Init()

	router := gin.Default()

	// CORS
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"}, // 前端地址
		AllowMethods:     []string{"GET", "POST"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "请在前端服务器访问！",
		})
	})

	// 所有 API 路由
	v1 := router.Group("/api")
	{
		// 登录 / 登出
		v1.POST("/login", user.Login())
		v1.POST("/logout", user.Logout())

		//全局鉴权中间件
		v1.Use(user.AuthMiddleware())

		// 任务管理
		tasks := v1.Group("/task")
		{
			tasks.POST("/create", task.Create())
			tasks.GET("/list", task.List())
			tasks.GET("/start", task.Start())
			tasks.GET("/stop", task.Stop())
			tasks.GET("/delete", task.Delete())
		}

		// 目标管理
		targets := v1.Group("/target")
		{
			targets.GET("/list", target.List())
			targets.POST("/add", target.Add())
			targets.POST("/delete", target.Delete())
			targets.GET("/result", target.Result())
		}

		// 日志管理
		v1.GET("/log", log.GetLog())
	}

	router.Run(":5003") // 在 5003 端口监听并启动服务
}
