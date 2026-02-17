package identity

import (
	"fmt"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestArchitecture_IdentityLegacyImportLocation(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve current test file path")
	}
	dir := filepath.Dir(currentFile)
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob files: %v", err)
	}

	fset := token.NewFileSet()
	var violations []string
	for _, file := range files {
		base := filepath.Base(file)
		if strings.HasSuffix(base, "_test.go") {
			continue
		}

		parsed, err := parser.ParseFile(fset, file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse file %s: %v", file, err)
		}
		for _, imp := range parsed.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if !strings.HasPrefix(importPath, "aim-chat/go-backend/internal/identity") {
				continue
			}
			if base == "legacy_manager.go" {
				continue
			}
			pos := fset.Position(imp.Path.Pos())
			violations = append(violations, fmt.Sprintf("%s:%d imports %q", base, pos.Line, importPath))
		}
	}

	if len(violations) == 0 {
		return
	}
	t.Fatalf("internal/identity import is allowed only in legacy_manager.go:\n- %s", strings.Join(violations, "\n- "))
}
