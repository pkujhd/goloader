package obj

func FindFileTab(filename string, namemap map[string]int, filetab []uint32) int32 {
	tab := namemap[filename]
	for index, value := range filetab {
		if uint32(tab) == value {
			return int32(index)
		}
	}
	return -1
}

func grow(bytes *[]byte, size int) {
	if len(*bytes) < size {
		*bytes = append(*bytes, make([]byte, size-len(*bytes))...)
	}
}
