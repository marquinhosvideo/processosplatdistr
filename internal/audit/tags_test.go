package audit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestEverySnapshotFieldIsTagged é a trava da §3.10: percorre o código Go do
// repositório e falha se algum campo exportado de snapshot não tiver
// audit:"plain" ou audit:"redacted". Snapshot é o tipo com nome terminado em
// Snapshot, o struct com alguma tag audit ou o tipo usado como Before/After
// de um audit.Entry literal.
func TestEverySnapshotFieldIsTagged(t *testing.T) {
	root := filepath.Join("..", "..")
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("raiz do módulo não encontrada: %v", err)
	}
	problems, scanned, err := scanSnapshots(root)
	if err != nil {
		t.Fatal(err)
	}
	if scanned == 0 {
		t.Fatal("nenhum pacote Go varrido")
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// O verificador acusa cada caso do fixture e aceita o snapshot correto.
func TestScanSnapshotsFindsUntaggedFields(t *testing.T) {
	problems, _, err := scanSnapshots(filepath.Join("testdata", "untagged"))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(problems, "\n")
	for _, want := range []string{"candidateSnapshot.FullName", "quarterState.Notes", "trainingRow.Cost"} {
		if !strings.Contains(joined, want) {
			t.Errorf("verificador não apontou %s:\n%s", want, joined)
		}
	}
	for _, unwanted := range []string{"internal", "okSnapshot", "candidateSnapshot.ID"} {
		if strings.Contains(joined, unwanted) {
			t.Errorf("falso positivo %s:\n%s", unwanted, joined)
		}
	}
	if len(problems) != 3 {
		t.Errorf("problemas = %d:\n%s", len(problems), joined)
	}
}

func scanSnapshots(root string) (problems []string, packages int, err error) {
	skip := []string{"node_modules", ".git", ".tmp", "web", "dist"}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		// testdata só entra quando é a própria raiz (o fixture).
		if path != root && (d.Name() == "testdata" || slices.Contains(skip, d.Name())) {
			return filepath.SkipDir
		}
		found, ok, err := scanDir(path)
		if ok {
			packages++
		}
		problems = append(problems, found...)
		return err
	})
	return problems, packages, err
}

func scanDir(dir string) ([]string, bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false, err
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			return nil, false, err
		}
		files = append(files, f)
	}
	if len(files) == 0 {
		return nil, false, nil
	}
	structs := map[string]*ast.TypeSpec{}
	used := map[string]bool{}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.TypeSpec:
				if _, ok := n.Type.(*ast.StructType); ok {
					structs[n.Name.Name] = n
				}
			case *ast.CompositeLit:
				if isAuditEntry(n.Type) {
					for _, el := range n.Elts {
						kv, ok := el.(*ast.KeyValueExpr)
						if !ok {
							continue
						}
						if key, ok := kv.Key.(*ast.Ident); ok && (key.Name == "Before" || key.Name == "After") {
							if name := literalType(kv.Value); name != "" {
								used[name] = true
							}
						}
					}
				}
			}
			return true
		})
	}

	var problems []string
	names := make([]string, 0, len(structs))
	for name := range structs {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		spec := structs[name]
		st := spec.Type.(*ast.StructType)
		if !strings.HasSuffix(name, "Snapshot") && !used[name] && !hasAuditTag(st) {
			continue
		}
		for _, field := range st.Fields.List {
			tag := ""
			if field.Tag != nil {
				raw, _ := strconv.Unquote(field.Tag.Value)
				tag = reflect.StructTag(raw).Get("audit")
			}
			if tag == TagPlain || tag == TagRedacted {
				continue
			}
			for _, fname := range fieldNames(field) {
				if !ast.IsExported(fname) {
					continue
				}
				pos := fset.Position(field.Pos())
				problems = append(problems, pos.String()+": "+name+"."+fname+": sem tag audit:\"plain\" ou audit:\"redacted\"")
			}
		}
	}
	return problems, true, nil
}

func isAuditEntry(e ast.Expr) bool {
	switch t := e.(type) {
	case *ast.SelectorExpr:
		x, ok := t.X.(*ast.Ident)
		return ok && x.Name == "audit" && t.Sel.Name == "Entry"
	case *ast.Ident:
		return t.Name == "Entry"
	}
	return false
}

// literalType devolve o nome do tipo de T{...} ou &T{...}.
func literalType(e ast.Expr) string {
	if u, ok := e.(*ast.UnaryExpr); ok && u.Op == token.AND {
		e = u.X
	}
	if lit, ok := e.(*ast.CompositeLit); ok {
		if id, ok := lit.Type.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

func hasAuditTag(st *ast.StructType) bool {
	for _, field := range st.Fields.List {
		if field.Tag == nil {
			continue
		}
		raw, _ := strconv.Unquote(field.Tag.Value)
		if _, ok := reflect.StructTag(raw).Lookup("audit"); ok {
			return true
		}
	}
	return false
}

// fieldNames devolve os nomes declarados ou, num campo embutido, o do tipo.
func fieldNames(f *ast.Field) []string {
	if len(f.Names) > 0 {
		out := make([]string, len(f.Names))
		for i, n := range f.Names {
			out[i] = n.Name
		}
		return out
	}
	t := f.Type
	if s, ok := t.(*ast.StarExpr); ok {
		t = s.X
	}
	switch t := t.(type) {
	case *ast.Ident:
		return []string{t.Name}
	case *ast.SelectorExpr:
		return []string{t.Sel.Name}
	}
	return nil
}
