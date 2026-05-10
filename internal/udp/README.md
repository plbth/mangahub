# B2 - UDP Notification Server

This package is intentionally small and independent so it can be tested without the database or TCP server.

## What it does
- Listens on UDP port `9091`
- Accepts subscription packets
- Stores subscribed client addresses in memory
- Broadcasts notifications to all subscribed clients using `WriteToUDP`

## Supported packets
- `SUBSCRIBE`
- `UNSUBSCRIBE`
- `LIST`
- `PING`
- `PUBLISH <message>`
- `NOTIFY <manga_id>|<message>`

## Quick test with netcat
```bash
echo -n "SUBSCRIBE" | nc -u -w1 localhost 9091
echo -n "PING" | nc -u -w1 localhost 9091
echo -n "NOTIFY one-piece|New chapter out now!" | nc -u -w1 localhost 9091
```

## Integration example
```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

srv := udp.NewServer(":9091")
if err := srv.Start(ctx); err != nil {
    log.Fatal(err)
}
```
