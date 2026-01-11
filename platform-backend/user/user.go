package user

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"demo/db/redisdb"

	"github.com/gin-gonic/gin"
)

// 固定用户 & 密码
const (
	fixedUsername = "Yuy0ung"
	fixedPassword = "Yuy0ung@test123"

	sessionPrefix = "session:"
	sessionTTL    = 24 * time.Hour
)

// 生成随机 token
func generateToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// 从请求中提取 token：优先 Authorization: Bearer <token>，退而求其次 query/cookie
func extractToken(c *gin.Context) string {
	// 1. Authorization: Bearer xxx
	auth := c.GetHeader("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(auth[len("Bearer "):])
	}

	// 2. URL ?token=xxx
	if t := c.Query("token"); t != "" {
		return t
	}

	// 3. cookie: token=xxx （如果你前端想用 cookie，可以设置）
	if cookie, err := c.Cookie("token"); err == nil && cookie != "" {
		return cookie
	}

	return ""
}

// 登录：POST /api/login
// body 可以是 form 或 json：username=Yuy0ung&password=Yuy0ung@test123
func Login() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 兼容 form + json
		var req struct {
			Username string `json:"username" form:"username"`
			Password string `json:"password" form:"password"`
		}
		if err := c.ShouldBind(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}

		if req.Username != fixedUsername || req.Password != fixedPassword {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
			return
		}

		// 生成 token 并写入 Redis
		token := generateToken()
		key := sessionPrefix + token

		if err := redisdb.Client.Set(redisdb.Ctx, key, fixedUsername, sessionTTL).Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
			return
		}

		// 可选：同时种一个 cookie，方便浏览器自动带 token
		// c.SetCookie("token", token, int(sessionTTL.Seconds()), "/", "", false, true)

		c.JSON(http.StatusOK, gin.H{
			"message":  "login success",
			"username": fixedUsername,
			"token":    token,
		})
	}
}

// 登出：DELETE /api/logout 或 POST /api/logout 都行，看你路由怎么配
func Logout() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token != "" {
			key := sessionPrefix + token
			_ = redisdb.Client.Del(redisdb.Ctx, key).Err()
		}

		// 如果你用了 cookie，也可以顺便清掉：
		// c.SetCookie("token", "", -1, "/", "", false, true)

		c.JSON(http.StatusOK, gin.H{"message": "logout success"})
	}
}

// 全局身份校验中间件
// 建议在 main.go 里：
//
//	r := gin.Default()
//	r.POST("/api/login", user.Login())
//	r.POST("/api/logout", user.Logout())
//	r.Use(user.AuthMiddleware())   // login/logout 之外的接口都要鉴权
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 放行无需登录的接口（可以按需增加）
		path := c.FullPath()
		if path == "/api/login" || path == "/api/logout" {
			c.Next()
			return
		}

		token := extractToken(c)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}

		key := sessionPrefix + token
		username, err := redisdb.Client.Get(redisdb.Ctx, key).Result()
		if err != nil || username == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		// 刷新一下过期时间（滑动过期，可选）
		_ = redisdb.Client.Expire(redisdb.Ctx, key, sessionTTL).Err()

		// 把用户名塞进上下文，后面的 handler 可以用 c.Get("username")
		c.Set("username", username)

		c.Next()
	}
}
