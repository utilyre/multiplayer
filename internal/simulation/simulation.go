package simulation

import (
	"image"
	"multiplayer/internal/state"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	ticksPerSecond = 60 // ebiten's default
	deltaTickTime  = time.Second / ticksPerSecond
)

type Simulation struct {
	done     <-chan struct{}
	houseImg *ebiten.Image

	// TODO: generalize to multiplayer
	inputBuffer1 <-chan state.Input
	inputBuffer2 <-chan state.Input

	state state.State
}

func New(done <-chan struct{}, houseImg image.Image) *Simulation {
	// bufferred channel for testing
	// TODO: should be a battle tested jitter buffer
	ch1 := make(chan state.Input, 1)
	// this "mocks" inputs coming in from a single client
	go func() {
		defer close(ch1)

		t := time.NewTicker(deltaTickTime)
		defer t.Stop()

		for {
			select {
			case <-done:
				return
			case <-t.C:
			}

			input := state.Input{
				Left:  ebiten.IsKeyPressed(ebiten.KeyH),
				Down:  ebiten.IsKeyPressed(ebiten.KeyJ),
				Up:    ebiten.IsKeyPressed(ebiten.KeyK),
				Right: ebiten.IsKeyPressed(ebiten.KeyL),
			}

			select {
			case ch1 <- input:
			default:
			}
		}
	}()

	// bufferred channel for testing
	// TODO: should be a battle tested jitter buffer
	ch2 := make(chan state.Input, 1)
	// this "mocks" inputs coming in from a single client
	go func() {
		defer close(ch2)

		t := time.NewTicker(deltaTickTime)
		defer t.Stop()

		for {
			select {
			case <-done:
				return
			case <-t.C:
			}

			input := state.Input{
				Left:  ebiten.IsKeyPressed(ebiten.KeyA),
				Down:  ebiten.IsKeyPressed(ebiten.KeyS),
				Up:    ebiten.IsKeyPressed(ebiten.KeyW),
				Right: ebiten.IsKeyPressed(ebiten.KeyD),
			}

			select {
			case ch2 <- input:
			default:
			}
		}
	}()

	return &Simulation{
		done:         done,
		houseImg:     ebiten.NewImageFromImage(houseImg),
		inputBuffer1: ch1,
		inputBuffer2: ch2,
	}
}

func (sim *Simulation) Layout(int, int) (int, int) {
	return 640, 480
}

func (sim *Simulation) Draw(screen *ebiten.Image) {
	var m1 ebiten.GeoM
	m1.Scale(0.2, 0.2)
	m1.Translate(sim.state.House1.Trans.X, sim.state.House1.Trans.Y)
	screen.DrawImage(sim.houseImg, &ebiten.DrawImageOptions{
		GeoM: m1,
	})

	var m2 ebiten.GeoM
	m2.Scale(0.2, 0.2)
	m2.Translate(sim.state.House2.Trans.X, sim.state.House2.Trans.Y)
	screen.DrawImage(sim.houseImg, &ebiten.DrawImageOptions{
		GeoM: m2,
	})
}

func (sim *Simulation) Update() error {
	select {
	case <-sim.done:
		return ebiten.Termination
	default:
	}

	// try to read input of each player
	// if no input for any player then they dont get to play on this frame
	// PERF: use reflect.Select to process the earliest, earlier

	var input1 state.Input
	select {
	case input1 = <-sim.inputBuffer1:
	default:
	}

	var input2 state.Input
	select {
	case input2 = <-sim.inputBuffer2:
	default:
	}

	// sim.state.CreateHouse()

	sim.state.Update(deltaTickTime, 1, input1)
	sim.state.Update(deltaTickTime, 2, input2)

	return nil
}
