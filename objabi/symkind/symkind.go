package symkind

//go:inline
func IsText(kind int) bool {
	return kind == STEXT || kind == STEXTFIPS
}
