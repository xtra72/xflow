package modbus

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"go.bug.st/serial"
)

// ---------------------------------------------------------------------------
// Modbus RTU 시리얼 동기식 마스터 (REQ-MODBUS-006-01, M3)
// ---------------------------------------------------------------------------
//
// ModbusRTUTransport 는 go.bug.st/serial 위에서 반이중(half-duplex) 동기식
// 시리얼 마스터로 동작하며 ModbusTransport 인터페이스를 만족한다. ADU 프레이밍은
// M2 의 buildRTUADU/parseRTUADU(rtu_adu.go) + modbusCRC16(rtu_crc.go)을 배선한다.
//
// 반이중 특성상 한 시점에 하나의 요청/응답 트랜잭션만 수행한다(A-4). 이를 위해
// turnaround mutex(mu)로 SendAndReceive 전체를 직렬화한다 — ModbusTCPTransport 의
// 라이프사이클/연결-에러 처리를 그대로 미러링한다.
//
// 테스트 가능성: 바이트 스트림은 io.ReadWriteCloser seam 뒤로 추상화되어,
// go.bug.st/serial.Port 와 mock 이 모두 이를 만족한다(AC-01/AC-07 은 하드웨어 없이
// mock 시리얼로 검증한다). 시리얼 오픈은 opener 함수 seam 으로 대체 가능하다.

// 컴파일 타임 인터페이스 체크
var _ ModbusTransport = (*ModbusRTUTransport)(nil)

// rtuCharBits 는 한 문자(char)당 비트 수이다: start(1)+data(8)+parity(1)+stop(1).
// T3.5 정적(silence) 산출에 사용한다(A-5).
const rtuCharBits = 11

// rtuSerialOpener 는 시리얼 포트를 여는 함수 seam 이다(테스트 훅).
// 운영 코드는 defaultRTUSerialOpener 를 사용한다.
type rtuSerialOpener func(cfg SerialConfig, readTimeout time.Duration) (io.ReadWriteCloser, error)

// readTimeoutSetter 는 요청별 read timeout 설정을 지원하는 선택 인터페이스이다.
// go.bug.st/serial.Port 는 이를 만족하며, mock 은 만족하지 않아도 무방하다.
type readTimeoutSetter interface {
	SetReadTimeout(timeout time.Duration) error
}

// ModbusRTUTransport 는 시리얼 기반 MODBUS RTU 트랜스포트 구현체이다.
// CRC-16 프레이밍(rtu_adu.go)을 소유하며, 반이중 turnaround 를 mu 로 직렬화한다.
type ModbusRTUTransport struct {
	serial         SerialConfig
	opener         rtuSerialOpener
	port           io.ReadWriteCloser
	mu             sync.Mutex // turnaround mutex: 한 시점에 하나의 요청/응답 트랜잭션(A-4)
	connected      bool
	requestTimeout time.Duration
	silence        time.Duration // T3.5 프레임 간 정적(A-5), baud 로부터 산출
	lastFrameAt    time.Time     // 직전 프레임 완료 시각(정적 확보 기준)
	logger         *slog.Logger
	obs            *clientObs // 프레임 로그 관측성 배선 (nil 이면 no-op, opt-in, F4)
}

// setObs 는 프레임 로그 관측성 배선을 주입한다(opt-in). nil 이면 프레임 로그는 no-op 이다.
func (t *ModbusRTUTransport) setObs(o *clientObs) {
	t.obs = o
}

// NewModbusRTUTransport 는 새로운 ModbusRTUTransport 를 생성한다.
// silence 는 SerialConfig.BaudRate 로부터 자동 산출된다(A-5).
func NewModbusRTUTransport(cfg SerialConfig, requestTimeout time.Duration, logger *slog.Logger) *ModbusRTUTransport {
	return &ModbusRTUTransport{
		serial:         cfg,
		opener:         defaultRTUSerialOpener,
		requestTimeout: requestTimeout,
		silence:        rtuInterFrameDelay(cfg.BaudRate),
		logger:         logger,
	}
}

// Connect 는 설정된 파라미터로 시리얼 포트를 연다. 이미 열려 있으면 no-op 이다.
func (t *ModbusRTUTransport) Connect(_ context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.connected {
		return nil
	}

	port, err := t.opener(t.serial, t.requestTimeout)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrSerialConnectionFailed, t.serial.Port, err)
	}

	t.port = port
	t.connected = true
	t.logger.Info("modbus: RTU 시리얼 연결 수립", "port", t.serial.Port, "baud", t.serial.BaudRate)

	return nil
}

// SendAndReceive 는 unitID 와 순수 PDU 를 받아 RTU ADU(CRC 프레이밍)를 부착해 전송하고,
// 응답 ADU 의 CRC 를 재계산·검증한 뒤 순수 응답 PDU 를 반환한다.
// turnaround mutex 하에서 전체 트랜잭션을 직렬화한다(반이중, A-4).
// 절차: (1) T3.5 정적 확보 → (2) ADU 전송 → (3) 응답 ADU 수신 → (4) CRC/unitID/FC 검증.
// CRC 불일치·손상 프레임은 오류로 반환하며 유효 결과로 상위에 넘기지 않는다(AC-07).
func (t *ModbusRTUTransport) SendAndReceive(ctx context.Context, unitID byte, pdu []byte) ([]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.connected || t.port == nil {
		return nil, ErrSerialConnectionFailed
	}
	if len(pdu) < 1 {
		return nil, ErrFrameTooShort
	}
	requestFC := pdu[0]

	// (1) 직전 프레임 이후 T3.5 이상의 정적 확보(A-5)
	t.enforceInterFrameSilence()

	// read timeout 을 ctx 데드라인 또는 requestTimeout 으로 설정
	t.applyReadTimeout(ctx)

	// (2) RTU ADU 프레이밍 후 시리얼 포트로 기록
	adu := buildRTUADU(unitID, pdu)

	// TX 프레임 로그 (log_frames): ADU 프레이밍 직후, 전체 아웃바운드 RTU 요청 ADU(A-9).
	if t.obs.framesOn() {
		logFrame(t.logger, true, t.obs.rawOn(), "TX", t.serial.Port, unitID, requestFC, adu)
	}

	if _, err := t.port.Write(adu); err != nil {
		t.handleConnError()
		return nil, fmt.Errorf("modbus: RTU write failed: %w", err)
	}

	// (3) 응답 ADU 수신: RTU 는 길이 헤더가 없으므로 요청 FC 로 프레임 길이를 결정한다.
	respADU, err := readRTUResponse(t.port, requestFC)
	if err != nil {
		t.handleConnError()
		return nil, fmt.Errorf("modbus: RTU read failed: %w", err)
	}

	// RX 프레임 로그 (log_frames): 응답 수신 직후, 전체 인바운드 RTU 응답 ADU(A-9).
	// respADU = [unitID][respFC]... 이므로 응답 FC 는 두 번째 바이트에서 읽는다.
	if t.obs.framesOn() {
		var respFC byte
		if len(respADU) >= 2 {
			respFC = respADU[1]
		}
		logFrame(t.logger, true, t.obs.rawOn(), "RX", t.serial.Port, unitID, respFC, respADU)
	}

	// 다음 프레임 정적 확보를 위한 기준 시각 갱신
	t.lastFrameAt = time.Now()

	// (4) CRC 재검증 + unitID/FC 일치 확인 후 순수 응답 PDU 반환(AC-01/AC-07).
	// CRC 불일치는 연결 오류가 아니므로 연결을 초기화하지 않고 오류만 반환한다.
	respPDU, err := parseRTUADU(respADU, unitID, requestFC)
	if err != nil {
		return nil, err
	}
	return respPDU, nil
}

// Close 는 시리얼 포트를 닫는다.
func (t *ModbusRTUTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.connected || t.port == nil {
		return nil
	}

	err := t.port.Close()
	t.port = nil
	t.connected = false
	t.logger.Info("modbus: RTU 시리얼 연결 종료", "port", t.serial.Port)

	return err
}

// IsConnected 는 현재 연결 상태를 반환한다.
func (t *ModbusRTUTransport) IsConnected() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.connected
}

// handleConnError 는 시리얼 I/O 오류 발생 시 연결 상태를 초기화한다.
func (t *ModbusRTUTransport) handleConnError() {
	if t.port != nil {
		_ = t.port.Close()
	}
	t.port = nil
	t.connected = false
}

// enforceInterFrameSilence 는 직전 프레임 이후 T3.5 이상의 정적을 확보한다(A-5).
// 첫 프레임(lastFrameAt zero)은 대기하지 않는다.
func (t *ModbusRTUTransport) enforceInterFrameSilence() {
	if t.lastFrameAt.IsZero() {
		return
	}
	if elapsed := time.Since(t.lastFrameAt); elapsed < t.silence {
		time.Sleep(t.silence - elapsed)
	}
}

// applyReadTimeout 은 포트가 readTimeoutSetter 를 만족하면 요청별 read timeout 을
// ctx 데드라인(있으면) 또는 requestTimeout 으로 설정한다. mock 포트는 이를
// 만족하지 않아도 되며, 그 경우 아무 동작도 하지 않는다.
func (t *ModbusRTUTransport) applyReadTimeout(ctx context.Context) {
	setter, ok := t.port.(readTimeoutSetter)
	if !ok {
		return
	}
	timeout := t.requestTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if d := time.Until(deadline); d > 0 {
			timeout = d
		}
	}
	_ = setter.SetReadTimeout(timeout)
}

// ---------------------------------------------------------------------------
// RTU 응답 프레임 리더 (길이 헤더 없음 — 요청 FC 로 결정)
// ---------------------------------------------------------------------------

// readRTUResponse 는 요청 function code 로부터 RTU 응답 프레임 길이를 결정하고
// io.ReadFull 로 정확히 그만큼 읽어 완전한 응답 ADU 를 반환한다. RTU 는 MBAP 와
// 달리 길이 헤더가 없으므로 다음 규칙으로 프레임 경계를 판별한다:
//   - 예외 응답  [unitID][FC|0x80][excCode][CRC(2)]                 = 5바이트
//   - 읽기 응답  [unitID][FC][byteCount][data(byteCount)][CRC(2)]   = 3+byteCount+2
//   - 쓰기 응답  [unitID][FC][addr(2)][value/qty(2)][CRC(2)]        = 8바이트
//
// 먼저 [unitID][FC] 2바이트를 읽어 예외 여부를 판별하고, 정상 응답은 요청 FC 의
// 범주(읽기 FC01~04 / 쓰기 FC05·06·15·16)로 나머지 길이를 정한다. 짧은 읽기는
// io.ReadFull 이 io.ErrUnexpectedEOF 로 처리한다.
func readRTUResponse(r io.Reader, requestFC byte) ([]byte, error) {
	// 헤더 2바이트: [unitID][respFC]
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}
	respFC := header[1]

	// 예외 응답: excCode(1) + CRC(2) = 3바이트 추가
	if respFC&0x80 != 0 {
		rest := make([]byte, 3)
		if _, err := io.ReadFull(r, rest); err != nil {
			return nil, err
		}
		return append(header, rest...), nil
	}

	switch {
	case requestFC >= FC01ReadCoils && requestFC <= FC04ReadInputRegisters:
		// 읽기 응답: byteCount(1) 를 먼저 읽어 data(byteCount)+CRC(2) 를 읽는다.
		bc := make([]byte, 1)
		if _, err := io.ReadFull(r, bc); err != nil {
			return nil, err
		}
		byteCount := int(bc[0])
		rest := make([]byte, byteCount+2) // data + CRC(2)
		if _, err := io.ReadFull(r, rest); err != nil {
			return nil, err
		}
		adu := make([]byte, 0, 3+byteCount+2)
		adu = append(adu, header...)
		adu = append(adu, bc...)
		adu = append(adu, rest...)
		return adu, nil

	case requestFC == FC05WriteSingleCoil || requestFC == FC06WriteSingleRegister ||
		requestFC == FC15WriteMultipleCoils || requestFC == FC16WriteMultipleRegisters:
		// 쓰기 응답: addr(2) + value/qty(2) + CRC(2) = 6바이트 추가
		rest := make([]byte, 6)
		if _, err := io.ReadFull(r, rest); err != nil {
			return nil, err
		}
		return append(header, rest...), nil

	default:
		return nil, ErrInvalidFunctionCode
	}
}

// ---------------------------------------------------------------------------
// T3.5 프레임 간 정적 산출 (A-5)
// ---------------------------------------------------------------------------

// rtuInterFrameDelay 는 baudrate 로부터 T3.5(3.5 문자시간) 정적 시간을 산출한다(A-5).
//   - baud ≤ 19200: 문자시간 기반 = 3.5 × 11비트 / baud 초
//   - baud > 19200: 고정 1.75ms 규약
func rtuInterFrameDelay(baud int) time.Duration {
	if baud > 19200 {
		return 1750 * time.Microsecond
	}
	if baud <= 0 {
		baud = 9600
	}
	return time.Duration(float64(rtuCharBits) * 3.5 / float64(baud) * float64(time.Second))
}

// ---------------------------------------------------------------------------
// 기본 시리얼 오픈 (go.bug.st/serial, century 패턴 준용)
// ---------------------------------------------------------------------------

// defaultRTUSerialOpener 는 go.bug.st/serial 로 시리얼 포트를 연다.
// century/transport_serial.go 의 오픈 패턴을 준용하되, CRC 는 재사용하지 않는다
// (RTU CRC 는 rtu_crc.go).
func defaultRTUSerialOpener(cfg SerialConfig, readTimeout time.Duration) (io.ReadWriteCloser, error) {
	mode := &serial.Mode{
		BaudRate: cfg.BaudRate,
		DataBits: cfg.DataBits,
	}
	switch cfg.StopBits {
	case 2:
		mode.StopBits = serial.TwoStopBits
	default:
		mode.StopBits = serial.OneStopBit
	}
	switch cfg.Parity {
	case "none", "":
		mode.Parity = serial.NoParity
	case "even":
		mode.Parity = serial.EvenParity
	case "odd":
		mode.Parity = serial.OddParity
	default:
		return nil, fmt.Errorf("modbus rtu: unsupported parity %q", cfg.Parity)
	}

	port, err := serial.Open(cfg.Port, mode)
	if err != nil {
		return nil, fmt.Errorf("modbus rtu: open %s: %w", cfg.Port, err)
	}
	if readTimeout <= 0 {
		readTimeout = 3 * time.Second
	}
	if err := port.SetReadTimeout(readTimeout); err != nil {
		_ = port.Close()
		return nil, fmt.Errorf("modbus rtu: set read timeout %s: %w", cfg.Port, err)
	}
	return port, nil
}
