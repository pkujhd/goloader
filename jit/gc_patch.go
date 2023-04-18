package jit

import (
	"bytes"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

const flagSnippet = `
	ExportTypes        bool         "help:\"emit GoAuxTypes for package exports\""`

const objSnippet = `	if base.Flag.ExportTypes {
		for _, export := range typecheck.Target.Exports {
			s := export.Linksym()

			if strings.HasSuffix(s.Name, "..inittask") && s.OnList() {
				continue
			}

			t := export.Type()
			if t == nil || (t.IsPtr() && t.Elem() == nil) || t.IsUntyped() {
				continue
			}
			s.Gotype = reflectdata.TypeLinksym(export.Type())
		}
	}

`
const objAnchor = `
func dumpdata() {
`

const flagAnchor = `
	EmbedCfg           func(string) "help:\"read go:embed configuration from ` + "`file`" + `\""`

var patchCache sync.Map

// PatchGC checks whether the go compiler at a given GOROOT requires patching
// to emit export types and if so, applies a patch and rebuilds it and tests again
func PatchGC(goBinary string, debugLog bool) error {
	var goRootPath string
	if filepath.IsAbs(goBinary) {
		goRootPath = filepath.Dir(filepath.Dir(goBinary))
	} else {
		var err error
		goBinary, err = exec.LookPath(goBinary)
		if err != nil {
			return fmt.Errorf("could not find %s in path: %w", goBinary, err)
		}
		goRootPath = filepath.Dir(filepath.Dir(goBinary))
	}
	if _, ok := patchCache.Load(goRootPath); ok {
		if debugLog {
			log.Printf("go compiler in GOROOT %s already patched - skipping\n", goRootPath)
		}
		return nil
	}
	goBinaryPath := filepath.Join(goRootPath, "bin", "go")
	goBinStat, err := os.Stat(goBinaryPath)
	if err != nil {
		return fmt.Errorf("could not stat %s: %w", goBinaryPath, err)
	}
	if goBinStat.IsDir() {
		return fmt.Errorf("go bin path '%s' is directory", goBinaryPath)
	}
	helpCmd := exec.Command(goBinaryPath, "tool", "compile", "-help")
	stderrBuf := &bytes.Buffer{}
	helpCmd.Stderr = stderrBuf
	err = helpCmd.Run()

	helpOutput := stderrBuf.Bytes()

	if bytes.Index(helpOutput, []byte("usage:")) == -1 {
		return fmt.Errorf("could not execute '%s tool compile -help': %w\n%s", goBinaryPath, err, helpOutput)
	}

	if bytes.Index(helpOutput, []byte("-exporttypes")) != -1 {
		// Compiler already patched
		if debugLog {
			log.Printf("go compiler in GOROOT %s already patched - skipping\n", goRootPath)
		}
		patchCache.Store(goRootPath, true)
		return nil
	}

	objPath := filepath.Join(goRootPath, "src", "cmd", "compile", "internal", "gc", "obj.go")
	flagPath := filepath.Join(goRootPath, "src", "cmd", "compile", "internal", "base", "flag.go")

	objFile, err := os.ReadFile(objPath)
	objFileStat, _ := os.Stat(objPath)

	if err != nil {
		return fmt.Errorf("could not read '%s': %w", objPath, err)
	}
	flagFile, err := os.ReadFile(flagPath)
	flagFileStat, _ := os.Stat(flagPath)

	if err != nil {
		return fmt.Errorf("could not read '%s': %w", flagPath, err)
	}

	if bytes.Index(flagFile, []byte(flagSnippet)) == -1 {
		if bytes.Index(flagFile, []byte(flagAnchor)) == -1 {
			return fmt.Errorf("could not find anchor (EmbedCfg) to patch '%s'", flagPath)
		}
		newFlagFile := bytes.Replace(flagFile, []byte(flagAnchor), []byte(flagAnchor+flagSnippet), 1)
		err = os.WriteFile(flagPath, newFlagFile, flagFileStat.Mode())
		if err != nil {
			return fmt.Errorf("could not write patched '%s': %w", flagPath, err)
		}
		if debugLog {
			log.Printf("patched %s\n", flagPath)
		}
	} else {
		if debugLog {
			log.Printf("%s already patched - skipping\n", flagPath)
		}
	}

	if bytes.Index(objFile, []byte(objSnippet)) == -1 {
		if bytes.Index(objFile, []byte(objAnchor)) == -1 {
			return fmt.Errorf("could not find anchor (dumpdata()) to patch '%s'", objPath)
		}
		newObjFile := bytes.Replace(objFile, []byte(objAnchor), []byte(objAnchor+objSnippet), 1)
		err = os.WriteFile(objPath, newObjFile, objFileStat.Mode())
		if err != nil {
			return fmt.Errorf("could not write patched '%s': %w", objPath, err)
		}
		if debugLog {
			log.Printf("patched %s\n", objPath)
		}
	} else {
		if debugLog {
			log.Printf("%s already patched - skipping\n", objPath)
		}
	}

	tmpDir, err := os.MkdirTemp("", "gcpatch")
	if err != nil {
		return fmt.Errorf("could not create temp dir: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	newCompilerPath := filepath.Join(tmpDir, "compile")
	buildCmd := exec.Command(goBinaryPath, "build", "-o", newCompilerPath, "cmd/compile")
	if debugLog {
		log.Printf("compiling %s\n", newCompilerPath)
		buildCmd.Stderr = os.Stderr
		buildCmd.Stdout = os.Stdout
	}
	err = buildCmd.Run()
	if err != nil {
		return fmt.Errorf("failed to compile cmd/compile: %w", err)
	}
	goCompilerPath := filepath.Join(goRootPath, "pkg", "tool", runtime.GOOS+"_"+runtime.GOARCH, "compile")
	err = os.Rename(goCompilerPath, goCompilerPath+".bak")
	if debugLog {
		log.Printf("backed up %s\n", goCompilerPath+".bak")
	}
	if err != nil {
		return fmt.Errorf("failed to move %s: %w", goCompilerPath, err)
	}

	err = os.Rename(newCompilerPath, goCompilerPath)
	if err != nil {
		return fmt.Errorf("failed to move %s: %w", newCompilerPath, err)
	}
	if debugLog {
		log.Printf("replaced %s\n", goCompilerPath)
	}
	patchCache.Store(goRootPath, true)
	return nil
}
