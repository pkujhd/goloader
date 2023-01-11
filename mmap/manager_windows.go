package mmap

import (
	"bytes"
	"fmt"
	"github.com/pkujhd/goloader/mmap/mapping"
	"os"
	"sort"
	"syscall"
	"unsafe"
)

const (
	TH32CS_SNAPMODULE32 = 0x00000010
	TH32CS_SNAPMODULE   = 0x00000008
	TH32CS_SNAPTHREAD   = 0x00000004
	TH32CS_SNAPHEAPLIST = 0x00000001
)

func getIntPtr() *int {
	var x int
	x = 5
	return &x
}

func getCurrentProcMaps() ([]mapping.Mapping, error) {
	// TODO - this function currently only fetches executable code addresses and native heaps. Still need to:
	//  Fetch all OS threads from the snapshot, SuspendThread run GetThreadContext and get their stack addresses
	//  Find out how sysinternals VMMap retrieves a list of all VirtualAlloc blocks...
	//  Find a way to get the mapping addresses of all mapped files - again, sysinternals VMMap seems to manage it

	// TODO - could also populate more stuff using VirtualQueryEx (e.g. permissions etc into mappings), for now only addresses are populated

	var mappings []mapping.Mapping

	hSnapshot, err := CreateToolhelp32Snapshot(TH32CS_SNAPMODULE|TH32CS_SNAPMODULE32|TH32CS_SNAPTHREAD|TH32CS_SNAPHEAPLIST, uint32(os.Getpid()))
	if err != nil {
		return nil, fmt.Errorf("failed to CreateToolhelp32Snapshot: %w", err)
	}
	m := ModuleEntry32{}
	ok, err := Module32First(hSnapshot, &m)
	if err != nil {
		return nil, fmt.Errorf("failed to Module32First: %w", err)
	}
	var modules = []ModuleEntry32{m}

	if ok {
		for err == nil && ok {
			m = ModuleEntry32{}
			ok, err = Module32Next(hSnapshot, &m)
			if ok {
				modules = append(modules, m)
			}
		}
	}

	for _, m := range modules {
		path := string(m.szExePath[:bytes.IndexByte(m.szExePath[:], 0)])
		mappings = append(mappings, mapping.Mapping{
			StartAddr: uintptr(m.modBaseAddr),
			EndAddr:   uintptr(m.modBaseAddr + uint64(m.modBaseSize)),
			PathName:  path,
		})
	}
	buf, err := RtlCreateQueryDebugBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to RtlCreateQueryDebugBuffer: %w", err)
	}

	err = RtlQueryProcessDebugInformation(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to RtlQueryProcessDebugInformation: %w", err)
	}

	var heapNodeCount int
	if buf.HeapInformation != nil {
		heapNodeCount = int(*buf.HeapInformation)
	}

	heapInfos := (*(*[1 << 31]DebugHeapInfo)(unsafe.Pointer(uintptr(unsafe.Pointer(buf.HeapInformation)) + 8)))[:heapNodeCount:heapNodeCount]
	for i := range heapInfos {
		mappings = append(mappings, mapping.Mapping{
			StartAddr: heapInfos[i].Base,
			EndAddr:   heapInfos[i].Base + heapInfos[i].Committed,
		})
	}
	err = RtlDestroyQueryDebugBuffer(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to RtlDestroyQueryDebugBuffer: %w", err)
	}

	sort.Slice(mappings, func(i, j int) bool {
		return mappings[i].StartAddr < mappings[j].StartAddr
	})

	err = syscall.CloseHandle(hSnapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to CloseHandle: %w", err)
	}
	return mappings, nil
}
