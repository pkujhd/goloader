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
func IsBss(kind int) bool {
	return kind == SBSS
}
