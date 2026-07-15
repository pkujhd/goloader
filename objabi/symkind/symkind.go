package symkind

//go:inline
func IsText(kind int) bool {
	return kind == STEXT || kind == STEXTFIPS
}

//go:inline
func IsData(kind int) bool {
	return kind == SDATA || kind == SDATAFIPS
}

//go:inline
func IsROData(kind int) bool {
	return kind == SRODATA || kind == SRODATAFIPS
}

//go:inline
func IsNoPtrData(kind int) bool {
	return kind == SNOPTRDATA || kind == SNOPTRDATAFIPS
}

//go:inline
func IsBss(kind int) bool {
	return kind == SBSS
}
