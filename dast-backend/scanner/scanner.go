/**
 * 扫描入口
 */
package scanner

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"demo/db/redisdb"
)

// 保存每个任务的 cancel 函数
// key: taskId, value: context.CancelFunc
var taskCancels sync.Map

// Cancel 取消指定 taskId 对应的扫描（无论现在在端口扫描、测活还是 nuclei）
func Cancel(taskId string) {
	if v, ok := taskCancels.Load(taskId); ok {
		if cancel, ok2 := v.(context.CancelFunc); ok2 && cancel != nil {
			cancel()
		}
	}
}

// 总入口：
// 1. 判断是否指定端口
// 2. 未指定端口的目标做端口扫描
// 3. 对所有 host:port 做 HTTP/HTTPS 测活，HTTP 活的转成 URL
// 4. HTTP URL + 其余 host:port 一起丢给 NucleiScan
// 5. 更新 Redis 中 task 的状态（pending -> running -> finished/error/stopped）
func Run(taskId string, rawTargets []string, infoKey string) {
	// 整个任务级别 context，可被 Cancel 中断
	ctx, cancel := context.WithCancel(context.Background())
	taskCancels.Store(taskId, cancel)
	defer taskCancels.Delete(taskId)

	// 任务开始：标记为 running
	setStatus(infoKey, "running", "")

	// 1. 拆分目标
	withPort, hostOnly := splitTargets(rawTargets)

	// 2. 对 hostOnly 做端口扫描
	var hostPortTargets []string
	hostPortTargets = append(hostPortTargets, withPort...)

	if len(hostOnly) > 0 {
		openPorts, err := PortScan(ctx, hostOnly)
		if errors.Is(err, context.Canceled) || ctx.Err() == context.Canceled {
			setStatus(infoKey, "stopped", "")
			return
		}
		if err != nil {
			setStatus(infoKey, "error", fmt.Sprintf("port scan error: %v", err))
			return
		}
		hostPortTargets = append(hostPortTargets, openPorts...)
	}

	// 3. 没有任何 host:port，就算扫描完成
	if len(hostPortTargets) == 0 {
		setStatus(infoKey, "finished", "")
		fmt.Println("[scanner] no targets after port scan")
		return
	}

	// 4. HTTP/HTTPS 测活：
	//    返回一个 map：原始 host:port -> 存活 URL (http:// 或 https://)
	aliveMap, err := HttpAliveProbe(ctx, hostPortTargets)
	if errors.Is(err, context.Canceled) || ctx.Err() == context.Canceled {
		setStatus(infoKey, "stopped", "")
		return
	}
	if err != nil {
		setStatus(infoKey, "error", fmt.Sprintf("http probe error: %v", err))
		return
	}

	// 5. 组装 nuclei 最终 target 列表：
	//    - 如果某个 host:port 在 aliveMap 中：用 http(s)://host:port
	//    - 否则：保留 host:port（给非 HTTP 模板用，比如 redis 等）
	var nucleiTargets []string
	for _, hp := range hostPortTargets {
		if urlStr, ok := aliveMap[hp]; ok && urlStr != "" {
			nucleiTargets = append(nucleiTargets, urlStr)
		} else {
			nucleiTargets = append(nucleiTargets, hp)
		}
	}

	if len(nucleiTargets) == 0 {
		setStatus(infoKey, "finished", "")
		fmt.Println("[scanner] no targets for nuclei after http probe")
		return
	}

	// 6. 调用 nuclei 扫描（这里既有 URL 也有 host:port，让不同协议的模板自己匹配）
	if err := NucleiScan(ctx, taskId, nucleiTargets); err != nil {
		if errors.Is(err, context.Canceled) || ctx.Err() == context.Canceled {
			setStatus(infoKey, "stopped", "")
		} else {
			setStatus(infoKey, "error", fmt.Sprintf("nuclei error: %v", err))
		}
		return
	}

	// 7. 正常完成
	setStatus(infoKey, "finished", "")
	fmt.Println("[+] scanner finished")
}

// -----------------------------------------------------------
// 目标规范化 & 拆分
// -----------------------------------------------------------

// normalizeTarget 尝试把 target 转换成 “host” 或 “host:port”
// 支持：
//   - http://43.153.23.153:8888/
//   - http://43.153.23.153:8888
//   - http://43.153.23.153
//   - https://xxx.com
//   - 1.2.3.4
//   - 1.2.3.4:80
func normalizeTarget(t string) string {
	t = strings.TrimSpace(t)
	if t == "" {
		return ""
	}

	// 如果是 URL，解析出 Host 部分（自带端口）
	if strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://") {
		if u, err := url.Parse(t); err == nil && u.Host != "" {
			// u.Host 形如 "43.153.23.153" 或 "43.153.23.153:8888"
			return u.Host
		}
	}

	// 否则直接返回原始（可能是 ip 或 ip:port 或 域名）
	return t
}

// splitTargets 把原始 target 切分成：
// - withPort: 形如 host:port（能被 net.SplitHostPort 正常解析）
// - hostOnly: 其他（只有主机/IP/域名）
func splitTargets(targets []string) (withPort []string, hostOnly []string) {
	for _, raw := range targets {
		// target 规范化
		t := normalizeTarget(raw)
		if t == "" {
			continue
		}

		if isHostPort(t) {
			withPort = append(withPort, t)
		} else {
			hostOnly = append(hostOnly, t)
		}
	}
	return
}

// isHostPort 尝试用 net.SplitHostPort 判断是否是 host:port 格式
// 例如：1.2.3.4:80、example.com:443
func isHostPort(s string) bool {
	// 没有冒号肯定不是
	if !strings.Contains(s, ":") {
		return false
	}
	// 尝试按 host:port 解析
	_, _, err := net.SplitHostPort(s)
	return err == nil
}

// 状态更新
func setStatus(infoKey, status, errMsg string) {
	data := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now().Format("2006-01-02 15:04:05"),
	}
	if errMsg != "" {
		data["error_msg"] = errMsg
	}
	_ = redisdb.Client.HMSet(redisdb.Ctx, infoKey, data).Err()
}
