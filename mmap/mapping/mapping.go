package mapping

type Mapping struct {
	StartAddr   uintptr
	EndAddr     uintptr
	ReadPerm    bool
	WritePerm   bool
	ExecutePerm bool
	SharedPerm  bool
	PrivatePerm bool
	Offset      uintptr
	Dev         string
	Inode       uint64
	PathName    string
}
