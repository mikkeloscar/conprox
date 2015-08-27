package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

const (
	defSocket = "conprox.sock"
	umask     = 0
)

func runCommand(c net.Conn) {
	var cmd *exec.Cmd
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var ret uint16
	buf := make([]byte, 512)

	for {
		ret = 0
		n, err := c.Read(buf)
		if err != nil {
			log.Println(err)
			if err == io.EOF {
				c.Close()
				log.Println("client disconnect")
				return
			}
		}

		command := buf[0:n]
		log.Printf("running command: '%s'\n", command)

		args := strings.Split(string(command), " ")

		cmd = exec.Command(args[0], args[1:]...)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Run()
		if err != nil {
			log.Println(err)

			if exiterr, ok := err.(*exec.ExitError); ok {
				if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
					ret = uint16(status.ExitStatus())
					stdout = stderr
				}
			} else {
				ret = 127 // command not found
				stdout.WriteString(err.Error())
			}
		}

		// len of output + msg len and return value
		response := make([]byte, stdout.Len()+6)

		binary.BigEndian.PutUint32(response, uint32(stdout.Len()))

		for i, b := range stdout.Bytes() {
			response[4+i] = b
		}

		binary.BigEndian.PutUint16(response[stdout.Len()+4:], ret)

		_, err = c.Write(response)
		if err != nil {
			log.Println(err)
		}
		stdout.Reset()
		stderr.Reset()
	}
}

func main() {
	socket := defSocket

	if len(os.Args) > 1 {
		socket = os.Args[2]
	}

	// set Umask to allow creating files with specified permissions
	_ = syscall.Umask(umask)

	l, err := net.ListenUnix("unix", &net.UnixAddr{socket, "unix"})
	if err != nil {
		log.Fatal("listen error:", err)
	}
	defer l.Close()

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				log.Fatal("accept error:", err)
			}

			go runCommand(conn)
		}
	}()

	// close socket on exit
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-signals:
		switch sig {
		case syscall.SIGTERM, os.Interrupt:
			err := os.Remove(socket)
			if err != nil {
				log.Fatal(err)
			}

			log.Println("exit")
			os.Exit(0)
		}
	}
}
