package forwarder

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"netunnel/agent/internal/control"
)

const bridgeActivateByte = byte(1)

type tunnelWorkerGroup struct {
	agent  control.Agent
	tunnel control.Tunnel

	ctx    context.Context
	cancel context.CancelFunc

	pending int
	idle    int
	active  int

	lastError         string
	lastErrorLoggedAt time.Time
	suppressedErrors  int
}

type BridgeManager struct {
	bridgeAddr string

	mu sync.Mutex

	groups map[string]*tunnelWorkerGroup

	dialAttempts           uint64
	dialFailures           uint64
	helloFailures          uint64
	activationWaitFailures uint64
	localDialFailures      uint64
	bridgeAttachSuccesses  uint64

	summaryOnce sync.Once
}

const repeatedWorkerErrorLogInterval = 10 * time.Second
const (
	initialWorkerRetryDelay = 1 * time.Second
	maxWorkerRetryDelay     = 30 * time.Second
	bridgeSummaryInterval   = 1 * time.Minute
)

func NewBridgeManager(bridgeAddr string) *BridgeManager {
	return &BridgeManager{
		bridgeAddr: bridgeAddr,
		groups:     make(map[string]*tunnelWorkerGroup),
	}
}

func (m *BridgeManager) Sync(ctx context.Context, agent control.Agent, tunnels []control.Tunnel) {
	m.summaryOnce.Do(func() {
		go m.logSummaries(ctx)
	})

	active := make(map[string]control.Tunnel)
	for _, tunnel := range tunnels {
		if (tunnel.Type != "tcp" && tunnel.Type != "http_host") || !tunnel.Enabled {
			continue
		}
		active[tunnel.ID] = tunnel
		m.upsertGroup(ctx, agent, tunnel)
	}

	var stale []*tunnelWorkerGroup

	m.mu.Lock()
	for tunnelID, group := range m.groups {
		if _, ok := active[tunnelID]; ok {
			continue
		}
		stale = append(stale, group)
		delete(m.groups, tunnelID)
	}
	m.mu.Unlock()

	for _, group := range stale {
		group.cancel()
	}
}

func (m *BridgeManager) logSummaries(ctx context.Context) {
	ticker := time.NewTicker(bridgeSummaryInterval)
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

	var totalPending, totalIdle, totalActive, failingGroups int
	for _, group := range m.groups {
		totalPending += group.pending
		totalIdle += group.idle
		totalActive += group.active
		if group.lastError != "" {
			failingGroups++
		}
	}

	log.Printf(
		"agent bridge summary: tunnels=%d pending=%d idle=%d active=%d failing=%d dial_attempts=%d dial_failures=%d hello_failures=%d activation_wait_failures=%d local_dial_failures=%d bridge_attached=%d",
		len(m.groups),
		totalPending,
		totalIdle,
		totalActive,
		failingGroups,
		m.dialAttempts,
		m.dialFailures,
		m.helloFailures,
		m.activationWaitFailures,
		m.localDialFailures,
		m.bridgeAttachSuccesses,
	)
}

func (m *BridgeManager) upsertGroup(ctx context.Context, agent control.Agent, tunnel control.Tunnel) {
	m.mu.Lock()
	group, exists := m.groups[tunnel.ID]
	if !exists {
		groupCtx, cancel := context.WithCancel(ctx)
		group = &tunnelWorkerGroup{
			agent:  agent,
			tunnel: tunnel,
			ctx:    groupCtx,
			cancel: cancel,
		}
		m.groups[tunnel.ID] = group
	} else {
		group.agent = agent
		group.tunnel = tunnel
	}

	shouldSpawn := group.idle+group.pending == 0
	if shouldSpawn {
		group.pending++
	}
	m.mu.Unlock()

	if shouldSpawn {
		m.spawnWorker(tunnel.ID)
	}
}

func (m *BridgeManager) spawnWorker(tunnelID string) {
	m.mu.Lock()
	group, ok := m.groups[tunnelID]
	if !ok {
		m.mu.Unlock()
		return
	}
	agent := group.agent
	tunnel := group.tunnel
	ctx := group.ctx
	m.mu.Unlock()

	go m.runWorker(ctx, agent, tunnel)
}

func (m *BridgeManager) markPending(tunnelID string, delta int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	group, ok := m.groups[tunnelID]
	if !ok {
		return false
	}

	group.pending += delta
	if group.pending < 0 {
		group.pending = 0
	}
	return true
}

func (m *BridgeManager) markIdle(tunnelID string, delta int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	group, ok := m.groups[tunnelID]
	if !ok {
		return false
	}

	group.idle += delta
	if group.idle < 0 {
		group.idle = 0
	}
	return true
}

func (m *BridgeManager) markActive(tunnelID string, delta int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	group, ok := m.groups[tunnelID]
	if !ok {
		return false
	}

	group.active += delta
	if group.active < 0 {
		group.active = 0
	}
	return true
}

func (m *BridgeManager) runWorker(ctx context.Context, agent control.Agent, tunnel control.Tunnel) {
	retryDelay := initialWorkerRetryDelay

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := m.bridgeOnce(ctx, agent, tunnel); err != nil {
			m.logWorkerError(tunnel.ID, err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(retryDelay):
			}
			retryDelay *= 2
			if retryDelay > maxWorkerRetryDelay {
				retryDelay = maxWorkerRetryDelay
			}
		} else {
			m.resetWorkerErrorState(tunnel.ID)
			retryDelay = initialWorkerRetryDelay
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}
}

func (m *BridgeManager) logWorkerError(tunnelID string, err error) {
	now := time.Now()
	errText := err.Error()

	m.mu.Lock()
	group, ok := m.groups[tunnelID]
	if !ok {
		m.mu.Unlock()
		log.Printf("bridge worker failed: tunnel=%s err=%v", tunnelID, err)
		return
	}

	shouldLog := group.lastError != errText || now.Sub(group.lastErrorLoggedAt) >= repeatedWorkerErrorLogInterval
	suppressed := group.suppressedErrors

	if shouldLog {
		group.lastError = errText
		group.lastErrorLoggedAt = now
		group.suppressedErrors = 0
	} else {
		group.suppressedErrors++
	}
	m.mu.Unlock()

	if !shouldLog {
		return
	}

	if suppressed > 0 {
		log.Printf("bridge worker failed: tunnel=%s err=%v suppressed=%d", tunnelID, err, suppressed)
		return
	}

	log.Printf("bridge worker failed: tunnel=%s err=%v", tunnelID, err)
}

func (m *BridgeManager) resetWorkerErrorState(tunnelID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	group, ok := m.groups[tunnelID]
	if !ok {
		return
	}

	group.lastError = ""
	group.lastErrorLoggedAt = time.Time{}
	group.suppressedErrors = 0
}

func (m *BridgeManager) bridgeOnce(ctx context.Context, agent control.Agent, tunnel control.Tunnel) error {
	m.mu.Lock()
	m.dialAttempts++
	m.mu.Unlock()

	bridgeConn, err := net.DialTimeout("tcp", m.bridgeAddr, 10*time.Second)
	if err != nil {
		m.mu.Lock()
		m.dialFailures++
		m.mu.Unlock()
		m.markPending(tunnel.ID, -1)
		return fmt.Errorf("dial bridge: %w", err)
	}
	defer bridgeConn.Close()

	if err := json.NewEncoder(bridgeConn).Encode(map[string]string{
		"tunnel_id":  tunnel.ID,
		"secret_key": agent.SecretKey,
	}); err != nil {
		m.mu.Lock()
		m.helloFailures++
		m.mu.Unlock()
		m.markPending(tunnel.ID, -1)
		return fmt.Errorf("send bridge hello: %w", err)
	}

	m.markPending(tunnel.ID, -1)
	if !m.markIdle(tunnel.ID, 1) {
		return nil
	}
	defer m.markIdle(tunnel.ID, -1)

	activate := []byte{0}
	if _, err := io.ReadFull(bridgeConn, activate); err != nil {
		m.mu.Lock()
		m.activationWaitFailures++
		m.mu.Unlock()
		return fmt.Errorf("wait bridge activation: %w", err)
	}
	if activate[0] != bridgeActivateByte {
		return fmt.Errorf("unexpected bridge activation byte: %d", activate[0])
	}

	m.markIdle(tunnel.ID, -1)
	idleReleased := true
	m.markActive(tunnel.ID, 1)
	defer m.markActive(tunnel.ID, -1)

	// As soon as one bridge is consumed, create the next standby bridge so
	// the same tunnel can serve concurrent connections.
	m.mu.Lock()
	group, ok := m.groups[tunnel.ID]
	shouldSpawn := ok && group.idle+group.pending == 0
	if shouldSpawn {
		group.pending++
	}
	m.mu.Unlock()
	if shouldSpawn {
		m.spawnWorker(tunnel.ID)
	}

	localConn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", tunnel.LocalHost, tunnel.LocalPort), 10*time.Second)
	if err != nil {
		m.mu.Lock()
		m.localDialFailures++
		m.mu.Unlock()
		return fmt.Errorf("dial local target: %w", err)
	}
	defer localConn.Close()

	m.mu.Lock()
	m.bridgeAttachSuccesses++
	m.mu.Unlock()
	log.Printf("bridge attached: tunnel=%s local=%s:%d", tunnel.ID, tunnel.LocalHost, tunnel.LocalPort)

	errCh := make(chan error, 2)
	go func() {
		_, err := io.Copy(localConn, bridgeConn)
		errCh <- err
	}()
	go func() {
		_, err := io.Copy(bridgeConn, localConn)
		errCh <- err
	}()

	firstErr := <-errCh
	_ = bridgeConn.Close()
	_ = localConn.Close()
	secondErr := <-errCh

	if !idleReleased {
		m.markIdle(tunnel.ID, -1)
	}

	if firstErr != nil && firstErr != io.EOF {
		return firstErr
	}
	if secondErr != nil && secondErr != io.EOF {
		return secondErr
	}
	return nil
}
