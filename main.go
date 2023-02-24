package main

import (
	"flag"
	"log"
	"net"
	"sync"
	"time"

	"net/http"

	"golang.org/x/net/websocket"
)

var timeout = flag.Int("timeout", 30, "session timeout in seconds")
var restartSleep = flag.Int("restart-sleep", 3, "restart sleep in seconds")
var verboseLogging = flag.Bool("verbose", false, "verbose logging")
var bufferSize = flag.Int("buffer-size", 1600, "buffer size in bytes, the max UDP package size.")

var clientMode = flag.Bool("client-mode", false, "running mode, true for client mode, false for server mode")

var serverAddress = flag.String("server-address", "127.0.0.1:3000", "server listen address")
var serverPath = flag.String("server-path", "/path", "server websocket path")

func verbosePrintf(format string, v ...interface{}) {
	if *verboseLogging {
		log.Printf(format, v...)
	}
}

func reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

func server(serverPath, serverBindAddrStr string, bufferSize int, timeout time.Duration) {
	sessionMap := sync.Map{}
	http.Handle(serverPath, websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()
		log.Printf("got ws connection")
		defer log.Printf("ws connection ended")
		data := make([]byte, bufferSize)
		n, err := ws.Read(data)
		if err != nil {
			log.Printf("failed to read ws %s", err)
			return
		}
		sessionKey := string(data[:n])
		reverseSessionKey := reverse(sessionKey)

		if previousWs, ok := sessionMap.Load(reverseSessionKey); ok {
			previousWs.(*websocket.Conn).Close()
		}

		sessionMap.LoadOrStore(reverseSessionKey, ws)
		defer sessionMap.CompareAndDelete(reverseSessionKey, ws)

		for {
			ws.SetReadDeadline(time.Now().Add(timeout))
			n, err := ws.Read(data)
			if err != nil {
				log.Printf("error during ws read: %s\n", err)
				break
			}
			if wsWrite, ok := sessionMap.Load(sessionKey); ok {
				if _, err = wsWrite.(*websocket.Conn).Write(data[:n]); err != nil {
					log.Printf("error during ws write: %s\n", err)
					break
				}
			}
		}
	}))
	if err := http.ListenAndServe(serverBindAddrStr, nil); err != nil {
		log.Printf("ListenAndServe %s ", err.Error())
	}
}

func client(serverWsUrl, localSrcAddrStr, localDestinationStr, sessionKey string, bufferSize int, timeout time.Duration) {
	localSrcAddr, err := net.ResolveUDPAddr("udp", localSrcAddrStr)
	if err != nil {
		log.Printf("failed to parse local source addr %s", err)
		return
	}
	localDestinationAddr, err := net.ResolveUDPAddr("udp", localDestinationStr)
	if err != nil {
		log.Printf("failed to parse local destination addr %s", err)
		return
	}

	localConn, err := net.DialUDP("udp", localSrcAddr, localDestinationAddr)
	if err != nil {
		log.Printf("error while client listening %s", err)
		return
	}
	defer localConn.Close()

	origin := "http://localhost/"
	ws, err := websocket.Dial(serverWsUrl, "", origin)
	if err != nil {
		log.Printf("error while client dial ws %s", err)
		return
	}
	defer ws.Close()
	ws.Write([]byte(sessionKey))

	var wg sync.WaitGroup
	wg.Add(2)
	var done = false
	setDone := func() {
		defer wg.Done()
		done = true
	}

	// client -> server
	go func() {
		defer setDone()
		data := make([]byte, bufferSize)
		for !done {
			var n int
			ws.SetReadDeadline(time.Now().Add(timeout))
			n, err = localConn.Read(data)
			if err != nil {
				log.Printf("error during read: %s\n", err)
				break
			}
			if done {
				break
			}
			verbosePrintf("client -> server, size: %d\n", n)
			if _, err := ws.Write(data[:n]); err != nil {
				log.Printf("error send data to ws: %s\n", err)
				break
			}
		}
	}()

	// server -> client
	go func() {
		defer setDone()
		data := make([]byte, bufferSize)
		for !done {
			var n int
			ws.SetReadDeadline(time.Now().Add(timeout))
			n, err = ws.Read(data)
			if err != nil {
				log.Printf("error during ws read: %s\n", err)
				break
			}
			if done {
				break
			}
			verbosePrintf("server -> client, size: %d\n", n)
			localConn.Write(data[:n])
		}
	}()

	log.Printf("Client working on %s <-> %s, at %s using session %s\n", localSrcAddrStr, localDestinationStr, serverWsUrl, sessionKey)
	wg.Wait()
}

func main() {
	flag.Parse()
	for {
		if *clientMode {
			// client()
		} else {
			// server()
		}
		log.Printf("restarting in %d seconds...\n", *restartSleep)
		time.Sleep(time.Duration(*restartSleep) * time.Second)
	}
}
