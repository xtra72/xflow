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

// ModbusTransport 는 MODBUS 트랜스포트 레이어 인터페이스이다.
// ADU-중립 설계: 상위 계층은 순수 PDU(function code + payload)만 다루며,
// 각 트랜스포트 구현이 ADU 프레이밍(TCP=MBAP, RTU=CRC)을 담당한다.
// 테스트 가능성을 위해 인터페이스로 분리한다.
type ModbusTransport interface {
	// Connect 는 MODBUS 디바이스에 연결을 수립한다.
	Connect(ctx context.Context) error

	// SendAndReceive 는 unitID 와 순수 PDU 를 받아 트랜스포트별 ADU 프레이밍을
	// 부착해 전송하고, 응답에서 ADU 를 제거한 순수 응답 PDU 를 반환한다.
	// TCP 구현은 MBAP(트랜잭션 ID 관리 + Length + de-framing)를 소유한다.
	SendAndReceive(ctx context.Context, unitID byte, pdu []byte) (respPDU []byte, err error)

	// Close 는 연결을 종료한다.
	Close() error

	// IsConnected 는 현재 연결 상태를 반환한다.
	IsConnected() bool
}

// ModbusTCPTransport 는 net.Conn 기반 MODBUS/TCP 트랜스포트 구현체이다.
// MBAP 프레이밍(트랜잭션 ID 관리 포함)을 소유한다.
type ModbusTCPTransport struct {
	host           string
	port           int
	conn           net.Conn
	mu             sync.Mutex
	connected      bool
	transactionID  uint16 // MBAP 트랜잭션 ID 카운터 (MBAP 관심사이므로 트랜스포트가 소유)
	requestTimeout time.Duration
	logger         *slog.Logger
	obs            *clientObs // 프레임 로그 관측성 배선 (nil 이면 no-op, opt-in, F4)
}

// setObs 는 프레임 로그 관측성 배선을 주입한다(opt-in). nil 이면 프레임 로그는 no-op 이다.
func (t *ModbusTCPTransport) setObs(o *clientObs) {
	t.obs = o
}

// endpointAddr 는 프레임 로그용 대상 엔드포인트 주소 문자열을 반환한다.
func (t *ModbusTCPTransport) endpointAddr() string {
	return fmt.Sprintf("%s:%d", t.host, t.port)
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

// SendAndReceive 는 unitID 와 순수 PDU 를 받아 MBAP ADU 를 부착해 전송하고,
// 응답에서 MBAP 를 제거한 순수 응답 PDU 를 반환한다.
// 트랜잭션 ID 관리·MBAP 프레이밍·de-framing 은 트랜스포트가 소유한다(ADU 관심사).
// MODBUS/TCP 는 순차 통신이므로 mu 로 직렬화한다.
// 응답 읽기 전략: MBAP 헤더(7바이트)를 먼저 읽어 Length 필드(바이트 4-5)를 추출하고,
// 나머지 (Length - 1)바이트(UnitID 는 이미 헤더에 포함)를 읽는다.
func (t *ModbusTCPTransport) SendAndReceive(ctx context.Context, unitID byte, pdu []byte) ([]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.connected || t.conn == nil {
		return nil, ErrConnectionFailed
	}

	// MBAP ADU 프레이밍: 트랜잭션 ID 를 발급하고 MBAP 헤더를 부착한다.
	txID := t.transactionID
	t.transactionID++
	frame := buildMBAPFrame(txID, unitID, pdu)

	// TX 프레임 로그 (log_frames): ADU 프레이밍 직후, 전체 아웃바운드 ADU = MBAP 프레임(A-9).
	if t.obs.framesOn() {
		var reqFC byte
		if len(pdu) > 0 {
			reqFC = pdu[0]
		}
		logFrame(t.logger, true, t.obs.rawOn(), "TX", t.endpointAddr(), unitID, reqFC, frame)
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

	// 응답 PDU(ADU-stripped) 반환: MBAP 헤더는 상위로 노출하지 않는다.
	respPDU := make([]byte, pduSize)
	if pduSize > 0 {
		if _, err := io.ReadFull(t.conn, respPDU); err != nil {
			t.handleConnError()
			return nil, fmt.Errorf("modbus: read PDU failed: %w", err)
		}
	}

	// RX 프레임 로그 (log_frames): 응답 수신 직후, 전체 인바운드 ADU = MBAP 헤더(7) + 응답 PDU(A-9).
	if t.obs.framesOn() {
		rxADU := make([]byte, 0, len(header)+len(respPDU))
		rxADU = append(rxADU, header...)
		rxADU = append(rxADU, respPDU...)
		var respFC byte
		if len(respPDU) > 0 {
			respFC = respPDU[0]
		}
		logFrame(t.logger, true, t.obs.rawOn(), "RX", t.endpointAddr(), unitID, respFC, rxADU)
	}

	return respPDU, nil
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
