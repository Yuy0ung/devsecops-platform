package aiaudit

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ExtractCalledFunctions analyzes the source code snippet and finds definitions of called functions.
// Returns a list of function bodies.
func ExtractCalledFunctions(sourceCode string, projectRoot string, fileExt string) ([]string, error) {
	if fileExt == ".java" {
		return resolveJavaCalls(sourceCode, projectRoot)
	}
	// TODO: Go support
	return nil, nil
}

func resolveJavaCalls(sourceCode string, projectRoot string) ([]string, error) {
	// 1. Identify calls
	calls := findJavaMethodCalls(sourceCode)
	if len(calls) == 0 {
		return nil, nil
	}

	// 2. Find definitions
	var definitions []string
	seen := make(map[string]bool)

	// Limit the number of resolved calls to avoid explosion
	// Prioritize calls that look like validators (check, validate, is, has)
	// But user wants "this_is_func_a", so general calls too.
	// Let's just take unique ones.

	for _, call := range calls {
		if seen[call] {
			continue
		}
		seen[call] = true

		def, err := findJavaDefinition(projectRoot, call)
		if err == nil && def != "" {
			definitions = append(definitions, fmt.Sprintf("Definition of %s:\n%s", call, def))
		}
	}
	return definitions, nil
}

func findJavaMethodCalls(code string) []string {
	// Remove strings and comments to avoid false positives
	masked := maskJavaContent(code)

	// Simple regex: word followed by (
	re := regexp.MustCompile(`\b([a-zA-Z0-9_]+)\s*\(`)
	matches := re.FindAllStringSubmatch(masked, -1)

	var calls []string
	keywords := map[string]bool{
		"if": true, "for": true, "while": true, "switch": true, "catch": true,
		"try": true, "synchronized": true, "super": true, "this": true,
		"println": true, "print": true, "equals": true, "hashCode": true, "toString": true,
		"main": true, "getClass": true, "wait": true, "notify": true, "notifyAll": true,
		"assert": true, "return": true, "throw": true, "new": true,
		"log": true, "info": true, "error": true, "warn": true, "debug": true,
		"append": true, "builder": true, "length": true, "size": true, "get": true, "set": true,
		"stream": true, "filter": true, "map": true, "collect": true, "of": true, "asList": true,
	}

	for _, m := range matches {
		name := m[1]
		if !keywords[name] {
			calls = append(calls, name)
		}
	}
	return calls
}

func findJavaDefinition(root string, funcName string) (string, error) {
	// Heuristic: search for "Type funcName(" or "void funcName(" or "public ... funcName("

	var foundFile string
	var foundLine int

	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Skip .git, node_modules, etc.
			if strings.HasPrefix(info.Name(), ".") || info.Name() == "node_modules" || info.Name() == "target" || info.Name() == "build" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".java") {
			return nil
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		sContent := string(content)

		// Fast fail
		if !strings.Contains(sContent, funcName) {
			return nil
		}

		// Mask content for precise check
		masked := maskJavaContent(sContent)
		lines := strings.Split(masked, "\n")

		// Regex to identify definition
		// Must not have . before name (call)
		// Must have ( after name
		// Must have return type or modifiers before (simplified check)

		// We iterate lines to find the definition line
		for i, line := range lines {
			if strings.Contains(line, funcName) {
				if isJavaDefinition(line, funcName) {
					foundFile = path
					foundLine = i + 1
					return fmt.Errorf("found") // Stop walking
				}
			}
		}
		return nil
	})

	if foundFile != "" {
		return ExtractFunctionBody(foundFile, foundLine)
	}

	return "", fmt.Errorf("not found")
}

func isJavaDefinition(line string, funcName string) bool {
	trimmed := strings.TrimSpace(line)

	// Exclude obvious non-definitions
	if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") {
		return false
	}
	if strings.Contains(trimmed, "."+funcName+"(") {
		return false
	} // Method call
	if strings.HasPrefix(trimmed, "return ") || strings.HasPrefix(trimmed, "throw ") {
		return false
	}
	if strings.HasPrefix(trimmed, "new ") {
		return false
	}

	// Check for "funcName("
	idx := strings.Index(trimmed, funcName+"(")
	if idx == -1 {
		idx = strings.Index(trimmed, funcName+" (")
	}
	if idx == -1 {
		return false
	}

	// If at start, could be constructor "MyClass() {"
	if idx == 0 {
		return true
	}

	// Must be preceded by space or tab
	charBefore := trimmed[idx-1]
	if charBefore != ' ' && charBefore != '\t' {
		return false
	}

	// Check if line ends with { or throws ... {
	// (Not strictly required as brace might be on next line, but good heuristic)

	return true
}

// ExtractFunctionBody attempts to extract the function body containing the specified line.
// Currently supports Go files using go/ast.
// For other languages, it falls back to a simple context window (TODO: implement language-specific parsers).
func ExtractFunctionBody(filePath string, line int) (string, error) {
	if strings.HasSuffix(filePath, ".go") {
		return extractGoFunction(filePath, line)
	}
	if strings.HasSuffix(filePath, ".java") {
		return extractJavaFunction(filePath, line)
	}
	// TODO: Add Python support
	return extractGenericContext(filePath, line, 10)
}

func extractJavaFunction(filePath string, targetLine int) (string, error) {
	// 1. Read all lines
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	// Mask comments and strings to avoid brace confusion
	maskedContent := maskJavaContent(string(content))

	lines := strings.Split(string(content), "\n")
	maskedLines := strings.Split(maskedContent, "\n")

	if targetLine < 1 || targetLine > len(lines) {
		return "", fmt.Errorf("line %d out of bounds", targetLine)
	}

	// 0-indexed target
	targetIdx := targetLine - 1

	// 2. Scan backwards to find the function definition
	// Strategy: Track brace balance. We are looking for the opening brace '{'
	// that encloses the target line and belongs to a function definition.

	balance := 0
	funcStartIdx := -1

	// Regex to identify control flow statements that use braces but aren't functions
	// Matches lines starting with if, for, while, switch, catch, try, else, synchronized
	// We use masked lines for this check too, to avoid matching commented code like // if (...)
	controlFlowRegex := regexp.MustCompile(`^\s*(if|for|while|switch|catch|try|else|synchronized)\b`)

	// Regex to identify class definitions (we don't want to extract the whole class)
	classRegex := regexp.MustCompile(`^\s*(public\s+|private\s+|protected\s+)?(class|interface|enum)\b`)

	for i := targetIdx; i >= 0; i-- {
		// Use masked line for brace counting and structure checks
		line := maskedLines[i]

		// Adjust balance for the current line (scanning backwards)
		// If we see '}', it closes a block we are in (or haven't entered yet if moving back).
		// Wait, scanning backwards:
		// '}' means we are entering a block that ended here. Balance increases.
		// '{' means we are exiting a block that started here. Balance decreases.
		// We are looking for a state where we "exit" the current block at the top.

		// We iterate chars backwards starting from the end of targetLine.
		// (Assuming targetLine itself might contain the brace)

		chars := []rune(line)
		for j := len(chars) - 1; j >= 0; j-- {
			c := chars[j]
			if c == '}' {
				balance++
			} else if c == '{' {
				balance--
				if balance < 0 {
					// Found an enclosing brace!
					// Is this a function?
					// Check the line content "to the left" of this brace, or previous lines.

					// Reconstruct the line up to the brace
					prefix := string(chars[:j])
					trimmedPrefix := strings.TrimSpace(prefix)

					checkLine := line
					checkIdx := i

					// If the brace is at the start of the line, or just preceded by whitespace,
					// we might need to look at the previous line for the signature.
					if trimmedPrefix == "" {
						if i > 0 {
							checkIdx = i - 1
							checkLine = maskedLines[checkIdx]
						}
					}

					// Now check if 'checkLine' looks like a control flow or class
					if controlFlowRegex.MatchString(checkLine) {
						// It's an if/for/etc. We are inside it.
						// We want the function, so we treat this as just another nested block.
						// Effectively, we "consumed" this enclosing brace, but we want the NEXT one.
						// So we continue scanning.
						// Current balance is -1. We want to find -2?
						// No, finding this '{' brought us to -1.
						// If we continue, we are now "outside" this if-block.
						// We want to find the brace that makes us go to -1 relative to THIS new state.
						// So effectively, we keep balance at -1?
						// No, balance is -1. We continue. Next '}' will take us to 0. Next '{' to -2.
						// So we are looking for the next drop in balance.
					} else if classRegex.MatchString(checkLine) {
						// We hit the class level. Stop.
						// This means the target line was directly in the class (field?) or we missed the function.
						return "", fmt.Errorf("hit class definition before function")
					} else {
						// Likely a function!
						// Check if it has '()' to be sure it's a method
						// Note: we check the *masked* line, so comments/strings won't trigger this.
						if strings.Contains(checkLine, "(") && strings.Contains(checkLine, ")") {
							funcStartIdx = checkIdx
							goto FoundStart
						}
						// If no '()', maybe it's a static block or array init? continue.
					}
				}
			}
		}
	}

FoundStart:
	if funcStartIdx == -1 {
		return "", fmt.Errorf("could not find function start for line %d", targetLine)
	}

	// 3. Scan forward from funcStartIdx to find the matching closing brace
	// Now we scan forward character by character to handle nested blocks correctly.

	balance = 0
	funcEndIdx := -1
	foundOpen := false

	for i := funcStartIdx; i < len(maskedLines); i++ {
		line := maskedLines[i]
		for _, c := range line {
			if c == '{' {
				balance++
				foundOpen = true
			} else if c == '}' {
				balance--
				if foundOpen && balance == 0 {
					funcEndIdx = i
					goto FoundEnd
				}
			}
		}
	}

FoundEnd:
	if funcEndIdx == -1 {
		// Fallback: just take until end of file or some limit
		funcEndIdx = len(lines) - 1
	}

	// 4. Include Annotations
	// Scan up from funcStartIdx to include lines starting with @
	realStartIdx := funcStartIdx
	for i := funcStartIdx - 1; i >= 0; i-- {
		// Use original lines for annotation check (though masked is fine too if annotations don't contain //)
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "@") {
			realStartIdx = i
		} else if trimmed == "" {
			continue // Skip empty lines between annotation and function?
		} else {
			break // Stop at non-annotation
		}
	}

	// 5. Return content
	// Use 1-based line numbers for display
	// Also mark the target line with a special marker for AI visibility
	var resultLines []string
	for i := realStartIdx; i <= funcEndIdx; i++ {
		lineNum := i + 1
		lineContent := lines[i]
		if lineNum == targetLine {
			// Add a visual pointer for the AI
			resultLines = append(resultLines, fmt.Sprintf("%d: %s  <-- 关注这里 (Focus Here)", lineNum, lineContent))
		} else {
			resultLines = append(resultLines, fmt.Sprintf("%d: %s", lineNum, lineContent))
		}
	}

	return strings.Join(resultLines, "\n"), nil
}

// maskJavaContent replaces comments and string literals with spaces, keeping newlines.
func maskJavaContent(content string) string {
	var out strings.Builder
	runes := []rune(content)
	length := len(runes)

	inString := false
	inChar := false
	inLineComment := false
	inBlockComment := false

	for i := 0; i < length; i++ {
		c := runes[i]

		if inLineComment {
			if c == '\n' {
				inLineComment = false
				out.WriteRune(c)
			} else {
				out.WriteRune(' ')
			}
			continue
		}

		if inBlockComment {
			if c == '*' && i+1 < length && runes[i+1] == '/' {
				inBlockComment = false
				out.WriteRune(' ')
				out.WriteRune(' ') // consume /
				i++
			} else if c == '\n' {
				out.WriteRune(c) // preserve newline
			} else {
				out.WriteRune(' ')
			}
			continue
		}

		if inString {
			if c == '\\' {
				out.WriteRune(' ') // Mask escaped char
				if i+1 < length {
					out.WriteRune(' ')
					i++
				}
			} else if c == '"' {
				inString = false
				out.WriteRune(' ') // mask quote
			} else {
				out.WriteRune(' ')
			}
			continue
		}

		if inChar {
			if c == '\\' {
				out.WriteRune(' ')
				if i+1 < length {
					out.WriteRune(' ')
					i++
				}
			} else if c == '\'' {
				inChar = false
				out.WriteRune(' ')
			} else {
				out.WriteRune(' ')
			}
			continue
		}

		// Normal CODE state
		if c == '"' {
			inString = true
			out.WriteRune(' ')
		} else if c == '\'' {
			inChar = true
			out.WriteRune(' ')
		} else if c == '/' {
			if i+1 < length {
				next := runes[i+1]
				if next == '/' {
					inLineComment = true
					out.WriteRune(' ')
					out.WriteRune(' ')
					i++
				} else if next == '*' {
					inBlockComment = true
					out.WriteRune(' ')
					out.WriteRune(' ')
					i++
				} else {
					out.WriteRune(c)
				}
			} else {
				out.WriteRune(c)
			}
		} else {
			out.WriteRune(c)
		}
	}
	return out.String()
}

func extractGoFunction(filePath string, targetLine int) (string, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("failed to parse go file: %v", err)
	}

	var foundFunc *ast.FuncDecl

	ast.Inspect(node, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			start := fset.Position(fn.Pos()).Line
			end := fset.Position(fn.End()).Line
			if targetLine >= start && targetLine <= end {
				foundFunc = fn
				return false // Found, stop searching
			}
		}
		return true
	})

	if foundFunc == nil {
		return "", fmt.Errorf("line %d is not inside a function", targetLine)
	}

	// Read the file content to extract the exact string
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	startOffset := fset.Position(foundFunc.Pos()).Offset
	endOffset := fset.Position(foundFunc.End()).Offset

	return string(content[startOffset:endOffset]), nil
}

// extractGenericContext reads N lines before and after the target line
func extractGenericContext(filePath string, targetLine, contextLines int) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	currentLine := 0
	for scanner.Scan() {
		currentLine++
		if currentLine >= targetLine-contextLines && currentLine <= targetLine+contextLines {
			lines = append(lines, fmt.Sprintf("%d: %s", currentLine, scanner.Text()))
		}
	}
	return strings.Join(lines, "\n"), scanner.Err()
}
