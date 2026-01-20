package aiaudit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AuditState represents the state of the vulnerability analysis.
type AuditState struct {
	VulnerabilityID string
	RuleID          string
	VulnDescription string         // Added basic description
	CodeFlows       []CodeLocation // Extracted from SARIF
	Context         []string       // Extracted code snippets (function bodies)
	IsFalsePositive bool
	Reason          string
	Thinking        string
}

type CodeLocation struct {
	FilePath string
	Line     int
}

// RunAuditPipeline executes the AI audit workflow.
// This is a simplified "Graph" execution: ContextBuilder -> SecurityAuditor
func RunAuditPipeline(ctx context.Context, state *AuditState) error {
	// Node 1: ContextBuilder
	if err := buildContext(state); err != nil {
		return fmt.Errorf("context builder failed: %w", err)
	}

	// Node 2: SecurityAuditor (AI)
	if err := runSecurityAuditor(ctx, state); err != nil {
		return fmt.Errorf("security auditor failed: %w", err)
	}

	return nil
}

// Node 1: ContextBuilder
// Iterates through code flows and extracts full function bodies.
func buildContext(state *AuditState) error {
	var contexts []string
	seenFunctions := make(map[string]bool)
	var projectRoot string

	// Determine project root from the first file path
	if len(state.CodeFlows) > 0 {
		projectRoot = findProjectRoot(state.CodeFlows[0].FilePath)
	}

	for i, loc := range state.CodeFlows {
		// Use our extractor to get the full function body
		code, err := ExtractFunctionBody(loc.FilePath, loc.Line)
		if err != nil {
			// Fallback: just record the line if extraction fails
			contexts = append(contexts, fmt.Sprintf("Step %d (Extraction Failed): %s:%d", i+1, loc.FilePath, loc.Line))
			continue
		}

		// Avoid duplicate function bodies to save tokens
		// A simple hash/signature check could be added here
		// For now, we assume if the extracted code is identical, skip it
		if seenFunctions[code] {
			continue
		}
		seenFunctions[code] = true

		contexts = append(contexts, fmt.Sprintf("Step %d (%s:%d):\n%s", i+1, loc.FilePath, loc.Line, code))

		// Enrich context with definitions of called functions
		if strings.HasSuffix(loc.FilePath, ".java") && projectRoot != "" {
			defs, _ := ExtractCalledFunctions(code, projectRoot, ".java")
			for _, def := range defs {
				// def is "Definition of X:\nBody..."
				// We check if the Body part is already seen?
				// The def string includes a header.
				// Let's just check the whole string or extract body.
				// For simplicity, check the whole string.
				if !seenFunctions[def] {
					seenFunctions[def] = true
					contexts = append(contexts, fmt.Sprintf("Supplementary Context:\n%s", def))
				}
			}
		}
	}

	state.Context = contexts
	return nil
}

func findProjectRoot(startPath string) string {
	dir := filepath.Dir(startPath)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "pom.xml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir || parent == "." || parent == "/" {
			return filepath.Dir(startPath) // Fallback
		}
		dir = parent
	}
}

// Node 2: SecurityAuditor
// Calls the LLM to analyze the context.
func runSecurityAuditor(ctx context.Context, state *AuditState) error {
	if len(state.Context) == 0 {
		return fmt.Errorf("no context available for analysis")
	}

	// Construct the prompt
	prompt := fmt.Sprintf(`你现在的目标是**仅识别误报(False Positive)**。
分析代码路径，寻找是否存在**净化函数(Sanitizer)**、**校验函数(Validator)**或**白名单(Allowlist)**。

漏洞描述：
%s

规则：
1. **重点关注**代码中标注了 "<-- 关注这里 (Focus Here)" 的行。这些是污点数据流经过的关键步骤（如 Source、Sink 或中间传递点）。
2. 如果代码中包含多个逻辑分支（如 switch/if），**仅分析**数据流实际经过的路径。
   - 上下文中的 "Step X" 会告诉你数据流在第几行。请核对行号。
   - 不要被其他未执行的分支代码（如 switch 中的其他 case）干扰。
   - 如果漏洞发生在 case "raw"，而 case "writeList" 中有白名单，这**不能**证明 case "raw" 是安全的。

3. 如果代码中**没有发现**任何疑似的净化、校验或白名单函数：
   - **不要**分析漏洞原理。
   - **不要**浪费时间解释为什么是漏洞。
   - 直接判定为 **TRUE POSITIVE**。
   - [THINKING] 填写：未在上下文中发现净化或校验函数。

4. 仅当**发现**了疑似净化/校验函数时：
   - 深入分析该函数是否能有效防御 %s 漏洞。
   - 必须确认该函数是否针对**该漏洞的特定攻击向量**有效（例如：SQL注入是否防御了IN子句注入？路径遍历是否防御了..跳转？）。
   - 如果函数名暗示安全（如 checkWhiteList）但**看不到具体实现**，且无标准库文档支持，应保持怀疑，判定为 **TRUE POSITIVE**，并在理由中说明“缺失函数实现上下文”。
   - 如果有效，判定为 **FALSE POSITIVE**。
   - 如果无效，判定为 **TRUE POSITIVE**。

代码上下文：
%s

请严格使用以下格式回复（参考示例）：
[THINKING]: 
1. 漏洞发生在 Step 3 (行号 45)，位于 switch 的 case "raw" 分支中。
2. 虽然同函数内的 case "safe" 分支有过滤逻辑，但数据流并未经过该分支。
3. 在 case "raw" 分支中，输入变量直接拼接到了 SQL 语句中，且未发现任何净化函数。
4. 因此判定为真阳性。

[VERDICT]: FALSE POSITIVE / TRUE POSITIVE
[REASON]: <一句话中文解释原因，引用具体的函数或变量名>
`, state.VulnDescription, state.RuleID, strings.Join(state.Context, "\n\n---\n\n"))

	// Call LLM (Real or Mock based on Env)
	llmResponse, err := callLLM(ctx, prompt)
	if err != nil {
		return err
	}

	// Parse response
	state.Thinking = parseSection(llmResponse, "[THINKING]:")
	state.Reason = parseSection(llmResponse, "[REASON]:")

	// Robust Verdict Parsing:
	// 1. Check for specific "FALSE POSITIVE" string (case insensitive)
	// 2. Be tolerant of whitespace and punctuation
	upperResponse := strings.ToUpper(llmResponse)
	// Normalize whitespace to single spaces to handle [VERDICT]:  FALSE POSITIVE
	normalized := strings.Join(strings.Fields(upperResponse), " ")

	if strings.Contains(normalized, "[VERDICT]: FALSE POSITIVE") {
		state.IsFalsePositive = true
	} else {
		state.IsFalsePositive = false
	}

	// Log for debugging
	fmt.Printf("AI Audit Result for %s:\nVerdict: %v\nThinking: %s\n", state.RuleID, state.IsFalsePositive, state.Thinking)

	return nil
}

func mockLLMCall(prompt string) (string, error) {
	// Simulation logic: if the code contains "Sanitize" or "Validate", assume AI says it's safe.
	if strings.Contains(prompt, "Sanitize") || strings.Contains(prompt, "Validate") || strings.Contains(prompt, "filter") {
		return `[THINKING]: 
1. 我分析了代码执行路径，在第45行发现了对 'SanitizeInput' 函数的调用。
2. 'SanitizeInput' 函数明确地将特殊字符（如 <, >, '）替换为 HTML 实体。
3. 这一操作在恶意数据到达 sink 之前有效地中和了 XSS 攻击载荷。
4. 因此，污点数据流被阻断，该漏洞不可被利用。

[VERDICT]: FALSE POSITIVE
[REASON]: 在执行路径中发现了有效的过滤函数 'SanitizeInput'。`, nil
	}
	return `[THINKING]: 
1. 我追踪了数据从输入源 'request.Query' 到 sink 'db.Exec' 的完整流程。
2. 数据虽然经过了 'formatString' 函数处理，但该函数仅进行了大小写转换，并未对 SQL 特殊字符进行转义。
3. 在整个路径中未发现其他有效的验证或过滤逻辑。
4. 原始输入直接参与了 SQL 查询的构造，导致了 SQL 注入漏洞。

[VERDICT]: TRUE POSITIVE
[REASON]: 在 source 和 sink 之间未检测到有效的过滤或验证逻辑。`, nil
}

func parseSection(response, header string) string {
	parts := strings.Split(response, header)
	if len(parts) > 1 {
		// Find the end of this section (start of next section or end of string)
		content := parts[1]
		nextHeaderIndex := -1

		headers := []string{"[THINKING]:", "[VERDICT]:", "[REASON]:"}
		for _, h := range headers {
			if h == header {
				continue
			}
			idx := strings.Index(content, h)
			if idx != -1 {
				if nextHeaderIndex == -1 || idx < nextHeaderIndex {
					nextHeaderIndex = idx
				}
			}
		}

		if nextHeaderIndex != -1 {
			return strings.TrimSpace(content[:nextHeaderIndex])
		}
		return strings.TrimSpace(content)
	}
	return ""
}
