# DAST demo

流程：IP - 端口 - 指纹 - POC

要求：扫描节点服务器与性能呈线性关系，有几倍节点，速度就快几倍

## 设计

考虑到开发效率问题，暂时以现成的工具做实现：

* 针对IP做端口扫描，naabu很不错，projectdiscovery的项目，在go的库中也有sdk
* 指纹用ehole，go写的比较快（待定）
* poc用nuclei，比较好找poc，也有稳定的go的SDK
* 后端语言使用go，框架使用gin，轻量快速
* 前端vue+ant-design+axios，构建效率快
* 任务调度使用redis，数据库使用mysql

## ToDoList

- [x] workflow设计
- [x] 工具选择
- [x] nuclei-demo
- [x] naabu-demo
- [x] httpx-demo
- [x] 工具接口开发
- [x] 任务队列
- [x] 数据整理
- [x] 前端设计
- [x] 权限校验
- [ ] 复杂功能、自由度
- [ ] 分布式

## 工作流（暂无指纹）

~~~mermaid
flowchart TD
    A[传入IP列表 -] --> B[调用naabu进行端口扫描 - -]
    B --> C[httpx测活 -]
    C --> D{http协议？}
    D -->|是| E[加上http/https头 --]
    E --> F[nuclei引擎  -]
    D -->|否| F
~~~

## 功能

当前功能实现：

![QQ_1764141406805](https://yuy0ung.oss-cn-chengdu.aliyuncs.com/QQ_1764141406805.png)

## 技术

技术栈

* 前端：antdesign-vue + axios
* 后端：gin
* 数据库：redis + go-redis + mysql + gorm

## 配置

~~~sh
sudo apt install redis-server -y

go install -v github.com/projectdiscovery/nuclei/v3/cmd/nuclei@latest

go mod tidy

go run main.go
~~~

Nginx配置：

~~~sh
vim /etc/nginx/sites-available/spa
sudo tee /etc/nginx/sites-available/spa > /dev/null <<'EOF'
server {
    listen 80;
    server_name _;

    root /var/www/html;
    index index.html;

    access_log /var/log/nginx/spa.access.log;
    error_log  /var/log/nginx/spa.error.log warn;

    # 优先代理后端 API（如果你的后端地址不同请修改 proxy_pass）
    location /api/ {
        proxy_pass http://127.0.0.1:5003;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        add_header Cache-Control "no-store";
    }

    # SPA history fallback：若找不到静态文件则返回 index.html
    location / {
        try_files $uri $uri/ /index.html;
    }
}
EOF
sudo ln -s /etc/nginx/sites-available/spa /etc/nginx/sites-enabled/spa
sudo rm -f /etc/nginx/sites-enabled/default
sudo nginx -t
~~~

mysql：

~~~mysql
ALTER USER 'root'@'localhost' IDENTIFIED BY '你的mysql密码;

CREATE DATABASE IF NOT EXISTS dast
  DEFAULT CHARACTER SET utf8mb4
  COLLATE utf8mb4_general_ci;
~~~
