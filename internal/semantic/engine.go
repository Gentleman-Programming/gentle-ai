package semantic

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Engine implementa el motor de análisis semántico estático mediante AST nativo en Go.
type Engine struct{}

// NewEngine inicializa el motor semántico AST.
func NewEngine() *Engine {
	return &Engine{}
}

// ExtractionResult agrupa los símbolos y dependencias extraídos de un directorio.
type ExtractionResult struct {
	Symbols      []SymbolItem
	Dependencies []DependencyRelation
	Packages     []string
}

// AnalyzeDirectory recorre recursivamente un directorio e indexa sus símbolos y dependencias.
func (e *Engine) AnalyzeDirectory(rootDir string, role string) (*ExtractionResult, error) {
	result := &ExtractionResult{
		Symbols:      make([]SymbolItem, 0),
		Dependencies: make([]DependencyRelation, 0),
		Packages:     make([]string, 0),
	}

	pkgMap := make(map[string]bool)
	fset := token.NewFileSet()

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		// Ignorar directorios ocultos, vendor, node_modules y .git
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		// Procesar únicamente archivos Go (incluyendo tests si se desea o filtrando)
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		node, err := parser.ParseFile(fset, path, src, parser.ParseComments)
		if err != nil {
			return nil
		}

		packageName := node.Name.Name
		if !pkgMap[packageName] {
			pkgMap[packageName] = true
			result.Packages = append(result.Packages, packageName)
		}

		relPath, _ := filepath.Rel(rootDir, path)
		if relPath == "" {
			relPath = path
		}
		relPath = filepath.ToSlash(relPath)

		// 1. Extraer dependencias de importación
		for _, imp := range node.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			isInternal := strings.Contains(importPath, "axiom") || strings.Contains(importPath, "gentle-ai") || strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../")
			result.Dependencies = append(result.Dependencies, DependencyRelation{
				SourcePackage: packageName,
				TargetPackage: importPath,
				IsInternal:    isInternal,
				FilePath:      relPath,
				Role:          role,
			})
		}

		// 2. Extraer declaraciones de tipos, structs e interfaces
		for _, decl := range node.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						sym := SymbolItem{
							Name:       typeSpec.Name.Name,
							Package:    packageName,
							FilePath:   relPath,
							LineNumber: fset.Position(typeSpec.Pos()).Line,
							Role:       role,
						}

						if d.Doc != nil {
							sym.DocComment = strings.TrimSpace(d.Doc.Text())
						}

						switch t := typeSpec.Type.(type) {
						case *ast.StructType:
							sym.Kind = KindStruct
							fieldCount := 0
							if t.Fields != nil {
								fieldCount = len(t.Fields.List)
							}
							sym.Signature = fmt.Sprintf("struct (%d campos)", fieldCount)
						case *ast.InterfaceType:
							sym.Kind = KindInterface
							methodCount := 0
							if t.Methods != nil {
								methodCount = len(t.Methods.List)
							}
							sym.Signature = fmt.Sprintf("interface (%d métodos)", methodCount)
						default:
							sym.Kind = KindType
							sym.Signature = fmt.Sprintf("type %s", typeSpec.Name.Name)
						}

						result.Symbols = append(result.Symbols, sym)
					}
				}

			case *ast.FuncDecl:
				sym := SymbolItem{
					Name:       d.Name.Name,
					Package:    packageName,
					FilePath:   relPath,
					LineNumber: fset.Position(d.Pos()).Line,
					Role:       role,
				}

				if d.Doc != nil {
					sym.DocComment = strings.TrimSpace(d.Doc.Text())
				}

				// Determinar si es método o función
				if d.Recv != nil && len(d.Recv.List) > 0 {
					sym.Kind = KindMethod
					recvType := formatReceiver(d.Recv.List[0].Type)
					sym.Receiver = recvType
					sym.Signature = fmt.Sprintf("func (%s) %s%s", recvType, d.Name.Name, formatFuncSignature(d.Type))
				} else {
					sym.Kind = KindFunc
					sym.Signature = fmt.Sprintf("func %s%s", d.Name.Name, formatFuncSignature(d.Type))
				}

				result.Symbols = append(result.Symbols, sym)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	sort.Strings(result.Packages)
	return result, nil
}

// formatReceiver convierte el tipo receptor del AST a texto.
func formatReceiver(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return "*" + ident.Name
		}
		return "*T"
	case *ast.Ident:
		return t.Name
	default:
		return "T"
	}
}

// formatFuncSignature formatea los parámetros y retornos de una función.
func formatFuncSignature(funcType *ast.FuncType) string {
	params := formatFieldList(funcType.Params)
	results := formatFieldList(funcType.Results)

	if results == "" {
		return fmt.Sprintf("(%s)", params)
	}
	if strings.Contains(results, ",") {
		return fmt.Sprintf("(%s) (%s)", params, results)
	}
	return fmt.Sprintf("(%s) %s", params, results)
}

func formatFieldList(fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return ""
	}
	var items []string
	for _, field := range fl.List {
		typeName := typeToString(field.Type)
		if len(field.Names) == 0 {
			items = append(items, typeName)
		} else {
			for _, name := range field.Names {
				items = append(items, fmt.Sprintf("%s %s", name.Name, typeName))
			}
		}
	}
	return strings.Join(items, ", ")
}

func typeToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeToString(t.X)
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", typeToString(t.X), t.Sel.Name)
	case *ast.ArrayType:
		return "[]" + typeToString(t.Elt)
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", typeToString(t.Key), typeToString(t.Value))
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.Ellipsis:
		return "..." + typeToString(t.Elt)
	default:
		return "any"
	}
}
