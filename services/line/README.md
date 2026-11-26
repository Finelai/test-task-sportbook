# Line service

The local stand uses the fixed player ID `player-1`. The HTTP offer endpoint
issues offers for that player. Use the same ID in gRPC confirmation requests.

The `line-check` Compose profile exposes gRPC at `127.0.0.1:9092` for manual
checks. The gRPC listener uses port 9092 inside the Compose network.

`LINE_GENERATION_INTERVAL` sets the interval between source updates as a Go
duration. The default is `5s`. Tests may set a longer interval to control
source data; source freshness remains 15 seconds.
