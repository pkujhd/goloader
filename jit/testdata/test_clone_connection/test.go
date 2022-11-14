package test_clone_connection

import (
	"github.com/pkujhd/goloader/jit/testdata/common"
	"net"
)

type ConnDialer struct {
	conn net.Conn
}

func (c *ConnDialer) Dial(addr string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	c.conn = conn
	return nil
}

func (c *ConnDialer) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func NewConnDialer() common.MessageWriter {
	return &ConnDialer{}
}

func (c *ConnDialer) WriteMessage(data string) (int, error) {
	return c.conn.Write([]byte(data))
}
