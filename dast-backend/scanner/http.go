package scanner

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HttpAliveProbe 对给定的目标做 HTTP/HTTPS 测活：
// 输入可以是 host、host:port、url：
//   - 如果本身带 http:// 或 https://，会以该 URL 为主进行探测
//   - 否则会按 host:port 猜测 http/https（带端口会先探测端口是否支持 http/https）
//
// 返回：map[原始输入(规范化后的 host:port)]存活URL
func HttpAliveProbe(ctx context.Context, targets []string) (map[string]string, error) {
	fmt.Println("[+]HttpAliveProbe start")
	aliveMap := make(map[string]string)
	if len(targets) == 0 {
		return aliveMap, nil
	}

	// HTTP client 用于对带 scheme 的候选发 GET 验证
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // 自签证书/内网友好
		},
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}

	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	sem := make(chan struct{}, 50) // 并发限制

	for _, raw := range targets {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}

		wg.Add(1)
		go func(target string) {
			defer wg.Done()

			// respect context & concurrency slot
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			// key 使用你原来的 normalizeTarget（保持兼容）
			key := normalizeTarget(target)
			if key == "" {
				return
			}

			// 构造候选：现在由 buildURLCandidates 做端口探测并返回合适候选
			candidates := buildURLCandidates(ctx, target)
			fmt.Println("[+]target:", candidates)

			for _, cand := range candidates {
				if ctx.Err() != nil {
					return
				}

				// 如果 candidate 带 scheme -> 用 HTTP GET 验证状态码
				if strings.HasPrefix(cand, "http://") || strings.HasPrefix(cand, "https://") {
					req, err := http.NewRequestWithContext(ctx, "GET", cand, nil)
					if err != nil {
						continue
					}
					resp, err := client.Do(req)
					if err != nil {
						continue
					}
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()

					if resp.StatusCode >= 200 && resp.StatusCode < 400 {
						mu.Lock()
						aliveMap[key] = cand
						mu.Unlock()
						return
					}
					// 未满足状态码则继续尝试下一个候选
					continue
				}

				// candidate 不带 scheme（如 "ip:port"），用 TCP 连接判断是否可达
				dialer := &net.Dialer{}
				connCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				conn, err := dialer.DialContext(connCtx, "tcp", cand)
				cancel()
				if err != nil {
					continue
				}
				conn.Close()
				// TCP 可连，认为存活（非 HTTP/HTTPS），返回 ip:port
				mu.Lock()
				aliveMap[key] = cand
				mu.Unlock()
				return
			}
		}(raw)
	}

	wg.Wait()
	if ctx.Err() != nil {
		return aliveMap, ctx.Err()
	}
	return aliveMap, nil
}

// buildURLCandidates 根据 target 构造一组可能的 URL
// 保持外部签名不变，但内部实现会先探测端口协议类型：
// - 如果 target 带 scheme，则直接返回该 URL
// - 若为 host（无端口），返回 ["http://host","https://host"]
// - 若为 host:port，先探测明文 HTTP（HEAD），再探测 HTTPS（TLS+HEAD）
//   - 若探测到 HTTP -> 返回 ["http://host:port"]
//   - 若探测到 HTTPS -> 返回 ["https://host:port"]
//   - 否则返回 ["host:port"]（不加 scheme）
func buildURLCandidates(ctx context.Context, target string) []string {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}

	// 若已带 scheme，直接返回
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return []string{target}
	}

	// 若无端口，按原来逻辑尝试 http/https
	if _, _, err := net.SplitHostPort(target); err != nil {
		return []string{
			"http://" + target,
			"https://" + target,
		}
	}

	// 带端口：先探测 HTTP，再探测 HTTPS
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		// 异常回退到不带 scheme
		return []string{target}
	}
	hp := net.JoinHostPort(host, port)

	// 优先探测明文 HTTP（超时短）
	if isHTTPPort(ctx, hp, 500*time.Millisecond) {
		return []string{"http://" + hp}
	}
	// 再探测 HTTPS（TLS）
	if isHTTPSPort(ctx, host, port, 800*time.Millisecond) {
		return []string{"https://" + hp}
	}
	// 都不是：返回原样 host:port（不加协议）
	return []string{hp}
}

// isHTTPPort: 快速明文 HTTP 探测，发送 HEAD 并读取响应首行判断是否以 "HTTP/" 开头
func isHTTPPort(ctx context.Context, hostport string, timeout time.Duration) bool {
	dialer := &net.Dialer{}
	dctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := dialer.DialContext(dctx, "tcp", hostport)
	if err != nil {
		return false
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))
	_, err = conn.Write([]byte("HEAD / HTTP/1.0\r\n\r\n"))
	if err != nil {
		return false
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "HTTP/")
}

// isHTTPSPort: 通过建立 TLS 握手并发送 HEAD 确认 HTTPS（使用 InsecureSkipVerify）
func isHTTPSPort(ctx context.Context, host, port string, timeout time.Duration) bool {
	hp := net.JoinHostPort(host, port)
	dialer := &net.Dialer{}
	dctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	rawConn, err := dialer.DialContext(dctx, "tcp", hp)
	if err != nil {
		return false
	}
	// 确保在返回前关闭
	defer rawConn.Close()

	tlsCfg := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         host,
	}
	tlsConn := tls.Client(rawConn, tlsCfg)
	_ = tlsConn.SetDeadline(time.Now().Add(timeout))
	if err := tlsConn.Handshake(); err != nil {
		return false
	}

	_, err = tlsConn.Write([]byte("HEAD / HTTP/1.0\r\n\r\n"))
	if err != nil {
		return false
	}
	reader := bufio.NewReader(tlsConn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "HTTP/")
}
