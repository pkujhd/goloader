//go:build go1.8 && !go1.16
// +build go1.8,!go1.16

package obj

import (
	"cmd/objfile/goobj"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type readAtSeeker struct {
	io.ReadSeeker
}

func (r *readAtSeeker) BytesAt(offset, size int64) (bytes []byte, err error) {
	bytes = make([]byte, size)
	_, err = r.Seek(offset, io.SeekStart)
	if err == nil {
		_, err = r.Read(bytes)
	}
	return
}

func (pkg *Pkg) Symbols() error {
	file, err := os.Open(pkg.File)
	if err != nil {
		return err
	}
	defer file.Close()
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
		pkg.Syms[sym.Name] = symbol
	}
	for _, path := range obj.Imports {
		path = path[:len(path)-len(filepath.Ext(path))]
		pkg.ImportPkgs = append(pkg.ImportPkgs, path)
	}
	return nil
}
