package main

import (
	"demo/db/mysqldb"
	"demo/db/redisdb"
	"demo/log"
	"demo/sast"
	"demo/sca"
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
	sast.Init()
	sca.Init()

	router := gin.Default()
	// Set a lower memory limit for multipart forms (default is 32 MiB)
	router.MaxMultipartMemory = 8 << 20 // 8 MiB

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

		// DAST 模块
		dast := v1.Group("/dast")
		{
			// 任务管理
			tasks := dast.Group("/task")
			{
				tasks.POST("/create", task.Create())
				tasks.GET("/list", task.List())
				tasks.GET("/start", task.Start())
				tasks.GET("/stop", task.Stop())
				tasks.GET("/delete", task.Delete())
			}

			// 目标管理
			targets := dast.Group("/target")
			{
				targets.GET("/list", target.List())
				targets.POST("/add", target.Add())
				targets.POST("/delete", target.Delete())
				targets.GET("/result", target.Result())
			}
		}

		// SAST 模块
		sastGroup := v1.Group("/sast")
		{
			sastGroup.POST("/codeql/create", sast.Create())
			// Upload functionality removed
			sastGroup.GET("/vuln/list", sast.List())
			sastGroup.GET("/vuln/result/:id", sast.Result())
			sastGroup.POST("/vuln/delete/:id", sast.Delete())
			sastGroup.GET("/file/:id", sast.GetFileContent())
		}

		// SCA 模块
		scaGroup := v1.Group("/sca")
		{
			scaGroup.POST("/create", sca.Create())
			// Chunked upload endpoints
			scaGroup.POST("/upload/init", sca.InitUpload())
			scaGroup.POST("/upload/chunk", sca.UploadChunk())
			scaGroup.POST("/upload/merge", sca.MergeChunks())
			scaGroup.GET("/list", sca.List())
			scaGroup.GET("/vuln/result/:id", sca.Result())
			scaGroup.POST("/vuln/delete/:id", sca.Delete())
		}

		// 日志管理
		v1.GET("/log", log.GetLog())
	}

	router.Run(":5003") // 在 5003 端口监听并启动服务
}
