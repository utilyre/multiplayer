package main

import (
	"math"
	"multiplayer/internal/types"
	"time"
)

type Simulation struct {
	types.State
	inputQueue    *InputQueue
	snapshotQueue chan types.State
}

func NewSimulation(inputQueue *InputQueue) *Simulation {
	return &Simulation{
		inputQueue:    inputQueue,
		snapshotQueue: make(chan types.State, 1),
	}
}

func (g *Simulation) Close() {
	close(g.snapshotQueue)
}

func (g *Simulation) Run() {
	const fps = 60
	ticker := time.NewTicker(time.Second / fps)
	defer ticker.Stop()

	for ; ; <-ticker.C {
		input, open := g.inputQueue.Dequeue()
		if !open {
			break
		}

		g.Update(input)

		g.snapshotQueue <- g.State
	}
}

func (g *Simulation) SnapshotQueue() <-chan types.State {
	return g.snapshotQueue
}

func (g *Simulation) Update(input types.Input) {
	dx := 0.0
	dy := 0.0
	if input.Up {
		dy -= 1
	}
	if input.Left {
		dx -= 1
	}
	if input.Down {
		dy += 1
	}
	if input.Right {
		dx += 1
	}

	magnitude := math.Sqrt(dx*dx + dy*dy)
	if magnitude > 0 {
		dx /= magnitude
		dy /= magnitude
	}

	g.Position.X += dx
	g.Position.Y += dy
}
