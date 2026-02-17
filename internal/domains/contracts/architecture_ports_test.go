package contracts

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestArchitecture_ContractsPortsNoInfraImports(t *testing.T) {
	file := loadContractsPortsFile(t)
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if strings.HasPrefix(path, "aim-chat/go-backend/internal/crypto") ||
			strings.HasPrefix(path, "aim-chat/go-backend/internal/storage") ||
			strings.HasPrefix(path, "aim-chat/go-backend/internal/waku") {
			t.Fatalf("contracts/ports must not import infra packages, found %q", path)
		}
	}
}

func TestArchitecture_ContractsPortsForbidMovedTypeNames(t *testing.T) {
	file := loadContractsPortsFile(t)
	forbidden := map[string]struct{}{
		"SessionDomain":     {},
		"MessageRepository": {},
		"TransportNode":     {},
		"ServiceOptions":    {},
		"WirePayload":       {},
	}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			if _, exists := forbidden[ts.Name.Name]; exists {
				t.Fatalf("type %q must not be defined in contracts/ports; move it to contracts root", ts.Name.Name)
			}
		}
	}
}

func TestArchitecture_ContractsPortsAggregateInterfacesShape(t *testing.T) {
	file := loadContractsPortsFile(t)
	core := mustFindInterface(t, file, "CoreAPI")
	daemon := mustFindInterface(t, file, "DaemonService")

	expectedEmbeds := []string{
		"GroupAPI",
		"IdentityAPI",
		"InboxAPI",
		"MessagingAPI",
		"NetworkAPI",
		"PrivacyAPI",
	}

	assertInterfaceEmbedsOnly(t, core, expectedEmbeds)
	assertInterfaceEmbedsAndMethods(
		t,
		daemon,
		expectedEmbeds,
		[]string{"ListenAddresses", "StartNetworking", "StopNetworking", "SubscribeNotifications"},
	)
}

func loadContractsPortsFile(t *testing.T) *ast.File {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve current file path")
	}
	portsFile := filepath.Join(filepath.Dir(currentFile), "ports", "contracts.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, portsFile, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse ports/contracts.go: %v", err)
	}
	return file
}

func mustFindInterface(t *testing.T, file *ast.File, name string) *ast.InterfaceType {
	t.Helper()
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != name {
				continue
			}
			iface, ok := ts.Type.(*ast.InterfaceType)
			if !ok {
				t.Fatalf("type %q exists but is not an interface", name)
			}
			return iface
		}
	}
	t.Fatalf("interface %q not found", name)
	return nil
}

func assertInterfaceEmbedsOnly(t *testing.T, iface *ast.InterfaceType, expectedEmbeds []string) {
	t.Helper()
	embeds := make([]string, 0)
	for _, field := range iface.Methods.List {
		if len(field.Names) > 0 {
			t.Fatalf("aggregate interface must not declare methods directly, found %q", field.Names[0].Name)
		}
		name, ok := embeddedName(field.Type)
		if !ok {
			t.Fatalf("unsupported embedded type expression %T", field.Type)
		}
		embeds = append(embeds, name)
	}
	slices.Sort(embeds)
	exp := append([]string(nil), expectedEmbeds...)
	slices.Sort(exp)
	if !slices.Equal(embeds, exp) {
		t.Fatalf("unexpected embedded interfaces: got=%v want=%v", embeds, exp)
	}
}

func assertInterfaceEmbedsAndMethods(t *testing.T, iface *ast.InterfaceType, expectedEmbeds, expectedMethods []string) {
	t.Helper()
	embeds := make([]string, 0)
	methods := make([]string, 0)
	for _, field := range iface.Methods.List {
		if len(field.Names) == 0 {
			name, ok := embeddedName(field.Type)
			if !ok {
				t.Fatalf("unsupported embedded type expression %T", field.Type)
			}
			embeds = append(embeds, name)
			continue
		}
		for _, name := range field.Names {
			methods = append(methods, name.Name)
		}
	}
	slices.Sort(embeds)
	slices.Sort(methods)
	expEmbeds := append([]string(nil), expectedEmbeds...)
	expMethods := append([]string(nil), expectedMethods...)
	slices.Sort(expEmbeds)
	slices.Sort(expMethods)
	if !slices.Equal(embeds, expEmbeds) {
		t.Fatalf("unexpected embedded interfaces: got=%v want=%v", embeds, expEmbeds)
	}
	if !slices.Equal(methods, expMethods) {
		t.Fatalf("unexpected declared methods: got=%v want=%v", methods, expMethods)
	}
}

func embeddedName(expr ast.Expr) (string, bool) {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name, true
	case *ast.SelectorExpr:
		if pkg, ok := v.X.(*ast.Ident); ok {
			return fmt.Sprintf("%s.%s", pkg.Name, v.Sel.Name), true
		}
		return "", false
	default:
		return "", false
	}
}
