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

	mu                      sync.Mutex
	listeners               map[string]net.Listener
	tunnelActiveConnections map[string]int

	activeConnections          int
	totalAccepted              uint64
	dataSessionAcquireFailures uint64
	dataSessionSuccesses       uint64
	dataSessionFailures        uint64
	copyFailures               uint64
	deniedConnections          uint64
	limitRejected              uint64
	idleTimeoutCloses          uint64
	summaryOnce                sync.Once
}

const runtimeSummaryInterval = 1 * time.Minute
const connectionProgressFlushInterval = 15 * time.Second
const tunnelIOIdleTimeout = 60 * time.Second
const maxActiveConnectionsPerTunnel = 32

func NewRuntime(ctx context.Context, bridge *BridgeManager, recorder usageRecorder, authorizer tunnelAuthorizer) *Runtime {
	runtime := &Runtime{
		ctx:                     ctx,
		bridge:                  bridge,
		recorder:                recorder,
		authorizer:              authorizer,
		listeners:               make(map[string]net.Listener),
		tunnelActiveConnections: make(map[string]int),
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
		"tcp runtime summary: listeners=%d active_connections=%d accepted=%d data_session_successes=%d data_session_failures=%d data_session_acquire_failures=%d denied=%d limit_rejected=%d copy_failures=%d idle_timeouts=%d",
		len(r.listeners),
		r.activeConnections,
		r.totalAccepted,
		r.dataSessionSuccesses,
		r.dataSessionFailures,
		r.dataSessionAcquireFailures,
		r.deniedConnections,
		r.limitRejected,
		r.copyFailures,
		r.idleTimeoutCloses,
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
	if !r.tryAcquireTunnelSlot(tunnel.ID) {
		r.mu.Lock()
		r.limitRejected++
		r.mu.Unlock()
		log.Printf("tcp tunnel rejected by active limit: tunnel=%s limit=%d", tunnel.ID, maxActiveConnectionsPerTunnel)
		return
	}
	defer r.releaseTunnelSlot(tunnel.ID)
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

	bridgeConn, err := r.bridge.OpenDataStream(acquireCtx, tunnel.AgentID, tunnel.ID)
	if err != nil {
		r.mu.Lock()
		r.dataSessionFailures++
		r.mu.Unlock()
	} else {
		r.mu.Lock()
		r.dataSessionSuccesses++
		r.mu.Unlock()
	}
	if err != nil {
		r.mu.Lock()
		r.dataSessionAcquireFailures++
		r.mu.Unlock()
		log.Printf("open data stream failed: tunnel=%s err=%v", tunnel.ID, err)
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
	stopClosing := make(chan struct{})

	progressCtx, stopProgress := context.WithCancel(context.Background())
	defer stopProgress()
	if r.recorder != nil && connectionID != "" {
		go r.flushConnectionProgress(progressCtx, connectionID, tunnel, &ingressBytes, &egressBytes)
	}

	go func() {
		select {
		case <-ctx.Done():
			_ = bridgeConn.Close()
			_ = remoteConn.Close()
		case <-stopClosing:
		}
	}()
	go func() {
		written, err := copyConnWithIdleTimeout(bridgeConn, remoteConn, tunnelIOIdleTimeout, &ingressBytes)
		resultCh <- copyResult{bytes: written, err: err}
	}()
	go func() {
		written, err := copyConnWithIdleTimeout(remoteConn, bridgeConn, tunnelIOIdleTimeout, &egressBytes)
		resultCh <- copyResult{bytes: written, err: err}
	}()

	first := <-resultCh
	_ = bridgeConn.Close()
	_ = remoteConn.Close()
	second := <-resultCh
	close(stopClosing)
	stopProgress()
	if isNetTimeout(first.err) || isNetTimeout(second.err) {
		r.mu.Lock()
		r.idleTimeoutCloses++
		r.mu.Unlock()
	}

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

func (r *Runtime) tryAcquireTunnelSlot(tunnelID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	active := r.tunnelActiveConnections[tunnelID]
	if active >= maxActiveConnectionsPerTunnel {
		return false
	}
	r.tunnelActiveConnections[tunnelID] = active + 1
	return true
}

func (r *Runtime) releaseTunnelSlot(tunnelID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	active := r.tunnelActiveConnections[tunnelID] - 1
	if active <= 0 {
		delete(r.tunnelActiveConnections, tunnelID)
		return
	}
	r.tunnelActiveConnections[tunnelID] = active
}

func copyConnWithIdleTimeout(dst net.Conn, src net.Conn, idleTimeout time.Duration, counter *atomic.Int64) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64

	for {
		if idleTimeout > 0 {
			_ = src.SetReadDeadline(time.Now().Add(idleTimeout))
		}
		nr, readErr := src.Read(buf)
		if nr > 0 {
			if idleTimeout > 0 {
				_ = dst.SetWriteDeadline(time.Now().Add(idleTimeout))
			}
			nw, writeErr := dst.Write(buf[:nr])
			written += int64(nw)
			if counter != nil && nw > 0 {
				counter.Add(int64(nw))
			}
			if writeErr != nil {
				return written, writeErr
			}
			if nw != nr {
				return written, io.ErrShortWrite
			}
		}
		if readErr != nil {
			return written, readErr
		}
	}
}

func isNetTimeout(err error) bool {
	if err == nil {
		return false
	}
	netErr, ok := err.(net.Error)
	return ok && netErr.Timeout()
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
