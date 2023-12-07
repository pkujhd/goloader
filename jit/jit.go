package jit

import (
	"bytes"
	"cmd/objfile/objabi"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"github.com/eh-steve/goloader"
	"github.com/eh-steve/goloader/libc"
	"github.com/eh-steve/goloader/obj"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	_ "unsafe"
)

// A shared map used across all importers of this JIT package within a binary to store all the packages and symbols included in the main binary
var globalMutex = sync.Mutex{}
var globalPkgSet = make(map[string]struct{})
var globalSymPtr = make(map[string]uintptr)

func init() {
	err := RegisterSymbols()
	if err != nil {
		log.Printf("jit package failed to register symbols of current binary: %s\n", err)
	}
}

func GlobalSymPtr() map[string]uintptr {
	clone := make(map[string]uintptr)
	globalMutex.Lock()
	defer globalMutex.Unlock()
	for k, v := range globalSymPtr {
		clone[k] = v
	}
	return clone
}

func GlobalPkgSet() map[string]struct{} {
	clone := make(map[string]struct{})
	globalMutex.Lock()
	defer globalMutex.Unlock()
	for k, v := range globalPkgSet {
		clone[k] = v
	}
	return clone
}

func RegisterSymbols() error {
	globalMutex.Lock()
	defer globalMutex.Unlock()

	// Only register the main binary's symbols once, even if we have multiple callers
	if len(globalSymPtr) == 0 {
		err := goloader.RegSymbol(globalSymPtr, globalPkgSet)
		return err
	}
	return nil
}

func RegisterTypes(types ...interface{}) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	goloader.RegTypes(globalSymPtr, types...)
}

func RegisterCGoSymbol(symNameC string, symNameGo string) bool {
	if runtime.GOOS == "darwin" {
		// For some reason, error doesn't follow the usual pattern of just stripping "libc_"
		// See runtime/sys_darwin.go
		if symNameC == "error" {
			symNameC = "__error"
		}
	}
	addr, err := libc.LookupDynamicSymbol(symNameC)
	if err != nil {
		log.Println(err)
		return false
	} else {
		globalMutex.Lock()
		globalSymPtr[symNameGo] = addr
		globalMutex.Unlock()
		return true
	}
}

type BuildConfig struct {
	GoBinary                         string // Path to go binary, defaults to "go"
	KeepTempFiles                    bool
	ExtraBuildFlags                  []string
	BuildEnv                         []string
	TmpDir                           string
	DebugLog                         bool
	SymbolNameOrder                  []string // Control the layout of symbols in the linker's linear memory - useful for reproducing bugs
	RandomSymbolNameOrder            bool     // Randomise the order of linker symbols (may identify linker bugs)
	RelocationDebugWriter            io.Writer
	DumpTextBeforeAfterRelocation    bool
	SkipTypeDeduplicationForPackages []string
	UnsafeBlindlyUseFirstmoduleTypes bool
	Dynlink                          bool
}

func mergeBuildFlags(extraBuildFlags []string, dynlink bool) []string {
	// This -exporttypes flag requires the Go toolchain to have been patched via PatchGC()
	var gcFlags = []string{"-exporttypes"}
	if dynlink {
		// Also add -dynlink to force R_PCREL relocs to use R_GOTPCREL to allow offsets larger than 32-bits for inter-package relocs
		gcFlags = append(gcFlags, "-dynlink")
	}
	var buildFlags []string
	for _, bf := range extraBuildFlags {
		// Merge together user supplied -gcflags into a single flag
		if strings.HasPrefix(strings.TrimLeft(bf, " "), "-gcflags") {
			flagSet := flag.NewFlagSet("", flag.ContinueOnError)
			f := flagSet.String("gcflags", "", "")
			err := flagSet.Parse([]string{bf})
			if err != nil {
				panic(err)
			}
			gcFlags = append(gcFlags, *f)
		} else {
			buildFlags = append(buildFlags, bf)
		}
	}

	buildFlags = append(buildFlags, fmt.Sprintf(`-gcflags=%s`, strings.Join(gcFlags, " ")))
	return buildFlags
}

func execBuild(config BuildConfig, workDir, outputFilePath string, targets []string) error {
	var args = []string{"build"}
	args = append(args, mergeBuildFlags(config.ExtraBuildFlags, config.Dynlink)...)

	args = append(args, "-o", outputFilePath)
	args = append(args, targets...)
	if config.GoBinary == "" {
		config.GoBinary = "go"
	}
	cmd := exec.Command(config.GoBinary, args...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), config.BuildEnv...)

	bufStdout := &bytes.Buffer{}
	bufStdErr := &bytes.Buffer{}

	if config.DebugLog {
		cmd.Stdout = io.MultiWriter(os.Stdout, bufStdout)
		cmd.Stderr = io.MultiWriter(os.Stderr, bufStdErr)
	} else {
		cmd.Stdout = bufStdout
		cmd.Stderr = bufStdErr
	}

	err := cmd.Run()
	if err != nil {
		var stdoutStr string
		if bufStdout.Len() > 0 {
			stdoutStr = fmt.Sprintf("stdout:\n%s", bufStdout.String())
		}
		return fmt.Errorf("could not build with cmd:\n'%s': %w. %s\nstderr:\n%s", strings.Join(cmd.Args, " "), err, stdoutStr, bufStdErr.String())
	}
	return nil
}

func resolveDependencies(config BuildConfig, workDir, buildDir string, outputFilePath, packageName string, pkg *Package, linkerOpts []goloader.LinkerOptFunc, stdLibPkgs map[string]struct{}) (*goloader.Linker, error) {
	// Now check whether all imported packages are available in the main binary, otherwise we need to build and load them too
	linker, err := goloader.ReadObjs([]string{outputFilePath}, []string{packageName}, globalSymPtr, linkerOpts...)

	if err != nil {
		return nil, fmt.Errorf("could not read symbols from object file '%s': %w", outputFilePath, err)
	}

	globalMutex.Lock()
	externalSymbols := linker.UnresolvedExternalSymbols(globalSymPtr, config.SkipTypeDeduplicationForPackages, stdLibPkgs, config.UnsafeBlindlyUseFirstmoduleTypes)
	externalSymbolsWithoutSkip := linker.UnresolvedExternalSymbols(globalSymPtr, nil, stdLibPkgs, config.UnsafeBlindlyUseFirstmoduleTypes)
	externalPackages := linker.UnresolvedPackageReferences(pkg.Deps)
	globalMutex.Unlock()

	addCGoSymbols(externalSymbols)

	var depImportPaths, depBinaries = []string{packageName}, []string{outputFilePath}
	// Prevent infinite recursion
	seen := map[string]struct{}{}

	sortedDeps := make([]string, len(pkg.Deps)+len(externalPackages))
	copy(sortedDeps, pkg.Deps)
	copy(sortedDeps[len(pkg.Deps):], externalPackages)
	// Sort deps by length descending so that symbol check is most specific first
	sort.Slice(sortedDeps, func(i, j int) bool {
		return len(sortedDeps[i]) > len(sortedDeps[j])
	})

	if len(externalSymbols) > 0 {
		if config.DebugLog {
			log.Printf("%d unresolved external symbols missing from main binary, will attempt to build dependencies\n", len(externalSymbolsWithoutSkip))
		}
		errDeps := buildAndLoadDeps(config, workDir, buildDir, sortedDeps, externalSymbols, externalSymbolsWithoutSkip, seen, &depImportPaths, &depBinaries, 0, linkerOpts, stdLibPkgs)
		if errDeps != nil {
			return nil, errDeps
		}

		depsLinker, err := goloader.ReadObjs(depBinaries, depImportPaths, globalSymPtr, linkerOpts...)
		if err != nil {
			return nil, fmt.Errorf("could not read symbols from dependency object files '%s': %w", depImportPaths, err)
		}

		requiredBy := depsLinker.UnresolvedExternalSymbolUsers(globalSymPtr)
		if len(requiredBy) > 0 {
			unresolvedList := make([]string, 0, len(requiredBy))
			for symName, requiredByList := range requiredBy {
				unresolvedList = append(unresolvedList, fmt.Sprintf("%s     required by: \n    %s\n", symName, strings.Join(requiredByList, "\n    ")))
			}
			sort.Strings(unresolvedList)
			return nil, fmt.Errorf("still have %d unresolved external symbols despite building and linking dependencies...: \n%s", len(requiredBy), strings.Join(unresolvedList, "\n"))
		}
		linker.UnloadStrings()
		linker = depsLinker
	}
	return linker, nil
}

var escapes = make(map[string]string)

func init() {
	// According to cmd/internal/objabi.PathToPrefix(), char codes less than ' ', or equal to the last '.' or '%' or '"', or greater than 0x7F will be escaped. Make a list of these
	for i := uint8(0); i < 255; i++ {
		if i <= ' ' || i == '.' || i == '%' || i == '"' || i >= 0x7F {
			escapes["%"+hex.EncodeToString([]byte{i})] = string(rune(i))
		}
	}
}

func unescapeSymName(name string) string {
	for escaped, nonEscaped := range escapes {
		name = strings.Replace(name, escaped, nonEscaped, -1)
	}
	return name
}

func getMissingDeps(sortedDeps []string, unresolvedSymbols, unresolvedSymbolsWithoutSkip map[string]*obj.Sym, seen map[string]struct{}, debug bool) map[string]struct{} {
	var missingDeps = map[string]struct{}{}
	unresolvedSymbolNames := make([]string, 0, len(unresolvedSymbols))
	for symName := range unresolvedSymbols {
		unresolvedSymbolNames = append(unresolvedSymbolNames, symName)
	}
	sort.Strings(unresolvedSymbolNames)
	for _, symNameEscaped := range unresolvedSymbolNames {
		for _, dep := range sortedDeps {
			// Unescape dots in the symName path since the compiler would have escaped them in cmd/internal/objabi.PathToPrefix()
			symName := unescapeSymName(symNameEscaped)
			if unresolvedSymbols[symNameEscaped].Pkg == objabi.PathToPrefix(dep) {
				if _, haveSeen := seen[dep]; !haveSeen {
					if _, ok := globalPkgSet[dep]; ok && debug {
						if _, ok := unresolvedSymbolsWithoutSkip[symNameEscaped]; !ok {
							log.Printf("main binary contains package '%s', but symbol deduplication was skipped so forcing rebuild %s\n", dep, symName)
						} else {
							log.Printf("main binary contains partial package '%s', but not symbol %s\n", dep, symName)
						}
					}
					missingDeps[dep] = struct{}{}
				}
			}
		}
	}
	return missingDeps
}

func addCGoSymbols(externalUnresolvedSymbols map[string]*obj.Sym) {
	if runtime.GOOS == "darwin" {
		for k := range externalUnresolvedSymbols {
			if strings.IndexByte(k, '.') == -1 && !strings.HasPrefix(k, goloader.TypePrefix) {
				if strings.HasPrefix(k, "libc_") {
					// For dynlib symbols in $GOROOT/src/syscall/syscall_darwin.go
					RegisterCGoSymbol(strings.TrimPrefix(k, "libc_"), k)
				} else if strings.HasPrefix(k, "libresolv_") {
					// For dynlib symbols in $GOROOT/src/internal/syscall/net_darwin.go etc.
					RegisterCGoSymbol(strings.TrimPrefix(k, "libresolv_"), k)
				} else if strings.HasPrefix(k, "x509_") {
					// For dynlib symbols in $GOROOT/src/crypto/x509/internal/macos/corefoundation.go
					RegisterCGoSymbol(strings.TrimPrefix(k, "x509_"), k)
				} else {
					if k[0] == '_' {
						RegisterCGoSymbol(k[1:], k)
					} else {
						RegisterCGoSymbol(k, k)
					}
				}
			}
			// TODO - if more symbols use the //go:cgo_import_dynamic linker pragma, then they would also need to be registered here
		}
	} else {
		for k := range externalUnresolvedSymbols {
			// CGo symbols don't have a package name
			if strings.IndexByte(k, '.') == -1 && !strings.HasPrefix(k, goloader.TypePrefix) {
				RegisterCGoSymbol(k, k)
			}
		}
	}
}

func buildAndLoadDeps(config BuildConfig,
	workDir, buildDir string,
	sortedDeps []string,
	unresolvedSymbols, unresolvedSymbolsWithoutSkip map[string]*obj.Sym,
	seen map[string]struct{},
	builtPackageImportPaths, buildPackageFilePaths *[]string,
	depth int,
	linkerOpts []goloader.LinkerOptFunc,
	stdLibPkgs map[string]struct{}) error {
	const maxRecursionDepth = 150
	if depth > maxRecursionDepth {
		return fmt.Errorf("failed to buildAndLoadDeps: recursion depth %d exceeded maximum of %d", depth, maxRecursionDepth)
	}
	missingDeps := getMissingDeps(sortedDeps, unresolvedSymbols, unresolvedSymbolsWithoutSkip, seen, config.DebugLog)

	if len(missingDeps) == 0 {
		return nil
	}
	wg := sync.WaitGroup{}
	var errs []error
	var errsMutex sync.Mutex
	wg.Add(len(missingDeps))

	missingDepsSorted := make([]string, 0, len(missingDeps))
	for k := range missingDeps {
		missingDepsSorted = append(missingDepsSorted, k)
	}
	sort.Strings(missingDepsSorted)

	concurrencyLimit := make(chan struct{}, runtime.GOMAXPROCS(0))
	for _, missingDep := range missingDepsSorted {
		if _, ok := seen[missingDep]; ok {
			continue
		}
		h := sha256.New()
		h.Write([]byte(missingDep))

		filename := filepath.Join(buildDir, hex.EncodeToString(h.Sum(nil))+"___pkg___.a")

		concurrencyLimit <- struct{}{}
		go func(filename, missingDep string) {
			if config.DebugLog {
				log.Printf("Building dependency '%s' (%s)\n", missingDep, filename)
			}
			if config.GoBinary == "" {
				config.GoBinary = "go"
			}

			args := []string{"build"}
			args = append(args, mergeBuildFlags(config.ExtraBuildFlags, config.Dynlink)...)
			args = append(args, "-o", filename, missingDep)
			command := exec.Command(config.GoBinary, args...)
			if config.DebugLog {
				command.Stderr = os.Stderr
				command.Stderr = os.Stdout
			}
			command.Dir = workDir
			bufStdout := &bytes.Buffer{}
			bufStdErr := &bytes.Buffer{}
			if config.DebugLog {
				command.Stdout = io.MultiWriter(os.Stdout, bufStdout)
				command.Stderr = io.MultiWriter(os.Stderr, bufStdErr)
			} else {
				command.Stdout = bufStdout
				command.Stderr = bufStdErr
			}

			err := command.Run()
			if err != nil {
				errsMutex.Lock()
				errs = append(errs, fmt.Errorf("failed to build dependency '%s': %w\nstdout:\n %s\nstderr:\n%s", missingDep, err, bufStdout.String(), bufStdErr.String()))
				errsMutex.Unlock()
			}
			wg.Done()
			<-concurrencyLimit
		}(filename, missingDep)
		existingImport := false
		for _, existing := range *builtPackageImportPaths {
			if missingDep == existing {
				existingImport = true
			}
		}
		if !existingImport {
			// Prepend, so that deps get loaded first
			*builtPackageImportPaths = append([]string{missingDep}, *builtPackageImportPaths...)
			*buildPackageFilePaths = append([]string{filename}, *buildPackageFilePaths...)
		}
	}
	wg.Wait()
	if len(errs) > 0 {
		var extra string
		if len(errs) > 1 {
			extra = fmt.Sprintf(". (extra errors: %s)", errs)
		}
		return fmt.Errorf("got %d during build of dependencies: %w%s", len(errs), errs[0], extra)
	}

	linker, err := goloader.ReadObjs(*buildPackageFilePaths, *builtPackageImportPaths, globalSymPtr, linkerOpts...)
	if err != nil {
		return fmt.Errorf("linker failed to read symbols from dependency object files (%s): %w", *builtPackageImportPaths, err)
	}

	globalMutex.Lock()
	nextUnresolvedSymbols := linker.UnresolvedExternalSymbols(globalSymPtr, nil, stdLibPkgs, config.UnsafeBlindlyUseFirstmoduleTypes)
	nextUnresolvedPackages := linker.UnresolvedPackageReferences(sortedDeps)
	globalMutex.Unlock()

	sortedDeps = append(sortedDeps, nextUnresolvedPackages...)
	addCGoSymbols(nextUnresolvedSymbols)
	linker.UnloadStrings()

	if len(nextUnresolvedSymbols) > 0 {
		var newSortedDeps []string
	outer:
		for _, dep := range sortedDeps {
			for _, existing := range *builtPackageImportPaths {
				if dep == existing {
					continue outer
				}
			}
			newSortedDeps = append(newSortedDeps, dep)
		}
		if config.DebugLog {
			var missingList []string
			for k := range getMissingDeps(newSortedDeps, nextUnresolvedSymbols, nextUnresolvedSymbols, seen, config.DebugLog) {
				missingList = append(missingList, k)
			}
			sort.Strings(missingList)
			missingSyms := make([]string, 0, len(nextUnresolvedSymbols))
			for symName, objSym := range nextUnresolvedSymbols {
				missingSyms = append(missingSyms, symName+" (package: '"+objSym.Pkg+"')")
			}
			log.Printf("Still have %d unresolved symbols \n[\n  %s\n]\n after building dependencies. Recursing further to build: \n[\n  %s\n]\n", len(nextUnresolvedSymbols), strings.Join(missingSyms, ",\n  "), strings.Join(missingList, ",\n  "))
		}
		return buildAndLoadDeps(config, workDir, buildDir, newSortedDeps, nextUnresolvedSymbols, nextUnresolvedSymbols, seen, builtPackageImportPaths, buildPackageFilePaths, depth+1, linkerOpts, stdLibPkgs)
	}
	return nil
}

func (config *BuildConfig) linkerOpts() []goloader.LinkerOptFunc {
	var linkerOpts []goloader.LinkerOptFunc
	if len(config.SymbolNameOrder) > 0 {
		linkerOpts = append(linkerOpts, goloader.WithSymbolNameOrder(config.SymbolNameOrder))
	}
	if config.RandomSymbolNameOrder {
		linkerOpts = append(linkerOpts, goloader.WithRandomSymbolNameOrder())
	}
	if config.RelocationDebugWriter != nil {
		linkerOpts = append(linkerOpts, goloader.WithRelocationDebugWriter(config.RelocationDebugWriter))
	}
	if config.DumpTextBeforeAfterRelocation {
		linkerOpts = append(linkerOpts, goloader.WithDumpTextBeforeAndAfterRelocs())
	}
	if len(config.SkipTypeDeduplicationForPackages) > 0 {
		linkerOpts = append(linkerOpts, goloader.WithSkipTypeDeduplicationForPackages(config.SkipTypeDeduplicationForPackages))
	}
	return linkerOpts
}

func BuildGoFiles(config BuildConfig, pathToGoFile string, extraFiles ...string) (*LoadableUnit, error) {
	absPath, err := filepath.Abs(pathToGoFile)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path at %s: %w", pathToGoFile, err)
	}
	for i := range extraFiles {
		newPath, err := filepath.Abs(extraFiles[i])
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path at %s: %w", extraFiles[i], err)
		}
		extraFiles[i] = newPath
	}
	workDir := filepath.Dir(absPath)

	if config.GoBinary == "" {
		config.GoBinary = "go"
	}
	err = PatchGC(config.GoBinary, config.DebugLog)
	if err != nil {
		return nil, fmt.Errorf("failed to patch gc: %w", err)
	}
	pkg, err := GoList(config.GoBinary, absPath, workDir, config.DebugLog)
	if err != nil {
		return nil, err
	}

	if len(pkg.DepsErrors) > 0 {
		err = GoModDownload(config.GoBinary, workDir, config.DebugLog)
		if err != nil {
			return nil, err
		}
		err = GoGet(config.GoBinary, workDir, workDir, config.DebugLog)
		if err != nil {
			return nil, err
		}
		pkg, err = GoList(config.GoBinary, absPath, "", config.DebugLog)
		if err != nil {
			return nil, err
		}
		if len(pkg.DepsErrors) > 0 {
			return nil, fmt.Errorf("could not resolve dependency errors after go mod download + go get: %s", pkg.DepsErrors[0].Err)
		}
	}

	if config.TmpDir != "" {
		absPathBuildDir, err := filepath.Abs(config.TmpDir)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path of tmp dir at %s: %w", config.TmpDir, err)
		}
		config.TmpDir = absPathBuildDir
		_, err = os.Stat(config.TmpDir)
		if errors.Is(err, os.ErrNotExist) {
			err = os.MkdirAll(config.TmpDir, os.ModePerm)
			if err != nil {
				return nil, fmt.Errorf("could not create new temp dir at %s: %w", config.TmpDir, err)
			}
			if !config.KeepTempFiles {
				defer os.RemoveAll(config.TmpDir)
			}
		}
	}

	buildDir, err := os.MkdirTemp(config.TmpDir, strings.TrimSuffix(path.Base(pathToGoFile), ".go")+"_*")
	if err != nil {
		return nil, fmt.Errorf("could not create new tmp dir: %w", err)
	}
	if !config.KeepTempFiles {
		defer os.RemoveAll(buildDir)
	}

	files := append([]string{absPath}, extraFiles...)
	h := sha256.New()
	h.Write([]byte(strings.Join(files, "|")))
	outputFilePath := filepath.Join(buildDir, hex.EncodeToString(h.Sum(nil))+".a")

	err = execBuild(config, workDir, outputFilePath, files)
	if err != nil {
		return nil, err
	}

	linkerOpts := config.linkerOpts()
	stdLibPkgs := GoListStd(config.GoBinary)

	linker, err := resolveDependencies(config, workDir, buildDir, outputFilePath, pkg.ImportPath, pkg, linkerOpts, stdLibPkgs)
	if err != nil {
		return nil, err
	}

	return &LoadableUnit{
		Linker:     linker,
		ImportPath: pkg.ImportPath,
		Package:    pkg,
	}, nil
}

func BuildGoText(config BuildConfig, goText string) (*LoadableUnit, error) {
	h := sha256.New()
	h.Write([]byte(goText))
	hexHash := hex.EncodeToString(h.Sum(nil))
	if config.TmpDir == "" {
		absPathBuildDir, err := filepath.Abs("./")
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path of tmp dir at %s: %w", config.TmpDir, err)
		}
		config.TmpDir = absPathBuildDir
	}
	buildDir, err := os.MkdirTemp(config.TmpDir, "jit_*")
	if err != nil {
		return nil, fmt.Errorf("could not create new tmp directory: %w", err)
	}

	if !config.KeepTempFiles {
		defer os.RemoveAll(buildDir)
	}

	tmpFilePath := filepath.Join(buildDir, hexHash+".go")

	err = os.WriteFile(tmpFilePath, []byte(goText), 0655)
	if err != nil {
		return nil, fmt.Errorf("could not write tmpFile '%s': %w", tmpFilePath, err)
	}

	if config.GoBinary == "" {
		config.GoBinary = "go"
	}
	err = PatchGC(config.GoBinary, config.DebugLog)
	if err != nil {
		return nil, fmt.Errorf("failed to patch gc: %w", err)
	}
	pkg, err := GoList(config.GoBinary, tmpFilePath, "", config.DebugLog)
	if err != nil {
		return nil, err
	}

	if len(pkg.DepsErrors) > 0 {
		err = GoModDownload(config.GoBinary, buildDir, config.DebugLog)
		if err != nil {
			return nil, err
		}
		absPackagePath, err := filepath.Abs(path.Dir(tmpFilePath))
		if err != nil {
			return nil, fmt.Errorf("could not get absolute path of directory containing file %s: %w", tmpFilePath, err)
		}

		err = GoGet(config.GoBinary, absPackagePath, "", config.DebugLog)
		if err != nil {
			return nil, err
		}
		pkg, err = GoList(config.GoBinary, tmpFilePath, "", config.DebugLog)
		if err != nil {
			return nil, err
		}
		if len(pkg.DepsErrors) > 0 {
			return nil, fmt.Errorf("could not resolve dependency errors after go mod download + go get: %s", pkg.DepsErrors[0].Err)
		}
	}

	outputFilePath := filepath.Join(buildDir, hexHash+".a")

	err = execBuild(config, "", outputFilePath, []string{tmpFilePath})
	if err != nil {
		return nil, err
	}

	linkerOpts := config.linkerOpts()
	stdLibPkgs := GoListStd(config.GoBinary)
	linker, err := resolveDependencies(config, "", buildDir, outputFilePath, pkg.ImportPath, pkg, linkerOpts, stdLibPkgs)
	if err != nil {
		return nil, err
	}

	return &LoadableUnit{
		Linker:     linker,
		ImportPath: pkg.ImportPath,
		Package:    pkg,
	}, nil
}

func BuildGoPackage(config BuildConfig, pathToGoPackage string) (*LoadableUnit, error) {
	absPath, err := filepath.Abs(pathToGoPackage)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path at %s: %w", pathToGoPackage, err)
	}
	fileInfo, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("could not stat path at %s: %w", absPath, err)
	}
	if !fileInfo.IsDir() {
		return nil, fmt.Errorf("path at %s is not a directory", absPath)
	}

	if config.GoBinary == "" {
		config.GoBinary = "go"
	}
	err = PatchGC(config.GoBinary, config.DebugLog)
	if err != nil {
		return nil, fmt.Errorf("failed to patch gc: %w", err)
	}
	// Execute list from within the package folder so that go list resolves the module correctly from that path
	pkg, err := GoList(config.GoBinary, absPath, absPath, config.DebugLog)
	if err != nil {
		return nil, err
	}

	if pkg.Module == nil || pkg.Module.GoMod == "" {
		return nil, fmt.Errorf("could not find module/go.mod file for path %s", absPath)
	}

	if len(pkg.DepsErrors) > 0 {
		err = GoModDownload(config.GoBinary, absPath, config.DebugLog)
		if err != nil {
			return nil, err
		}
		err = GoGet(config.GoBinary, absPath, absPath, config.DebugLog)
		if err != nil {
			return nil, err
		}
		pkg, err = GoList(config.GoBinary, absPath, "", config.DebugLog)
		if err != nil {
			return nil, err
		}
		if len(pkg.DepsErrors) > 0 {
			return nil, fmt.Errorf("could not resolve dependency errors after go mod download + go get: %s", pkg.DepsErrors[0].Err)
		}
	}
	h := sha256.New()
	h.Write([]byte(absPath))

	hexHash := hex.EncodeToString(h.Sum(nil))

	if config.TmpDir != "" {
		absPathBuildDir, err := filepath.Abs(config.TmpDir)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path of tmp dir at %s: %w", config.TmpDir, err)
		}
		config.TmpDir = absPathBuildDir
		_, err = os.Stat(config.TmpDir)
		if errors.Is(err, os.ErrNotExist) {
			err = os.MkdirAll(config.TmpDir, os.ModePerm)
			if err != nil {
				return nil, fmt.Errorf("could not create new temp dir at %s: %w", config.TmpDir, err)
			}
			if !config.KeepTempFiles {
				defer os.RemoveAll(config.TmpDir)
			}
		}
	}

	rootBuildDir1, err := os.MkdirTemp(config.TmpDir, hexHash+"_*")
	if err != nil {
		return nil, fmt.Errorf("could not create new tmp directory: %w", err)
	}
	rootBuildDir, err := filepath.Abs(rootBuildDir1)
	if err != nil {
		return nil, fmt.Errorf("could not get absolute path of root build %s: %w", rootBuildDir, err)
	}
	if !config.KeepTempFiles {
		defer os.RemoveAll(rootBuildDir)
	}

	outputFilePath := filepath.Join(rootBuildDir, hexHash+".a")

	importPath := pkg.ImportPath
	err = execBuild(config, absPath, outputFilePath, []string{absPath})
	if err != nil {
		return nil, err
	}

	linkerOpts := config.linkerOpts()
	stdLibPkgs := GoListStd(config.GoBinary)
	linker, err := resolveDependencies(config, absPath, rootBuildDir, outputFilePath, importPath, pkg, linkerOpts, stdLibPkgs)
	if err != nil {
		return nil, err
	}

	return &LoadableUnit{
		Linker:     linker,
		ImportPath: importPath,
		Package:    pkg,
	}, nil
}

func BuildGoPackageRemote(config BuildConfig, goPackage string, version string) (*LoadableUnit, error) {
	if config.GoBinary == "" {
		config.GoBinary = "go"
	}
	err := PatchGC(config.GoBinary, config.DebugLog)
	if err != nil {
		return nil, fmt.Errorf("failed to patch gc: %w", err)
	}
	// Execute list from within the package folder so that go list resolves the module correctly from that path
	workDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current working director: %w", err)
	}

	stdLibPkgs := GoListStd(config.GoBinary)
	_, isStdLibPkg := stdLibPkgs[goPackage]

	if version == "" {
		version = "latest"
	}
	var versionSuffix string
	if !isStdLibPkg {
		versionSuffix = "@" + version
	}

	err = GoGet(config.GoBinary, goPackage+versionSuffix, workDir, config.DebugLog)
	if err != nil {
		return nil, err
	}

	pkg, err := GoList(config.GoBinary, goPackage, workDir, config.DebugLog)
	if err != nil {
		return nil, err
	}

	if (pkg.Module == nil || pkg.Module.GoMod == "") && !isStdLibPkg {
		return nil, fmt.Errorf("could not find module/go.mod file for package %s", goPackage)
	}

	if len(pkg.DepsErrors) > 0 {
		err = GoModDownload(config.GoBinary, workDir, config.DebugLog, pkg.Module.Path)
		if err != nil {
			return nil, err
		}
		err = GoGet(config.GoBinary, goPackage, workDir, config.DebugLog)
		if err != nil {
			return nil, err
		}
		pkg, err = GoList(config.GoBinary, goPackage, "", config.DebugLog)
		if err != nil {
			return nil, err
		}
		if len(pkg.DepsErrors) > 0 {
			return nil, fmt.Errorf("could not resolve dependency errors after go mod download + go get: %s", pkg.DepsErrors[0].Err)
		}
	}
	h := sha256.New()
	h.Write([]byte(goPackage))

	hexHash := hex.EncodeToString(h.Sum(nil))

	if config.TmpDir != "" {
		absPathBuildDir, err := filepath.Abs(config.TmpDir)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path of tmp dir at %s: %w", config.TmpDir, err)
		}
		config.TmpDir = absPathBuildDir
		_, err = os.Stat(config.TmpDir)
		if errors.Is(err, os.ErrNotExist) {
			err = os.MkdirAll(config.TmpDir, os.ModePerm)
			if err != nil {
				return nil, fmt.Errorf("could not create new temp dir at %s: %w", config.TmpDir, err)
			}
			if !config.KeepTempFiles {
				defer os.RemoveAll(config.TmpDir)
			}
		}
	}

	rootBuildDir1, err := os.MkdirTemp(config.TmpDir, hexHash+"_*")
	if err != nil {
		return nil, fmt.Errorf("could not create new tmp directory: %w", err)
	}
	rootBuildDir, err := filepath.Abs(rootBuildDir1)
	if err != nil {
		return nil, fmt.Errorf("could not get absolute path of root build %s: %w", rootBuildDir, err)
	}
	if !config.KeepTempFiles {
		defer os.RemoveAll(rootBuildDir)
	}

	outputFilePath := filepath.Join(rootBuildDir, hexHash+".a")

	importPath := pkg.ImportPath
	err = execBuild(config, workDir, outputFilePath, []string{goPackage})
	if err != nil {
		return nil, err
	}
	linkerOpts := config.linkerOpts()
	linker, err := resolveDependencies(config, workDir, rootBuildDir, outputFilePath, importPath, pkg, linkerOpts, stdLibPkgs)
	if err != nil {
		return nil, err
	}

	return &LoadableUnit{
		Linker:     linker,
		ImportPath: importPath,
		Package:    pkg,
	}, nil
}
