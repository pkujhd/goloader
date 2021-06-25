package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type ObjInfo struct {
	PackageName string
	ObjFilePath string
	ObjFileHash string
}

type Builder struct {
	ObjInfos    []ObjInfo
	ExecuteFunc []string
	files       []string
	packageName string
	packagePath string
}

func (b *Builder) AddExecuteFunc(packageName string, funcName string) {
	b.ExecuteFunc = append(b.ExecuteFunc, packageName+"."+funcName)
}

func GetEnv() {
	fmt.Println(runtime.GOROOT())
	fmt.Println(os.Getenv("GOPATH"))
}

func CleanCache() error {
	return exec.Command("go", "clean", "--cache").Run()
}

func BuildPackage(name string) (output string, workDir string, err error) {
	cmd := exec.Command("go", "build", "-a", "-x", "-work", name)
	outBuf, err := cmd.CombinedOutput()
	output = string(outBuf)
	lines := strings.Split(output, "\n")
	if len(lines) <= 0 || !strings.Contains(lines[0], "WORK=") {
		fmt.Println("err")
	}
	workDir = filepath.Join(strings.TrimPrefix(lines[0], "WORK="), "b001")
	return output, workDir, err
}

func RenameAndCopyObjFile(packageName string, oldPath string, prefix string) string {
	newName := strings.ReplaceAll(packageName, ".", "_")
	newName = strings.ReplaceAll(newName, "/", "_")
	newName = strings.ReplaceAll(newName, "\\", "_")
	newName = "goloader_" + newName + ".a"
	if prefix != "" {
		newName = filepath.Join(prefix, newName)
	}
	err := os.Rename(oldPath, newName)
	if err != nil {
		fmt.Println("Rename Failed", err)
	}
	newName, _ = filepath.Abs(newName)
	return newName
}

var clean = true

func (b *Builder) BuildPackageObjs() {
	CleanCache()
	_, workDir, _ := BuildPackage(b.packageName)
	file, err := os.Open(filepath.Join(workDir, "importcfg"))
	if err != nil {
		panic(err)
	}
	defer file.Close()
	packageFiles, err := ioutil.ReadAll(file)
	if err != nil {
		return
	}
	b.ObjInfos = append(b.ObjInfos, ObjInfo{PackageName: b.packageName, ObjFilePath: RenameAndCopyObjFile(b.packageName, workDir+"/_pkg_.a", "")})
	for _, packagefile := range strings.Split(string(packageFiles), "\n") {
		if strings.HasPrefix(packagefile, "packagefile ") {
			tmp := strings.Split(strings.TrimPrefix(packagefile, "packagefile "), "=")
			b.ObjInfos = append(b.ObjInfos, ObjInfo{PackageName: tmp[0], ObjFilePath: RenameAndCopyObjFile(tmp[0], tmp[1], "")})
		}
	}
	if clean {
		os.RemoveAll(workDir)
	}
}

func (b *Builder) SearchExecute() {
	for _, file := range b.files {
		srcData, err := ioutil.ReadFile(filepath.Join(b.packagePath, file))
		if err != nil {
			fmt.Println(err)
		}
		fset := token.NewFileSet()
		fparser, err := parser.ParseFile(fset, "", srcData, parser.ParseComments)
		if err != nil {
			fmt.Println(err)
		}
		// TODO:support dependency.
		for _, decl := range fparser.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Doc != nil && fn.Doc.List != nil {
				for _, doc := range fn.Doc.List {
					if doc.Text == "//go:execute" {
						b.AddExecuteFunc(fparser.Name.String(), fn.Name.String())
					}
				}
			}
		}
	}

}

func (b *Builder) MarshalData() {
	mar, _ := json.MarshalIndent(b, "", "\t")
	fmt.Println(string(mar))
}

func (b *Builder) WriteToFile(path string) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	mar, _ := json.MarshalIndent(b, "", "\t")
	f.Write(mar)
}

func InitBuild(packageName string) *Builder {
	b := &Builder{packageName: packageName}

	// Get dir path.
	cmd := exec.Command("go", "list", "-f", "{{.Dir}}", packageName)
	outBuf, _ := cmd.CombinedOutput()
	b.packagePath = strings.Trim(string(outBuf), "\n")

	// Get all files.
	// TODO: support build tags.
	cmd = exec.Command("go", "list", "-f", "{{.GoFiles}}", packageName)
	outBuf, _ = cmd.CombinedOutput()
	b.files = strings.Split(strings.Trim(string(outBuf), "[]\n"), " ")
	return b
}

func main() {
	var packageName = flag.String("p", "", "package name")
	var jsonFilePath = flag.String("j", "./goloader.json", "json file path")
	flag.Parse()

	builder := InitBuild(*packageName)
	builder.BuildPackageObjs()
	builder.SearchExecute()
	builder.WriteToFile(*jsonFilePath)
}
