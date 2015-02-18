package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/ralreegorganon/rudia"
)

var clientListenerPort = flag.String("clientListenerPort", "32779", "TCP port to listen for relay clients on")
var upstreamListenerPort = flag.String("upstreamListenerPort", "32799", "TCP port to listen for upstreams on")
var upstreamProxyIdleTimeout = flag.Int("upstreamProxyIdleTimeout", 10, "Idle timeout in seconds before a proxied upstream connection is considered dead")
var upstreamListenerIdleTimeout = flag.Int("upstreamListenerIdleTimeout", 600, "Idle timeout in seconds before an upstream listener connection is considered dead")
var retryInterval = flag.Int("retry", 10, "Retry interval in seconds for attempting to reconnect")
var upstreamProxy remotes
var push remotes

func init() {
	log.SetLevel(log.DebugLevel)
	flag.Var(&upstreamProxy, "proxy", "Upstream address to proxy, repeat the flag to proxy multiple")
	flag.Var(&push, "push", "Client address to push to, repeat the flag to push to multiple")
}

func main() {
	flag.Parse()

	ro := &rudia.RepeaterOptions{
		UpstreamProxyIdleTimeout:    time.Duration(*upstreamProxyIdleTimeout) * time.Second,
		UpstreamListenerIdleTimeout: time.Duration(*upstreamListenerIdleTimeout) * time.Second,
		RetryInterval:               time.Duration(*retryInterval) * time.Second,
	}

	r := rudia.NewRepeater(ro)

	for _, u := range upstreamProxy {
		r.Proxy(u)
	}

	for _, c := range push {
		r.Push(c)
	}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	go r.ListenAndAcceptClients(":" + *clientListenerPort)
	go r.ListenAndAcceptUpstreams(":" + *upstreamListenerPort)

	<-interrupt
	r.Shutdown()
}

type remotes []string

func (r *remotes) String() string {
	return fmt.Sprint(*r)
}

func (r *remotes) Set(value string) error {
	*r = append(*r, value)
	return nil
}
