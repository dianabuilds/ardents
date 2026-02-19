package domains

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestArchitecture_DomainPackagesDisallowAdapterCompositionInfraImports(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve current test file path")
	}
	domainsDir := filepath.Dir(currentFile)
	allowedLegacyImports := map[string]struct{}{
		filepath.Join(domainsDir, "identity", "legacy_manager.go") + "|aim-chat/go-backend/internal/identity": {},
	}
	forbiddenPrefixes := []string{
		"aim-chat/go-backend/internal/adapters",
		"aim-chat/go-backend/internal/composition",
		"aim-chat/go-backend/internal/infrastructure",
		"aim-chat/go-backend/internal/app",
		"aim-chat/go-backend/internal/api",
		"aim-chat/go-backend/internal/application",
		"aim-chat/go-backend/internal/identity",
	}

	fset := token.NewFileSet()
	var violations []string
	walkErr := filepath.WalkDir(domainsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		parsed, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse file %s: %w", path, err)
		}
		for _, imp := range parsed.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			for _, prefix := range forbiddenPrefixes {
				if !hasPrefixImport(importPath, prefix) {
					continue
				}
				allowKey := filepath.Clean(path) + "|" + prefix
				if _, allowed := allowedLegacyImports[allowKey]; allowed {
					break
				}
				pos := fset.Position(imp.Path.Pos())
				relPath, relErr := filepath.Rel(domainsDir, path)
				if relErr != nil {
					relPath = path
				}
				violations = append(violations, fmt.Sprintf("%s:%d imports %q", relPath, pos.Line, importPath))
				break
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk domains tree: %v", walkErr)
	}
	if len(violations) > 0 {
		t.Fatalf("domain boundary violations detected:\n- %s", strings.Join(violations, "\n- "))
	}
}

func hasPrefixImport(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}
