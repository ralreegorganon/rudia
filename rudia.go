// Package rudia implements a simple library for relaying TCP string messages
// from one source to many clients.
package rudia

import (
	"bufio"
	"net"
	"time"

	log "github.com/Sirupsen/logrus"
)

type upstream struct {
	readMessages chan string
	dead         chan bool
	conn         net.Conn
	reader       *bufio.Reader
	idleTimeout  time.Duration
}

func newUpstream(conn net.Conn, idleTimeout time.Duration) *upstream {
	r := bufio.NewReader(conn)
	u := &upstream{
		readMessages: make(chan string),
		dead:         make(chan bool),
		conn:         conn,
		reader:       r,
		idleTimeout:  idleTimeout,
	}

	go u.read()

	return u
}

func (u *upstream) read() {
	for {
		u.conn.SetReadDeadline(time.Now().Add(u.idleTimeout))
		line, err := u.reader.ReadString('\n')
		if err != nil {
			log.WithFields(log.Fields{
				"err":      err,
				"upstream": u.conn.RemoteAddr(),
			}).Warn("Failed to read from upstream")

			u.dead <- true
			break
		}

		u.readMessages <- line
	}
}

type client struct {
	messagesToWrite chan string
	dead            chan bool
	conn            net.Conn
	writer          *bufio.Writer
	idleTimeout     time.Duration
}

func newClient(conn net.Conn) *client {
	w := bufio.NewWriter(conn)
	c := &client{
		messagesToWrite: make(chan string),
		dead:            make(chan bool),
		conn:            conn,
		writer:          w,
	}

	go c.write()

	return c
}

func (c *client) write() {
	for data := range c.messagesToWrite {
		_, err := c.writer.WriteString(data)
		if err != nil {
			log.WithFields(log.Fields{
				"err":    err,
				"client": c.conn.RemoteAddr(),
			}).Warn("Failed to write to client")
			c.dead <- true
			break
		}

		err = c.writer.Flush()
		if err != nil {
			log.WithFields(log.Fields{
				"err":    err,
				"client": c.conn.RemoteAddr(),
			}).Warn("Failed to flush")
			c.dead <- true
			break
		}
	}
}

type RepeaterOptions struct {
	RetryInterval time.Duration
	IdleTimeout   time.Duration
}

// A Repeater connects to an upstream TCP endpoint and relays the messages
// it receives to all connected clients.
type Repeater struct {
	clients         map[string]*client
	upstreams       map[string]*upstream
	readMessages    chan string
	messagesToWrite chan string
	options         *RepeaterOptions
}

// NewRepeater creates a new Repeater and starts listening for client
// connections.
func NewRepeater(options *RepeaterOptions) *Repeater {
	r := &Repeater{
		clients:         make(map[string]*client),
		upstreams:       make(map[string]*upstream),
		readMessages:    make(chan string),
		messagesToWrite: make(chan string),
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
		fault := make(chan bool)
		for {
			log.WithFields(log.Fields{
				"upstream": address,
			}).Info("Dialing upstream")

			conn, err := net.Dial("tcp", address)
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
			r.joinUpstream(conn, fault)
			_ = <-fault
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

		r.joinClient(conn)
	}
}

func (r *Repeater) broadcast(data string) {
	for _, c := range r.clients {
		c.messagesToWrite <- data
	}
}

func (r *Repeater) joinUpstream(conn net.Conn, fault chan bool) {
	log.WithFields(log.Fields{
		"upstream": conn.RemoteAddr(),
	}).Info("Creating upstream")

	u := newUpstream(conn, r.options.IdleTimeout)
	r.upstreams[conn.RemoteAddr().String()] = u
	go func() {
		for {
			select {
			case message := <-u.readMessages:
				r.readMessages <- message
			case _ = <-u.dead:
				log.WithFields(log.Fields{
					"upstream": u.conn.RemoteAddr(),
				}).Warn("Dead upstream")

				deadRemote := u.conn.RemoteAddr().String()
				delete(r.upstreams, deadRemote)
				fault <- true
			}
		}
	}()
}

func (r *Repeater) joinClient(conn net.Conn) {
	log.WithFields(log.Fields{
		"client": conn.RemoteAddr(),
	}).Info("Creating client")

	c := newClient(conn)
	r.clients[conn.RemoteAddr().String()] = c
	go func() {
		for {
			select {
			case _ = <-c.dead:
				log.WithFields(log.Fields{
					"client": c.conn.RemoteAddr(),
				}).Warn("Dead client")

				deadRemote := c.conn.RemoteAddr().String()
				delete(r.clients, deadRemote)
			}
		}
	}()
}

func (r *Repeater) listen() {
	go func() {
		for {
			select {
			case data := <-r.readMessages:
				r.broadcast(data)
			}
		}
	}()
}
