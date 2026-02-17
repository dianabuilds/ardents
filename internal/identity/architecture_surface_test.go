package identity

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestArchitecture_NoLegacyHelperExports(t *testing.T) {
	forbidden := map[string]struct{}{
		"BuildIdentityID":   {},
		"VerifyIdentityID":  {},
		"SignContactCard":   {},
		"VerifyContactCard": {},
	}

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
		node, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse file %s: %v", file, err)
		}
		for _, decl := range node.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name == nil || fn.Recv != nil {
				continue
			}
			if _, isForbidden := forbidden[fn.Name.Name]; !isForbidden {
				continue
			}
			pos := fset.Position(fn.Name.Pos())
			violations = append(violations, fmt.Sprintf("%s:%d has forbidden export %s", base, pos.Line, fn.Name.Name))
		}
	}

	if len(violations) == 0 {
		return
	}
	t.Fatalf("legacy helper exports are forbidden in internal/identity:\n- %s", strings.Join(violations, "\n- "))
}

func TestArchitecture_IdentityImportsDomainPolicyOnly(t *testing.T) {
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
			if !strings.HasPrefix(importPath, "aim-chat/go-backend/internal/domains/identity") {
				continue
			}
			if importPath == "aim-chat/go-backend/internal/domains/identity/policy" {
				continue
			}
			pos := fset.Position(imp.Path.Pos())
			violations = append(violations, fmt.Sprintf("%s:%d imports forbidden domain package %q", base, pos.Line, importPath))
		}
	}
	if len(violations) == 0 {
		return
	}
	t.Fatalf("internal/identity may import only domains/identity/policy:\n- %s", strings.Join(violations, "\n- "))
}
