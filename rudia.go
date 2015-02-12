// Package rudia implements a simple library for relaying TCP string messages
// from one source to many clients.
package rudia

import (
	"bufio"
	"net"
	"sync"
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
		readMessages: make(chan string, 1),
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

func (u *upstream) shutdown() {
	log.WithFields(log.Fields{
		"upstream": u.conn.RemoteAddr(),
	}).Info("Shutting down upstream")
	u.conn.Close()
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
		messagesToWrite: make(chan string, 1),
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

func (c *client) shutdown() {
	log.WithFields(log.Fields{
		"client": c.conn.RemoteAddr(),
	}).Info("Shutting down client")
	c.conn.Close()
}

// RepeaterOptions provides configuration options for controlling the
// behavior of the Repeater.
type RepeaterOptions struct {
	RetryInterval time.Duration
	IdleTimeout   time.Duration
}

// A Repeater connects to an upstream TCP endpoint and relays the messages
// it receives to all connected clients.
type Repeater struct {
	clientListener          net.Listener
	clientsLock             sync.RWMutex
	clients                 map[string]*client
	clientsDone             chan bool
	clientCleanupComplete   chan bool
	upstreamListener        net.Listener
	upstreamsLock           sync.RWMutex
	upstreams               map[string]*upstream
	upstreamsDone           chan bool
	upstreamCleanupComplete chan bool
	readMessages            chan string
	messagesToWrite         chan string
	options                 *RepeaterOptions
}

// NewRepeater creates a new Repeater and starts listening for client
// connections.
func NewRepeater(options *RepeaterOptions) *Repeater {
	r := &Repeater{
		clients:                 make(map[string]*client),
		upstreams:               make(map[string]*upstream),
		readMessages:            make(chan string, 1),
		messagesToWrite:         make(chan string, 1),
		options:                 options,
		clientsDone:             make(chan bool, 1),
		upstreamsDone:           make(chan bool, 1),
		clientCleanupComplete:   make(chan bool, 1),
		upstreamCleanupComplete: make(chan bool, 1),
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

// ListenAndAcceptClients listens to the specified adddress for new TCP clients
// and upon successful connect, adds them to the pool of clients to relay
// any messages received to.
func (r *Repeater) ListenAndAcceptClients(address string) {
	defer func() {
		r.clientsLock.Lock()
		for k, c := range r.clients {
			c.shutdown()
			delete(r.clients, k)
		}
		r.clientsLock.Unlock()

		r.clientCleanupComplete <- true
	}()

	l, err := net.Listen("tcp", address)

	if err != nil {
		log.WithFields(log.Fields{
			"address": address,
			"err":     err,
		}).Fatal("Unable to listen")
	}
	defer l.Close()
	r.clientListener = l

	log.WithFields(log.Fields{
		"address": r.clientListener.Addr(),
	}).Info("Listening for clients")

	for {
		conn, err := r.clientListener.Accept()
		if err != nil {
			select {
			case <-r.clientsDone:
				return
			default:
				log.WithFields(log.Fields{
					"err":     err,
					"address": r.clientListener.Addr(),
				}).Error("Unable to accept client")
				continue
			}
		}

		r.joinClient(conn)
	}
}

// ListenAndAcceptUpstreams listens to the specified adddress for new TCP
// upstreams and upon successful connect, adds them to the pool of upstreams
// to relay  any messages received from.
func (r *Repeater) ListenAndAcceptUpstreams(address string) {
	defer func() {
		r.upstreamsLock.Lock()
		for k, u := range r.upstreams {
			u.shutdown()
			delete(r.upstreams, k)
		}
		r.upstreamsLock.Unlock()

		r.upstreamCleanupComplete <- true
	}()

	l, err := net.Listen("tcp", address)

	if err != nil {
		log.WithFields(log.Fields{
			"address": address,
			"err":     err,
		}).Fatal("Unable to listen")
	}
	defer l.Close()
	r.upstreamListener = l

	log.WithFields(log.Fields{
		"address": r.upstreamListener.Addr(),
	}).Info("Listening for upstreams")

	fault := make(chan bool)
	for {
		conn, err := r.upstreamListener.Accept()
		if err != nil {
			select {
			case <-r.upstreamsDone:
				return
			default:
				log.WithFields(log.Fields{
					"err":     err,
					"address": r.upstreamListener.Addr(),
				}).Error("Unable to accept upstream")
				continue
			}
		}

		r.joinUpstream(conn, fault)
		go func() { _ = <-fault }()
	}
}

// Shutdown shuts down the repeater, stops taking new connections and
// closing existing connections
func (r *Repeater) Shutdown() {
	log.Info("Shutting down repeater")
	r.clientsDone <- true
	r.upstreamsDone <- true
	r.clientListener.Close()
	r.upstreamListener.Close()
	<-r.clientCleanupComplete
	<-r.upstreamCleanupComplete
}

func (r *Repeater) broadcast(data string) {
	r.clientsLock.RLock()
	defer r.clientsLock.RUnlock()
	for _, c := range r.clients {
		select {
		case c.messagesToWrite <- data:
		default:
		}
	}
}

func (r *Repeater) joinUpstream(conn net.Conn, fault chan bool) {
	log.WithFields(log.Fields{
		"upstream": conn.RemoteAddr(),
	}).Info("Creating upstream")

	u := newUpstream(conn, r.options.IdleTimeout)
	r.upstreamsLock.Lock()
	r.upstreams[conn.RemoteAddr().String()] = u
	r.upstreamsLock.Unlock()
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
				r.upstreamsLock.Lock()
				delete(r.upstreams, deadRemote)
				r.upstreamsLock.Unlock()
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
	r.clientsLock.Lock()
	r.clients[conn.RemoteAddr().String()] = c
	r.clientsLock.Unlock()
	go func() {
		for {
			select {
			case _ = <-c.dead:
				deadRemote := c.conn.RemoteAddr().String()
				r.clientsLock.Lock()
				r.clients[deadRemote].shutdown()
				delete(r.clients, deadRemote)
				r.clientsLock.Unlock()
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
