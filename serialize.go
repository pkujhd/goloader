package goloader

import (
	"encoding/binary"
	"encoding/gob"
	"io"
)

func Serialize(linker *Linker, writer io.Writer) error {
	gob.Register(binary.LittleEndian)
	gob.Register(binary.BigEndian)
	encoder := gob.NewEncoder(writer)
	err := encoder.Encode(linker)
	if err != nil {
		return err
	}
	return nil
}

func UnSerialize(reader io.Reader) (*Linker, error) {
	gob.Register(binary.LittleEndian)
	gob.Register(binary.BigEndian)
	linker := initLinker()
	decoder := gob.NewDecoder(reader)
	err := decoder.Decode(linker)
	if err != nil {
		return nil, err
	}
	return linker, nil
}
