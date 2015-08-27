package client

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
)

// response format from server
type response struct {
	output string
	ret    uint16
	err    error
}

// Conprox defines a connection to a conprox server
type Conprox struct {
	conn   *net.UnixConn
	cmd    chan string
	result chan *response
	sync.Mutex
}

// Dial and setup connection to unix socket
func Dial(socket string) (*Conprox, error) {
	var err error
	conprox := &Conprox{
		cmd:    make(chan string),
		result: make(chan *response),
	}
	conprox.conn, err = net.DialUnix("unix", nil, &net.UnixAddr{socket, "unix"})
	if err != nil {
		return nil, err
	}

	go handleConn(conprox)

	return conprox, nil
}

// Close connection
func (c *Conprox) Close() error {
	return c.conn.Close()
}

// Cmd issue a single command to run on the server
func (c *Conprox) Cmd(cmd string) (string, uint16, error) {
	// only allow running one command at a time on this connection
	c.Lock()
	defer c.Unlock()

	c.cmd <- cmd

	result := <-c.result

	if result.err != nil {
		return "", 0, result.err
	}

	return result.output, result.ret, nil
}

// handle connection to server
func handleConn(c *Conprox) {
	for {
		cmd := <-c.cmd

		_, err := c.conn.Write([]byte(cmd))
		if err != nil {
			c.result <- &response{err: err}
			continue
		}

		resp := readResp(c.conn)
		c.result <- resp
	}
}

// read command response/output
func readResp(c *net.UnixConn) *response {
	buf := make([]byte, 1024)

	var output []byte

	var read int
	var ret uint16

	n, err := c.Read(buf[:4])
	if err != nil {
		return &response{err: err}
	}

	// start read
	if n != 4 {
		return &response{err: fmt.Errorf("invalid response data: %s", buf[0:n])}
	}

	// length of output
	toRead := int(binary.BigEndian.Uint32(buf[0:n]))

	for read < toRead {
		min := min(toRead-read, 1024)

		n, err := c.Read(buf[:min])
		if err != nil {
			return &response{err: err}
		}

		output = append(output, buf[0:n]...)
		read += n
	}

	n, err = c.Read(buf[:2])
	if err != nil {
		return &response{err: err}
	}

	if n != 2 {
		return &response{err: fmt.Errorf("invalid response data: %s", buf[0:n])}
	}

	ret = binary.BigEndian.Uint16(buf[0:n])

	return &response{output: string(output), ret: ret}
}

// find minimum of two ints
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
