//go:build go1.8 && !go1.16
// +build go1.8,!go1.16

package obj

import (
	"cmd/objfile/goobj"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkujhd/goloader/objabi/symkind"
)

type readAtSeeker struct {
	io.ReadSeeker
}

type Archive struct {
}

func (r *readAtSeeker) BytesAt(offset, size int64) (bytes []byte, err error) {
	bytes = make([]byte, size)
	_, err = r.Seek(offset, io.SeekStart)
	if err == nil {
		_, err = r.Read(bytes)
	}
	return
}

func decodeCgoImports(str string) [][]string {
	var cgo_imports [][]string
	if strings.HasPrefix(str, "cgo_") {
		lines := strings.Split(str, "\n")
		for _, line := range lines {
			cgo_imports = append(cgo_imports, strings.Split(line, " "))
		}

	} else {
		json.NewDecoder(strings.NewReader(str)).Decode(&cgo_imports)
	}
	return cgo_imports
}

func (pkg *Pkg) addCgoImports(file *os.File) {
	bytes, _ := ioutil.ReadAll(file)
	content := string(bytes)
	for {
		index := strings.Index(content, "$$  // cgo")
		if index == -1 {
			break
		}
		content = content[index+len("$$  // cgo")+1:]
		index = strings.Index(content, "$$")
		jsonStr := content[:index-len("$$")]
		cgo_imports := decodeCgoImports(jsonStr)
		for _, cgo_import := range cgo_imports {
			switch cgo_import[0] {
			case "cgo_import_dynamic":
				pkg.CgoImports[cgo_import[1]] = &CgoImport{cgo_import[1], cgo_import[2], cgo_import[3]}
			case "cgo_import_static":
			case "cgo_export_dynamic":
			case "cgo_export_static":
			case "cgo_ldflag":
				//nothing
			}
		}
		content = content[index:]
	}

}
func (pkg *Pkg) Symbols() error {
	file, err := os.Open(pkg.File)
	if err != nil {
		return err
	}
	defer file.Close()

	pkg.addCgoImports(file)
	file.Seek(0, io.SeekStart)
	obj, err := goobj.Parse(file, pkg.PkgPath)
	if err != nil {
		return fmt.Errorf("read error: %v", err)
	}
	pkg.Arch = obj.Arch
	fd := readAtSeeker{ReadSeeker: file}
	for _, sym := range obj.Syms {
		symbol := &ObjSymbol{}
		symbol.Name = sym.Name
		symbol.Kind = int(sym.Kind)
		symbol.DupOK = sym.DupOK
		symbol.Size = int64(sym.Size)
		symbol.Data, err = fd.BytesAt(sym.Data.Offset, sym.Data.Size)
		symbol.Type = sym.Type.Name
		if err != nil {
			return fmt.Errorf("read error: %v", err)
		}
		Grow(&symbol.Data, (int)(symbol.Size))
		for _, loc := range sym.Reloc {
			reloc := Reloc{
				Offset:  int(loc.Offset),
				SymName: loc.Sym.Name,
				Type:    int(loc.Type),
				Size:    int(loc.Size),
				Add:     int(loc.Add)}
			symbol.Reloc = append(symbol.Reloc, reloc)
		}
		if sym.Func != nil {
			symbol.Func = &FuncInfo{}
			symbol.Func.Args = uint32(sym.Func.Args)
			symbol.Func.File = sym.Func.File
			symbol.Func.CUOffset = 0
			pkg.CUFiles = append(pkg.CUFiles, symbol.Func.File...)
			symbol.Func.PCSP, err = fd.BytesAt(sym.Func.PCSP.Offset, sym.Func.PCSP.Size)
			if err != nil {
				return fmt.Errorf("read error: %v", err)
			}
			symbol.Func.PCFile, err = fd.BytesAt(sym.Func.PCFile.Offset, sym.Func.PCFile.Size)
			if err != nil {
				return fmt.Errorf("read error: %v", err)
			}
			symbol.Func.PCLine, err = fd.BytesAt(sym.Func.PCLine.Offset, sym.Func.PCLine.Size)
			if err != nil {
				return fmt.Errorf("read error: %v", err)
			}

			for _, data := range sym.Func.PCData {
				pcdata, err := fd.BytesAt(data.Offset, data.Size)
				if err != nil {
					return fmt.Errorf("read error: %v", err)
				}
				symbol.Func.PCData = append(symbol.Func.PCData, pcdata)
			}
			for _, data := range sym.Func.FuncData {
				symbol.Func.FuncData = append(symbol.Func.FuncData, data.Sym.Name)
			}

			if err = initInline(sym.Func, symbol.Func, pkg.PkgPath, &fd); err != nil {
				return fmt.Errorf("read error: %v", err)
			}
		}
		if symbol.Kind > symkind.Sxxx && symbol.Kind <= symkind.STLSBSS {
			pkg.Syms[symbol.Name] = symbol
		}

	}
	for _, path := range obj.Imports {
		path = path[:len(path)-len(filepath.Ext(path))]
		pkg.ImportPkgs = append(pkg.ImportPkgs, path)
	}
	return nil
}

func (pkg *Pkg) AddCgoFuncs(cgoFuncs map[string]int) {
}

func (pkg *Pkg) AddSymIndex(cgoFuncs map[string]int) {
}

func (pkg *Pkg) ResolveSymbols(packages map[string]*Pkg, ObjSymbolMap map[string]*ObjSymbol, CUOffset int32) {
	for _, sym := range pkg.Syms {
		replacePkgPath(sym, pkg.PkgPath)
		ObjSymbolMap[sym.Name] = sym
	}
}
