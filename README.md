# rudia

rudia is a simple application that will connect to upstream TCP endpoints 
and relay the contents to a second local TCP endpoint which accepts many
connections. If an upstream connection is severed, the application will 
attempt to reconnect while maintaining all of the other client and upstream
connections.

#### Example
```
Usage of rudia:
  -idle=60: Idle timeout in seconds before a connection is considered dead
  -port="32779": TCP port to relay to
  -retry=60: Retry interval in seconds for attempting to reconnect

rudia -port 9090 someremoteserver:9999 someotherserver:6666
```
