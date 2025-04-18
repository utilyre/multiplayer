package udp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	listenerBufSize    = 100
	numListenerReaders = 5
)

type Envelope struct {
	Sender  net.Addr
	Message Message
}

type Listener struct {
	conn        net.PacketConn
	clients     map[string]struct{} // set of active client addrs
	clientsLock sync.RWMutex
	servers     map[string]struct{} // set of active server addrs
	serversLock sync.RWMutex
	msgc        chan Envelope
}

func Listen(addr string) (*Listener, error) {
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("binding to udp %q: %w", addr, err)
	}

	ln := &Listener{
		msgc:    make(chan Envelope, listenerBufSize),
		conn:    conn,
		clients: map[string]struct{}{},
		servers: map[string]struct{}{},
	}

	for range numListenerReaders {
		go ln.readLoop()
	}

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for ; ; <-ticker.C {
			n := numHandled.Load()
			slog.Debug("read rate", "rate", n)
			numHandled.Store(0)
		}
	}()

	return ln, nil
}

func (ln *Listener) Close(ctx context.Context) error {
	var errs []error

	ln.serversLock.RLock()
	for addr := range ln.servers {
		ln.serversLock.RUnlock()
		udpAddr := must(net.ResolveUDPAddr("udp", addr))
		err := ln.Farewell(ctx, udpAddr)
		if err != nil {
			errs = append(errs, fmt.Errorf("farewelling servers: %w", err))
		}
		ln.serversLock.RLock()
	}
	ln.serversLock.RUnlock()

	err := ln.conn.Close()
	if err != nil {
		errs = append(errs, fmt.Errorf("closing udp %q: %w", ln.LocalAddr(), err))
	}

	close(ln.msgc)

	return errors.Join(errs...)
}

func (ln *Listener) Inbox() <-chan Envelope { return ln.msgc }

func (ln *Listener) LocalAddr() net.Addr { return ln.conn.LocalAddr() }

var (
	ErrAlreadyGreeted = errors.New("already greeted")
	ErrServerNotFound = errors.New("server not found")
)

func (ln *Listener) Greet(ctx context.Context, dest net.Addr) error {
	if ln.serverExists(dest.String()) {
		return ErrAlreadyGreeted
	}

	msg := newMessage(nil, flagHi)
	err := ln.TrySend(ctx, dest, msg) // TODO: make sure it's been received (requires ack)
	if err != nil {
		return err
	}
	ln.serversLock.Lock()
	ln.servers[dest.String()] = struct{}{}
	ln.serversLock.Unlock()
	return nil
}

func (ln *Listener) Farewell(ctx context.Context, dest net.Addr) error {
	if !ln.serverExists(dest.String()) {
		return ErrServerNotFound
	}

	msg := newMessage(nil, flagBye)
	err := ln.TrySend(ctx, dest, msg)
	if err != nil {
		return err
	}
	ln.serversLock.Lock()
	delete(ln.servers, dest.String())
	ln.serversLock.Unlock()
	return nil
}

func (ln *Listener) serverExists(addr string) bool {
	ln.serversLock.RLock()
	defer ln.serversLock.RUnlock()
	_, exists := ln.servers[addr]
	return exists
}

// TODO: add Listener.Send (w/ ack)
// TODO: add Listener.SendAll (sends to all clients w/ ack)

func (ln *Listener) TrySendAll(ctx context.Context, msg Message) error {
	g, ctx := errgroup.WithContext(ctx)
	ln.clientsLock.RLock()
	for addr := range ln.clients {
		ln.clientsLock.RUnlock()
		g.Go(func() error {
			udpAddr := must(net.ResolveUDPAddr("udp", addr))
			return ln.TrySend(ctx, udpAddr, msg)
		})
		ln.clientsLock.RLock()
	}
	ln.clientsLock.RUnlock()
	return g.Wait()
}

func (ln *Listener) TrySend(ctx context.Context, dest net.Addr, msg Message) error {
	data, err := msg.MarshalBinary()
	if err != nil {
		return fmt.Errorf("marshaling message: %w", err)
	}

	// set write deadline based on ctx
	if deadline, ok := ctx.Deadline(); ok {
		err = ln.conn.SetDeadline(deadline)
		if err != nil {
			return fmt.Errorf("setting write deadline: %w", err)
		}
	}
	done := make(chan struct{})
	defer close(done)
	var goroutineErr error
	go func() {
		select {
		case <-ctx.Done():
			goroutineErr = ln.conn.SetWriteDeadline(time.Now())
			<-done // proceed to handling goroutineErr
		case <-done:
		}
	}()

	_, err = ln.conn.WriteTo(data, dest)
	if err != nil {
		return fmt.Errorf("writing message to udp %q: %w", dest, err)
	}

	err = ln.conn.SetWriteDeadline(time.Time{})
	if err != nil {
		return fmt.Errorf("resetting write deadline: %w", err)
	}

	done <- struct{}{} // close(done) does not wait until the goroutine catches up
	if goroutineErr != nil {
		return fmt.Errorf("setting write deadline: %w", err)
	}

	return nil
}

const bufSize = 1024

var numHandled atomic.Uint32

func (ln *Listener) readLoop() {
	buf := make([]byte, bufSize)
	for {
		n, addr, readErr := ln.conn.ReadFrom(buf)
		if errors.Is(readErr, net.ErrClosed) {
			// cannot remove from ln.clients since addr is nil
			slog.Info("connection closed", "address", addr)
			break
		}

		var msg Message
		err := msg.UnmarshalBinary(buf[:n])
		if err != nil {
			slog.Warn("failed to unmarshal message", "error", err)
			continue
		}

		if msg.flags&flagHi != 0 {
			ln.clientsLock.Lock()
			ln.clients[addr.String()] = struct{}{}
			ln.clientsLock.Unlock()
			slog.Info("new client connected", "address", addr)
			continue
		} else if msg.flags&flagBye != 0 {
			ln.clientsLock.Lock()
			delete(ln.clients, addr.String())
			ln.clientsLock.Unlock()
			slog.Info("client disconnected", "address", addr)
			continue
		}

		ln.msgc <- Envelope{
			Sender:  addr,
			Message: msg,
		}
		numHandled.Add(1)

		if readErr != nil {
			slog.Warn("failed to read from udp", "error", err)
			continue
		}
	}
}
