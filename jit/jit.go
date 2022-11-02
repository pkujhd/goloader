package jit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/pkujhd/goloader"
	"github.com/pkujhd/goloader/obj"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
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
	"crypto/subtle":           {}, // All inlined
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

type BuildConfig struct {
	KeepTempFiles   bool
	ExtraBuildFlags []string
	BuildEnv        []string
	TmpDir          string
	DebugLog        bool
}

func execBuild(config BuildConfig, outputFilePath string, targets []string) error {
	var args = []string{"build"}
	args = append(args, config.ExtraBuildFlags...)

	args = append(args, "-o", outputFilePath)
	args = append(args, targets...)
	cmd := exec.Command("go", args...)
	cmd.Env = config.BuildEnv

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

func resolveDependencies(config BuildConfig, outputFilePath, packageName string, pkg *Package) (*goloader.Linker, error) {
	// Now check whether all imported packages are available in the main binary, otherwise we need to build and load them too
	linker, err := goloader.ReadObjs([]string{outputFilePath}, []string{packageName})

	if err != nil {
		return nil, fmt.Errorf("could not read symbols from object file '%s': %w", outputFilePath, err)
	}

	globalMutex.Lock()
	externalSymbols := linker.UnresolvedExternalSymbols(globalSymPtr)
	globalMutex.Unlock()

	var depImportPaths, depBinaries []string
	// Prevent infinite recursion
	seen := map[string]struct{}{}

	sortedDeps := make([]string, len(pkg.Deps))
	copy(sortedDeps, pkg.Deps)
	// Sort deps by length descending so that symbol check is most specific first
	sort.Slice(sortedDeps, func(i, j int) bool {
		return len(sortedDeps[i]) > len(sortedDeps[j])
	})

	if len(externalSymbols) > 0 {
		if config.DebugLog {
			log.Printf("%d unresolved external symbols missing from main binary, will attempt to build dependencies\n", len(externalSymbols))
		}
		errDeps := buildAndLoadDeps(config, sortedDeps, externalSymbols, seen, &depImportPaths, &depBinaries, 0)
		if errDeps != nil {
			return nil, errDeps
		}
		depsLinker, err := goloader.ReadObjs(append(depBinaries, outputFilePath), append(depImportPaths, packageName))
		if err != nil {
			return nil, fmt.Errorf("could not read symbols from dependency object files '%s': %w", depImportPaths, err)
		}

		externalSymbols := depsLinker.UnresolvedExternalSymbols(globalSymPtr)
		if len(externalSymbols) > 0 {
			unresolvedList := make([]string, 0, len(externalSymbols))
			for symName := range externalSymbols {
				unresolvedList = append(unresolvedList, symName)
			}
			sort.Strings(unresolvedList)
			return nil, fmt.Errorf("still have %d unresolved external symbols despite building and linking dependencies...: \n%s", len(externalSymbols), strings.Join(unresolvedList, "\n"))
		}
		linker = depsLinker
	}
	return linker, nil
}

func getMissingDeps(sortedDeps []string, unresolvedSymbols map[string]*obj.Sym, seen map[string]struct{}, debug bool) map[string]struct{} {
	var missingDeps = map[string]struct{}{}
	unresolvedSymbolNames := make([]string, 0, len(unresolvedSymbols))
	for symName := range unresolvedSymbols {
		unresolvedSymbolNames = append(unresolvedSymbolNames, symName)
	}
	sort.Strings(unresolvedSymbolNames)
	for _, symName := range unresolvedSymbolNames {
		for _, dep := range sortedDeps {
			// Unescape dots in the symName path since the compiler would have escaped them
			symName = strings.Replace(symName, "%2e", ".", -1)
			if strings.Contains(symName, dep+".") {
				if _, forbidden := forbiddenSystemPkgs[dep]; !forbidden {
					if _, haveSeen := seen[dep]; !haveSeen {
						if _, ok := globalPkgSet[dep]; ok && debug {
							log.Printf("main binary contains partial package '%s', but not symbol %s\n", dep, symName)
						}
						missingDeps[dep] = struct{}{}
					}
				}
			}
		}
	}
	return missingDeps
}

func buildAndLoadDeps(config BuildConfig, sortedDeps []string, unresolvedSymbols map[string]*obj.Sym, seen map[string]struct{}, builtPackageImportPaths, buildPackageFilePaths *[]string, depth int) error {
	const maxRecursionDepth = 150
	if depth > maxRecursionDepth {
		return fmt.Errorf("failed to buildAndLoadDeps: recursion depth %d exceeded maximum of %d", depth, maxRecursionDepth)
	}
	missingDeps := getMissingDeps(sortedDeps, unresolvedSymbols, seen, config.DebugLog)

	if len(missingDeps) == 0 {
		return nil
	}
	wg := sync.WaitGroup{}
	var errs []error
	var errsMutex sync.Mutex
	wg.Add(len(missingDeps))
	for missingDep := range missingDeps {
		if _, ok := seen[missingDep]; ok {
			continue
		}
		h := sha256.New()
		h.Write([]byte(missingDep))

		filename := path.Join(os.TempDir(), hex.EncodeToString(h.Sum(nil))+"___pkg___.a")

		go func(filename, missingDep string) {
			if config.DebugLog {
				log.Printf("Building dependency '%s' (%s)\n", missingDep, filename)
			}

			command := exec.Command("go", "build", "-o", filename, missingDep)
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

	linker, err := goloader.ReadObjs(*buildPackageFilePaths, *builtPackageImportPaths)
	if err != nil {
		return fmt.Errorf("linker failed to read symbols from dependency object files (%s): %w", *builtPackageImportPaths, err)
	}

	globalMutex.Lock()
	nextUnresolvedSymbols := linker.UnresolvedExternalSymbols(globalSymPtr)
	globalMutex.Unlock()

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
			for k := range getMissingDeps(newSortedDeps, nextUnresolvedSymbols, seen, false) {
				missingList = append(missingList, k)
			}
			sort.Strings(missingList)
			log.Printf("Still have %d unresolved symbols after building dependencies. Recursing further to build: [\n  %s\n]\n", len(nextUnresolvedSymbols), strings.Join(missingList, ",\n  "))
		}
		return buildAndLoadDeps(config, newSortedDeps, nextUnresolvedSymbols, seen, builtPackageImportPaths, buildPackageFilePaths, depth+1)
	}
	return nil
}

func BuildGoFiles(config BuildConfig, pathToGoFile string, extraFiles ...string) (*LoadableUnit, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("could not get current working directory: %w", err)
	}
	defer os.Chdir(currentDir)

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
	dir := filepath.Dir(absPath)
	err = os.Chdir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to chdir into %s: %w", dir, err)
	}

	pkg, err := GoList(absPath)
	if err != nil {
		return nil, err
	}

	if len(pkg.DepsErrors) > 0 {
		err = GoGet(dir)
		if err != nil {
			return nil, err
		}
	}

	if config.TmpDir != "" {
		_, err := os.Stat(config.TmpDir)
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
		newPath := path.Join(buildDir, path.Base(file))
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

	outputFilePath := path.Join(buildDir, hex.EncodeToString(h.Sum(nil))+".a")

	tmpReflectFilePath := strings.TrimSuffix(newFiles[0], ".go") + "___reflect.go"
	err = os.WriteFile(tmpReflectFilePath, reflectCode, 0655)
	if err != nil {
		return nil, fmt.Errorf("could not create new reflection file at %s: %w", tmpReflectFilePath, err)
	}

	newFiles = append(newFiles, tmpReflectFilePath)

	importPath := "command-line-arguments"
	err = execBuild(config, outputFilePath, newFiles)
	if err != nil {
		return nil, err
	}

	linker, err := resolveDependencies(config, outputFilePath, parsedFiles[0].PackageName, pkg)
	if err != nil {
		return nil, err
	}

	return &LoadableUnit{
		Linker:               linker,
		ImportPath:           importPath,
		SymbolTypeFuncLookup: symbolToTypeFuncName,
	}, nil
}

func BuildGoText(config BuildConfig, goText string) (*LoadableUnit, error) {
	h := sha256.New()
	h.Write([]byte(goText))
	hexHash := hex.EncodeToString(h.Sum(nil))
	buildDir, err := os.MkdirTemp(config.TmpDir, hexHash+"_*")
	if err != nil {
		return nil, fmt.Errorf("could not create new tmp directory: %w", err)
	}

	if !config.KeepTempFiles {
		defer os.RemoveAll(buildDir)
	}

	tmpFilePath := path.Join(buildDir, hexHash+".go")

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

	pkg, err := GoList(tmpFilePath)
	if err != nil {
		return nil, err
	}

	if len(pkg.DepsErrors) > 0 {
		err = GoModDownload()
		if err != nil {
			return nil, err
		}
		absPackagePath, err := filepath.Abs(path.Dir(tmpFilePath))
		if err != nil {
			return nil, fmt.Errorf("could not get absolute path of directory containing file %s: %w", tmpFilePath, err)
		}

		err = GoGet(absPackagePath)
		if err != nil {
			return nil, err
		}
	}

	outputFilePath := path.Join(buildDir, hexHash+".a")

	importPath := "command-line-arguments"
	err = execBuild(config, outputFilePath, []string{tmpFilePath, tmpReflectFilePath})
	if err != nil {
		return nil, err
	}

	linker, err := resolveDependencies(config, outputFilePath, parsed.PackageName, pkg)
	if err != nil {
		return nil, err
	}

	return &LoadableUnit{
		Linker:               linker,
		ImportPath:           importPath,
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

	currentDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("could not get current working directory: %w", err)
	}
	defer os.Chdir(currentDir)

	// Chdir into the package folder so that go list resolves the module correctly from that path
	err = os.Chdir(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to chdir into %s: %w", absPath, err)
	}

	pkg, err := GoList(absPath)
	if err != nil {
		return nil, err
	}

	if pkg.Module == nil || pkg.Module.GoMod == "" {
		return nil, fmt.Errorf("could not find module/go.mod file for path %s", absPath)
	}

	if len(pkg.DepsErrors) > 0 {
		err = GoGet(absPath)
		if err != nil {
			return nil, err
		}
	}
	h := sha256.New()

	for _, goFile := range append(pkg.GoFiles, pkg.CgoFiles...) {
		file := path.Join(absPath, goFile)
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read file '%s': %w", file, err)
		}
		h.Write(data)
	}

	hexHash := hex.EncodeToString(h.Sum(nil))

	if config.TmpDir != "" {
		_, err := os.Stat(config.TmpDir)
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

	// Copy the directory structure of the module the package is in
	err = Copy(pkg.Module.Dir, rootBuildDir)
	if err != nil {
		return nil, fmt.Errorf("could not copy package module %s: %w", pkg.Module.Dir, err)
	}

	buildDir := path.Join(rootBuildDir, strings.TrimPrefix(pkg.Dir, pkg.Module.Dir))
	err = os.MkdirAll(buildDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("could not create new tmp directory %s: %w", buildDir, err)
	}

	if !config.KeepTempFiles {
		defer os.RemoveAll(rootBuildDir)
	}

	err = os.Chdir(buildDir)
	if err != nil {
		return nil, fmt.Errorf("failed to chdir into %s: %w", buildDir, err)
	}

	newFiles := make([]string, 0, len(pkg.GoFiles)+len(pkg.CgoFiles)+1)
	var parsedFiles []*ParsedFile
	for _, goFile := range append(pkg.GoFiles, pkg.CgoFiles...) {
		file := path.Join(absPath, goFile)
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
		newPath := path.Join(buildDir, path.Base(file))
		err = os.WriteFile(newPath, data, 0655)
		if err != nil {
			return nil, fmt.Errorf("failed to write file '%s': %w", newPath, err)
		}
		newFiles = append(newFiles, newPath)
	}

	outputFilePath := path.Join(buildDir, hexHash+".a")

	reflectCode, symbolToTypeFuncName, err := generateReflectCode(parsedFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to generated reflect code: %w", err)
	}

	tmpReflectFilePath := strings.TrimSuffix(newFiles[0], ".go") + "___reflect.go"
	err = os.WriteFile(tmpReflectFilePath, reflectCode, 0655)
	if err != nil {
		return nil, fmt.Errorf("could not create new reflection file at %s: %w", tmpReflectFilePath, err)
	}

	newFiles = append(newFiles, tmpReflectFilePath)

	importPath := pkg.ImportPath
	err = execBuild(config, outputFilePath, []string{buildDir})
	if err != nil {
		return nil, err
	}

	linker, err := resolveDependencies(config, outputFilePath, parsedFiles[0].PackageName, pkg)
	if err != nil {
		return nil, err
	}

	return &LoadableUnit{
		Linker:               linker,
		ImportPath:           importPath,
		SymbolTypeFuncLookup: symbolToTypeFuncName,
	}, nil
}
