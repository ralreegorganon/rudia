# rudia

rudia is a simple application that will both connect to upstream TCP endpoints 
and accept connections from upstream TCP clients, and relay the messages read
to a second local TCP endpoint which accepts many connections. If an upstream 
connection is severed, the application will attempt to reconnect while maintaining 
all of the other client and upstream connections. 

#### Example
```
Usage of rudia:
  -clientPort="32779": TCP port to listen for relay clients on
  -idle=10: Idle timeout in seconds before a connection is considered dead
  -retry=10: Retry interval in seconds for attempting to reconnect
  -upstreamPort="32799": TCP port to listen for upstreams on

rudia -clientPort 9090 -upstreamPort 9091 someremoteserver:9999 someotherserver:6666
```

