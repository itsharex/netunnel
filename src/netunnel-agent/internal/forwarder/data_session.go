package forwarder

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"netunnel/agent/internal/control"
)

const (
	dataSessionHelloType        = "data_session"
	dataSessionFrameOpen        = "open"
	dataSessionFrameOpenOK      = "open_ok"
	dataSessionFrameOpenFail    = "open_fail"
	dataSessionFrameData        = "data"
	dataSessionFrameClose       = "close"
	dataSessionFramePing        = "ping"
	dataSessionFramePong        = "pong"
	dataSessionDialTimeout      = 10 * time.Second
	dataSessionReadTimeout      = 70 * time.Second
	dataSessionWriteTimeout     = 15 * time.Second
	dataSessionRetryDelay       = 5 * time.Second
	dataSessionSummaryInterval  = 1 * time.Minute
	dataSessionKeepAlivePeriod  = 30 * time.Second
	dataSessionLocalDialTimeout = 10 * time.Second
	dataSessionIOIdleTimeout    = 60 * time.Second
)

type dataSessionFrame struct {
	Type     string `json:"type"`
	StreamID string `json:"stream_id,omitempty"`
	TunnelID string `json:"tunnel_id,omitempty"`
	Error    string `json:"error,omitempty"`
	Payload  []byte `json:"payload,omitempty"`
	SentAt   int64  `json:"sent_at,omitempty"`
}

type DataSessionClient struct {
	bridgeAddr string

	mu                sync.Mutex
	agent             control.Agent
	tunnels           map[string]control.Tunnel
	connectedSessions uint64
	retries           uint64
	openSuccesses     uint64
	openFailures      uint64
	streamCloses      uint64
	activeStreams     int
	remoteCloses      uint64
	localEOFCloses    uint64
	writeFailCloses   uint64
	contextCloses     uint64
	summaryOnce       sync.Once
}

type dataSessionWriter struct {
	rw *bufio.ReadWriter
	mu sync.Mutex
}

func NewDataSessionClient(bridgeAddr string) *DataSessionClient {
	return &DataSessionClient{bridgeAddr: bridgeAddr, tunnels: make(map[string]control.Tunnel)}
}

func (c *DataSessionClient) Update(agent control.Agent, tunnels []control.Tunnel) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.agent = agent
	updated := make(map[string]control.Tunnel)
	for _, tunnel := range tunnels {
		if (tunnel.Type != "tcp" && tunnel.Type != "http_host") || !tunnel.Enabled {
			continue
		}
		updated[tunnel.ID] = tunnel
	}
	c.tunnels = updated
}

func (c *DataSessionClient) Run(ctx context.Context) {
	c.summaryOnce.Do(func() {
		go c.logSummaries(ctx)
	})
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := c.runSession(ctx); err != nil && ctx.Err() == nil {
			c.mu.Lock()
			c.retries++
			c.mu.Unlock()
			log.Printf("data session failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(dataSessionRetryDelay):
		}
	}
}

func (c *DataSessionClient) logSummaries(ctx context.Context) {
	ticker := time.NewTicker(dataSessionSummaryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.logSummary()
		}
	}
}

func (c *DataSessionClient) logSummary() {
	c.mu.Lock()
	defer c.mu.Unlock()
	log.Printf(
		"agent data session summary: active_streams=%d connected=%d retries=%d open_successes=%d open_failures=%d stream_closes=%d remote_closes=%d local_eof_closes=%d write_fail_closes=%d context_closes=%d tunnels=%d",
		c.activeStreams,
		c.connectedSessions,
		c.retries,
		c.openSuccesses,
		c.openFailures,
		c.streamCloses,
		c.remoteCloses,
		c.localEOFCloses,
		c.writeFailCloses,
		c.contextCloses,
		len(c.tunnels),
	)
}

func (c *DataSessionClient) runSession(ctx context.Context) error {
	c.mu.Lock()
	agent := c.agent
	c.mu.Unlock()
	if agent.ID == "" || agent.SecretKey == "" {
		return fmt.Errorf("data session agent not ready")
	}

	dialer := &net.Dialer{Timeout: dataSessionDialTimeout, KeepAlive: dataSessionKeepAlivePeriod}
	conn, err := dialer.DialContext(ctx, "tcp", c.bridgeAddr)
	if err != nil {
		return err
	}
	defer conn.Close()
	configureDataSessionTCPKeepAlive(conn, dataSessionKeepAlivePeriod)
	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	if err := writeDataSessionFrame(rw, map[string]string{
		"type":       dataSessionHelloType,
		"agent_id":   agent.ID,
		"secret_key": agent.SecretKey,
	}); err != nil {
		return err
	}
	log.Printf("data session connected: agent=%s", agent.ID)
	c.mu.Lock()
	c.connectedSessions++
	connected := c.connectedSessions
	retries := c.retries
	c.mu.Unlock()
	log.Printf("agent data session stats: connected=%d retries=%d", connected, retries)

	streams := make(map[string]net.Conn)
	var mu sync.Mutex
	writer := &dataSessionWriter{rw: rw}
	decoder := json.NewDecoder(rw)
	defer func() {
		mu.Lock()
		defer mu.Unlock()
		for streamID, localConn := range streams {
			delete(streams, streamID)
			_ = localConn.Close()
		}
	}()
	for {
		_ = conn.SetReadDeadline(time.Now().Add(dataSessionReadTimeout))
		var frame dataSessionFrame
		if err := decoder.Decode(&frame); err != nil {
			return err
		}
		switch frame.Type {
		case dataSessionFramePing:
			if err := writer.writeFrame(dataSessionFrame{Type: dataSessionFramePong, SentAt: frame.SentAt}); err != nil {
				return err
			}
		case dataSessionFrameOpen:
			c.mu.Lock()
			tunnel, ok := c.tunnels[frame.TunnelID]
			c.mu.Unlock()
			if !ok {
				c.mu.Lock()
				c.openFailures++
				c.mu.Unlock()
				if err := writer.writeFrame(dataSessionFrame{Type: dataSessionFrameOpenFail, StreamID: frame.StreamID, Error: "tunnel unavailable"}); err != nil {
					return err
				}
				continue
			}
			localDialer := &net.Dialer{Timeout: dataSessionLocalDialTimeout, KeepAlive: dataSessionKeepAlivePeriod}
			localConn, err := localDialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", tunnel.LocalHost, tunnel.LocalPort))
			if err != nil {
				c.mu.Lock()
				c.openFailures++
				c.mu.Unlock()
				if err := writer.writeFrame(dataSessionFrame{Type: dataSessionFrameOpenFail, StreamID: frame.StreamID, Error: err.Error()}); err != nil {
					return err
				}
				continue
			}
			configureDataSessionTCPKeepAlive(localConn, dataSessionKeepAlivePeriod)
			mu.Lock()
			streams[frame.StreamID] = localConn
			mu.Unlock()
			c.mu.Lock()
			c.openSuccesses++
			c.activeStreams++
			openSuccesses := c.openSuccesses
			openFailures := c.openFailures
			c.mu.Unlock()
			if err := writer.writeFrame(dataSessionFrame{Type: dataSessionFrameOpenOK, StreamID: frame.StreamID}); err != nil {
				_ = localConn.Close()
				return err
			}
			log.Printf("agent data session stream opened: stream=%s success=%d failures=%d", frame.StreamID, openSuccesses, openFailures)
			go c.pipeLocalToSession(ctx, writer, &mu, streams, frame.StreamID, localConn)
		case dataSessionFrameData:
			mu.Lock()
			localConn := streams[frame.StreamID]
			mu.Unlock()
			if localConn == nil {
				continue
			}
			_ = localConn.SetWriteDeadline(time.Now().Add(dataSessionWriteTimeout))
			if _, err := localConn.Write(frame.Payload); err != nil {
				_ = writer.writeFrame(dataSessionFrame{Type: dataSessionFrameClose, StreamID: frame.StreamID})
				mu.Lock()
				delete(streams, frame.StreamID)
				mu.Unlock()
				_ = localConn.Close()
			}
		case dataSessionFrameClose:
			mu.Lock()
			localConn := streams[frame.StreamID]
			delete(streams, frame.StreamID)
			mu.Unlock()
			if localConn != nil {
				_ = localConn.Close()
				c.mu.Lock()
				c.streamCloses++
				c.remoteCloses++
				if c.activeStreams > 0 {
					c.activeStreams--
				}
				streamCloses := c.streamCloses
				c.mu.Unlock()
				log.Printf("agent data session remote closed stream: stream=%s closes=%d", frame.StreamID, streamCloses)
			}
		}
	}
}

func (c *DataSessionClient) pipeLocalToSession(ctx context.Context, writer *dataSessionWriter, mu *sync.Mutex, streams map[string]net.Conn, streamID string, localConn net.Conn) {
	closeReason := "local_eof"
	defer func() {
		mu.Lock()
		delete(streams, streamID)
		mu.Unlock()
		_ = localConn.Close()
		c.mu.Lock()
		c.streamCloses++
		switch closeReason {
		case "context_cancel":
			c.contextCloses++
		case "write_fail":
			c.writeFailCloses++
		default:
			c.localEOFCloses++
		}
		if c.activeStreams > 0 {
			c.activeStreams--
		}
		streamCloses := c.streamCloses
		c.mu.Unlock()
		_ = writer.writeFrame(dataSessionFrame{Type: dataSessionFrameClose, StreamID: streamID})
		log.Printf("agent data session stream closed: stream=%s closes=%d", streamID, streamCloses)
	}()

	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			closeReason = "context_cancel"
			return
		default:
		}
		_ = localConn.SetReadDeadline(time.Now().Add(dataSessionIOIdleTimeout))
		n, err := localConn.Read(buf)
		if n > 0 {
			payload := append([]byte(nil), buf[:n]...)
			if writeErr := writer.writeFrame(dataSessionFrame{Type: dataSessionFrameData, StreamID: streamID, Payload: payload}); writeErr != nil {
				closeReason = "write_fail"
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func writeDataSessionFrame(rw *bufio.ReadWriter, payload any) error {
	if err := json.NewEncoder(rw).Encode(payload); err != nil {
		return err
	}
	return rw.Flush()
}

func (w *dataSessionWriter) writeFrame(payload any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return writeDataSessionFrame(w.rw, payload)
}

func configureDataSessionTCPKeepAlive(conn net.Conn, period time.Duration) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return
	}
	_ = tcpConn.SetKeepAlive(true)
	_ = tcpConn.SetKeepAlivePeriod(period)
}
