package main

import (
	"flag"

	log "github.com/Sirupsen/logrus"
	"github.com/ralreegorganon/rudia"
)

var relayPort = flag.String("port", "32779", "TCP port to relay to")

func init() {
	log.SetLevel(log.DebugLevel)
}

func main() {
	flag.Parse()
	r := rudia.NewRepeater()

	for _, arg := range flag.Args() {
		r.Proxy(arg)
	}

	r.ListenAndAccept(":" + *relayPort)
}
