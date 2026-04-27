package modbus

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"
)

// ModbusTransport 는 MODBUS/TCP 트랜스포트 레이어 인터페이스이다.
// 테스트 가능성을 위해 인터페이스로 분리한다.
type ModbusTransport interface {
	// Connect 는 MODBUS/TCP 디바이스에 TCP 연결을 수립한다.
	Connect(ctx context.Context) error

	// SendAndReceive 는 MBAP 프레임을 전송하고 응답을 수신한다.
	// MBAP 헤더(7바이트)를 먼저 읽어 Length 필드로 나머지 PDU 크기를 결정한다.
	SendAndReceive(ctx context.Context, frame []byte) ([]byte, error)

	// Close 는 TCP 연결을 종료한다.
	Close() error

	// IsConnected 는 현재 연결 상태를 반환한다.
	IsConnected() bool
}

// ModbusTCPTransport 는 net.Conn 기반 MODBUS/TCP 트랜스포트 구현체이다.
type ModbusTCPTransport struct {
	host           string
	port           int
	conn           net.Conn
	mu             sync.Mutex
	connected      bool
	requestTimeout time.Duration
	logger         *slog.Logger
}

// NewModbusTCPTransport 는 새로운 ModbusTCPTransport 를 생성한다.
func NewModbusTCPTransport(host string, port int, requestTimeout time.Duration, logger *slog.Logger) *ModbusTCPTransport {
	return &ModbusTCPTransport{
		host:           host,
		port:           port,
		requestTimeout: requestTimeout,
		logger:         logger,
	}
}

// Connect 는 MODBUS/TCP 디바이스에 TCP 연결을 수립한다.
func (t *ModbusTCPTransport) Connect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.connected {
		return nil
	}

	addr := fmt.Sprintf("%s:%d", t.host, t.port)
	dialer := &net.Dialer{Timeout: t.requestTimeout}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrConnectionFailed, addr, err)
	}

	t.conn = conn
	t.connected = true
	t.logger.Info("modbus: TCP 연결 수립", "addr", addr)

	return nil
}

// SendAndReceive 는 MBAP 프레임을 전송하고 응답을 수신한다.
// MODBUS/TCP 는 순차 통신이므로 mu 로 직렬화한다.
// 응답 읽기 전략: MBAP 헤더(7바이트)를 먼저 읽어 Length 필드(바이트 4-5)를 추출하고,
// 나머지 (Length - 1)바이트(UnitID 는 이미 헤더에 포함)를 읽는다.
func (t *ModbusTCPTransport) SendAndReceive(ctx context.Context, frame []byte) ([]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.connected || t.conn == nil {
		return nil, ErrConnectionFailed
	}

	// 컨텍스트 또는 requestTimeout 으로 데드라인 설정
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(t.requestTimeout)
	}
	if err := t.conn.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("modbus: set deadline failed: %w", err)
	}

	// 요청 프레임 전송
	if _, err := t.conn.Write(frame); err != nil {
		t.handleConnError()
		return nil, fmt.Errorf("modbus: write failed: %w", err)
	}

	// MBAP 헤더 수신 (7바이트)
	header := make([]byte, MBAPHeaderSize)
	if _, err := io.ReadFull(t.conn, header); err != nil {
		t.handleConnError()
		return nil, fmt.Errorf("modbus: read MBAP header failed: %w", err)
	}

	// Length 필드 추출 (바이트 4-5): UnitID(1) + PDU 크기
	length := binary.BigEndian.Uint16(header[4:6])
	if length < 1 {
		return nil, ErrFrameTooShort
	}

	// 나머지 PDU 바이트 수신: Length - 1 (UnitID 는 이미 헤더 바이트 6에 포함)
	pduSize := int(length) - 1
	if pduSize < 0 {
		return nil, ErrFrameTooShort
	}

	pdu := make([]byte, pduSize)
	if pduSize > 0 {
		if _, err := io.ReadFull(t.conn, pdu); err != nil {
			t.handleConnError()
			return nil, fmt.Errorf("modbus: read PDU failed: %w", err)
		}
	}

	// 전체 응답 프레임 = MBAP 헤더 + PDU
	response := make([]byte, MBAPHeaderSize+pduSize)
	copy(response, header)
	copy(response[MBAPHeaderSize:], pdu)

	return response, nil
}

// Close 는 TCP 연결을 종료한다.
func (t *ModbusTCPTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.connected || t.conn == nil {
		return nil
	}

	err := t.conn.Close()
	t.conn = nil
	t.connected = false
	t.logger.Info("modbus: TCP 연결 종료", "host", t.host, "port", t.port)

	return err
}

// IsConnected 는 현재 연결 상태를 반환한다.
func (t *ModbusTCPTransport) IsConnected() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.connected
}

// handleConnError 는 연결 에러 발생 시 연결 상태를 초기화한다.
func (t *ModbusTCPTransport) handleConnError() {
	if t.conn != nil {
		_ = t.conn.Close()
	}
	t.conn = nil
	t.connected = false
}
