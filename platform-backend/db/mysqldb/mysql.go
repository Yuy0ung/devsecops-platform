package mysqldb

import (
	"demo/models"
	"fmt"
	"log"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// 全局 DB 句柄
var DB *gorm.DB

func Init(user string, pass string, host string, name string) {
	if user == "" || pass == "" || host == "" || name == "" {
		log.Fatal("mysql env not set: MYSQL_USER/MYSQL_PASS/MYSQL_HOST/MYSQL_DB")
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", user, pass, host, name)
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: false,
		},
	})
	if err != nil {
		log.Fatalf("gorm open failed: %v", err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		log.Fatalf("gorm db failed: %v", err)
	}
	// 可选连接池设置
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// 自动迁移创建/修改表结构（生产中用 migrations 管理）
	if err := DB.AutoMigrate(&models.Task{}, &models.Target{}, &models.Finding{}, &models.TaskLog{}); err != nil {
		log.Fatalf("auto migrate failed: %v", err)
	}
}
