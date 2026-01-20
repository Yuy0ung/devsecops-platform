package aiaudit

import (
	"context"
	"fmt"
	"strings"
)

// AuditState represents the state of the vulnerability analysis.
type AuditState struct {
	VulnerabilityID string
	RuleID          string
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
	}
	state.Context = contexts
	return nil
}

// Node 2: SecurityAuditor
// Calls the LLM to analyze the context.
func runSecurityAuditor(ctx context.Context, state *AuditState) error {
	if len(state.Context) == 0 {
		return fmt.Errorf("no context available for analysis")
	}

	// Construct the prompt
	prompt := fmt.Sprintf(`You are a senior application security expert.
Analyze the following code execution path for a potential %s vulnerability.
The path consists of sequential function calls where data flows from source to sink.

Code Context:
%s

Task:
Determine if there is a sanitizer, validator, or logic check that effectively prevents the vulnerability.
If yes, mark as FALSE POSITIVE.
If no, or if you are unsure, mark as TRUE POSITIVE.

Response Format:
[THINKING]: <Step-by-step reasoning, explaining which functions were analyzed and why they are safe/unsafe>
[VERDICT]: FALSE POSITIVE / TRUE POSITIVE
[REASON]: <One sentence explanation for the UI>
`, state.RuleID, strings.Join(state.Context, "\n\n---\n\n"))

	// Call LLM (Real or Mock based on Env)
	llmResponse, err := callLLM(ctx, prompt)
	if err != nil {
		return err
	}

	// Parse response
	state.Thinking = parseSection(llmResponse, "[THINKING]:")
	state.Reason = parseSection(llmResponse, "[REASON]:")

	if strings.Contains(llmResponse, "[VERDICT]: FALSE POSITIVE") {
		state.IsFalsePositive = true
	} else {
		state.IsFalsePositive = false
	}

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
