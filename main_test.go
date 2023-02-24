package main

import (
	"net"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func dailHelper(t *testing.T, src, dst string) *net.UDPConn {
	addrS, err := net.ResolveUDPAddr("udp", src)
	assert.NilError(t, err)
	addrD, err := net.ResolveUDPAddr("udp", dst)
	assert.NilError(t, err)
	localConn, err := net.DialUDP("udp", addrS, addrD)
	assert.NilError(t, err)
	return localConn
}

func TestItWorks(t *testing.T) {
	serverPath := "/path"
	serverBindAddrStr := "127.0.0.1:57414"
	timeout := time.Second * 30
	bufferSize := 1600
	go server(serverPath, serverBindAddrStr, bufferSize, timeout)

	time.Sleep(time.Second * 1)

	localSrcAddrStrA := "127.0.0.1:51374"
	localDestinationStrA := "127.0.0.1:53458"
	sessionKeyA := "Aj84Xos945x"
	go client("ws://"+serverBindAddrStr+serverPath, localSrcAddrStrA, localDestinationStrA, sessionKeyA, bufferSize, timeout)

	localSrcAddrStrB := "127.0.0.1:51373"
	localDestinationStrB := "127.0.0.1:53452"
	sessionKeyB := "x549soX48jA"
	go client("ws://"+serverBindAddrStr+serverPath, localSrcAddrStrB, localDestinationStrB, sessionKeyB, bufferSize, timeout)

	connA := dailHelper(t, localDestinationStrA, localSrcAddrStrA)
	connB := dailHelper(t, localDestinationStrB, localSrcAddrStrB)

	_, err := connA.Write([]byte("hello world"))
	assert.NilError(t, err)
	buffer := make([]byte, bufferSize)
	n, err := connB.Read(buffer)
	assert.NilError(t, err)
	assert.Equal(t, string(buffer[:n]), "hello world")

	_, err = connB.Write([]byte("hello world 2"))
	assert.NilError(t, err)
	n, err = connA.Read(buffer)
	assert.NilError(t, err)
	assert.Equal(t, string(buffer[:n]), "hello world 2")
}
