package aiaudit

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

// ExtractFunctionBody attempts to extract the function body containing the specified line.
// Currently supports Go files using go/ast.
// For other languages, it falls back to a simple context window (TODO: implement language-specific parsers).
func ExtractFunctionBody(filePath string, line int) (string, error) {
	if strings.HasSuffix(filePath, ".go") {
		return extractGoFunction(filePath, line)
	}
	// TODO: Add Java/Python support
	return extractGenericContext(filePath, line, 10)
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
