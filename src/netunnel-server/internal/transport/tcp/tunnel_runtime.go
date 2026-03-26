package tcp

import (
	"context"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"netunnel/server/internal/domain"
)

type usageRecorder interface {
	StartTunnelConnection(ctx context.Context, params domain.TunnelConnectionStart) (string, error)
	UpdateTunnelConnectionProgress(ctx context.Context, params domain.TunnelConnectionProgress) error
	FinishTunnelConnection(ctx context.Context, params domain.TunnelConnectionFinish) error
}

type tunnelAuthorizer interface {
	AuthorizeTunnelOpen(ctx context.Context, userID string) error
}

type Runtime struct {
	ctx        context.Context
	bridge     *BridgeManager
	recorder   usageRecorder
	authorizer tunnelAuthorizer

	mu        sync.Mutex
	listeners map[string]net.Listener

	activeConnections      int
	totalAccepted          uint64
	bridgeAcquireFailures  uint64
	copyFailures           uint64
	deniedConnections      uint64
	summaryOnce            sync.Once
}

const runtimeSummaryInterval = 1 * time.Minute
const connectionProgressFlushInterval = 15 * time.Second

func NewRuntime(ctx context.Context, bridge *BridgeManager, recorder usageRecorder, authorizer tunnelAuthorizer) *Runtime {
	runtime := &Runtime{
		ctx:        ctx,
		bridge:     bridge,
		recorder:   recorder,
		authorizer: authorizer,
		listeners:  make(map[string]net.Listener),
	}
	runtime.summaryOnce.Do(func() {
		go runtime.logSummaries(ctx)
	})
	return runtime
}

func (r *Runtime) logSummaries(ctx context.Context) {
	ticker := time.NewTicker(runtimeSummaryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.logSummary()
		}
	}
}

func (r *Runtime) logSummary() {
	r.mu.Lock()
	defer r.mu.Unlock()

	log.Printf(
		"tcp runtime summary: listeners=%d active_connections=%d accepted=%d bridge_acquire_failures=%d denied=%d copy_failures=%d",
		len(r.listeners),
		r.activeConnections,
		r.totalAccepted,
		r.bridgeAcquireFailures,
		r.deniedConnections,
		r.copyFailures,
	)
}

func (r *Runtime) changeActiveConnections(delta int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.activeConnections += delta
	if r.activeConnections < 0 {
		r.activeConnections = 0
	}
}

func (r *Runtime) Ensure(ctx context.Context, tunnel domain.Tunnel) error {
	if tunnel.Type != "tcp" || tunnel.RemotePort == nil || !tunnel.Enabled {
		return nil
	}

	r.mu.Lock()
	if _, exists := r.listeners[tunnel.ID]; exists {
		r.mu.Unlock()
		return nil
	}

	ln, err := net.Listen("tcp", ":"+strconv.Itoa(*tunnel.RemotePort))
	if err != nil {
		r.mu.Unlock()
		return err
	}
	r.listeners[tunnel.ID] = ln
	r.mu.Unlock()

	log.Printf("tcp runtime listening: tunnel=%s remote_port=%d", tunnel.ID, *tunnel.RemotePort)
	go r.serveTunnel(r.ctx, tunnel, ln)
	return nil
}

func (r *Runtime) Disable(tunnelID string) error {
	r.mu.Lock()
	ln, exists := r.listeners[tunnelID]
	if exists {
		delete(r.listeners, tunnelID)
	}
	r.mu.Unlock()

	if !exists {
		return nil
	}
	if err := ln.Close(); err != nil {
		return err
	}
	log.Printf("tcp runtime closed: tunnel=%s", tunnelID)
	return nil
}

func (r *Runtime) serveTunnel(ctx context.Context, tunnel domain.Tunnel, ln net.Listener) {
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				log.Printf("accept remote conn failed: tunnel=%s err=%v", tunnel.ID, err)
				return
			}
		}
		go r.handleRemoteConn(ctx, tunnel, conn)
	}
}

func (r *Runtime) handleRemoteConn(ctx context.Context, tunnel domain.Tunnel, remoteConn net.Conn) {
	defer remoteConn.Close()
	r.mu.Lock()
	r.totalAccepted++
	r.mu.Unlock()
	r.changeActiveConnections(1)
	defer r.changeActiveConnections(-1)

	if r.authorizer != nil {
		if err := r.authorizer.AuthorizeTunnelOpen(ctx, tunnel.UserID); err != nil {
			r.mu.Lock()
			r.deniedConnections++
			r.mu.Unlock()
			log.Printf("tcp tunnel denied by billing: tunnel=%s user=%s err=%v", tunnel.ID, tunnel.UserID, err)
			return
		}
	}

	connectionID := ""
	if r.recorder != nil {
		startedID, err := r.recorder.StartTunnelConnection(ctx, domain.TunnelConnectionStart{
			TunnelID:   tunnel.ID,
			AgentID:    tunnel.AgentID,
			Protocol:   "tcp",
			SourceAddr: remoteConn.RemoteAddr().String(),
			TargetAddr: net.JoinHostPort(tunnel.LocalHost, strconv.Itoa(tunnel.LocalPort)),
		})
		if err != nil {
			log.Printf("start tunnel connection failed: tunnel=%s err=%v", tunnel.ID, err)
		} else {
			connectionID = startedID
		}
	}

	acquireCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	bridgeConn, err := r.bridge.Acquire(acquireCtx, tunnel.ID)
	if err != nil {
		r.mu.Lock()
		r.bridgeAcquireFailures++
		r.mu.Unlock()
		log.Printf("acquire bridge failed: tunnel=%s err=%v", tunnel.ID, err)
		return
	}
	defer bridgeConn.Close()

	type copyResult struct {
		bytes int64
		err   error
	}
	var ingressBytes atomic.Int64
	var egressBytes atomic.Int64
	resultCh := make(chan copyResult, 2)

	progressCtx, stopProgress := context.WithCancel(context.Background())
	defer stopProgress()
	if r.recorder != nil && connectionID != "" {
		go r.flushConnectionProgress(progressCtx, connectionID, tunnel, &ingressBytes, &egressBytes)
	}

	go func() {
		written, err := io.Copy(bridgeConn, io.TeeReader(remoteConn, &atomicCounterWriter{counter: &ingressBytes}))
		resultCh <- copyResult{bytes: written, err: err}
	}()
	go func() {
		written, err := io.Copy(remoteConn, io.TeeReader(bridgeConn, &atomicCounterWriter{counter: &egressBytes}))
		resultCh <- copyResult{bytes: written, err: err}
	}()

	first := <-resultCh
	_ = bridgeConn.Close()
	_ = remoteConn.Close()
	second := <-resultCh
	stopProgress()

	if first.err != nil && first.err != io.EOF {
		r.mu.Lock()
		r.copyFailures++
		r.mu.Unlock()
		log.Printf("tcp copy failed: tunnel=%s err=%v", tunnel.ID, first.err)
	}
	if second.err != nil && second.err != io.EOF {
		r.mu.Lock()
		r.copyFailures++
		r.mu.Unlock()
		log.Printf("tcp copy failed: tunnel=%s err=%v", tunnel.ID, second.err)
	}

	if r.recorder != nil && connectionID != "" {
		if err := r.recorder.FinishTunnelConnection(ctx, domain.TunnelConnectionFinish{
			ConnectionID: connectionID,
			UserID:       tunnel.UserID,
			AgentID:      tunnel.AgentID,
			TunnelID:     tunnel.ID,
			IngressBytes: first.bytes,
			EgressBytes:  second.bytes,
			Status:       "closed",
		}); err != nil {
			log.Printf("finish tunnel connection failed: tunnel=%s err=%v", tunnel.ID, err)
		}
	}
}

type atomicCounterWriter struct {
	counter *atomic.Int64
}

func (w *atomicCounterWriter) Write(p []byte) (int, error) {
	w.counter.Add(int64(len(p)))
	return len(p), nil
}

func (r *Runtime) flushConnectionProgress(
	ctx context.Context,
	connectionID string,
	tunnel domain.Tunnel,
	ingressBytes *atomic.Int64,
	egressBytes *atomic.Int64,
) {
	ticker := time.NewTicker(connectionProgressFlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentIngress := ingressBytes.Load()
			currentEgress := egressBytes.Load()
			if currentIngress == 0 && currentEgress == 0 {
				continue
			}

			progressCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := r.recorder.UpdateTunnelConnectionProgress(progressCtx, domain.TunnelConnectionProgress{
				ConnectionID: connectionID,
				UserID:       tunnel.UserID,
				AgentID:      tunnel.AgentID,
				TunnelID:     tunnel.ID,
				IngressBytes: currentIngress,
				EgressBytes:  currentEgress,
				Status:       "open",
			})
			cancel()
			if err != nil {
				log.Printf("update tunnel connection progress failed: tunnel=%s err=%v", tunnel.ID, err)
			}
		}
	}
}
