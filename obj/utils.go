package obj

func findFileTab(filename string, namemap map[string]int, filetab []uint32) uint32 {
	tab := namemap[filename]
	for index, value := range filetab {
		if uint32(tab) == value {
			return uint32(index)
		}
	}
	return 1<<32 - 1
}

func grow(bytes *[]byte, size int) {
	if len(*bytes) < size {
		*bytes = append(*bytes, make([]byte, size-len(*bytes))...)
	}
}
