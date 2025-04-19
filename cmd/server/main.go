package main

import (
	"context"
	"errors"
	"image"
	_ "image/png"
	_ "multiplayer/internal/config"
	"multiplayer/internal/simulation"
	"os"
	"os/signal"
	"syscall"

	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	ctx, cancel := newSignalContext()
	defer cancel()

	ebiten.SetWindowTitle("Multiplayer - Simulation")
	ebiten.SetWindowSize(640, 480)
	ebiten.SetWindowClosingHandled(true)

	// listener

	houseImg, err := openImage("./assets/house.png")
	if err != nil {
		panic(err)
	}

	// simulation loop
	sim := simulation.New(ctx.Done(), houseImg)
	err = ebiten.RunGame(sim)
	if err != nil {
		panic(err)
	}
}

func newSignalContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	quitCh := make(chan os.Signal, 1)
	signal.Notify(
		quitCh,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGHUP,
		syscall.SIGQUIT,
		syscall.SIGPIPE,
	)

	go func() {
		wasSIGINT := false

		for sig := range quitCh {
			if wasSIGINT && sig == syscall.SIGINT {
				os.Exit(1)
			}

			wasSIGINT = sig == syscall.SIGINT
			cancel()
		}
	}()

	return ctx, cancel
}

func openImage(name string) (img image.Image, err error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()

	img, _, err = image.Decode(f)
	if err != nil {
		return nil, err
	}

	return img, nil
}
