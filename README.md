# rudia

rudia is a simple application that will connect to one upstream TCP endpoint 
and relay the contents to a second local TCP endpoint which accepts many
connections. If the upstream connection is severed, the application will 
attempt to reconnect while maintaining all of the other client connections.

#### Example
```sh
rudia -port 9090 -upstream someremoteserver:9999
```
