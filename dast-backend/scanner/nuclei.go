/**
 * nuclei 扫描模块
 */
package scanner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"demo/db/redisdb"

	nuclei "github.com/projectdiscovery/nuclei/v3/lib"
	"github.com/projectdiscovery/nuclei/v3/pkg/catalog/disk"
	"github.com/projectdiscovery/nuclei/v3/pkg/output"
)

// NucleiScan 只负责把给定的 host:port 列表跑 nuclei，结果写入 Redis
// 状态（running/finished/error/stopped）由上层 Run 负责更新
func NucleiScan(ctx context.Context, taskId string, nucleiTargets []string) error {
	fmt.Println("[+]nuclei start")

	// 创建 nuclei 引擎（带 ctx），并指定本地 poc/templates 目录为 ./poc
	engine, err := nuclei.NewNucleiEngineCtx(ctx,
		nuclei.WithCatalog(disk.NewCatalog("./poc")),
		nuclei.DisableUpdateCheck(), // 关闭自动检查/下载模板
		nuclei.WithDisableClustering(),
	)
	if err != nil {
		fmt.Println("[+]create nuclei engine error:", err)
		return err
	}
	defer engine.Close()

	// 明确加载目录下的所有模板（确保模板被解析并缓存）
	if err := engine.LoadAllTemplates(); err != nil {
		// 如果加载失败，可以选择继续或直接返回错误
		return fmt.Errorf("[+]load templates failed: %w", err)
	}

	// 用内存里的 targets 构造一个 reader，效果等价于 "-l targets.txt"
	joined := strings.Join(nucleiTargets, "\n")
	reader := bufio.NewReader(strings.NewReader(joined))
	engine.LoadTargetsFromReader(reader, false)

	// nuclei 结果回调：只写入真正命中的漏洞结果
	writeCallback := func(ev *output.ResultEvent) {
		if ev == nil {
			return
		}

		// 只保留漏洞匹配成功的结果
		if !ev.MatcherStatus {
			return
		}
		// 防止 response 过大
		if len(ev.Response) > 10240 {
			ev.Response = ev.Response[:10240]
		}

		data, err := json.Marshal(ev)
		if err != nil {
			return
		}
		jsonStr := string(data)

		// 写入 Redis 结果命中的漏洞列表
		redisdb.Client.RPush(redisdb.Ctx, "task:"+taskId+":result", jsonStr)
		// 记录命中结果也同步写到 log
		redisdb.Client.RPush(redisdb.Ctx, "task:"+taskId+":log", jsonStr)
	}

	// 执行扫描
	if err := engine.ExecuteCallbackWithCtx(ctx, writeCallback); err != nil {
		fmt.Println("[+]nuclei start error:", err)
		return err
	}
	return nil
}
