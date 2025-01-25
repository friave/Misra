package main

import (
	"bufio"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"sync"
	"time"
)

type MyToken int

const (
	NONE MyToken = iota
	PING
	PONG
	BOTH
)

type TokenType int

const (
	PING_TOKEN TokenType = iota
	PONG_TOKEN
)

type MisraSocket struct {
	m           int64
	ping        int64
	pong        int64
	pingLost    float64
	myToken     MyToken
	readChannel chan int64
	sendConn    *net.Conn
	listenConn  *net.Conn
}

func Node(pingLost *float64) *MisraSocket {
	return &MisraSocket{
		sendConn:    nil,
		listenConn:  nil,
		m:           0,
		ping:        1,
		pong:        -1,
		myToken:     NONE,
		readChannel: make(chan int64, 2),
		pingLost:    *pingLost,
	}
}

func (socket *MisraSocket) listen(wg *sync.WaitGroup, port string) {
	defer wg.Done()
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	// defer listener.Close()

	conn, err := listener.Accept()
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	fmt.Printf("I am connected to %s\n", conn.RemoteAddr().String())
	socket.listenConn = &conn
}

func (socket *MisraSocket) connect(address string) {
	var conn net.Conn
	var err error
	for true {
		conn, err = net.Dial("tcp", address)
		if err != nil {
			fmt.Println("Error:", err)
		} else {
			fmt.Printf("Connected to %s\n", address)
			break
		}
	}
	socket.sendConn = &conn
}

func (socket *MisraSocket) handleMessage() {
	go socket.listenFromConn()
	for {
		switch socket.myToken {
		case NONE:
			select {
			case token := <-socket.readChannel:
				socket.receiveToken(token)
			}
		case PING:
			fmt.Printf("I have a ping token %d, entering critical section.\n", socket.myToken)
			time.Sleep(1 * time.Second)
			fmt.Println("Exiting critical section.")
			select {
			case token := <-socket.readChannel:
				socket.receiveToken(token)
				continue
			default:
				socket.send(PING_TOKEN)
			}
		case PONG:
			socket.send(PONG_TOKEN)
		case BOTH:
			fmt.Println("I have both tokens, incarnating them.")
			socket.ping++
			socket.pong = -socket.ping
			fmt.Printf("New token values: PING: %d PONG: %d\n", socket.ping, socket.pong)
			socket.send(PING_TOKEN)
			socket.send(PONG_TOKEN)
		}
	}

}

func (socket *MisraSocket) send(tokenType TokenType) {
	switch tokenType {
	case PING_TOKEN:
		if !(rand.Float64() < socket.pingLost) {
			_, err := fmt.Fprintln(*socket.sendConn, socket.ping)
			if err != nil {
				fmt.Println(err.Error())
				return
			}
		}
		socket.m = socket.ping
		if socket.myToken == PING {
			socket.myToken = NONE
		}
		if socket.myToken == BOTH {
			socket.myToken = PONG
		}
	case PONG_TOKEN:
		_, err := fmt.Fprintln(*socket.sendConn, socket.pong)
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		socket.m = socket.pong
		if socket.myToken == PONG {
			socket.myToken = NONE
		}
		if socket.myToken == BOTH {
			socket.myToken = PING
		}
	}
	var tokenToSend string
	if tokenType == PING_TOKEN {
		tokenToSend = "PING"
	} else {
		tokenToSend = "PONG"
	}
	fmt.Printf("Token send: %s, value: %d\n", tokenToSend, socket.m)
}

func (socket *MisraSocket) receiveToken(token int64) {
	if absolute(token) < absolute(socket.m) {
		fmt.Println("I recived old token.")
		return
	}

	if token == socket.m {
		if socket.m > 0 {
			fmt.Println("PONG is lost, regenerating.")
		}
		if socket.m < 0 {
			fmt.Println("PING is lost, regenerating.")
		}
		socket.myToken = BOTH
		return
	}

	if token > 0 { // PING
		socket.ping = token
		socket.pong = -token

		if socket.myToken == NONE {
			socket.myToken = PING
		}
		if socket.myToken == PONG {
			socket.myToken = BOTH
		}
		return
	}

	if token < 0 { //PONG
		socket.pong = token
		socket.ping = -token

		if socket.myToken == NONE {
			socket.myToken = PONG
		}
		if socket.myToken == PING {
			socket.myToken = BOTH
		}
		return

	}
}

func absolute(x int64) int64 {
	if x < 0 {
		x = -x
	}
	return x
}

func (socket *MisraSocket) listenFromConn() {
	scanner := bufio.NewScanner(*socket.listenConn)
	for {
		scanner.Scan()
		token, err := strconv.ParseInt(scanner.Text(), 10, 64)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
		socket.readChannel <- token
		continue
	}
}

func main() {
	listenPort := flag.String("l", "", "On what port should I listen?")
	sendAddress := flag.String("s", "", "On what adress should I send tokens?")
	notInitiator := flag.Bool("i", false, "Am I initiator?")
	pingLost := flag.Float64("p", 0, "Ping lost probability 0..1")
	flag.Parse()

	if *listenPort == "" || *sendAddress == "" {
		fmt.Println("You need to give me something: go run main.go -l <port> -s <address:port> (-i) <initiator> ((-p <ping lost probability>))")
		os.Exit(1)
	}

	misraSocket := Node(pingLost)

	wg := sync.WaitGroup{}
	wg.Add(1)

	if *notInitiator {
		go misraSocket.listen(&wg, *listenPort)
		misraSocket.connect(*sendAddress)
		misraSocket.myToken = BOTH
		misraSocket.send(PING_TOKEN)
		misraSocket.send(PONG_TOKEN)
	} else {
		misraSocket.listen(&wg, *listenPort)
		misraSocket.connect(*sendAddress)
	}

	wg.Wait()

	misraSocket.handleMessage()

}
