package main

import (
	"crypto/sha1"
	"io"
	"log"
	"time"

	"github.com/xtaci/kcp-go/v5"
	"golang.org/x/crypto/pbkdf2"
)

func main() {
	key := pbkdf2.Key([]byte("demo pass"), []byte("demo salt"), 1024, 32, sha1.New)
	block, _ := kcp.NewAESBlockCrypt(key)
	ln, err := kcp.ListenWithOptions("127.0.0.1:12345", block, 10, 3)
	if err != nil {
		panic(err)
	}

	// spin-up the client
	go client()

	for {
		s, err := ln.AcceptKCP()
		if err != nil {
			log.Fatal(err)
		}
		go handleEcho(s)
	}
}

// handleEcho send back everything it received
func handleEcho(conn *kcp.UDPSession) {
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			log.Println(err)
			return
		}

		_, err = conn.Write(buf[:n])
		if err != nil {
			log.Println(err)
			return
		}
	}
}

func client() {
	key := pbkdf2.Key([]byte("demo pass"), []byte("demo salt"), 1024, 32, sha1.New)
	block, _ := kcp.NewAESBlockCrypt(key)

	// wait for server to become ready
	time.Sleep(time.Second)

	// dial to the echo server
	sess, err := kcp.DialWithOptions("127.0.0.1:12345", block, 10, 3)
	if err != nil {
		panic(err)
	}

	for {
		data := time.Now().String()
		buf := make([]byte, len(data))

		_, err := sess.Write([]byte(data))
		if err != nil {
			panic(err)
		}
		log.Println("sent:", data)

		// read back the data
		_, err = io.ReadFull(sess, buf)
		if err != nil {
			panic(err)
		}
		log.Println("recv:", string(buf))

		time.Sleep(time.Second)
	}
}
