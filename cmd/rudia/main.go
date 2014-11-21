package main

import (
	"flag"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/ralreegorganon/rudia"
)

var relayPort = flag.String("port", "32779", "TCP port to relay to")
var idleTimeout = flag.Int("idle", 10, "Idle timeout in seconds before a connection is considered dead")
var retryInterval = flag.Int("retry", 10, "Retry interval in seconds for attempting to reconnect")

func init() {
	log.SetLevel(log.DebugLevel)
}

func main() {
	flag.Parse()

	ro := &rudia.RepeaterOptions{
		IdleTimeout:   time.Duration(*idleTimeout) * time.Second,
		RetryInterval: time.Duration(*retryInterval) * time.Second,
	}

	r := rudia.NewRepeater(ro)

	for _, arg := range flag.Args() {
		r.Proxy(arg)
	}

	r.ListenAndAccept(":" + *relayPort)
}
