//go:build go1.8 && !go1.16
// +build go1.8,!go1.16

package goloader

func updateFuncnameTabInUnix(md *moduledata, baseAddr uintptr, pclntabSectData []byte) {
}

func updateFuncnameTabInPe(md *moduledata, off uintptr) {
}
