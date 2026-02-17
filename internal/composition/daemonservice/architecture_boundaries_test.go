package daemonservice

import (
	"fmt"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type boundaryViolation struct {
	file       string
	line       int
	importPath string
	reason     string
}

func TestArchitecture_DaemonServiceImportsOnlyAllowedPackages(t *testing.T) {
	violations := collectDaemonServiceImportViolations(t)
	if len(violations) == 0 {
		return
	}

	var b strings.Builder
	b.WriteString("daemonservice import boundary violations:\n")
	for _, v := range violations {
		b.WriteString(fmt.Sprintf("- %s:%d import %q (%s)\n", v.file, v.line, v.importPath, v.reason))
	}
	t.Fatal(b.String())
}

func TestBoundaryViolationReason(t *testing.T) {
	tests := []struct {
		importPath string
		expected   string
	}{
		{`aim-chat/go-backend/internal/app`, "legacy-app-import"},
		{`aim-chat/go-backend/internal/api`, "removed-api-layer-import"},
		{`aim-chat/go-backend/internal/identity`, "legacy-identity-import"},
		{`aim-chat/go-backend/internal/domains/messaging/usecase`, "domain-subpackage-import"},
		{`aim-chat/go-backend/internal/domains/messaging`, ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.importPath, func(t *testing.T) {
			got := daemonServiceBoundaryReason(tt.importPath)
			if got != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func collectDaemonServiceImportViolations(t *testing.T) []boundaryViolation {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve current test file path")
	}
	dir := filepath.Dir(currentFile)
	pattern := filepath.Join(dir, "*.go")
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob files: %v", err)
	}

	fset := token.NewFileSet()
	var violations []boundaryViolation
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse file %s: %v", file, err)
		}
		for _, imp := range parsed.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			reason := daemonServiceBoundaryReason(importPath)
			if reason == "" {
				continue
			}
			pos := fset.Position(imp.Path.Pos())
			violations = append(violations, boundaryViolation{
				file:       filepath.Base(file),
				line:       pos.Line,
				importPath: importPath,
				reason:     reason,
			})
		}
	}
	return violations
}

func daemonServiceBoundaryReason(importPath string) string {
	if hasImportPrefix(importPath, "aim-chat/go-backend/internal/app") {
		return "legacy-app-import"
	}
	if hasImportPrefix(importPath, "aim-chat/go-backend/internal/api") {
		return "removed-api-layer-import"
	}
	if hasImportPrefix(importPath, "aim-chat/go-backend/internal/application") {
		return "legacy-application-import"
	}
	if hasImportPrefix(importPath, "aim-chat/go-backend/internal/infrastructure") {
		return "legacy-infrastructure-import"
	}
	if hasImportPrefix(importPath, "aim-chat/go-backend/internal/identity") {
		return "legacy-identity-import"
	}
	if hasImportPrefix(importPath, "aim-chat/go-backend/internal/domains") {
		parts := strings.Split(importPath, "/")
		if len(parts) > 5 {
			return "domain-subpackage-import"
		}
		return ""
	}
	if hasImportPrefix(importPath, "aim-chat/go-backend/internal/domain") {
		return "legacy-domain-import"
	}
	return ""
}

func hasImportPrefix(importPath, pkg string) bool {
	if importPath == pkg {
		return true
	}
	return strings.HasPrefix(importPath, pkg+"/")
}
