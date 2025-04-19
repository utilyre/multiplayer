package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	_ "multiplayer/internal/config"
	"multiplayer/internal/mcp"
	"os"
	"os/signal"
)

func main() {
	run()
	select {}
}

func run() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	server, err := mcp.Listen(":3000")
	if err != nil {
		slog.Error("failed to start server", "error", err)
		return
	}
	defer func() {
		err = server.Close(ctx)
		if err != nil {
			slog.Error("failed to close server", "error", err)
		}
	}()
	slog.Info("server started", "address", server.LocalAddr())

	client, err := mcp.Dial(ctx, ":3000")
	if err != nil {
		slog.Error("failed to start client", "error", err)
		return
	}
	defer func() {
		err = client.Close(ctx)
		if err != nil {
			slog.Error("failed to close client", "error", err)
		}
	}()
	slog.Info("client dialed server", "address", client.LocalAddr())

	go provider(ctx, client)

	for ctx.Err() == nil {
		sess, err := server.Accept(ctx)
		if errors.Is(err, mcp.ErrClosed) {
			slog.Info("connection closed")
			break
		}
		if err != nil {
			slog.Error("failed to accept session", "error", err)
			continue
		}
		slog.Info("session accepted", "address", sess.RemoteAddr())

		go consumer(ctx, sess)
	}
}

func consumer(ctx context.Context, sess *mcp.Session) {
	// when this is uncommented, everything must be closed properly
	// defer func() {
	// 	err := sess.Close(ctx)
	// 	if err != nil {
	// 		slog.Error("failed to close consumer (server) session", "error", err)
	// 	}
	// }()
	for ctx.Err() == nil {
		data, err := sess.Receive(ctx)
		if err != nil {
			slog.Error("failed to receive data", "error", err)
			continue
		}

		slog.Info("received data",
			"remote", sess.RemoteAddr(), "data", string(data))
	}
}

func provider(ctx context.Context, sess *mcp.Session) {
	for i := range 10 {
		data := []byte(fmt.Sprintf("ping %d", i))
		err := sess.Send(ctx, data)
		if err != nil {
			slog.Error("failed to send data", "data", string(data), "error", err)
		}
		slog.Info("sent data", "remote", sess.RemoteAddr(), "data", string(data))
	}
}
