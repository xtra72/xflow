package modbusserver

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"

	modbus "github.com/xtra/xflow/internal/modbus"
	"go.bug.st/serial"
)

// ---------------------------------------------------------------------------
// RTU 시리얼 슬레이브 리스너 (M2)
// ---------------------------------------------------------------------------
//
// RTUListener 는 시리얼 포트를 MODBUS RTU 슬레이브로 열어 인바운드 RTU 요청 ADU 를
// 수신하고, CRC 를 검증한 뒤 PDU 를 ModbusHandler(DeviceManager 라우팅)로 전달하고,
// RTU 프레이밍한 응답을 되돌려 쓴다. ADU 프레이밍은 공유 패키지 internal/modbus 의
// CRC16/BuildRTUADU 를 사용한다(클라이언트 마스터와 동작 동일).
//
// TCP Listener 와 동일한 serverListener 인터페이스를 만족하여 ModbusServerAgent 가
// 트랜스포트에 무관하게 라이프사이클을 배선할 수 있다. PDU 레벨 처리는 ModbusHandler
// 의 라우팅 메서드(handleDeviceRequest/handleBroadcast)를 재사용한다 — 핸들러는 PDU
// 레벨에서 트랜스포트 무관이며, MBAP↔RTU 프레이밍 차이는 리스너 경계에서만 흡수한다.
//
// 테스트 가능성: 시리얼 오픈은 opener 함수 seam 으로 대체 가능하다. mock
// io.ReadWriteCloser 로 하드웨어 없이 요청→응답 경로를 검증한다.

// 컴파일 타임 인터페이스 체크
var _ serverListener = (*RTUListener)(nil)

// rtuSerialOpener 는 시리얼 포트를 여는 함수 seam 이다(테스트 훅).
// 운영 코드는 defaultServerRTUSerialOpener 를 사용한다.
type rtuSerialOpener func(cfg SerialConfig) (io.ReadWriteCloser, error)

// RTUListener 는 시리얼 기반 MODBUS RTU 슬레이브 리스너이다.
type RTUListener struct {
	serial  SerialConfig
	opener  rtuSerialOpener
	handler *ModbusHandler
	logger  *slog.Logger

	mu      sync.Mutex
	port    io.ReadWriteCloser
	cancel  context.CancelFunc
	running atomic.Bool
	active  atomic.Int32
	wg      sync.WaitGroup
}

// NewRTUListener 는 새로운 RTUListener 를 생성한다.
func NewRTUListener(cfg SerialConfig, handler *ModbusHandler, logger *slog.Logger) *RTUListener {
	return &RTUListener{
		serial:  cfg,
		opener:  defaultServerRTUSerialOpener,
		handler: handler,
		logger:  logger,
	}
}

// Start 는 시리얼 포트를 열고 요청 수신 루프를 시작한다.
// 이미 실행 중이면 ErrServerAlreadyRunning 을 반환한다.
func (l *RTUListener) Start(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.running.Load() {
		return ErrServerAlreadyRunning
	}

	port, err := l.opener(l.serial)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrListenFailed, err)
	}

	l.port = port
	l.running.Store(true)
	l.active.Store(1)

	childCtx, cancel := context.WithCancel(ctx)
	l.cancel = cancel

	l.logInfo("RTU listener started", "port", l.serial.Port, "baud", l.serial.BaudRate)

	l.wg.Add(1)
	go l.readLoop(childCtx)

	return nil
}

// Stop 은 시리얼 포트를 닫고 수신 루프가 종료될 때까지 대기한다.
func (l *RTUListener) Stop() error {
	l.mu.Lock()
	if l.cancel != nil {
		l.cancel()
	}
	if l.port != nil {
		_ = l.port.Close()
	}
	l.running.Store(false)
	l.active.Store(0)
	l.mu.Unlock()

	l.wg.Wait()

	l.logInfo("RTU listener stopped")
	return nil
}

// ActiveConnections 는 현재 시리얼 링크 활성 여부를 반환한다(0 또는 1).
// 시리얼은 단일 버스이므로 TCP 의 연결 수 개념과 달리 링크 상태를 나타낸다.
func (l *RTUListener) ActiveConnections() int32 {
	return l.active.Load()
}

// Addr 는 항상 nil 을 반환한다(시리얼은 net.Addr 이 없다).
func (l *RTUListener) Addr() net.Addr {
	return nil
}

// readLoop 는 인바운드 RTU 요청 ADU 를 반복 수신하여 라우팅하고 응답을 되돌려 쓴다.
func (l *RTUListener) readLoop(ctx context.Context) {
	defer l.wg.Done()

	remote := "rtu:" + l.serial.Port

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		adu, err := readRTURequest(l.port)
		if err != nil {
			// I/O 종료(EOF)·컨텍스트 취소·프레이밍 불가(미지원 FC)는 루프를 종료한다.
			if ctx.Err() != nil || !l.running.Load() {
				return
			}
			if err != io.EOF {
				l.logWarn("RTU request read failed", "error", err)
			}
			return
		}

		unitID, pdu, perr := parseRTURequestADU(adu)
		if perr != nil {
			// CRC 불일치 등 손상 프레임: 폐기하고 다음 프레임을 계속 수신한다.
			l.logWarn("RTU request parse failed", "error", perr)
			continue
		}
		if len(pdu) == 0 {
			continue
		}

		fc := pdu[0]

		// broadcast(unitID=0): 쓰기를 팬아웃 처리하되 RTU 규격상 응답하지 않는다.
		if unitID == 0 {
			l.handler.handleBroadcast(pdu, fc, remote)
			continue
		}

		dev := l.handler.deviceManager.GetDevice(unitID)
		if dev == nil {
			l.logWarn("RTU unit ID not found", "unitID", unitID)
			continue
		}

		respPDU := l.handler.handleDeviceRequest(dev, pdu, fc, remote)
		if respPDU == nil {
			continue
		}

		respADU := modbus.BuildRTUADU(unitID, respPDU)
		if _, err := l.port.Write(respADU); err != nil {
			if ctx.Err() == nil && l.running.Load() {
				l.logWarn("RTU response write failed", "error", err)
			}
			return
		}
	}
}

// logInfo logs an info message if a logger is available.
func (l *RTUListener) logInfo(msg string, args ...any) {
	if l.logger != nil {
		l.logger.Info(fmt.Sprintf("modbus-rtu-listener: %s", msg), args...)
	}
}

// logWarn logs a warning message if a logger is available.
func (l *RTUListener) logWarn(msg string, args ...any) {
	if l.logger != nil {
		l.logger.Warn(fmt.Sprintf("modbus-rtu-listener: %s", msg), args...)
	}
}

// ---------------------------------------------------------------------------
// RTU 요청 프레임 리더 / 파서 (길이 헤더 없음 — 요청 FC 로 결정)
// ---------------------------------------------------------------------------

// readRTURequest 는 요청 function code 로부터 RTU 요청 프레임 길이를 결정하고
// io.ReadFull 로 정확히 그만큼 읽어 완전한 요청 ADU 를 반환한다. RTU 는 길이 헤더가
// 없으므로 다음 규칙으로 프레임 경계를 판별한다:
//   - 읽기(FC01~04) / 단일 쓰기(FC05·06):
//     [unitID][FC][addr(2)][qty|value(2)][CRC(2)]                 = 8바이트
//   - 다중 쓰기(FC15·16):
//     [unitID][FC][addr(2)][qty(2)][byteCount(1)][data(bc)][CRC(2)] = 7+bc+2
//
// 미지원 FC 는 길이를 판별할 수 없으므로 오류를 반환한다(호출자가 루프를 종료).
func readRTURequest(r io.Reader) ([]byte, error) {
	// 헤더 2바이트: [unitID][FC]
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}
	fc := header[1]

	switch fc {
	case fcReadCoils, fcReadDiscreteInputs, fcReadHoldingRegisters, fcReadInputRegisters,
		fcWriteSingleCoil, fcWriteSingleRegister:
		// addr(2) + qty|value(2) + CRC(2) = 6바이트 추가
		rest := make([]byte, 6)
		if _, err := io.ReadFull(r, rest); err != nil {
			return nil, err
		}
		return append(header, rest...), nil

	case fcWriteMultipleCoils, fcWriteMultipleRegisters:
		// addr(2) + qty(2) + byteCount(1) 를 먼저 읽어 data(byteCount)+CRC(2) 를 읽는다.
		mid := make([]byte, 5)
		if _, err := io.ReadFull(r, mid); err != nil {
			return nil, err
		}
		byteCount := int(mid[4])
		rest := make([]byte, byteCount+2) // data + CRC(2)
		if _, err := io.ReadFull(r, rest); err != nil {
			return nil, err
		}
		adu := make([]byte, 0, 2+5+byteCount+2)
		adu = append(adu, header...)
		adu = append(adu, mid...)
		adu = append(adu, rest...)
		return adu, nil

	default:
		return nil, fmt.Errorf("modbus-server: unsupported RTU request function code 0x%02X", fc)
	}
}

// parseRTURequestADU 는 요청 ADU 의 CRC 를 재검증하고 unitID 와 순수 요청 PDU 를 반환한다.
// 슬레이브는 모든 unitID 를 수용(DeviceManager 라우팅)하므로 unitID 기대값 검사는 하지 않는다.
func parseRTURequestADU(adu []byte) (byte, []byte, error) {
	if len(adu) < 4 {
		return 0, nil, modbus.ErrRTUFrameTooShort
	}
	dataLen := len(adu) - 2
	wantCRC := uint16(adu[dataLen]) | uint16(adu[dataLen+1])<<8
	if modbus.CRC16(adu[:dataLen]) != wantCRC {
		return 0, nil, modbus.ErrCRCMismatch
	}
	unitID := adu[0]
	pdu := make([]byte, dataLen-1)
	copy(pdu, adu[1:dataLen])
	return unitID, pdu, nil
}

// ---------------------------------------------------------------------------
// 기본 시리얼 오픈 (go.bug.st/serial)
// ---------------------------------------------------------------------------

// defaultServerRTUSerialOpener 는 go.bug.st/serial 로 시리얼 포트를 슬레이브 용도로 연다.
// 슬레이브는 요청이 도착할 때까지 무기한 대기해야 하므로 read timeout 을 두지 않는다
// (serial.NoTimeout). 클라이언트 마스터의 오픈 패턴을 준용하되, 요청별 타임아웃은 없다.
func defaultServerRTUSerialOpener(cfg SerialConfig) (io.ReadWriteCloser, error) {
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
		return nil, fmt.Errorf("modbus-server rtu: unsupported parity %q", cfg.Parity)
	}

	port, err := serial.Open(cfg.Port, mode)
	if err != nil {
		return nil, fmt.Errorf("modbus-server rtu: open %s: %w", cfg.Port, err)
	}
	// 슬레이브는 무기한 블로킹 읽기: 요청 프레임이 올 때까지 대기.
	if err := port.SetReadTimeout(serial.NoTimeout); err != nil {
		_ = port.Close()
		return nil, fmt.Errorf("modbus-server rtu: set read timeout %s: %w", cfg.Port, err)
	}
	return port, nil
}
