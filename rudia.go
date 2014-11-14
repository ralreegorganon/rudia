// Package rudia implements a simple library for relaying TCP string messages
// from one source to many clients.
package rudia

import (
	"bufio"
	"net"
	"time"

	log "github.com/Sirupsen/logrus"
)

type client struct {
	incoming    chan string
	outgoing    chan string
	dead        chan bool
	isUpstream  bool
	conn        net.Conn
	reader      *bufio.Reader
	writer      *bufio.Writer
	idleTimeout time.Duration
}

func newClient(conn net.Conn, isUpstream bool, idleTimeout time.Duration) *client {
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	c := &client{
		incoming:    make(chan string),
		outgoing:    make(chan string),
		dead:        make(chan bool),
		isUpstream:  isUpstream,
		conn:        conn,
		reader:      r,
		writer:      w,
		idleTimeout: idleTimeout,
	}
	c.listen()
	return c
}

func (c *client) read() {
	for {
		if c.isUpstream {
			c.conn.SetReadDeadline(time.Now().Add(c.idleTimeout))
		}
		line, err := c.reader.ReadString('\n')
		if err != nil {
			log.WithFields(log.Fields{
				"err":    err,
				"client": c.conn.RemoteAddr(),
			}).Warn("Failed to read from client")

			c.dead <- true
			break
		}

		c.incoming <- line
	}
}

func (c *client) write() {
	for data := range c.outgoing {
		_, err := c.writer.WriteString(data)
		if err != nil {
			log.WithFields(log.Fields{
				"err":    err,
				"client": c.conn.RemoteAddr(),
			}).Warn("Failed to write to client")
			continue
		}

		err = c.writer.Flush()
		if err != nil {
			log.WithFields(log.Fields{
				"err":    err,
				"client": c.conn.RemoteAddr(),
			}).Warn("Failed to flush")
			continue
		}
	}
}

func (c *client) listen() {
	go c.read()
	go c.write()
}

type RepeaterOptions struct {
	RetryInterval time.Duration
	IdleTimeout   time.Duration
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
	options         *RepeaterOptions
}

// NewRepeater creates a new Repeater and starts listening for client
// connections.
func NewRepeater(options *RepeaterOptions) *Repeater {
	r := &Repeater{
		relayClients:    make(map[string]*client),
		upstreamClients: make(map[string]*client),
		relayJoins:      make(chan net.Conn),
		upstreamJoins:   make(chan net.Conn),
		incoming:        make(chan string),
		outgoing:        make(chan string),
		upstreamFault:   make(chan bool),
		options:         options,
	}
	r.listen()
	return r
}

// Proxy connects to the specified TCP address and relays all received
// messages to connected clients. If the connection to the specified
// address fails, an attempt will be made to reconnect. This will repeat
// until the program exits.
func (r *Repeater) Proxy(address string) {
	go func() {
		for {
			log.WithFields(log.Fields{
				"upstream": address,
			}).Info("Dialing upstream")

			c, err := net.Dial("tcp", address)
			if err != nil {
				log.WithFields(log.Fields{
					"upstream": address,
					"err":      err,
				}).Error("Error dialing upstream")

				log.WithFields(log.Fields{
					"upstream": address,
					"sleep":    r.options.RetryInterval,
				}).Info("Sleeping before retrying upstream")

				time.Sleep(r.options.RetryInterval)
				continue
			}
			r.upstreamJoins <- c
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
		log.WithFields(log.Fields{
			"address": address,
			"err":     err,
		}).Fatal("Unable to listen")
	}
	defer l.Close()

	log.WithFields(log.Fields{
		"address": l.Addr(),
	}).Info("Listening for clients")

	for {
		conn, err := l.Accept()
		if err != nil {
			log.WithFields(log.Fields{
				"err":     err,
				"address": l.Addr(),
			}).Error("Unable to accept client")
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

func (r *Repeater) join(conn net.Conn, clients map[string]*client, isUpstream bool) {
	log.WithFields(log.Fields{
		"client":     conn.RemoteAddr(),
		"isUpstream": isUpstream,
	}).Info("Creating client")

	c := newClient(conn, isUpstream, r.options.IdleTimeout)
	clients[conn.RemoteAddr().String()] = c
	go func() {
		for {
			select {
			case inc := <-c.incoming:
				if isUpstream {
					r.incoming <- inc
				}
			case _ = <-c.dead:
				log.WithFields(log.Fields{
					"client":     c.conn.RemoteAddr(),
					"isUpstream": isUpstream,
				}).Warn("Dead client")

				deadRemote := c.conn.RemoteAddr().String()
				delete(clients, deadRemote)
				if isUpstream {
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
