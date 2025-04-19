# Multiplayer

```
             _____________________________________________________
            /                       snapshot                      \
udp.Listener -> udp.Mux -> InputQueue -> Simulation -> SnapshotQueue
           \____________/
                 ack
```

## Goal

1. have multiple input queues for the simulation to consume

```go
for ; ; <-ticker.C {
	for q := range g.inputqueues {
		inputs := append(inputs, <-q)
	}
	input, open := g.inputQueue.Dequeue()
	if !open {
		break
	}

	for input := range inputs {
		g.Update(input)
	}

	g.snapshotQueue <- g.State
}
```

2. why do i even need to be notified when a new client connects??
	 - to make a new queue?
	 - can i make a single struct do all this?

## Development

1. [Install ebitengine dependencies][ebitengine_install].

2. Run the server:

   ```bash
   go run ./cmd/server
   ```

3. Run `n` clients (`n` > 1 is experimental):

   ```bash
   ./spawn_clients.sh <n> # replace <n> with the desired number of clients
   ```

[ebitengine_install]: https://ebitengine.org/en/documents/install

## Resources

- [Networked Physics](https://gafferongames.com/categories/networked-physics)
- [Sneaky Race Conditions and Granular Locks](https://blogtitle.github.io/sneaky-race-conditions-and-granular-locks)
- [Network Programming with Go](https://www.amazon.com/Network-Programming-Go-Adam-Woodbeck/dp/1718500882)
