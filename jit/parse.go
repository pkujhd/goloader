package jit

import (
	"bytes"
	"fmt"
	"github.com/pkujhd/goloader/jit/syntax"
	"text/template"
	"unicode"
)

func getTypeFuncName(symbol string) string {
	return "JITGet" + symbol + "Type"
}

func generateReflectCode(files []*ParsedFile) ([]byte, map[string]string, error) {
	if len(files) == 0 {
		return nil, nil, fmt.Errorf("expected at least 1 file")
	}
	pkgName := files[0].PackageName
	var exports []string
	for _, file := range files {
		if file.PackageName != pkgName {
			return nil, nil, fmt.Errorf("expected all package names to be the same, got: '%s' and '%s'", file.PackageName, pkgName)
		}
		for _, exportedFunc := range file.ExportedFunctions {
			exports = append(exports, exportedFunc.Name.Value)
		}
	}

	t := template.New("reflect")
	t.Funcs(template.FuncMap{
		"getTypeFuncName": getTypeFuncName,
	})

	t, err := t.Parse(`package {{ .package }}
import (
	"reflect"
)

{{ range $i, $name := .exports }}
func {{ $name | getTypeFuncName }}() reflect.Type { return reflect.TypeOf({{ $name }}) }
{{ end }}
`,
	)
	if err != nil {
		return nil, nil, err
	}
	buf := bytes.NewBuffer(nil)
	err = t.Execute(buf, map[string]interface{}{
		"exports": exports,
		"package": pkgName,
	})
	symbolToTypeFunc := make(map[string]string)
	for _, export := range exports {
		symbolToTypeFunc[export] = getTypeFuncName(export)
	}
	return buf.Bytes(), symbolToTypeFunc, err
}

type ParsedFile struct {
	PackageName       string
	ExportedFunctions []*syntax.FuncDecl
	Imports           []*syntax.ImportDecl
}

func ParseFile(filePath string) (*ParsedFile, error) {
	parserErrFunc := func(err error) {}

	parserPragmaFunc := func(pos syntax.Pos, blank bool, text string, current syntax.Pragma) syntax.Pragma {
		return current
	}

	file, err := syntax.ParseFile(filePath, parserErrFunc, parserPragmaFunc, syntax.CheckBranches)

	if err != nil {
		return nil, fmt.Errorf("failed to parse Go file at '%s: %w", filePath, err)
	}

	parsed := &ParsedFile{
		PackageName:       file.PkgName.Value,
		ExportedFunctions: nil,
		Imports:           nil,
	}

	// Collect all global and exported symbol names.
	// Since the compiler only outputs a ptab for main packages, we need to collect all exported symbol names
	// to build our own ptab equivalent by asking the compiler for the types of global exports by programmatically
	// generating reflection functions for each symbol name
	for _, decl := range file.DeclList {
		switch d := decl.(type) {
		case *syntax.ImportDecl:
			parsed.Imports = append(parsed.Imports, d)
		case *syntax.VarDecl:
			for _, name := range d.NameList {
				if unicode.IsUpper(rune(name.Value[0])) && unicode.IsLetter(rune(name.Value[0])) {
					// TODO - maybe support accessing exported global vars?
				}
			}
		case *syntax.FuncDecl:
			if unicode.IsUpper(rune(d.Name.Value[0])) && unicode.IsLetter(rune(d.Name.Value[0])) {
				// Don't add receiver methods
				if d.Recv == nil {
					parsed.ExportedFunctions = append(parsed.ExportedFunctions, d)
				}
			}
		}
	}

	return parsed, nil
}
