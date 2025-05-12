package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"multiplayer/internal/cli"
	_ "multiplayer/internal/config"
	"multiplayer/internal/game"
	"multiplayer/internal/mcp"
	"multiplayer/internal/simulation"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	var (
		serverAddr string
		remoteAddr string
	)
	flag.StringVar(&serverAddr, "listen", "", "specify address to listen on")
	flag.StringVar(&remoteAddr, "connect", "", "specify remote address for connecting to a server")
	flag.Parse()

	ctx, cancel := cli.NewSignalContext()
	defer cancel()

	if len(serverAddr) > 0 {
		listenAndSimulate(ctx, serverAddr)
	} else if len(remoteAddr) > 0 {
		connectAndRun(ctx, remoteAddr)
	} else {
		slog.Error("please specify either a -listen flag or a -connect flag")
		os.Exit(1)
	}
}

func listenAndSimulate(ctx context.Context, addr string) {
	sim, err := simulation.Start(addr)
	if err != nil {
		slog.Error("failed to instantiate simulation", "error", err)
		return
	}
	defer func() {
		err = sim.Close(ctx)
		if err != nil {
			slog.Error("failed to close simulation", "error", err)
		}
	}()

	ebiten.SetWindowTitle("Asteroids [SERVER]")
	ebiten.SetTPS(30)
	err = ebiten.RunGame(sim)
	if err != nil {
		slog.Error("failed to run simulation as an ebiten game", "error", err)
		return
	}
}

func connectAndRun(ctx context.Context, raddr string) {
	g, err := game.Start(ctx, raddr)
	if err != nil {
		slog.Error("failed to initialize game", "error", err)
		return
	}
	defer func() {
		err = g.Close(ctx)
		if err != nil && !errors.Is(err, mcp.ErrClosed) {
			slog.Error("failed to close game", "error", err)
		}
	}()

	ebiten.SetWindowTitle("Asteroids")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	err = ebiten.RunGame(g)
	if err != nil {
		slog.Error("failed to run game as an ebiten game", "error", err)
		return
	}
}
