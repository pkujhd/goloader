package mmap

import (
	"fmt"
	"testing"
)

func TestMmapVmData(t *testing.T) {
	mappings, err := getCurrentProcMaps()
	if err != nil {
		t.Fatal(err)
	}
	for _, mapping := range mappings {
		fmt.Printf("%016x - %016x\n", mapping.StartAddr, mapping.EndAddr)
	}
}

func TestMmapManager(t *testing.T) {
	data, err := Mmap(215123)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%p\n", &data[0])

	data2, err := Mmap(215123)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%p\n", &data2[0])

	err = Munmap(data)
	if err != nil {
		t.Fatal(err)
	}
	err = Munmap(data2)
	if err != nil {
		t.Fatal(err)
	}
}
