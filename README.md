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
  -proxy=[]: Upstream address to proxy, repeat the flag to proxy multiple
  -retry=10: Retry interval in seconds for attempting to reconnect
  -upstreamListenerIdleTimeout=600: Idle timeout in seconds before an upstream listener connection is considered dead
  -upstreamPort="32799": TCP port to listen for upstreams on
  -upstreamProxyIdleTimeout=10: Idle timeout in seconds before a proxied upstream connection is considered dead

rudia -clientPort 9090 -upstreamPort 9091 -proxy someremoteserver:9999 -proxy someotherserver:6666
```

