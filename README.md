# Multiplayer

```
             _____________________________________________________
            /                       snapshot                      \
udp.Listener -> udp.Mux -> InputQueue -> Simulation -> SnapshotQueue
           \_________/
               ack
```

## Development

1. [Install ebitengine dependencies][ebitengine_install].

2. Run the server:

   ```bash
   go run ./cmd/server
   ```

2. Run the client:

   ```bash
   go run ./cmd/client
   ```

[ebitengine_install]: https://ebitengine.org/en/documents/install
