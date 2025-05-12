package game

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	_ "image/png"
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

type snapshot struct {
	s state.State
	t time.Time
}

type Game struct {
	sess *mcp.Session

	inputBuffer     jitter.Buffer
	inputBufferLock sync.Mutex

	state          state.State
	prevSnapshot   snapshot
	nextSnapshot   snapshot
	lastStateIndex uint32
	snapshotLock   sync.Mutex
}

func Start(ctx context.Context, raddr string) (*Game, error) {
	sess, err := mcp.Dial(ctx, raddr, mcp.WithLogger(slog.Default()))
	if err != nil {
		return nil, err
	}

	g := &Game{
		sess:            sess,
		inputBuffer:     jitter.Buffer{},
		inputBufferLock: sync.Mutex{},
		state:           state.State{},
		lastStateIndex:  0,
		prevSnapshot:    snapshot{},
		nextSnapshot:    snapshot{},
		snapshotLock:    sync.Mutex{},
	}
	go g.receiveLoop(context.Background())
	return g, nil
}

func (g *Game) receiveLoop(ctx context.Context) {
	for {
		data, err := g.sess.Receive(ctx)
		if errors.Is(err, mcp.ErrClosed) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			break
		}
		if err != nil {
			slog.Warn("failed to receive message", "error", err)
			continue
		}
		r := bytes.NewReader(data)

		// TODO: this is stupid, come up with an actual protocol
		var typ uint16
		err = binary.Read(r, binary.BigEndian, &typ)
		if err != nil {
			slog.Warn("failed to read message type", "error", err)
		}

		switch typ {
		case 0: // input ack
			var index uint32
			err = binary.Read(r, binary.BigEndian, &index)
			if err != nil {
				slog.Warn("failed to read input acknowledgement index", "error", err)
				continue
			}

			g.inputBufferLock.Lock()
			g.inputBuffer.DiscardUntil(index)
			g.inputBufferLock.Unlock()

		case 1: // state
			var index uint32
			err = binary.Read(r, binary.BigEndian, &index)
			if err != nil {
				slog.Warn("failed to read state index", "error", err)
				continue
			}
			if index <= g.lastStateIndex {
				continue
			}

			var s state.State
			err = s.Decode(r)
			if err != nil {
				slog.Warn("failed to unmarshal state", "error", err)
				continue
			}
			g.snapshotLock.Lock()
			g.prevSnapshot = g.nextSnapshot
			g.nextSnapshot = snapshot{
				s: s,
				t: time.Now(),
			}
			g.snapshotLock.Unlock()
			g.lastStateIndex = index
		}
	}
}

func (g *Game) Close(ctx context.Context) error {
	return g.sess.Close(ctx)
}

func (g *Game) Layout(int, int) (int, int) {
	return state.WorldSize, state.WorldSize
}

func (g *Game) Draw(screen *ebiten.Image) {
	for _, bullet := range g.state.Bullets {
		var m ebiten.GeoM
		m.Scale(2, 2)
		m.Translate(bullet.Trans.X, bullet.Trans.Y)
		screen.DrawImage(assets.Bullet, &ebiten.DrawImageOptions{GeoM: m})
	}

	for _, asteroid := range g.state.Asteroids {
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

	for _, player := range g.state.Players {
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
		fmt.Sprintf("Total Score: %d", g.state.TotalScore),
		&text.GoTextFace{Source: assets.MPlus1pRegular, Size: 60},
		&text.DrawOptions{},
	)
}

func (g *Game) Update() error {
	if g.sess.Closed() {
		return ebiten.Termination
	}

	input := state.Input{
		Left:  ebiten.IsKeyPressed(ebiten.KeyA),
		Down:  ebiten.IsKeyPressed(ebiten.KeyS),
		Up:    ebiten.IsKeyPressed(ebiten.KeyW),
		Right: ebiten.IsKeyPressed(ebiten.KeyD),
		Space: ebiten.IsKeyPressed(ebiten.KeySpace),
	}

	var inputsBuf bytes.Buffer
	g.inputBufferLock.Lock()
	g.inputBuffer.Append(input)
	g.inputBuffer.Encode(&inputsBuf)
	g.inputBufferLock.Unlock()
	_ = g.sess.TrySend(inputsBuf.Bytes())

	g.snapshotLock.Lock()
	if !g.nextSnapshot.t.IsZero() {
		now := time.Now()

		// We'd ideally like to interpolate from the last frame to the next
		// frame, that is in the future. However, what actually happens is that
		// the two frames have already happened and our interpolation is forced
		// to delay from the reality in order to work properly. This is why t
		// is calculated by working out what fraction does $now - next$ have
		// over the duration between the two frames ($next - prev$), instead of
		// working out $now - prev$ over $next - prev$.
		//
		// Ideal:
		//  |-------|______________|
		// prev    now           next
		//
		// Reality:
		//  |______________________|-------|
		// prev                  next     now
		t := now.Sub(g.nextSnapshot.t).Seconds() / g.nextSnapshot.t.Sub(g.prevSnapshot.t).Seconds()

		g.state = g.prevSnapshot.s.Lerp(g.nextSnapshot.s, t)
	}
	g.snapshotLock.Unlock()

	return nil
}
