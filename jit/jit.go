package jit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/eh-steve/goloader"
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
	check()
}

// Forbidden packages should never be rebuilt as dependencies, a lot of the runtime
// assembly code expects there to be only 1 instance of certain runtime symbols
var forbiddenSystemPkgs = map[string]struct{}{
	"runtime":                 {}, // Not a good idea
	"runtime/internal/atomic": {}, // Not a good idea
	"runtime/internal":        {}, // Not a good idea
	"runtime/cpu":             {}, // Not a good idea
	"internal/cpu":            {}, // Not a good idea
	"reflect":                 {}, // Not a good idea
	"unsafe":                  {}, // Not a real package
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
	addr, err := LookupDynamicSymbol(symNameC)
	if err != nil {
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
	SkipCopyPatterns                 []string // Paths to exclude from module copy
	HeapStrings                      bool     // Whether to put strings on the heap and allow GC to manage their lifecycle
	StringContainerSize              int      // Whether to separately mmap a container for strings, to allow unmapping independently of unloading code modules
	SymbolNameOrder                  []string // Control the layout of symbols in the linker's linear memory - useful for reproducing bugs
	RandomSymbolNameOrder            bool     // Randomise the order of linker symbols (may identify linker bugs)
	RelocationDebugWriter            io.Writer
	SkipTypeDeduplicationForPackages []string
}

func execBuild(config BuildConfig, workDir, outputFilePath string, targets []string) error {
	var args = []string{"build"}
	args = append(args, config.ExtraBuildFlags...)

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

func resolveDependencies(config BuildConfig, workDir, buildDir string, outputFilePath, packageName string, pkg *Package, linkerOpts []goloader.LinkerOptFunc) (*goloader.Linker, error) {
	// Now check whether all imported packages are available in the main binary, otherwise we need to build and load them too
	linker, err := goloader.ReadObjs([]string{outputFilePath}, []string{packageName}, linkerOpts...)

	if err != nil {
		return nil, fmt.Errorf("could not read symbols from object file '%s': %w", outputFilePath, err)
	}

	globalMutex.Lock()
	externalSymbols := linker.UnresolvedExternalSymbols(globalSymPtr, config.SkipTypeDeduplicationForPackages)
	externalSymbolsWithoutSkip := linker.UnresolvedExternalSymbols(globalSymPtr, nil)
	externalPackages := linker.UnresolvedPackageReferences(pkg.Deps)
	globalMutex.Unlock()

	addCGoSymbols(externalSymbols)

	var depImportPaths, depBinaries []string
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
		errDeps := buildAndLoadDeps(config, workDir, buildDir, sortedDeps, externalSymbols, externalSymbolsWithoutSkip, seen, &depImportPaths, &depBinaries, 0, linkerOpts)
		if errDeps != nil {
			return nil, errDeps
		}

		depsLinker, err := goloader.ReadObjs(append(depBinaries, outputFilePath), append(depImportPaths, packageName), linkerOpts...)
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
		_ = linker.UnloadStrings()
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
	for _, symName := range unresolvedSymbolNames {
		for _, dep := range sortedDeps {
			// Unescape dots in the symName path since the compiler would have escaped them in cmd/internal/objabi.PathToPrefix()
			symName = unescapeSymName(symName)
			// TODO - see if there's a way to reliably infer the package path of a symbol during loading phase
			if strings.Contains(symName, goloader.ObjSymbolSeparator+dep+".") || strings.Contains(symName, "/"+dep+".") || strings.HasPrefix(symName, dep+".") {
				if _, forbidden := forbiddenSystemPkgs[dep]; !forbidden {
					if _, haveSeen := seen[dep]; !haveSeen {
						if _, ok := globalPkgSet[dep]; ok && debug {
							if _, ok := unresolvedSymbolsWithoutSkip[symName]; !ok {
								log.Printf("main binary contains package '%s', but symbol deduplication was skipped so forcing rebuild\n", dep)
							} else {
								log.Printf("main binary contains partial package '%s', but not symbol %s\n", dep, symName)
							}
						}
						missingDeps[dep] = struct{}{}
					}
				}
			}
		}
	}
	return missingDeps
}

func addCGoSymbols(externalUnresolvedSymbols map[string]*obj.Sym) {
	if runtime.GOOS == "darwin" {
		for k := range externalUnresolvedSymbols {
			if strings.IndexByte(k, '.') == -1 {
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
					RegisterCGoSymbol(k, k)
					if k[0] == '_' {
						RegisterCGoSymbol(k[1:], k)
					}
				}
			}
			// TODO - if more symbols use the //go:cgo_import_dynamic linker pragma, then they would also need to be registered here
		}
	} else {
		for k := range externalUnresolvedSymbols {
			// CGo symbols don't have a package name
			if strings.IndexByte(k, '.') == -1 {
				RegisterCGoSymbol(k, k)
			}
		}
	}
}

func buildAndLoadDeps(config BuildConfig, workDir, buildDir string, sortedDeps []string, unresolvedSymbols, unresolvedSymbolsWithoutSkip map[string]*obj.Sym, seen map[string]struct{}, builtPackageImportPaths, buildPackageFilePaths *[]string, depth int, linkerOpts []goloader.LinkerOptFunc) error {
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

	for _, missingDep := range missingDepsSorted {
		if _, ok := seen[missingDep]; ok {
			continue
		}
		h := sha256.New()
		h.Write([]byte(missingDep))

		filename := filepath.Join(buildDir, hex.EncodeToString(h.Sum(nil))+"___pkg___.a")

		go func(filename, missingDep string) {
			if config.DebugLog {
				log.Printf("Building dependency '%s' (%s)\n", missingDep, filename)
			}
			if config.GoBinary == "" {
				config.GoBinary = "go"
			}
			command := exec.Command(config.GoBinary, "build", "-o", filename, missingDep)
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
		}(filename, missingDep)
		existingImport := false
		for _, existing := range *builtPackageImportPaths {
			if missingDep == existing {
				existingImport = true
			}
		}
		if !existingImport {
			*builtPackageImportPaths = append(*builtPackageImportPaths, missingDep)
			*buildPackageFilePaths = append(*buildPackageFilePaths, filename)
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

	linker, err := goloader.ReadObjs(*buildPackageFilePaths, *builtPackageImportPaths, linkerOpts...)
	if err != nil {
		return fmt.Errorf("linker failed to read symbols from dependency object files (%s): %w", *builtPackageImportPaths, err)
	}

	globalMutex.Lock()
	nextUnresolvedSymbols := linker.UnresolvedExternalSymbols(globalSymPtr, nil)
	globalMutex.Unlock()

	addCGoSymbols(nextUnresolvedSymbols)
	_ = linker.UnloadStrings()

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
			log.Printf("Still have %d unresolved symbols after building dependencies. Recursing further to build: [\n  %s\n]\n", len(nextUnresolvedSymbols), strings.Join(missingList, ",\n  "))
		}
		return buildAndLoadDeps(config, workDir, buildDir, newSortedDeps, nextUnresolvedSymbols, nextUnresolvedSymbols, seen, builtPackageImportPaths, buildPackageFilePaths, depth+1, linkerOpts)
	}
	return nil
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
	pkg, err := GoList(config.GoBinary, absPath, workDir)
	if err != nil {
		return nil, err
	}

	if len(pkg.DepsErrors) > 0 {
		err = GoModDownload(config.GoBinary, workDir)
		if err != nil {
			return nil, err
		}
		err = GoGet(config.GoBinary, workDir, workDir)
		if err != nil {
			return nil, err
		}
		pkg, err = GoList(config.GoBinary, absPath, "")
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
	newFiles := make([]string, 0, len(files)+1)
	h := sha256.New()
	var parsedFiles []*ParsedFile
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read file '%s': %w", file, err)
		}
		h.Write(data)
		if strings.HasSuffix(file, ".go") {
			parsed, err := ParseFile(file)
			if err != nil {
				return nil, fmt.Errorf("failed to parse Go file '%s': %w", file, err)
			}
			parsedFiles = append(parsedFiles, parsed)
		}
		newPath := filepath.Join(buildDir, filepath.Base(file))
		err = os.WriteFile(newPath, data, 0655)
		if err != nil {
			return nil, fmt.Errorf("failed to write file '%s': %w", newPath, err)
		}
		newFiles = append(newFiles, newPath)
	}
	reflectCode, symbolToTypeFuncName, err := generateReflectCode(parsedFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to generated reflect code: %w", err)
	}

	outputFilePath := filepath.Join(buildDir, hex.EncodeToString(h.Sum(nil))+".a")

	tmpReflectFilePath := strings.TrimSuffix(newFiles[0], ".go") + "___reflect.go"
	err = os.WriteFile(tmpReflectFilePath, reflectCode, 0655)
	if err != nil {
		return nil, fmt.Errorf("could not create new reflection file at %s: %w", tmpReflectFilePath, err)
	}

	newFiles = append(newFiles, tmpReflectFilePath)

	err = execBuild(config, workDir, outputFilePath, newFiles)
	if err != nil {
		return nil, err
	}

	var linkerOpts []goloader.LinkerOptFunc
	if config.HeapStrings {
		linkerOpts = append(linkerOpts, goloader.WithHeapStrings())
	}
	if config.StringContainerSize > 0 {
		linkerOpts = append(linkerOpts, goloader.WithStringContainer(config.StringContainerSize))
	}
	if len(config.SymbolNameOrder) > 0 {
		linkerOpts = append(linkerOpts, goloader.WithSymbolNameOrder(config.SymbolNameOrder))
	}
	if config.RandomSymbolNameOrder {
		linkerOpts = append(linkerOpts, goloader.WithRandomSymbolNameOrder())
	}
	if config.RelocationDebugWriter != nil {
		linkerOpts = append(linkerOpts, goloader.WithRelocationDebugWriter(config.RelocationDebugWriter))
	}
	if len(config.SkipTypeDeduplicationForPackages) > 0 {
		linkerOpts = append(linkerOpts, goloader.WithSkipTypeDeduplicationForPackages(config.SkipTypeDeduplicationForPackages))
	}
	linker, err := resolveDependencies(config, workDir, buildDir, outputFilePath, pkg.ImportPath, pkg, linkerOpts)
	if err != nil {
		return nil, err
	}

	return &LoadableUnit{
		Linker:               linker,
		ImportPath:           pkg.ImportPath,
		ParsedFiles:          parsedFiles,
		SymbolTypeFuncLookup: symbolToTypeFuncName,
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

	parsed, err := ParseFile(tmpFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Go file '%s': %w", tmpFilePath, err)
	}

	reflectCode, symbolToTypeFuncName, err := generateReflectCode([]*ParsedFile{parsed})
	if err != nil {
		return nil, fmt.Errorf("failed to generated reflect code: %w", err)
	}

	tmpReflectFilePath := strings.TrimSuffix(tmpFilePath, ".go") + "___reflect.go"
	err = os.WriteFile(tmpReflectFilePath, reflectCode, 0655)

	if config.GoBinary == "" {
		config.GoBinary = "go"
	}
	pkg, err := GoList(config.GoBinary, tmpFilePath, "")
	if err != nil {
		return nil, err
	}

	if len(pkg.DepsErrors) > 0 {
		err = GoModDownload(config.GoBinary, buildDir)
		if err != nil {
			return nil, err
		}
		absPackagePath, err := filepath.Abs(path.Dir(tmpFilePath))
		if err != nil {
			return nil, fmt.Errorf("could not get absolute path of directory containing file %s: %w", tmpFilePath, err)
		}

		err = GoGet(config.GoBinary, absPackagePath, "")
		if err != nil {
			return nil, err
		}
		pkg, err = GoList(config.GoBinary, tmpFilePath, "")
		if err != nil {
			return nil, err
		}
		if len(pkg.DepsErrors) > 0 {
			return nil, fmt.Errorf("could not resolve dependency errors after go mod download + go get: %s", pkg.DepsErrors[0].Err)
		}
	}

	outputFilePath := filepath.Join(buildDir, hexHash+".a")

	err = execBuild(config, "", outputFilePath, []string{tmpFilePath, tmpReflectFilePath})
	if err != nil {
		return nil, err
	}

	var linkerOpts []goloader.LinkerOptFunc
	if config.HeapStrings {
		linkerOpts = append(linkerOpts, goloader.WithHeapStrings())
	}
	if config.StringContainerSize > 0 {
		linkerOpts = append(linkerOpts, goloader.WithStringContainer(config.StringContainerSize))
	}
	if len(config.SymbolNameOrder) > 0 {
		linkerOpts = append(linkerOpts, goloader.WithSymbolNameOrder(config.SymbolNameOrder))
	}
	if config.RandomSymbolNameOrder {
		linkerOpts = append(linkerOpts, goloader.WithRandomSymbolNameOrder())
	}
	if config.RelocationDebugWriter != nil {
		linkerOpts = append(linkerOpts, goloader.WithRelocationDebugWriter(config.RelocationDebugWriter))
	}
	if len(config.SkipTypeDeduplicationForPackages) > 0 {
		linkerOpts = append(linkerOpts, goloader.WithSkipTypeDeduplicationForPackages(config.SkipTypeDeduplicationForPackages))
	}
	linker, err := resolveDependencies(config, "", buildDir, outputFilePath, pkg.ImportPath, pkg, linkerOpts)
	if err != nil {
		return nil, err
	}

	return &LoadableUnit{
		Linker:               linker,
		ImportPath:           pkg.ImportPath,
		ParsedFiles:          []*ParsedFile{parsed},
		SymbolTypeFuncLookup: symbolToTypeFuncName,
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
	// Execute list from within the package folder so that go list resolves the module correctly from that path
	pkg, err := GoList(config.GoBinary, absPath, absPath)
	if err != nil {
		return nil, err
	}

	if pkg.Module == nil || pkg.Module.GoMod == "" {
		return nil, fmt.Errorf("could not find module/go.mod file for path %s", absPath)
	}

	if len(pkg.DepsErrors) > 0 {
		err = GoModDownload(config.GoBinary, absPath)
		if err != nil {
			return nil, err
		}
		err = GoGet(config.GoBinary, absPath, absPath)
		if err != nil {
			return nil, err
		}
		pkg, err = GoList(config.GoBinary, absPath, "")
		if err != nil {
			return nil, err
		}
		if len(pkg.DepsErrors) > 0 {
			return nil, fmt.Errorf("could not resolve dependency errors after go mod download + go get: %s", pkg.DepsErrors[0].Err)
		}
	}
	h := sha256.New()

	for _, goFile := range append(pkg.GoFiles, pkg.CgoFiles...) {
		file := filepath.Join(absPath, goFile)
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read file '%s': %w", file, err)
		}
		h.Write(data)
	}

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

	// Check if source package is writable, if so, work there, otherwise, copy the entire parent module to a tmp dir
	tmpTestFile := filepath.Join(absPath, "test")
	err = os.WriteFile(tmpTestFile, nil, os.ModePerm)
	if err != nil {
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

		// Copy the directory structure of the module the package is in
		err = Copy(pkg.Module.Dir, rootBuildDir, config.SkipCopyPatterns)
		if err != nil {
			return nil, fmt.Errorf("could not copy package module %s: %w", pkg.Module.Dir, err)
		}

		buildDir := filepath.Join(rootBuildDir, strings.TrimPrefix(pkg.Dir, pkg.Module.Dir))
		err = os.MkdirAll(buildDir, 0755)
		if err != nil {
			return nil, fmt.Errorf("could not create new tmp directory %s: %w", buildDir, err)
		}

		newFiles := make([]string, 0, len(pkg.GoFiles)+len(pkg.CgoFiles)+1)
		var parsedFiles []*ParsedFile
		for _, goFile := range append(pkg.GoFiles, pkg.CgoFiles...) {
			file := filepath.Join(absPath, goFile)
			data, err := os.ReadFile(file)
			if err != nil {
				return nil, fmt.Errorf("failed to read file '%s': %w", file, err)
			}
			if strings.HasSuffix(file, ".go") {
				parsed, err := ParseFile(file)
				if err != nil {
					return nil, fmt.Errorf("failed to parse Go file '%s': %w", file, err)
				}
				parsedFiles = append(parsedFiles, parsed)
			}
			newPath := filepath.Join(buildDir, filepath.Base(file))
			err = os.WriteFile(newPath, data, 0655)
			if err != nil {
				return nil, fmt.Errorf("failed to write file '%s': %w", newPath, err)
			}
			newFiles = append(newFiles, newPath)
		}

		outputFilePath := filepath.Join(buildDir, hexHash+".a")

		reflectCode, symbolToTypeFuncName, err := generateReflectCode(parsedFiles)
		tmpReflectFilePath := strings.TrimSuffix(newFiles[0], ".go") + "___reflect.go"
		err = os.WriteFile(tmpReflectFilePath, reflectCode, 0655)
		if err != nil {
			return nil, fmt.Errorf("could not create new reflection file at %s: %w", tmpReflectFilePath, err)
		}

		newFiles = append(newFiles, tmpReflectFilePath)

		if err != nil {
			return nil, fmt.Errorf("failed to generated reflect code: %w", err)
		}

		err = execBuild(config, buildDir, outputFilePath, []string{buildDir})
		if err != nil {
			return nil, err
		}

		var linkerOpts []goloader.LinkerOptFunc
		if config.HeapStrings {
			linkerOpts = append(linkerOpts, goloader.WithHeapStrings())
		}
		if config.StringContainerSize > 0 {
			linkerOpts = append(linkerOpts, goloader.WithStringContainer(config.StringContainerSize))
		}
		if len(config.SymbolNameOrder) > 0 {
			linkerOpts = append(linkerOpts, goloader.WithSymbolNameOrder(config.SymbolNameOrder))
		}
		if config.RandomSymbolNameOrder {
			linkerOpts = append(linkerOpts, goloader.WithRandomSymbolNameOrder())
		}
		if config.RelocationDebugWriter != nil {
			linkerOpts = append(linkerOpts, goloader.WithRelocationDebugWriter(config.RelocationDebugWriter))
		}
		if len(config.SkipTypeDeduplicationForPackages) > 0 {
			linkerOpts = append(linkerOpts, goloader.WithSkipTypeDeduplicationForPackages(config.SkipTypeDeduplicationForPackages))
		}
		linker, err := resolveDependencies(config, buildDir, buildDir, outputFilePath, pkg.ImportPath, pkg, linkerOpts)
		if err != nil {
			return nil, err
		}

		return &LoadableUnit{
			Linker:               linker,
			ImportPath:           pkg.ImportPath,
			ParsedFiles:          parsedFiles,
			SymbolTypeFuncLookup: symbolToTypeFuncName,
		}, nil
	} else {
		_ = os.Remove(tmpTestFile)

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

		var parsedFiles []*ParsedFile
		allGoFiles := append(pkg.GoFiles, pkg.CgoFiles...)
		for _, goFile := range allGoFiles {
			file := filepath.Join(absPath, goFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read file '%s': %w", file, err)
			}
			if strings.HasSuffix(file, ".go") {
				parsed, err := ParseFile(file)
				if err != nil {
					return nil, fmt.Errorf("failed to parse Go file '%s': %w", file, err)
				}
				parsedFiles = append(parsedFiles, parsed)
			}
		}

		outputFilePath := filepath.Join(rootBuildDir, hexHash+".a")

		reflectCode, symbolToTypeFuncName, err := generateReflectCode(parsedFiles)
		tmpReflectFilePath := filepath.Join(absPath, strings.TrimSuffix(allGoFiles[0], ".go")+"___reflect.go")
		err = os.WriteFile(tmpReflectFilePath, reflectCode, 0655)
		if !config.KeepTempFiles {
			defer os.Remove(tmpReflectFilePath)
		}
		if err != nil {
			return nil, fmt.Errorf("could not create new reflection file at %s: %w", tmpReflectFilePath, err)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to generated reflect code: %w", err)
		}

		importPath := pkg.ImportPath
		err = execBuild(config, absPath, outputFilePath, []string{absPath})
		if err != nil {
			return nil, err
		}

		var linkerOpts []goloader.LinkerOptFunc
		if config.HeapStrings {
			linkerOpts = append(linkerOpts, goloader.WithHeapStrings())
		}
		if config.StringContainerSize > 0 {
			linkerOpts = append(linkerOpts, goloader.WithStringContainer(config.StringContainerSize))
		}
		if len(config.SymbolNameOrder) > 0 {
			linkerOpts = append(linkerOpts, goloader.WithSymbolNameOrder(config.SymbolNameOrder))
		}
		if config.RandomSymbolNameOrder {
			linkerOpts = append(linkerOpts, goloader.WithRandomSymbolNameOrder())
		}
		if config.RelocationDebugWriter != nil {
			linkerOpts = append(linkerOpts, goloader.WithRelocationDebugWriter(config.RelocationDebugWriter))
		}
		if len(config.SkipTypeDeduplicationForPackages) > 0 {
			linkerOpts = append(linkerOpts, goloader.WithSkipTypeDeduplicationForPackages(config.SkipTypeDeduplicationForPackages))
		}
		linker, err := resolveDependencies(config, absPath, rootBuildDir, outputFilePath, importPath, pkg, linkerOpts)
		if err != nil {
			return nil, err
		}

		return &LoadableUnit{
			Linker:               linker,
			ImportPath:           importPath,
			ParsedFiles:          parsedFiles,
			SymbolTypeFuncLookup: symbolToTypeFuncName,
		}, nil
	}
}
