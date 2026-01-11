sudo apt install redis-server -y
go install -v github.com/projectdiscovery/nuclei/v3/cmd/nuclei@latest
go mod tidy
export CGO_ENABLED=1 CC=gcc 
export CGO_CFLAGS="-I/usr/include/pcap" 
export CGO_LDFLAGS="-lpcap" 
go run main.go