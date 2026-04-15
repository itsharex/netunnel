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

const bridgeKeepAlivePeriod = 30 * time.Second

type BridgeManager struct {
	listenAddr  string
	runtimeRepo *repository.TunnelRuntimeRepository
	dataSession *DataSessionManager

	mu                      sync.Mutex
	acceptedConnections     uint64
	handshakeDecodeFailures uint64
	validationFailures      uint64
	summaryOnce             sync.Once
}

const bridgeManagerSummaryInterval = 1 * time.Minute

func NewBridgeManager(listenAddr string, runtimeRepo *repository.TunnelRuntimeRepository) *BridgeManager {
	return &BridgeManager{
		listenAddr:  listenAddr,
		runtimeRepo: runtimeRepo,
		dataSession: NewDataSessionManager(runtimeRepo),
	}
}

func (m *BridgeManager) Start(ctx context.Context) error {
	m.summaryOnce.Do(func() {
		go m.logSummaries(ctx)
	})
	m.dataSession.StartSummary(ctx)

	ln, err := net.Listen("tcp", m.listenAddr)
	if err != nil {
		return fmt.Errorf("listen data session endpoint: %w", err)
	}
	log.Printf("data session endpoint listening on %s", m.listenAddr)

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

	log.Printf(
		"data session listener summary: accepted=%d handshake_decode_failures=%d validation_failures=%d",
		m.acceptedConnections,
		m.handshakeDecodeFailures,
		m.validationFailures,
	)
}

func (m *BridgeManager) handleConn(ctx context.Context, conn net.Conn) {
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	var raw map[string]string
	if err := json.NewDecoder(conn).Decode(&raw); err != nil {
		m.mu.Lock()
		m.handshakeDecodeFailures++
		m.mu.Unlock()
		log.Printf("data session handshake decode failed: %v", err)
		_ = conn.Close()
		return
	}
	_ = conn.SetDeadline(time.Time{})
	if raw["type"] != dataSessionHelloType {
		m.mu.Lock()
		m.validationFailures++
		m.mu.Unlock()
		log.Printf("unexpected legacy bridge connection rejected")
		_ = conn.Close()
		return
	}

	hello := dataSessionHello{Type: raw["type"], AgentID: raw["agent_id"], SecretKey: raw["secret_key"]}
	configureBridgeTCPKeepAlive(conn, bridgeKeepAlivePeriod)
	if err := m.dataSession.HandleConn(ctx, conn, hello); err != nil {
		log.Printf("data session closed: agent=%s err=%v", hello.AgentID, err)
	}
}

func (m *BridgeManager) OpenDataStream(ctx context.Context, agentID, tunnelID string) (net.Conn, error) {
	return m.dataSession.OpenStream(ctx, agentID, tunnelID)
}

func configureBridgeTCPKeepAlive(conn net.Conn, period time.Duration) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return
	}
	_ = tcpConn.SetKeepAlive(true)
	_ = tcpConn.SetKeepAlivePeriod(period)
}
