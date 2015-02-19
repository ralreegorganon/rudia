# rudia

rudia is a simple application that will both connect to upstream TCP listener 
endpoints and accept connections from upstream TCP clients, and then relay
the messages read to a second local TCP listener endpoint which accepts many
connections and any number of client listeners. If connections that were 
explictly made are severed (e.g. to upstream or client listeners), the 
application will attempt to reconnect while maintaining all of the other
client and upstream connections. 

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
