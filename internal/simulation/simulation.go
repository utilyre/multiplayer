package simulation

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"multiplayer/assets"
	"multiplayer/internal/jitter"
	"multiplayer/internal/mcp"
	"multiplayer/internal/state"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

type Simulation struct {
	ln             *mcp.Listener
	clients        map[string]client
	clientLock     sync.Mutex
	state          state.State
	lastStateIndex uint32

	remoteJoinedAddrCh chan string
	remoteLeftAddrCh   chan string
}

func Start(laddr string) (*Simulation, error) {
	ln, err := mcp.Listen(laddr, mcp.WithLogger(slog.Default()))
	if err != nil {
		return nil, err
	}
	slog.Info("bound udp/mcp listener", "address", ln.LocalAddr())

	sim := &Simulation{
		ln:                 ln,
		clients:            map[string]client{},
		clientLock:         sync.Mutex{},
		state:              state.Init(),
		lastStateIndex:     0,
		remoteJoinedAddrCh: make(chan string, 10),
		remoteLeftAddrCh:   make(chan string, 10),
	}
	go sim.acceptLoop(context.Background())
	return sim, nil
}

type client struct {
	sess   *mcp.Session
	inputc chan state.Input
}

func (c client) receiveLoop(ctx context.Context) {
	logger := slog.With("remote", c.sess.RemoteAddr())

	for {
		data, err := c.sess.Receive(ctx)
		if errors.Is(err, mcp.ErrClosed) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			break
		}
		if err != nil {
			logger.Warn("failed to receive inputs from session", "error", err)
			continue
		}

		var buf jitter.Buffer
		err = buf.Decode(bytes.NewReader(data))
		if err != nil {
			logger.Warn("failed to unmarshal inputs", "error", err)
			continue
		}

		if indices := buf.Indices(); len(indices) > 0 {
			b := make([]byte, 2+4)
			binary.BigEndian.PutUint16(b, 0 /* type = input ack */)
			binary.BigEndian.PutUint32(b[2:], indices[len(indices)-1])
			// i refuse to spawn a new goroutine just to do this
			_ = c.sess.TrySend(b)
		}

		for _, input := range buf.Inputs() {
			select {
			case c.inputc <- input:
			default:
			}
		}
	}
}

func (sim *Simulation) acceptLoop(ctx context.Context) {
	for {
		sess, err := sim.ln.Accept(ctx)
		if errors.Is(err, mcp.ErrClosed) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			break
		}
		if err != nil {
			slog.Warn("failed to accept session", "error", err)
			continue
		}

		c := client{
			sess:   sess,
			inputc: make(chan state.Input, 1),
		}
		raddr := sess.RemoteAddr().String()
		go func() {
			c.receiveLoop(context.Background())

			// The session should not be closed, as the only reason the previous
			// line would return is if the session were closed.

			sim.clientLock.Lock()
			delete(sim.clients, raddr)
			sim.clientLock.Unlock()
			sim.remoteLeftAddrCh <- raddr
		}()

		sim.clientLock.Lock()
		sim.clients[raddr] = c
		sim.clientLock.Unlock()
		sim.remoteJoinedAddrCh <- raddr

		slog.Info("client joined", "raddr", raddr)
	}
}

func (sim *Simulation) Close(ctx context.Context) error {
	return sim.ln.Close(ctx)
}

func (sim *Simulation) Layout(int, int) (int, int) {
	return state.WorldSize, state.WorldSize
}

func (sim *Simulation) Draw(screen *ebiten.Image) {
	for _, bullet := range sim.state.Bullets {
		var m ebiten.GeoM
		m.Scale(2, 2)
		m.Translate(bullet.Trans.X, bullet.Trans.Y)
		screen.DrawImage(assets.Bullet, &ebiten.DrawImageOptions{GeoM: m})
	}

	for _, asteroid := range sim.state.Asteroids {
		var m ebiten.GeoM
		bounds := assets.Rock.Bounds()
		m.Translate(-float64(bounds.Dx()/2), -float64(bounds.Dy()/2))
		m.Rotate(asteroid.Rotation)
		m.Scale(
			state.AsteroidDiam/float64(bounds.Dx()),
			state.AsteroidDiam/float64(bounds.Dy()),
		)
		m.Translate(asteroid.Trans.X, asteroid.Trans.Y)
		screen.DrawImage(assets.Rock, &ebiten.DrawImageOptions{GeoM: m})
	}

	for _, player := range sim.state.Players {
		var m ebiten.GeoM
		bounds := assets.Player.Bounds()
		m.Translate(-float64(bounds.Dx()/2), -float64(bounds.Dy()/2))
		m.Rotate(player.Rotation)
		m.Scale(
			state.PlayerDiam/float64(bounds.Dx()),
			state.PlayerDiam/float64(bounds.Dy()),
		)
		m.Translate(player.Trans.X, player.Trans.Y)
		screen.DrawImage(assets.Player, &ebiten.DrawImageOptions{
			GeoM: m,
		})

		op := &text.DrawOptions{}
		op.GeoM.Translate(player.Trans.X-state.PlayerDiam, player.Trans.Y-state.PlayerDiam)
		text.Draw(screen, fmt.Sprintf("%d", player.ID), &text.GoTextFace{
			Source: assets.MPlus1pRegular,
			Size:   50,
		}, op)
	}

	text.Draw(
		screen,
		fmt.Sprintf("Total Score: %d", sim.state.TotalScore),
		&text.GoTextFace{Source: assets.MPlus1pRegular, Size: 60},
		&text.DrawOptions{},
	)
}

func (sim *Simulation) Update() error {
	dt := time.Second / time.Duration(ebiten.TPS())

	ctx, cancel := context.WithTimeout(context.Background(), dt)
	defer cancel()

ADD_PLAYER_LOOP:
	for {
		select {
		case addr := <-sim.remoteJoinedAddrCh:
			sim.state.AddPlayer(addr)
		default:
			break ADD_PLAYER_LOOP
		}
	}
REMOVE_PLAYER_LOOP:
	for {
		select {
		case addr := <-sim.remoteLeftAddrCh:
			sim.state.RemovePlayer(addr)
		default:
			break REMOVE_PLAYER_LOOP
		}
	}

	inputs := map[string]state.Input{}
	sim.clientLock.Lock()
	for addr, client := range sim.clients {
		select {
		case input := <-client.inputc:
			inputs[addr] = input
		default:
		}
	}
	sim.clientLock.Unlock()

	sim.state.Update(dt, inputs)

	stateBuf := bytes.NewBuffer(make([]byte, 0, 6))
	_ = binary.Write(stateBuf, binary.BigEndian, uint16(1) /* type = state */)
	_ = binary.Write(stateBuf, binary.BigEndian, sim.lastStateIndex)
	sim.state.Encode(stateBuf)
	err := sim.ln.Broadcast(ctx, stateBuf.Bytes())
	if errors.Is(err, mcp.ErrClosed) {
		return ebiten.Termination
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	if err != nil {
		slog.Warn("failed to send state", "error", err)
		return nil
	}
	sim.lastStateIndex++

	return nil
}
