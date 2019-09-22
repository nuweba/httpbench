package tests

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"os"
)

type SockIpc struct {
	conn     net.Conn
	listener net.Listener
	Closed   bool
}

/* The unix socket communication between tests and integration test works like this:
From test side:
- Run the unix socket and wait for connection
- Run the server
- On accept- send the bench config
- Wait for "ready" from server
*/

func (sock *SockIpc) SendData(data []byte) {
	if sock.conn == nil {
		sock.Connect()
	}

	n, err := sock.conn.Write(data)
	if err != nil {
		log.Fatal("Communication failed to send data:", err)
	}

	if n != len(data) {
		log.Printf("Communication couldn't send all the data: send %d of %d\n", n, len(data))
	}
}

func (sock *SockIpc) ReadConfig() []byte {
	if sock.conn == nil {
		sock.Connect()
	}

	data := make([]byte, 4096)
	_, err := sock.conn.Read(data)
	if err != nil {
		fmt.Println("Failed to read config:", err)
		return nil
	}

	return bytes.Trim(data, "\x00")
}

func (sock *SockIpc) Connect() error {
	conn, err := net.Dial("unix", SockAddr)
	if err != nil {
		fmt.Println("Communication connect err:", err)
		return err
	}
	sock.conn = conn
	return nil
}

func (sock *SockIpc) HandleConnection(conn net.Conn, f func(data []byte)) {
	for {
		data := make([]byte, 4096) //clear to avoid carry for next read
		_, err := conn.Read(data)
		if err != nil {
			if err == io.EOF || sock.Closed {
				return
			}
		}

		f(bytes.Trim(data, "\x00"))
	}
}

func (sock *SockIpc) ListenAndServe(writeOnAccept []byte, recvFunc func(data []byte)) {
	if err := os.RemoveAll(SockAddr); err != nil {
		fmt.Println("Communication failure:", err)
		return
	}

	l, err := net.Listen("unix", SockAddr)
	if err != nil {
		fmt.Println("Communication listen error:", err)
		return
	}

	go func() {
		sock.listener = l
		defer l.Close()

		conn, err := l.Accept()
		if err != nil {
			if !sock.Closed {
				fmt.Println("Communication accept error:", err)
			}
			return
		}
		sock.conn = conn

		_, err = conn.Write([]byte(writeOnAccept))
		if err != nil {
			fmt.Println("Communication write error:", err)
			return
		}

		sock.HandleConnection(conn, recvFunc)
	}()
}

func (sock *SockIpc) Close() {
	sock.Closed = true
	if sock.conn != nil {
		sock.conn.Close()
	}
	if sock.listener != nil {
		sock.listener.Close()
	}
}
