/**
* naabu SDK实现端口扫描
 */
package scanner

import (
	"context"
	"fmt"
	"strings"
	"time"

	naaburesult "github.com/projectdiscovery/naabu/v2/pkg/result"
	naaburunner "github.com/projectdiscovery/naabu/v2/pkg/runner"
)

// PortScan 使用 naabu 对给定 host 列表做端口扫描，返回 host:port 列表。
// 注意：这里假设传入的 hosts 都是不带端口的，例如：1.2.3.4 / example.com
func PortScan(ctx context.Context, hosts []string) ([]string, error) {
	fmt.Println("[+]naabu start")
	if len(hosts) == 0 {
		return nil, nil
	}

	openTargets := make([]string, 0)

	options := &naaburunner.Options{
		// 这些参数可以按实际情况调整
		Rate:     1000, // 扫描速率
		TopPorts: "1000",
		Timeout:  5 * time.Second, // 单个探测的超时
		Threads:  25,              // 并发线程数

		Silent: true,  // 不往 stdout 打日志
		JSON:   false, // 不直接输出 JSON，我们用回调拿结果

		// 只要端口扫描，不做额外 Host 探测
		WithHostDiscovery: false,
		SkipHostDiscovery: true,

		// 每个 host扫描结果回调（naabu v2 提供的 HostResult）
		OnResult: func(hr *naaburesult.HostResult) {
			if hr == nil {
				return
			}
			host := hr.Host
			if host == "" {
				host = hr.IP
			}
			if host == "" {
				return
			}
			for _, p := range hr.Ports {
				openTargets = append(openTargets, fmt.Sprintf("%s:%d", host, p.Port))
			}
		},
	}

	r, err := naaburunner.NewRunner(options)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	// 把 host 加到 runner 里
	for _, h := range hosts {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		_ = r.AddTarget(h)
	}

	// 开始端口扫描
	if err := r.RunEnumeration(ctx); err != nil {
		return nil, err
	}
	return openTargets, nil
}
