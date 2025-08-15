//go:build go1.8 && !go1.22
// +build go1.8,!go1.22

package link

func registerTypeAssertInterfaceSwitchCache(symPtr map[string]uintptr) {}

func resetTypeAssertInterfaceSwitchCache() {}
