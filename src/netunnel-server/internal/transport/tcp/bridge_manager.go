package tcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"netunnel/server/internal/repository"
)

type BridgeHello struct {
	TunnelID  string `json:"tunnel_id"`
	SecretKey string `json:"secret_key"`
}

const bridgeActivateByte = byte(1)

type BridgeManager struct {
	listenAddr  string
	runtimeRepo *repository.TunnelRuntimeRepository

	mu              sync.Mutex
	queues          map[string][]net.Conn
	waiters         map[string][]chan net.Conn
	lastQueuedLogAt map[string]time.Time

	acceptedConnections     uint64
	handshakeDecodeFailures uint64
	validationFailures      uint64
	deliveredToWaiter       uint64
	queuedConnections       uint64
	staleQueuedClosed       uint64
	acquiredFromQueue       uint64
	acquiredFromWaiter      uint64
	acquireTimeouts         uint64
	activationFailures      uint64

	summaryOnce sync.Once
}

const bridgeManagerSummaryInterval = 1 * time.Minute

func NewBridgeManager(listenAddr string, runtimeRepo *repository.TunnelRuntimeRepository) *BridgeManager {
	return &BridgeManager{
		listenAddr:      listenAddr,
		runtimeRepo:     runtimeRepo,
		queues:          make(map[string][]net.Conn),
		waiters:         make(map[string][]chan net.Conn),
		lastQueuedLogAt: make(map[string]time.Time),
	}
}

func (m *BridgeManager) Start(ctx context.Context) error {
	m.summaryOnce.Do(func() {
		go m.logSummaries(ctx)
	})

	ln, err := net.Listen("tcp", m.listenAddr)
	if err != nil {
		return fmt.Errorf("listen bridge: %w", err)
	}
	log.Printf("tcp bridge listening on %s", m.listenAddr)

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		m.mu.Lock()
		m.acceptedConnections++
		m.mu.Unlock()
		go m.handleConn(ctx, conn)
	}
}

func (m *BridgeManager) logSummaries(ctx context.Context) {
	ticker := time.NewTicker(bridgeManagerSummaryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.logSummary()
		}
	}
}

func (m *BridgeManager) logSummary() {
	m.mu.Lock()
	defer m.mu.Unlock()

	totalQueued := 0
	for _, queued := range m.queues {
		totalQueued += len(queued)
	}

	totalWaiters := 0
	for _, waiters := range m.waiters {
		totalWaiters += len(waiters)
	}

	log.Printf(
		"server bridge summary: queued_tunnels=%d queued_conns=%d waiter_tunnels=%d waiters=%d accepted=%d delivered_to_waiter=%d queued=%d stale_closed=%d acquired_from_queue=%d acquired_from_waiter=%d acquire_timeouts=%d activation_failures=%d handshake_decode_failures=%d validation_failures=%d",
		len(m.queues),
		totalQueued,
		len(m.waiters),
		totalWaiters,
		m.acceptedConnections,
		m.deliveredToWaiter,
		m.queuedConnections,
		m.staleQueuedClosed,
		m.acquiredFromQueue,
		m.acquiredFromWaiter,
		m.acquireTimeouts,
		m.activationFailures,
		m.handshakeDecodeFailures,
		m.validationFailures,
	)
}

func (m *BridgeManager) handleConn(ctx context.Context, conn net.Conn) {
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	var hello BridgeHello
	if err := json.NewDecoder(conn).Decode(&hello); err != nil {
		m.mu.Lock()
		m.handshakeDecodeFailures++
		m.mu.Unlock()
		log.Printf("bridge handshake decode failed: %v", err)
		_ = conn.Close()
		return
	}
	_ = conn.SetDeadline(time.Time{})

	ok, err := m.runtimeRepo.ValidateAgentTunnel(ctx, hello.TunnelID, hello.SecretKey)
	if err != nil || !ok {
		m.mu.Lock()
		m.validationFailures++
		m.mu.Unlock()
		if err != nil {
			log.Printf("bridge validation failed: %v", err)
		}
		_ = conn.Close()
		return
	}

	m.mu.Lock()
	waiters := m.waiters[hello.TunnelID]
	if len(waiters) > 0 {
		waiter := waiters[0]
		m.waiters[hello.TunnelID] = waiters[1:]
		m.deliveredToWaiter++
		m.mu.Unlock()
		waiter <- conn
		close(waiter)
		log.Printf("bridge delivered directly to waiter: tunnel=%s", hello.TunnelID)
		return
	}

	// Agent workers proactively open bridge connections. For protocols like
	// MySQL, old queued sockets can go stale before a remote client acquires
	// them, so keep only the freshest idle bridge per tunnel.
	if queued := m.queues[hello.TunnelID]; len(queued) > 0 {
		for _, staleConn := range queued {
			_ = staleConn.Close()
		}
		m.staleQueuedClosed += uint64(len(queued))
	}
	m.queues[hello.TunnelID] = []net.Conn{conn}
	m.queuedConnections++
	shouldLogQueued := false
	now := time.Now()
	lastLoggedAt := m.lastQueuedLogAt[hello.TunnelID]
	if lastLoggedAt.IsZero() || now.Sub(lastLoggedAt) >= 10*time.Second {
		m.lastQueuedLogAt[hello.TunnelID] = now
		shouldLogQueued = true
	}
	m.mu.Unlock()
	if shouldLogQueued {
		log.Printf("bridge queued connection: tunnel=%s queued=%d", hello.TunnelID, 1)
	}
}

func (m *BridgeManager) Acquire(ctx context.Context, tunnelID string) (net.Conn, error) {
	for {
		m.mu.Lock()
		queue := m.queues[tunnelID]
		if len(queue) > 0 {
			conn := queue[0]
			m.queues[tunnelID] = queue[1:]
			remaining := len(m.queues[tunnelID])
			m.mu.Unlock()
			if err := activateBridgeConn(conn); err != nil {
				m.mu.Lock()
				m.activationFailures++
				m.mu.Unlock()
				log.Printf("bridge activation failed: tunnel=%s err=%v", tunnelID, err)
				_ = conn.Close()
				continue
			}
			m.mu.Lock()
			m.acquiredFromQueue++
			m.mu.Unlock()
			log.Printf("bridge acquired queued connection: tunnel=%s remaining=%d", tunnelID, remaining)
			return conn, nil
		}
		waiter := make(chan net.Conn, 1)
		m.waiters[tunnelID] = append(m.waiters[tunnelID], waiter)
		m.mu.Unlock()

		select {
		case conn := <-waiter:
			if err := activateBridgeConn(conn); err != nil {
				m.mu.Lock()
				m.activationFailures++
				m.mu.Unlock()
				log.Printf("bridge activation failed: tunnel=%s err=%v", tunnelID, err)
				_ = conn.Close()
				continue
			}
			m.mu.Lock()
			m.acquiredFromWaiter++
			m.mu.Unlock()
			log.Printf("bridge acquired from waiter: tunnel=%s", tunnelID)
			return conn, nil
		case <-ctx.Done():
			m.mu.Lock()
			waiters := m.waiters[tunnelID]
			filtered := waiters[:0]
			for _, candidate := range waiters {
				if candidate != waiter {
					filtered = append(filtered, candidate)
				}
			}
			if len(filtered) == 0 {
				delete(m.waiters, tunnelID)
			} else {
				m.waiters[tunnelID] = filtered
			}
			m.acquireTimeouts++
			m.mu.Unlock()
			log.Printf("bridge acquire timeout: tunnel=%s err=%v", tunnelID, ctx.Err())
			return nil, ctx.Err()
		}
	}
}

func activateBridgeConn(conn net.Conn) error {
	_, err := conn.Write([]byte{bridgeActivateByte})
	return err
}
