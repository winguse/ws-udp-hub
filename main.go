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

var bufferSize = flag.Int("buffer-size", 10240, "buffer size in bytes, the max UDP package size.")
var timeout = flag.Int("timeout", 3600, "session timeout in seconds")

var clientMode = flag.Bool("client-mode", false, "running mode, default to false (server mode), set to true to run client mode")
var restartSleep = flag.Int("restart-sleep", 1, "restart sleep in seconds")
var verboseLogging = flag.Bool("verbose", false, "verbose logging")

var serverBindAddrStr = flag.String("server-bind", "127.0.0.1:30000", "server listen address")
var serverPath = flag.String("server-path", "/path", "server websocket path")

var serverWsUrl = flag.String("server-ws-url", "ws://127.0.0.1:30000/path", "server websocket url")
var localSrcAddrStr = flag.String("local-src", "127.0.0.1:5000", "local source address")
var localDestinationStr = flag.String("local-dst", "127.0.0.1:6000", "local destination address")
var sessionKey = flag.String("session-key", "abcdef", "the session key tell the server to connect on server side, the peer connection should have session key reversed")

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
		data := make([]byte, bufferSize)
		n, err := ws.Read(data)
		if err != nil {
			log.Printf("failed to read ws %s", err)
			return
		}
		sessionKey := string(data[:n])
		reverseSessionKey := reverse(sessionKey)
		log.Printf("got %s ws connection", sessionKey)
		defer log.Printf("%s ws connection ended", sessionKey)

		if previousWs, ok := sessionMap.Load(reverseSessionKey); ok {
			log.Printf("closing %s\n", reverseSessionKey)
			sessionMap.LoadOrStore(reverseSessionKey, ws)
			previousWs.(*websocket.Conn).Close()
		} else {
			sessionMap.LoadOrStore(reverseSessionKey, ws)
		}

		defer sessionMap.CompareAndDelete(reverseSessionKey, ws)

		for {
			ws.SetReadDeadline(time.Now().Add(timeout))
			n, err := ws.Read(data)
			if err != nil {
				log.Printf("error during ws read: %s for %s\n", err, sessionKey)
				break
			}
			if wsWrite, ok := sessionMap.Load(sessionKey); ok {
				writeConn := wsWrite.(*websocket.Conn)
				writeConn.SetWriteDeadline(time.Now().Add(timeout))
				if _, err = writeConn.Write(data[:n]); err != nil {
					log.Printf("error during ws write: %s\n", err)
					break
				}
			} else {
				log.Printf("cannot find ws for %s\n", sessionKey)
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
		defer log.Printf("client -> server ended")
		data := make([]byte, bufferSize)
		for !done {
			n, err := localConn.Read(data)
			if err != nil {
				log.Printf("error during read: %s\n", err)
				break
			}
			if done {
				break
			}
			verbosePrintf("client -> server, size: %d\n", n)
			ws.SetWriteDeadline(time.Now().Add(timeout))
			if _, err := ws.Write(data[:n]); err != nil {
				log.Printf("error send data to ws: %s\n", err)
				break
			}
		}
	}()

	// server -> client
	go func() {
		defer setDone()
		defer log.Printf("server -> client ended")
		data := make([]byte, bufferSize)
		for !done {
			ws.SetReadDeadline(time.Now().Add(timeout))
			n, err := ws.Read(data)
			if err != nil {
				log.Printf("error during ws read: %s\n", err)
				break
			}
			if done {
				break
			}
			verbosePrintf("server -> client, size: %d\n", n)
			if _, err := localConn.Write(data[:n]); err != nil {
				log.Printf("error send data to local conn: %s\n", err)
				break
			}
		}
	}()

	log.Printf("Client working on %s <-> %s, at %s using session %s\n", localSrcAddrStr, localDestinationStr, serverWsUrl, sessionKey)
	wg.Wait()
}

func main() {
	flag.Parse()
	for {
		if *clientMode {
			client(*serverWsUrl, *localSrcAddrStr, *localDestinationStr, *sessionKey, *bufferSize, time.Duration(*timeout)*time.Second)
		} else {
			server(*serverPath, *serverBindAddrStr, *bufferSize, time.Duration(*timeout)*time.Second)
		}
		log.Printf("restarting in %d seconds...\n", *restartSleep)
		time.Sleep(time.Duration(*restartSleep) * time.Second)
	}
}
