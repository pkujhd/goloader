// +build go1.8 go1.9 go1.10 go1.11
// +build !go1.12,!go1.13

package goloader

func AddStackObject(code *CodeReloc, fi *funcInfoData, seg *segment, symPtr map[string]uintptr) {
}
