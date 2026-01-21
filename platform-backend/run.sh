#后端启动脚本
sudo apt install redis-server -y
go install -v github.com/projectdiscovery/nuclei/v3/cmd/nuclei@latest
go mod tidy

#配置AI的API KEY
export LLM_API_KEY="模型API key"
export LLM_API_URL="模型调用API URL"
export LLM_MODEL="模型名称" 

export CGO_ENABLED=1 CC=gcc 
export CGO_CFLAGS="-I/usr/include/pcap" 
export CGO_LDFLAGS="-lpcap" 
go run main.go