package jit

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const importSnippetReplacement = `"encoding/json"
	"fmt"
	"strings"
)`

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

const importAnchor = `"encoding/json"
	"fmt"
)`

const objAnchor = `
func dumpdata() {
`

const flagAnchor = `
	EmbedCfg           func(string) "help:\"read go:embed configuration from ` + "`file`" + `\""`

var patchCache sync.Map

func goEnv(goBinary string) (map[string]string, error) {
	goEnvCmd := exec.Command(goBinary, "env")
	buf := bytes.Buffer{}
	goEnvCmd.Stdout = &buf
	err := goEnvCmd.Run()
	if err != nil {
		return nil, fmt.Errorf("could not run '%s env': %w", goBinary, err)
	}
	lines := strings.Split(buf.String(), "\n")
	result := map[string]string{}
	for _, line := range lines {
		split := strings.SplitN(line, "=", 2)
		if len(split) != 2 {
			continue
		}
		key := split[0]
		val, err := strconv.Unquote(split[1])
		if err != nil && val != "" {
			return nil, fmt.Errorf("failed to unquote %s (%s): %w", key, val, err)
		}
		result[key] = val
	}
	return result, nil
}

// PatchGC checks whether the go compiler at a given GOROOT requires patching
// to emit export types and if so, applies a patch and rebuilds it and tests again
func PatchGC(goBinary string, debugLog bool) error {
	var goRootPath string
	var goToolDir string
	if !filepath.IsAbs(goBinary) {
		var err error
		goBinary, err = exec.LookPath(goBinary)
		if err != nil {
			return fmt.Errorf("could not find %s in path: %w", goBinary, err)
		}
	}
	env, err := goEnv(goBinary)
	if err != nil {
		return err
	}
	goRootPath = env["GOROOT"]
	goToolDir = env["GOTOOLDIR"]
	if goToolDir == "" || goRootPath == "" {
		return fmt.Errorf("could not find GOROOT/GOTOOLDIR for %s", goBinary)
	}
	if _, ok := patchCache.Load(goRootPath); ok {
		if debugLog {
			log.Printf("go compiler in GOROOT %s already patched - skipping\n", goRootPath)
		}
		return nil
	}
	helpCmd := exec.Command(goBinary, "tool", "compile", "-help")
	stderrBuf := &bytes.Buffer{}
	helpCmd.Stderr = stderrBuf
	err = helpCmd.Run()

	helpOutput := stderrBuf.Bytes()

	if bytes.Index(helpOutput, []byte("usage:")) == -1 {
		return fmt.Errorf("could not execute '%s tool compile -help': %w\n%s", goBinary, err, helpOutput)
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
			if strings.Contains(err.Error(), "permission denied") || strings.Contains(err.Error(), "not permitted") {
				return fmt.Errorf("could not write patched '%s': %w\nTry changing $GOROOT's owner to current user, or run patch with sudo\ngo install github.com/eh-steve/goloader/jit/patchgc@latest && sudo $GOPATH/bin/patchgc", flagPath, err)
			}
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

	if bytes.Index(objFile, []byte(importAnchor)) != -1 {
		objFile = bytes.Replace(objFile, []byte(importAnchor), []byte(importSnippetReplacement), 1)
	}
	if bytes.Index(objFile, []byte(objSnippet)) == -1 {
		if bytes.Index(objFile, []byte(objAnchor)) == -1 {
			return fmt.Errorf("could not find anchor (dumpdata()) to patch '%s'", objPath)
		}
		newObjFile := bytes.Replace(objFile, []byte(objAnchor), []byte(objAnchor+objSnippet), 1)
		err = os.WriteFile(objPath, newObjFile, objFileStat.Mode())
		if err != nil {
			if strings.Contains(err.Error(), "permission denied") || strings.Contains(err.Error(), "not permitted") {
				return fmt.Errorf("could not write patched '%s': %w\nTry changing $GOROOT's owner to current user, or run patch with sudo\ngo install github.com/eh-steve/goloader/jit/patchgc@latest && sudo $GOPATH/bin/patchgc", objPath, err)
			}
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
	buildCmd := exec.Command(goBinary, "build", "-o", newCompilerPath, "cmd/compile")
	if debugLog {
		log.Printf("compiling %s\n", newCompilerPath)
		buildCmd.Stderr = os.Stderr
		buildCmd.Stdout = os.Stdout
	}
	err = buildCmd.Run()
	if err != nil {
		return fmt.Errorf("failed to compile cmd/compile: %w", err)
	}
	goCompilerPath := filepath.Join(goToolDir, "compile")
	err = move(goCompilerPath, goCompilerPath+".bak")
	if debugLog {
		log.Printf("backed up %s\n", goCompilerPath+".bak")
	}
	if err != nil {
		if strings.Contains(err.Error(), "permission denied") || strings.Contains(err.Error(), "not permitted") {
			return fmt.Errorf("could not write patched '%s': %w\nTry changing $GOROOT's owner to current user, or run patch with sudo\ngo install github.com/eh-steve/goloader/jit/patchgc@latest && sudo $GOPATH/bin/patchgc", goCompilerPath+".bak", err)
		}
		return fmt.Errorf("failed to move %s: %w", goCompilerPath, err)
	}

	err = move(newCompilerPath, goCompilerPath)
	if err != nil {
		if strings.Contains(err.Error(), "permission denied") || strings.Contains(err.Error(), "not permitted") {
			return fmt.Errorf("could not write patched '%s': %w\nTry changing $GOROOT's owner to current user, or run patch with sudo\ngo install github.com/eh-steve/goloader/jit/patchgc@latest && sudo $GOPATH/bin/patchgc", goCompilerPath, err)
		}
		return fmt.Errorf("failed to move %s: %w", newCompilerPath, err)
	}
	if debugLog {
		log.Printf("replaced %s\n", goCompilerPath)
	}
	patchCache.Store(goRootPath, true)
	return nil
}

func move(source, destination string) error {
	err := os.Rename(source, destination)
	if err != nil && strings.Contains(err.Error(), "cross-device link") {
		return moveCrossDevice(source, destination)
	}
	return err
}

func moveCrossDevice(source, destination string) error {
	src, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", source, err)
	}
	srcStat, err := src.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", source, err)
	}
	dst, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcStat.Mode())
	if err != nil {
		_ = src.Close()
		return fmt.Errorf("failed to open %s: %w", destination, err)
	}
	_, err = io.Copy(dst, src)
	_ = src.Close()
	err2 := dst.Close()
	if err != nil {
		return fmt.Errorf("failed to copy to %s: %w", destination, err)
	}
	if err2 != nil {
		return fmt.Errorf("failed to close %s: %w", destination, err2)
	}
	err = os.Remove(source)
	if err != nil {
		return fmt.Errorf("failed to remove source %s: %w", source, err)
	}
	return nil
}
