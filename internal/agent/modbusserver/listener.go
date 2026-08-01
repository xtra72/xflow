package modbusserver

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// serverListener interface
// ---------------------------------------------------------------------------

// serverListener is the common interface implemented by both the TCP Listener
// and the RTU serial listener. It lets ModbusServerAgent switch transports
// (tcp/rtu) without changing its lifecycle wiring.
type serverListener interface {
	Start(ctx context.Context) error
	Stop() error
	ActiveConnections() int32
	Addr() net.Addr
}

// Compile-time interface check.
var _ serverListener = (*Listener)(nil)

// ---------------------------------------------------------------------------
// Listener
// ---------------------------------------------------------------------------

// Listener manages a TCP listener for MODBUS/TCP connections with connection
// limiting, idle timeout, and graceful shutdown.
type Listener struct {
	addr        string
	maxConns    int
	idleTimeout time.Duration
	handler     ConnectionHandler
	logger      *slog.Logger

	listener    net.Listener
	activeConns atomic.Int32
	wg          sync.WaitGroup
	mu          sync.Mutex
	running     atomic.Bool
}

// NewListener creates a new Listener.
func NewListener(addr string, maxConns int, idleTimeout time.Duration, handler ConnectionHandler, logger *slog.Logger) *Listener {
	return &Listener{
		addr:        addr,
		maxConns:    maxConns,
		idleTimeout: idleTimeout,
		handler:     handler,
		logger:      logger,
	}
}

// Start begins listening for TCP connections and starts the accept loop.
// Returns ErrServerAlreadyRunning if the listener is already active.
func (l *Listener) Start(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.running.Load() {
		return ErrServerAlreadyRunning
	}

	ln, err := net.Listen("tcp", l.addr)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrListenFailed, err)
	}

	l.listener = ln
	l.running.Store(true)

	l.logInfo("listener started", "addr", ln.Addr().String())

	// Start accept loop in goroutine
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		l.acceptLoop(ctx)
	}()

	return nil
}

// Stop gracefully shuts down the listener and waits for all active connections
// to complete.
func (l *Listener) Stop() error {
	l.mu.Lock()
	if l.listener != nil {
		l.listener.Close()
	}
	l.running.Store(false)
	l.mu.Unlock()

	// Wait for accept loop and all connection handlers to finish
	// (must be outside the mutex to avoid deadlock with acceptLoop)
	l.wg.Wait()

	l.logInfo("listener stopped")
	return nil
}

// ActiveConnections returns the current number of active connections.
func (l *Listener) ActiveConnections() int32 {
	return l.activeConns.Load()
}

// Addr returns the actual listening address. Returns nil if the listener is not started.
func (l *Listener) Addr() net.Addr {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.listener != nil {
		return l.listener.Addr()
	}
	return nil
}

// acceptLoop continuously accepts new TCP connections until the context is
// cancelled or the listener is closed.
func (l *Listener) acceptLoop(ctx context.Context) {
	for {
		conn, err := l.listener.Accept()
		if err != nil {
			// Check if context was cancelled or listener was closed
			select {
			case <-ctx.Done():
				return
			default:
			}

			// If the listener was closed (during Stop), exit gracefully
			if !l.running.Load() {
				return
			}

			l.logWarn("accept failed", "error", err)
			continue
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			conn.Close()
			return
		default:
		}

		// Check connection limit
		if int(l.activeConns.Load()) >= l.maxConns {
			l.logWarn("max connections reached, rejecting connection",
				"remote", conn.RemoteAddr().String(),
				"max", l.maxConns)
			conn.Close()
			continue
		}

		// Accept connection
		l.activeConns.Add(1)
		l.wg.Add(1)
		go l.handleConn(ctx, conn)
	}
}

// handleConn manages a single connection's lifecycle.
func (l *Listener) handleConn(ctx context.Context, conn net.Conn) {
	defer l.wg.Done()
	defer l.activeConns.Add(-1)
	defer conn.Close()

	if l.idleTimeout > 0 {
		conn.SetReadDeadline(time.Now().Add(l.idleTimeout))
	}

	// Monitor context cancellation to unblock any pending reads
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()

	l.handler.HandleConnection(ctx, conn)
	close(done)
}

// logInfo logs an info message if a logger is available.
func (l *Listener) logInfo(msg string, args ...any) {
	if l.logger != nil {
		l.logger.Info(fmt.Sprintf("modbus-listener: %s", msg), args...)
	}
}

// logWarn logs a warning message if a logger is available.
func (l *Listener) logWarn(msg string, args ...any) {
	if l.logger != nil {
		l.logger.Warn(fmt.Sprintf("modbus-listener: %s", msg), args...)
	}
}
