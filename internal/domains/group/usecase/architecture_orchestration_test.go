package usecase

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

func TestArchitecture_UsecaseDoesNotDefineInboundWireDecoders(t *testing.T) {
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
		parsed, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse file %s: %v", file, err)
		}
		for _, decl := range parsed.Decls {
			switch node := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range node.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if ts.Name.Name == "InboundGroupEventWire" {
						pos := fset.Position(ts.Pos())
						violations = append(violations, fmt.Sprintf("%s:%d type %s", base, pos.Line, ts.Name.Name))
					}
				}
			case *ast.FuncDecl:
				if node.Name.Name == "DecodeInboundGroupEvent" {
					pos := fset.Position(node.Pos())
					violations = append(violations, fmt.Sprintf("%s:%d func %s", base, pos.Line, node.Name.Name))
				}
			}
		}
	}

	if len(violations) > 0 {
		t.Fatalf("group/usecase must not define transport wire decoders:\n- %s", strings.Join(violations, "\n- "))
	}
}
