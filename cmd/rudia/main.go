package main

import (
	"flag"

	"github.com/ralreegorganon/rudia"
)

var relayPort = flag.String("port", "32779", "TCP port to relay to")
var upstream = flag.String("upstream", "localhost:32778", "Upstream address to relay from in host:port format")

func main() {
	flag.Parse()

	r := rudia.NewRepeater()
	r.Proxy(*upstream)
	r.ListenAndAccept(":" + *relayPort)
}
