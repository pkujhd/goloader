package jit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"
)

type Module struct {
	Path       string       // module path
	Query      string       // version query corresponding to this version
	Version    string       // module version
	Versions   []string     // available module versions
	Replace    *Module      // replaced by this module
	Time       *time.Time   // time version was created
	Update     *Module      // available update (with -u)
	Main       bool         // is this the main module?
	Indirect   bool         // module is only indirectly needed by main module
	Dir        string       // directory holding local copy of files, if any
	GoMod      string       // path to go.mod file describing module, if any
	GoVersion  string       // go version used in module
	Retracted  []string     // retraction information, if any (with -retracted or -u)
	Deprecated string       // deprecation message, if any (with -u)
	Error      *ModuleError // error loading module
	Origin     interface{}  // provenance of module
	Reuse      bool         // reuse of old module info is safe
}

type ModuleError struct {
	Err string // the error itself
}

type Package struct {
	Dir           string   // directory containing package sources
	ImportPath    string   // import path of package in dir
	ImportComment string   // path in import comment on package statement
	Name          string   // package name
	Doc           string   // package documentation string
	Target        string   // install path
	Shlib         string   // the shared library that contains this package (only set when -linkshared)
	Goroot        bool     // is this package in the Go root?
	Standard      bool     // is this package part of the standard Go library?
	Stale         bool     // would 'go install' do anything for this package?
	StaleReason   string   // explanation for Stale==true
	Root          string   // Go root or Go path dir containing this package
	ConflictDir   string   // this directory shadows Dir in $GOPATH
	BinaryOnly    bool     // binary-only package (no longer supported)
	ForTest       string   // package is only for use in named test
	Export        string   // file containing export data (when using -export)
	BuildID       string   // build ID of the compiled package (when using -export)
	Module        *Module  // info about package's containing module, if any (can be nil)
	Match         []string // command-line patterns matching this package
	DepOnly       bool     // package is only a dependency, not explicitly listed

	// Source files
	GoFiles           []string // .go source files (excluding CgoFiles, TestGoFiles, XTestGoFiles)
	CgoFiles          []string // .go source files that import "C"
	CompiledGoFiles   []string // .go files presented to compiler (when using -compiled)
	IgnoredGoFiles    []string // .go source files ignored due to build constraints
	IgnoredOtherFiles []string // non-.go source files ignored due to build constraints
	CFiles            []string // .c source files
	CXXFiles          []string // .cc, .cxx and .cpp source files
	MFiles            []string // .m source files
	HFiles            []string // .h, .hh, .hpp and .hxx source files
	FFiles            []string // .f, .F, .for and .f90 Fortran source files
	SFiles            []string // .s source files
	SwigFiles         []string // .swig files
	SwigCXXFiles      []string // .swigcxx files
	SysoFiles         []string // .syso object files to add to archive
	TestGoFiles       []string // _test.go files in package
	XTestGoFiles      []string // _test.go files outside package

	// Embedded files
	EmbedPatterns      []string // //go:embed patterns
	EmbedFiles         []string // files matched by EmbedPatterns
	TestEmbedPatterns  []string // //go:embed patterns in TestGoFiles
	TestEmbedFiles     []string // files matched by TestEmbedPatterns
	XTestEmbedPatterns []string // //go:embed patterns in XTestGoFiles
	XTestEmbedFiles    []string // files matched by XTestEmbedPatterns

	// Cgo directives
	CgoCFLAGS    []string // cgo: flags for C compiler
	CgoCPPFLAGS  []string // cgo: flags for C preprocessor
	CgoCXXFLAGS  []string // cgo: flags for C++ compiler
	CgoFFLAGS    []string // cgo: flags for Fortran compiler
	CgoLDFLAGS   []string // cgo: flags for linker
	CgoPkgConfig []string // cgo: pkg-config names

	// Dependency information
	Imports      []string          // import paths used by this package
	ImportMap    map[string]string // map from source import to ImportPath (identity entries omitted)
	Deps         []string          // all (recursively) imported dependencies
	TestImports  []string          // imports from TestGoFiles
	XTestImports []string          // imports from XTestGoFiles

	// Error information
	Incomplete bool            // this package or a dependency has an error
	Error      *PackageError   // error loading package
	DepsErrors []*PackageError // errors loading dependencies
}

type PackageError struct {
	ImportStack []string // shortest path from package named on command line to this one
	Pos         string   // position of error (if present, file:line:col)
	Err         string   // the error itself
}

func GoModDownload(goCmd, workDir string, verbose bool, args ...string) error {
	dlCmd := exec.Command(goCmd, append([]string{"mod", "download"}, args...)...)
	if verbose {
		dlCmd.Stdout = os.Stdout
		dlCmd.Stderr = os.Stderr
	}
	dlCmd.Dir = workDir
	err := dlCmd.Run()
	if err != nil {
		return fmt.Errorf("failed to go mod download %s", args)
	}

	tidyCmd := exec.Command(goCmd, "mod", "tidy")
	if verbose {
		tidyCmd.Stdout = os.Stdout
		tidyCmd.Stderr = os.Stderr
	}
	err = tidyCmd.Run()
	if err != nil {
		return fmt.Errorf("failed to go mod tidy: %s", err)
	}
	return nil
}

func GoGet(goCmd, packagePath, workDir string, verbose bool) error {
	goGetCmd := exec.Command(goCmd, "get", packagePath)
	goGetCmd.Dir = workDir
	if verbose {
		goGetCmd.Stderr = os.Stderr
		goGetCmd.Stdout = os.Stdout
	}
	output, err := goGetCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to go get %s: %s", packagePath, output)
	}
	return nil
}

var stdLibsByGoCmd sync.Map

func GoListStd(goCmd string) map[string]struct{} {
	cacheLookup, ok := stdLibsByGoCmd.Load(goCmd)
	if ok {
		return cacheLookup.(map[string]struct{})
	}
	stdLibPkgs := map[string]struct{}{}
	cmd := exec.Command(goCmd, "list", "std")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("goloader/jit failed to list std packages: %s\n", err)
		return nil
	}
	for _, pkgName := range bytes.Split(output, []byte("\n")) {
		stdLibPkgs[string(pkgName)] = struct{}{}
	}
	stdLibsByGoCmd.Store(goCmd, stdLibPkgs)
	return stdLibPkgs
}

func GoList(goCmd, absPath, workDir string, verbose bool) (*Package, error) {
	golistCmd := exec.Command(goCmd, "list", "-json", absPath)
	golistCmd.Dir = workDir
	stdoutReader, stdoutWriter := io.Pipe()
	stdErrBuf := &bytes.Buffer{}
	if verbose {
		golistCmd.Stdout = stdoutWriter
		golistCmd.Stderr = io.MultiWriter(os.Stderr, stdErrBuf)
	} else {
		golistCmd.Stdout = stdoutWriter
		golistCmd.Stderr = stdErrBuf
	}
	listDec := json.NewDecoder(stdoutReader)
	err := golistCmd.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start 'go list -json %s': %w\nstderr:\n%s", absPath, err, stdErrBuf.String())
	}
	pkg := Package{}
	err = listDec.Decode(&pkg)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response of 'go list -json %s': %w\nstderr:\n%s", absPath, err, stdErrBuf.String())
	}
	err = golistCmd.Wait()
	if err != nil {
		return nil, fmt.Errorf("failed to wait for 'go list -json %s': %w\nstderr:\n%s", absPath, err, stdErrBuf.String())
	}

	if len(pkg.GoFiles)+len(pkg.CgoFiles) == 0 {
		return nil, fmt.Errorf("no Go files found in directory %s", absPath)
	}
	return &pkg, nil
}
