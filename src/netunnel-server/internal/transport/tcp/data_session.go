package tcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"netunnel/server/internal/repository"
)

const (
	dataSessionHelloType      = "data_session"
	dataSessionFrameOpen      = "open"
	dataSessionFrameOpenOK    = "open_ok"
	dataSessionFrameOpenFail  = "open_fail"
	dataSessionFrameData      = "data"
	dataSessionFrameClose     = "close"
	dataSessionFramePing      = "ping"
	dataSessionFramePong      = "pong"
	dataSessionHeartbeatEvery = 5 * time.Second
	dataSessionHeartbeatMiss  = 12
	dataSessionWriteTimeout   = 15 * time.Second
	dataSessionReadTimeout    = 70 * time.Second
)

type dataSessionHello struct {
	Type      string `json:"type"`
	AgentID   string `json:"agent_id"`
	SecretKey string `json:"secret_key"`
}

type dataSessionFrame struct {
	Type     string `json:"type"`
	StreamID string `json:"stream_id,omitempty"`
	TunnelID string `json:"tunnel_id,omitempty"`
	Error    string `json:"error,omitempty"`
	Payload  []byte `json:"payload,omitempty"`
	SentAt   int64  `json:"sent_at,omitempty"`
}

type dataSessionStream struct {
	id       string
	tunnelID string
	session  *DataSession
	readCh   chan []byte
	errCh    chan error
	closed   chan struct{}
	once     sync.Once
	readMu   sync.Mutex
	pending  []byte
}

func newDataSessionStream(id, tunnelID string, session *DataSession) *dataSessionStream {
	return &dataSessionStream{
		id:       id,
		tunnelID: tunnelID,
		session:  session,
		readCh:   make(chan []byte, 16),
		errCh:    make(chan error, 1),
		closed:   make(chan struct{}),
	}
}

func (s *dataSessionStream) Read(p []byte) (int, error) {
	s.readMu.Lock()
	if len(s.pending) > 0 {
		n := copy(p, s.pending)
		s.pending = s.pending[n:]
		s.readMu.Unlock()
		return n, nil
	}
	s.readMu.Unlock()

	select {
	case payload := <-s.readCh:
		if payload == nil {
			return 0, io.EOF
		}
		n := copy(p, payload)
		if n < len(payload) {
			s.readMu.Lock()
			s.pending = append(s.pending[:0], payload[n:]...)
			s.readMu.Unlock()
		}
		return n, nil
	case err := <-s.errCh:
		if err == nil {
			return 0, io.EOF
		}
		return 0, err
	case <-s.closed:
		return 0, io.EOF
	}
}

func (s *dataSessionStream) Write(p []byte) (int, error) {
	if err := s.session.sendFrame(dataSessionFrame{Type: dataSessionFrameData, StreamID: s.id, Payload: append([]byte(nil), p...)}); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *dataSessionStream) Close() error {
	s.once.Do(func() {
		close(s.closed)
		_ = s.session.sendFrame(dataSessionFrame{Type: dataSessionFrameClose, StreamID: s.id})
		s.session.removeStream(s.id)
	})
	return nil
}

func (s *dataSessionStream) LocalAddr() net.Addr  { return s.session.conn.LocalAddr() }
func (s *dataSessionStream) RemoteAddr() net.Addr { return s.session.conn.RemoteAddr() }
func (s *dataSessionStream) SetDeadline(t time.Time) error {
	_ = s.SetReadDeadline(t)
	_ = s.SetWriteDeadline(t)
	return nil
}
func (s *dataSessionStream) SetReadDeadline(time.Time) error  { return nil }
func (s *dataSessionStream) SetWriteDeadline(time.Time) error { return nil }

type DataSession struct {
	agentID string
	conn    net.Conn
	rw      *bufio.ReadWriter
	manager *DataSessionManager

	mu            sync.Mutex
	streams       map[string]*dataSessionStream
	closed        bool
	seq           uint64
	pingMiss      uint32
	lastPong      atomic.Int64
	openedStreams uint64
	closedStreams uint64
}

type DataSessionManager struct {
	runtimeRepo *repository.TunnelRuntimeRepository

	mu                   sync.Mutex
	sessions             map[string]*DataSession
	summaryOnce          sync.Once
	connectedSessions    uint64
	disconnectedSessions uint64
	openSuccesses        uint64
	openFailures         uint64
	openTimeouts         uint64
	streamRemoteCloses   uint64
	streamLocalCloses    uint64
	streamOverflowCloses uint64
	perTunnelActive      map[string]int
}

const dataSessionOpenTimeout = 10 * time.Second

const dataSessionSummaryInterval = 1 * time.Minute

func NewDataSessionManager(runtimeRepo *repository.TunnelRuntimeRepository) *DataSessionManager {
	return &DataSessionManager{runtimeRepo: runtimeRepo, sessions: make(map[string]*DataSession), perTunnelActive: make(map[string]int)}
}

func (m *DataSessionManager) StartSummary(ctx context.Context) {
	m.summaryOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(dataSessionSummaryInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					m.logSummary()
				}
			}
		}()
	})
}

func (m *DataSessionManager) logSummary() {
	m.mu.Lock()
	defer m.mu.Unlock()
	activeStreams := 0
	perAgent := make([]string, 0, len(m.sessions))
	for _, session := range m.sessions {
		session.mu.Lock()
		streamCount := len(session.streams)
		activeStreams += streamCount
		perAgent = append(perAgent, fmt.Sprintf("%s:%d", session.agentID, streamCount))
		session.mu.Unlock()
	}
	log.Printf("data session summary: sessions=%d active_streams=%d", len(m.sessions), activeStreams)
	log.Printf(
		"data session counters: connected=%d disconnected=%d open_successes=%d open_failures=%d open_timeouts=%d remote_closes=%d local_closes=%d overflow_closes=%d",
		m.connectedSessions,
		m.disconnectedSessions,
		m.openSuccesses,
		m.openFailures,
		m.openTimeouts,
		m.streamRemoteCloses,
		m.streamLocalCloses,
		m.streamOverflowCloses,
	)
	if len(perAgent) > 0 {
		log.Printf("data session per-agent streams: %v", perAgent)
	}
	if len(m.perTunnelActive) > 0 {
		perTunnel := make([]string, 0, len(m.perTunnelActive))
		for tunnelID, active := range m.perTunnelActive {
			perTunnel = append(perTunnel, fmt.Sprintf("%s:%d", tunnelID, active))
		}
		log.Printf("data session per-tunnel streams: %v", perTunnel)
	}
}

func (m *DataSessionManager) HandleConn(ctx context.Context, conn net.Conn, hello dataSessionHello) error {
	ok, err := m.runtimeRepo.ValidateAgentSession(ctx, hello.AgentID, hello.SecretKey)
	if err != nil {
		return fmt.Errorf("validate data session: %w", err)
	}
	if !ok {
		return fmt.Errorf("invalid data session credentials")
	}

	session := &DataSession{
		agentID: hello.AgentID,
		conn:    conn,
		rw:      bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn)),
		manager: m,
		streams: make(map[string]*dataSessionStream),
	}
	session.lastPong.Store(time.Now().UnixNano())

	m.mu.Lock()
	if old := m.sessions[hello.AgentID]; old != nil {
		_ = old.Close()
	}
	m.sessions[hello.AgentID] = session
	m.connectedSessions++
	m.mu.Unlock()

	log.Printf("data session connected: agent=%s", hello.AgentID)
	go session.runHeartbeat()
	err = session.readLoop()
	_ = session.Close()
	m.mu.Lock()
	if m.sessions[hello.AgentID] == session {
		delete(m.sessions, hello.AgentID)
	}
	m.disconnectedSessions++
	m.mu.Unlock()
	if err != nil && err != io.EOF {
		return err
	}
	return nil
}

func (m *DataSessionManager) OpenStream(ctx context.Context, agentID, tunnelID string) (net.Conn, error) {
	m.mu.Lock()
	session := m.sessions[agentID]
	m.mu.Unlock()
	if session == nil {
		return nil, fmt.Errorf("no data session for agent %s", agentID)
	}

	streamID := fmt.Sprintf("%s-%d", tunnelID, atomic.AddUint64(&session.seq, 1))
	stream := newDataSessionStream(streamID, tunnelID, session)
	session.addStream(stream)
	atomic.AddUint64(&session.openedStreams, 1)
	m.mu.Lock()
	m.perTunnelActive[tunnelID]++
	m.mu.Unlock()
	if err := session.sendFrame(dataSessionFrame{Type: dataSessionFrameOpen, StreamID: streamID, TunnelID: tunnelID}); err != nil {
		session.removeStream(streamID)
		return nil, err
	}

	openTimer := time.NewTimer(dataSessionOpenTimeout)
	defer openTimer.Stop()
	select {
	case err := <-stream.errCh:
		if err == nil {
			m.mu.Lock()
			m.openSuccesses++
			m.mu.Unlock()
			return stream, nil
		}
		m.mu.Lock()
		m.openFailures++
		m.mu.Unlock()
		m.trackTunnelClose(tunnelID)
		session.removeStream(streamID)
		return nil, err
	case <-openTimer.C:
		m.mu.Lock()
		m.openFailures++
		m.openTimeouts++
		m.mu.Unlock()
		m.trackTunnelClose(tunnelID)
		session.removeStream(streamID)
		return nil, fmt.Errorf("data session open timeout: tunnel=%s", tunnelID)
	case <-ctx.Done():
		m.mu.Lock()
		m.openFailures++
		m.mu.Unlock()
		m.trackTunnelClose(tunnelID)
		session.removeStream(streamID)
		return nil, ctx.Err()
	}
}

func (s *DataSession) runHeartbeat() {
	ticker := time.NewTicker(dataSessionHeartbeatEvery)
	defer ticker.Stop()
	for range ticker.C {
		if time.Since(time.Unix(0, s.lastPong.Load())) > dataSessionHeartbeatEvery*time.Duration(dataSessionHeartbeatMiss) {
			_ = s.Close()
			return
		}
		_ = s.sendFrame(dataSessionFrame{Type: dataSessionFramePing, SentAt: time.Now().UnixNano()})
	}
}

func (s *DataSession) readLoop() error {
	decoder := json.NewDecoder(s.rw)
	for {
		_ = s.conn.SetReadDeadline(time.Now().Add(dataSessionReadTimeout))
		var frame dataSessionFrame
		if err := decoder.Decode(&frame); err != nil {
			return err
		}
		s.handleFrame(frame)
	}
}

func (s *DataSession) handleFrame(frame dataSessionFrame) {
	switch frame.Type {
	case dataSessionFramePong:
		s.lastPong.Store(time.Now().UnixNano())
	case dataSessionFrameOpenOK:
		if stream := s.getStream(frame.StreamID); stream != nil {
			select {
			case stream.errCh <- nil:
			default:
			}
		}
	case dataSessionFrameOpenFail:
		if stream := s.getStream(frame.StreamID); stream != nil {
			select {
			case stream.errCh <- fmt.Errorf(frame.Error):
			default:
			}
		}
	case dataSessionFrameData:
		if stream := s.getStream(frame.StreamID); stream != nil {
			select {
			case stream.readCh <- frame.Payload:
			default:
				if sessionManager := s.manager; sessionManager != nil {
					sessionManager.mu.Lock()
					sessionManager.streamOverflowCloses++
					sessionManager.mu.Unlock()
				}
				_ = stream.Close()
			}
		}
	case dataSessionFrameClose:
		if stream := s.getStream(frame.StreamID); stream != nil {
			sessionManager := s.manager
			if sessionManager != nil {
				sessionManager.mu.Lock()
				sessionManager.streamRemoteCloses++
				sessionManager.mu.Unlock()
				sessionManager.trackTunnelClose(stream.tunnelID)
			}
			stream.closeWithError(io.EOF)
			s.removeStream(frame.StreamID)
		}
	}
}

func (s *DataSession) sendFrame(frame dataSessionFrame) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return io.EOF
	}
	_ = s.conn.SetWriteDeadline(time.Now().Add(dataSessionWriteTimeout))
	if err := json.NewEncoder(s.rw).Encode(frame); err != nil {
		return err
	}
	return s.rw.Flush()
}

func (s *DataSession) addStream(stream *dataSessionStream) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streams[stream.id] = stream
}

func (s *DataSession) getStream(streamID string) *dataSessionStream {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.streams[streamID]
}

func (s *DataSession) removeStream(streamID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.streams[streamID]; ok {
		delete(s.streams, streamID)
		atomic.AddUint64(&s.closedStreams, 1)
	}
}

func (s *DataSession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	streams := make([]*dataSessionStream, 0, len(s.streams))
	for _, stream := range s.streams {
		streams = append(streams, stream)
	}
	s.streams = make(map[string]*dataSessionStream)
	s.mu.Unlock()
	for _, stream := range streams {
		if s.manager != nil {
			s.manager.mu.Lock()
			s.manager.streamLocalCloses++
			s.manager.mu.Unlock()
			s.manager.trackTunnelClose(stream.tunnelID)
		}
		stream.closeWithError(io.EOF)
	}
	return s.conn.Close()
}

func (m *DataSessionManager) trackTunnelClose(tunnelID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	active := m.perTunnelActive[tunnelID] - 1
	if active <= 0 {
		delete(m.perTunnelActive, tunnelID)
		return
	}
	m.perTunnelActive[tunnelID] = active
}

func (s *dataSessionStream) closeWithError(err error) {
	s.once.Do(func() {
		close(s.closed)
		select {
		case s.errCh <- err:
		default:
		}
	})
}
