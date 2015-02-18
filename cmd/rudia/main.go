package main

import (
	"flag"
	"os"
	"os/signal"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/ralreegorganon/rudia"
)

var clientPort = flag.String("clientPort", "32779", "TCP port to listen for relay clients on")
var upstreamPort = flag.String("upstreamPort", "32799", "TCP port to listen for upstreams on")
var upstreamProxyIdleTimeout = flag.Int("upstreamProxyIdleTimeout", 10, "Idle timeout in seconds before a proxied upstream connection is considered dead")
var upstreamListenerIdleTimeout = flag.Int("upstreamListenerIdleTimeout", 600, "Idle timeout in seconds before an upstream listener connection is considered dead")
var retryInterval = flag.Int("retry", 10, "Retry interval in seconds for attempting to reconnect")

func init() {
	log.SetLevel(log.DebugLevel)
}

func main() {
	flag.Parse()

	ro := &rudia.RepeaterOptions{
		UpstreamProxyIdleTimeout:    time.Duration(*upstreamProxyIdleTimeout) * time.Second,
		UpstreamListenerIdleTimeout: time.Duration(*upstreamListenerIdleTimeout) * time.Second,
		RetryInterval:               time.Duration(*retryInterval) * time.Second,
	}

	r := rudia.NewRepeater(ro)

	for _, arg := range flag.Args() {
		r.Proxy(arg)
	}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	go r.ListenAndAcceptClients(":" + *clientPort)
	go r.ListenAndAcceptUpstreams(":" + *upstreamPort)

	<-interrupt
	r.Shutdown()
}
