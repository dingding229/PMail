package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCatalogsMatchAndContainSourceKeys(t *testing.T) {
	for key, text := range cn {
		if text == "" || en[key] == "" {
			t.Errorf("missing or empty translation: %s", key)
		}
	}
	for key := range en {
		if cn[key] == "" {
			t.Errorf("missing zh-CN translation: %s", key)
		}
	}
	fset := token.NewFileSet()
	if err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		check := func(expr ast.Expr) {
			if value, ok := expr.(*ast.BasicLit); ok && value.Kind == token.STRING {
				key, _ := strconv.Unquote(value.Value)
				if en[key] == "" || cn[key] == "" {
					t.Errorf("%s: missing key %q", fset.Position(value.Pos()), key)
				}
			}
		}
		checkCall := func(c *ast.CallExpr, index int) {
			if len(c.Args) > index {
				check(c.Args[index])
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				switch f := n.Fun.(type) {
				case *ast.SelectorExpr:
					if pkg, ok := f.X.(*ast.Ident); ok {
						if pkg.Name == "i18n" && (f.Sel.Name == "T" || f.Sel.Name == "Text" || f.Sel.Name == "Delivery" || f.Sel.Name == "Diagnostic") {
							checkCall(n, 1)
						} else if pkg.Name == "response" && f.Sel.Name == "Localized" {
							checkCall(n, 2)
						}
					}
				case *ast.Ident:
					switch f.Name {
					case "apiError":
						checkCall(n, 4)
					case "externalFailure":
						checkCall(n, 3)
					case "fail":
						if filepath.Base(path) == "setup.go" {
							checkCall(n, 0)
						}
					case "T", "Text", "Delivery", "Diagnostic":
						if file.Name.Name == "i18n" {
							checkCall(n, 1)
						}
					}
				}
			case *ast.ValueSpec:
				if len(n.Names) == 1 && n.Names[0].Name == "serviceErrors" {
					for _, v := range n.Values {
						if table, ok := v.(*ast.CompositeLit); ok {
							for _, entry := range table.Elts {
								if row, ok := entry.(*ast.CompositeLit); ok && len(row.Elts) == 4 {
									check(row.Elts[3])
								}
							}
						}
					}
				}
			case *ast.AssignStmt:
				// A few handlers select a key by error type before translating it.
				base := filepath.Base(path)
				if (file.Name.Name == "controllers" && (base == "register.go" || base == "external.go")) || file.Name.Name == "res_init" {
					if len(n.Lhs) == 1 && len(n.Rhs) == 1 {
						if id, ok := n.Lhs[0].(*ast.Ident); ok && id.Name == "key" {
							check(n.Rhs[0])
						}
					}
				}
			case *ast.FuncDecl:
				if n.Name.Name == "forwardErrorKey" {
					ast.Inspect(n.Body, func(node ast.Node) bool {
						if ret, ok := node.(*ast.ReturnStmt); ok && len(ret.Results) == 1 {
							check(ret.Results[0])
						}
						return true
					})
				}
			}
			return true
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestResolveRequestLanguage(t *testing.T) {
	t.Setenv("PMAIL_DEFAULT_LANG", "zh-CN")
	for _, tc := range []struct{ explicit, accept, want string }{
		{"", "", Chinese},
		{"zh-TW", "en", Chinese},
		{"en-US", "zh-CN", English},
		{"", "en;q=0.4, zh-Hant;q=0.8", Chinese},
		{"", "ja-JP, zh;q=0.1", English},
		{"", "zh;q=0, en", English},
	} {
		r := httptest.NewRequest("GET", "/api/bootstrap", nil)
		r.Header.Set("X-PMail-Lang", tc.explicit)
		r.Header.Set("Accept-Language", tc.accept)
		if got := Resolve(r); got != tc.want {
			t.Errorf("explicit=%q accept=%q: language=%q, want %q", tc.explicit, tc.accept, got, tc.want)
		}
	}
}
