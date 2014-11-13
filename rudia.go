// Package rudia implements a simple library for relaying TCP string messages
// from one source to many clients.
package rudia

import (
	"bufio"
	"log"
	"net"
	"time"
)

type client struct {
	incoming chan string
	outgoing chan string
	dead     chan bool
	conn     net.Conn
	reader   *bufio.Reader
	writer   *bufio.Writer
}

func newClient(conn net.Conn) *client {
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	c := &client{
		incoming: make(chan string),
		outgoing: make(chan string),
		dead:     make(chan bool),
		conn:     conn,
		reader:   r,
		writer:   w,
	}
	c.listen()
	return c
}

func (c *client) read() {
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			log.Printf("%v from %v", err, c.conn.RemoteAddr())
			c.dead <- true
			break
		}

		c.incoming <- line
	}
}

func (c *client) write() {
	for data := range c.outgoing {
		c.writer.WriteString(data)
		c.writer.Flush()
	}
}

func (c *client) listen() {
	go c.read()
	go c.write()
}

// A Repeater connects to an upstream TCP endpoint and relays the messages
// it receives to all connected clients.
type Repeater struct {
	relayClients    map[string]*client
	upstreamClients map[string]*client
	relayJoins      chan net.Conn
	upstreamJoins   chan net.Conn
	outgoing        chan string
	incoming        chan string
	upstreamFault   chan bool
}

// NewRepeater creates a new Repeater and starts listening for client
// connections.
func NewRepeater() *Repeater {
	r := &Repeater{
		relayClients:    make(map[string]*client),
		upstreamClients: make(map[string]*client),
		relayJoins:      make(chan net.Conn),
		upstreamJoins:   make(chan net.Conn),
		incoming:        make(chan string),
		outgoing:        make(chan string),
		upstreamFault:   make(chan bool),
	}
	r.listen()
	return r
}

var retryInterval = 10 * time.Second

// Proxy connects to the specified TCP address and relays all received
// messages to connected clients. If the connection to the specified
// address fails, an attempt will be made to reconnect. This will repeat
// until the program exits.
func (r *Repeater) Proxy(address string) {
	go func() {
		for {
			log.Printf("Dialing %v", address)
			c, err := net.Dial("tcp", address)
			if err != nil {
				log.Println(err)
				log.Printf("Sleeping %v before retry", retryInterval)
				time.Sleep(retryInterval)
				continue
			}
			r.upstreamJoins <- c
			log.Printf("Connected to %v", address)
			_ = <-r.upstreamFault
		}
	}()
}

// ListenAndAccept listens to the specified adddress for new TCP clients
// and upon successful connect, adds them to the pool of clients to relay
// any messages received to.
func (r *Repeater) ListenAndAccept(address string) {
	l, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()

	log.Printf("Relaying data to %v", l.Addr().String())
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		r.relayJoins <- conn
	}
}

func (r *Repeater) broadcast(data string) {
	for _, c := range r.upstreamClients {
		c.outgoing <- data
	}
}

func (r *Repeater) join(conn net.Conn, clients map[string]*client, triggerUpstreamFault bool) {
	c := newClient(conn)
	clients[conn.RemoteAddr().String()] = c
	go func() {
		for {
			select {
			case inc := <-c.incoming:
				r.incoming <- inc
			case _ = <-c.dead:
				deadRemote := c.conn.RemoteAddr().String()
				delete(clients, deadRemote)
				if triggerUpstreamFault {
					r.upstreamFault <- true
				}
			}
		}
	}()
}

func (r *Repeater) listen() {
	go func() {
		for {
			select {
			case data := <-r.incoming:
				r.broadcast(data)
			case conn := <-r.upstreamJoins:
				r.join(conn, r.upstreamClients, true)
			case conn := <-r.relayJoins:
				r.join(conn, r.upstreamClients, false)
			}
		}
	}()
}
