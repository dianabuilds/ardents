package usecase

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestArchitecture_InboundServiceNoTransportImports(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve current test file path")
	}
	file := filepath.Join(filepath.Dir(currentFile), "inbound_service.go")
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse inbound_service.go: %v", err)
	}
	for _, imp := range parsed.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if path == "aim-chat/go-backend/internal/waku" {
			t.Fatalf("inbound_service.go must not import transport package %q", path)
		}
	}
}
