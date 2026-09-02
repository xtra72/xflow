package modbusserver

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	modbus "github.com/xtra/xflow/internal/agent/modbus"
)

// ---------------------------------------------------------------------------
// backedStore — upstream 백킹 registerStore 데코레이터 (REQ-MODBUS-010-02/03/04)
// ---------------------------------------------------------------------------
//
// backedStore 는 registerStore 인터페이스(request.go)를 만족하여 기존 서빙 경로
// (ModbusHandler → RequestHandler → registerStore)에 그대로 삽입된다. 순수 slave
// 디바이스는 *RegisterMap/*deviceView 를 직접 사용하고, 백킹 디바이스만 backedStore 로
// 감싼다 — 따라서 RequestHandler/ModbusHandler 골격은 불변이다(하위 호환 HARD).
//
// 상태 소유:
//   - inner     : 내부 저장(RegisterMap, 자체 RWMutex 보유). direct=조회 결과 캐시,
//     indirect=폴 결과 저장 + 서빙 소스.
//   - transport : upstream 통신(internal/agent/modbus 재사용, 자체 turnaround mutex 보유).
//   - upUnitID  : upstream 디바이스 unit id(가상 UnitID 와 독립).
//   - mode      : direct | indirect.
//   - timeout   : upstream 요청 데드라인(direct/indirect 쓰기) + indirect 읽기 stale 허용 한도.
//   - lastOK    : indirect 마지막 성공 폴 시각(UnixNano, atomic). direct 는 사용하지 않는다.
//
// 폴러 goroutine 자체(indirect 주기 폴링)와 트랜스포트 Start/Stop lifecycle 은 상위
// (agent.go)가 소유하며 M4/M5 에서 배선한다. backedStore 는 recordPollSuccess 및
// upstream 조회 헬퍼(upstreamReadRegisters 등)를 노출하여 그 배선의 seam 을 제공한다.
type backedStore struct {
	inner     *RegisterMap
	transport modbus.ModbusTransport
	upUnitID  byte
	mode      backingMode
	timeout   time.Duration
	lastOK    atomic.Int64 // indirect: 마지막 성공 폴 시각(UnixNano); direct 미사용

	// ---- 관측 메트릭 (SPEC-MODBUS-012 M3, REQ-06-01) ----
	//
	// 아래 카운터는 게이트웨이→upstream(실제 디바이스) 통신 통계이며, 마스터→게이트웨이
	// 서빙 통계인 DeviceStats(device_manager.go)와는 별개다(혼동 금지). 계측점은 단일
	// upstream 호출 지점 sendUpstream 하나뿐이므로 폴러 goroutine(indirect)과 서빙
	// goroutine(direct)이 동시에 증가시킬 수 있으나 전부 atomic 이라 race-safe 하다.
	reqCount   atomic.Int64 // upstream 요청 총수(성공+실패)
	errCount   atomic.Int64 // upstream 트랜스포트 실패 총수(SendAndReceive 오류)
	latencyNs  atomic.Int64 // upstream 누적 왕복 레이턴시(ns); avg = latencyNs/reqCount
	lastReqOK  atomic.Int64 // 마지막 성공 upstream 요청 시각(UnixNano); direct last_ok/connected 판정 소스
	lastReqErr atomic.Bool  // 직전 upstream 요청이 실패했는지; direct connected 판정 소스
}

// backingMode 는 백킹 동작 모드이다.
type backingMode int

const (
	backingDirect   backingMode = iota // 마스터 요청 시 upstream 즉시 조회
	backingIndirect                    // 폴 캐시 서빙 + stale 판정
)

// 컴파일 타임 인터페이스 체크: *backedStore 는 registerStore 를 만족한다.
var _ registerStore = (*backedStore)(nil)

// newBackedStore 는 백킹 store 를 생성한다. inner 는 디바이스 register_map 으로 구성된
// *RegisterMap, transport 는 upstream 트랜스포트, cfg 는 검증된 BackingConfig 이다.
func newBackedStore(inner *RegisterMap, transport modbus.ModbusTransport, cfg *BackingConfig) *backedStore {
	timeout := cfg.Timeout
	if timeout <= 0 {
		// direct 모드에서 timeout 미지정 시 안전한 기본 데드라인을 부여하여
		// 느리거나 죽은 upstream 이 리스너 goroutine 을 무기한 블로킹하지 않게 한다.
		timeout = time.Second
	}
	mode := backingDirect
	if cfg.Mode == BackingModeIndirect {
		mode = backingIndirect
	}
	return &backedStore{
		inner:     inner,
		transport: transport,
		upUnitID:  cfg.UnitID,
		mode:      mode,
		timeout:   timeout,
	}
}

// newUpstreamTransport 는 BackingConfig 에 따라 upstream 트랜스포트를 구성한다.
// internal/agent/modbus 가 export 하는 생성자를 재사용하며 client 패키지를 변경하지 않는다.
// 반환된 트랜스포트는 아직 Connect 되지 않았으며, 연결은 상위(agent.go, M5)가 관리한다.
func newUpstreamTransport(cfg *BackingConfig, logger *slog.Logger) (modbus.ModbusTransport, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = time.Second
	}
	switch cfg.Transport {
	case TransportRTU:
		return modbus.NewModbusRTUTransport(modbus.SerialConfig{
			Port:     cfg.Serial.Port,
			BaudRate: cfg.Serial.BaudRate,
			DataBits: cfg.Serial.DataBits,
			StopBits: cfg.Serial.StopBits,
			Parity:   cfg.Serial.Parity,
		}, timeout, logger), nil
	case TransportTCP, "":
		return modbus.NewModbusTCPTransport(cfg.Host, cfg.Port, timeout, logger), nil
	default:
		return nil, fmt.Errorf(
			"modbus-server: unsupported backing transport %q: %w", cfg.Transport, ErrInvalidBackingConfig)
	}
}

// ---------------------------------------------------------------------------
// registerStore 구현 — 읽기
// ---------------------------------------------------------------------------

// ReadCoils 는 FC01 읽기를 서빙한다. direct=upstream 즉시 조회 후 캐시, indirect=저장값 서빙.
func (bs *backedStore) ReadCoils(start, quantity uint16) ([]bool, error) {
	if bs.mode == backingIndirect {
		if bs.isStale() {
			return nil, ErrGatewayTargetFailed
		}
		return bs.inner.ReadCoils(start, quantity)
	}
	vals, err := bs.upstreamReadBits(modbus.FC01ReadCoils, start, quantity)
	if err != nil {
		return nil, err
	}
	_, _ = bs.inner.WriteCoils(start, vals) // best-effort 캐시(범위 밖이면 무시)
	return vals, nil
}

// ReadDiscreteInputs 는 FC02 읽기를 서빙한다.
func (bs *backedStore) ReadDiscreteInputs(start, quantity uint16) ([]bool, error) {
	if bs.mode == backingIndirect {
		if bs.isStale() {
			return nil, ErrGatewayTargetFailed
		}
		return bs.inner.ReadDiscreteInputs(start, quantity)
	}
	vals, err := bs.upstreamReadBits(modbus.FC02ReadDiscreteInputs, start, quantity)
	if err != nil {
		return nil, err
	}
	_, _ = bs.inner.WriteDiscreteInputs(start, vals) // best-effort 캐시
	return vals, nil
}

// ReadHoldingRegisters 는 FC03 읽기를 서빙한다.
func (bs *backedStore) ReadHoldingRegisters(start, quantity uint16) ([]uint16, error) {
	if bs.mode == backingIndirect {
		if bs.isStale() {
			return nil, ErrGatewayTargetFailed
		}
		return bs.inner.ReadHoldingRegisters(start, quantity)
	}
	vals, err := bs.upstreamReadRegisters(modbus.FC03ReadHoldingRegisters, start, quantity)
	if err != nil {
		return nil, err
	}
	_, _ = bs.inner.WriteHoldingRegisters(start, vals) // best-effort 캐시
	return vals, nil
}

// ReadInputRegisters 는 FC04 읽기를 서빙한다.
func (bs *backedStore) ReadInputRegisters(start, quantity uint16) ([]uint16, error) {
	if bs.mode == backingIndirect {
		if bs.isStale() {
			return nil, ErrGatewayTargetFailed
		}
		return bs.inner.ReadInputRegisters(start, quantity)
	}
	vals, err := bs.upstreamReadRegisters(modbus.FC04ReadInputRegisters, start, quantity)
	if err != nil {
		return nil, err
	}
	_, _ = bs.inner.WriteInputRegisters(start, vals) // best-effort 캐시
	return vals, nil
}

// ---------------------------------------------------------------------------
// registerStore 구현 — 쓰기 (direct/indirect 공통: 폴 캐시 우회, 즉시 upstream 전달)
// ---------------------------------------------------------------------------

// WriteCoils 는 FC05/FC15 쓰기를 upstream 으로 전달하고, 성공 시에만 inner 에 미러한다.
// upstream 미도달 시 ErrGatewayTargetFailed 를 반환하며 inner 를 변형하지 않는다(AC-06).
func (bs *backedStore) WriteCoils(start uint16, values []bool) (*ChangeSet, error) {
	if err := bs.upstreamWriteCoils(start, values); err != nil {
		return nil, err
	}
	return bs.inner.WriteCoils(start, values)
}

// WriteHoldingRegisters 는 FC06/FC16 쓰기를 upstream 으로 전달하고, 성공 시에만 미러한다.
func (bs *backedStore) WriteHoldingRegisters(start uint16, values []uint16) (*ChangeSet, error) {
	if err := bs.upstreamWriteRegisters(start, values); err != nil {
		return nil, err
	}
	return bs.inner.WriteHoldingRegisters(start, values)
}

// ---------------------------------------------------------------------------
// indirect stale 판정 + 폴 성공 기록 seam (M4 배선용)
// ---------------------------------------------------------------------------

// isStale 는 indirect 읽기 서빙 직전 stale 여부를 판정한다.
//   - 한 번도 성공 폴이 없으면(lastOK==0) stale.
//   - now - lastOK > timeout 이면 stale(초과 → 0x0B).
//   - now - lastOK <= timeout 이면 정상(stale 저장값 허용).
func (bs *backedStore) isStale() bool {
	last := bs.lastOK.Load()
	if last == 0 {
		return true
	}
	return time.Since(time.Unix(0, last)) > bs.timeout
}

// recordPollSuccess 는 indirect 폴러(M4)가 성공적 폴 이후 호출하여 마지막 성공 갱신
// 시각을 원자적으로 기록한다. direct 모드는 사용하지 않는다.
func (bs *backedStore) recordPollSuccess(t time.Time) {
	bs.lastOK.Store(t.UnixNano())
}

// ---------------------------------------------------------------------------
// 관측 메트릭 스냅샷 (SPEC-MODBUS-012 M3, REQ-06-02) — agent.go 노출용
// ---------------------------------------------------------------------------

// backingMetrics 는 백킹(upstream) 관측 메트릭의 스냅샷이다. get_device_status 의 backing
// 서브객체로 직렬화된다. LastOKMillis 는 epoch 밀리초(프로젝트 타임스탬프 규약)이며 0 이면
// 성공 이력이 없음을 뜻한다.
type backingMetrics struct {
	Mode         string  // "direct" | "indirect"
	Connected    bool    // 연결/유효 상태(모드별 판정)
	RequestCount int64   // upstream 요청 총수
	ErrorCount   int64   // upstream 실패 총수
	AvgLatencyMs float64 // 평균 왕복 레이턴시(ms); 요청 0 이면 0
	LastOKMillis int64   // 마지막 성공 시각(epoch ms); 0=없음
}

// modeString 은 백킹 모드를 config 상수 문자열("direct"|"indirect")로 반환한다.
func (bs *backedStore) modeString() string {
	if bs.mode == backingIndirect {
		return BackingModeIndirect
	}
	return BackingModeDirect
}

// isConnected 는 모드별로 연결/유효 상태를 판정한다.
//   - indirect: 폴 신선도 기준. stale(마지막 성공 폴 초과)이 아니면 연결로 본다(!isStale).
//   - direct  : 마지막 upstream 요청 성공 여부 기준. 요청 이력이 없으면(reqCount==0)
//     트랜스포트 연결 상태(IsConnected)로 대체한다.
func (bs *backedStore) isConnected() bool {
	if bs.mode == backingIndirect {
		return !bs.isStale()
	}
	if bs.reqCount.Load() == 0 {
		return bs.transport.IsConnected()
	}
	return !bs.lastReqErr.Load()
}

// metrics 는 관측 카운터의 원자적 스냅샷을 반환한다. 모든 필드는 atomic 로드로
// 취득하므로 폴러/서빙 goroutine 과 동시 호출해도 race-safe 하다.
func (bs *backedStore) metrics() backingMetrics {
	req := bs.reqCount.Load()
	var avgMs float64
	if req > 0 {
		avgMs = float64(bs.latencyNs.Load()) / float64(req) / 1e6
	}

	// 마지막 성공 시각: indirect 는 폴 성공(lastOK), direct 는 요청 성공(lastReqOK) 기준.
	var lastNs int64
	if bs.mode == backingIndirect {
		lastNs = bs.lastOK.Load()
	} else {
		lastNs = bs.lastReqOK.Load()
	}
	var lastMs int64
	if lastNs > 0 {
		lastMs = lastNs / int64(time.Millisecond)
	}

	return backingMetrics{
		Mode:         bs.modeString(),
		Connected:    bs.isConnected(),
		RequestCount: req,
		ErrorCount:   bs.errCount.Load(),
		AvgLatencyMs: avgMs,
		LastOKMillis: lastMs,
	}
}

// ---------------------------------------------------------------------------
// upstream 통신 헬퍼 (PDU 조립/디코딩)
// ---------------------------------------------------------------------------
//
// internal/agent/modbus 의 PDU 조립/디코딩 헬퍼(buildReadPDU/parseReadResponse 등)는
// 모두 unexported 이므로 재사용할 수 없다. 트랜스포트가 PDU-중립이므로 modbusserver 측에서
// FC별 요청 PDU 를 최소 조립하고, 응답 PDU 디코딩에는 request.go 의 기존 헬퍼
// (decodeCoilBits/decodeRegisterBytes)를 재사용한다. 신규 외부 라이브러리는 도입하지 않는다.

// sendUpstream 은 timeout 데드라인 하에서 upstream 에 순수 PDU 를 전송하고 응답 PDU 를 받는다.
//
// 모든 upstream 헬퍼(upstreamReadRegisters/upstreamReadBits/upstreamWrite*)가 이 함수를
// 경유하므로, 관측 메트릭(요청 수·에러 수·레이턴시)을 여기 단일 지점에서 계측한다
// (SPEC-MODBUS-012 M3, REQ-06-01). 에러 판정은 트랜스포트 SendAndReceive 결과 기준이며,
// MODBUS 예외 응답(fc&0x80)은 상위 파서가 별도 처리하므로 errCount 에 포함되지 않는다.
func (bs *backedStore) sendUpstream(pdu []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), bs.timeout)
	defer cancel()

	start := time.Now()
	resp, err := bs.transport.SendAndReceive(ctx, bs.upUnitID, pdu)
	bs.reqCount.Add(1)
	bs.latencyNs.Add(int64(time.Since(start)))
	if err != nil {
		bs.errCount.Add(1)
		bs.lastReqErr.Store(true)
		return resp, err
	}
	bs.lastReqErr.Store(false)
	bs.lastReqOK.Store(time.Now().UnixNano())
	return resp, nil
}

// upstreamReadRegisters 는 FC03/FC04 읽기 요청을 upstream 에 보내 레지스터 값을 조회한다.
func (bs *backedStore) upstreamReadRegisters(fc byte, start, quantity uint16) ([]uint16, error) {
	resp, err := bs.sendUpstream(buildUpstreamReadPDU(fc, start, quantity))
	if err != nil {
		return nil, ErrGatewayTargetFailed
	}
	data, err := parseUpstreamReadResponse(resp)
	if err != nil {
		return nil, err
	}
	return decodeRegisterBytes(data), nil
}

// upstreamReadBits 는 FC01/FC02 읽기 요청을 upstream 에 보내 코일/이산입력 값을 조회한다.
func (bs *backedStore) upstreamReadBits(fc byte, start, quantity uint16) ([]bool, error) {
	resp, err := bs.sendUpstream(buildUpstreamReadPDU(fc, start, quantity))
	if err != nil {
		return nil, ErrGatewayTargetFailed
	}
	data, err := parseUpstreamReadResponse(resp)
	if err != nil {
		return nil, err
	}
	return decodeCoilBits(data, int(quantity)), nil
}

// upstreamWriteRegisters 는 값 개수에 따라 FC06(단일)/FC16(다중) 쓰기를 upstream 에 전달한다.
func (bs *backedStore) upstreamWriteRegisters(start uint16, values []uint16) error {
	resp, err := bs.sendUpstream(buildUpstreamWriteRegistersPDU(start, values))
	if err != nil {
		return ErrGatewayTargetFailed
	}
	return parseUpstreamWriteResponse(resp)
}

// upstreamWriteCoils 는 값 개수에 따라 FC05(단일)/FC15(다중) 쓰기를 upstream 에 전달한다.
func (bs *backedStore) upstreamWriteCoils(start uint16, values []bool) error {
	resp, err := bs.sendUpstream(buildUpstreamWriteCoilsPDU(start, values))
	if err != nil {
		return ErrGatewayTargetFailed
	}
	return parseUpstreamWriteResponse(resp)
}

// ---------------------------------------------------------------------------
// PDU 조립/파싱 (ADU-중립: 트랜스포트가 MBAP/CRC 프레이밍 담당)
// ---------------------------------------------------------------------------

// buildUpstreamReadPDU 는 읽기 요청 PDU [fc][start(2)][quantity(2)] 를 조립한다.
func buildUpstreamReadPDU(fc byte, start, quantity uint16) []byte {
	pdu := make([]byte, 5)
	pdu[0] = fc
	binary.BigEndian.PutUint16(pdu[1:3], start)
	binary.BigEndian.PutUint16(pdu[3:5], quantity)
	return pdu
}

// parseUpstreamReadResponse 는 읽기 응답 PDU [fc][byteCount][data...] 에서 data 를 추출한다.
// 예외 응답(fc&0x80)·손상 프레임은 ErrGatewayTargetFailed 로 반환한다.
func parseUpstreamReadResponse(resp []byte) ([]byte, error) {
	if len(resp) < 2 {
		return nil, ErrGatewayTargetFailed
	}
	if resp[0]&0x80 != 0 {
		// upstream 이 MODBUS 예외를 반환 → 게이트웨이 대상 실패로 간주.
		return nil, ErrGatewayTargetFailed
	}
	byteCount := int(resp[1])
	if len(resp) < 2+byteCount {
		return nil, ErrGatewayTargetFailed
	}
	return resp[2 : 2+byteCount], nil
}

// buildUpstreamWriteRegistersPDU 는 레지스터 쓰기 요청 PDU 를 조립한다.
// 값이 1개면 FC06(단일), 2개 이상이면 FC16(다중)을 사용한다.
func buildUpstreamWriteRegistersPDU(start uint16, values []uint16) []byte {
	if len(values) == 1 {
		pdu := make([]byte, 5)
		pdu[0] = fcWriteSingleRegister
		binary.BigEndian.PutUint16(pdu[1:3], start)
		binary.BigEndian.PutUint16(pdu[3:5], values[0])
		return pdu
	}
	byteCount := len(values) * 2
	pdu := make([]byte, 6+byteCount)
	pdu[0] = fcWriteMultipleRegisters
	binary.BigEndian.PutUint16(pdu[1:3], start)
	binary.BigEndian.PutUint16(pdu[3:5], uint16(len(values)))
	pdu[5] = byte(byteCount)
	copy(pdu[6:], encodeRegisters(values))
	return pdu
}

// buildUpstreamWriteCoilsPDU 는 코일 쓰기 요청 PDU 를 조립한다.
// 값이 1개면 FC05(단일), 2개 이상이면 FC15(다중)을 사용한다.
func buildUpstreamWriteCoilsPDU(start uint16, values []bool) []byte {
	if len(values) == 1 {
		pdu := make([]byte, 5)
		pdu[0] = fcWriteSingleCoil
		binary.BigEndian.PutUint16(pdu[1:3], start)
		if values[0] {
			pdu[3] = 0xFF
			pdu[4] = 0x00
		}
		return pdu
	}
	data := encodeCoils(values)
	pdu := make([]byte, 6+len(data))
	pdu[0] = fcWriteMultipleCoils
	binary.BigEndian.PutUint16(pdu[1:3], start)
	binary.BigEndian.PutUint16(pdu[3:5], uint16(len(values)))
	pdu[5] = byte(len(data))
	copy(pdu[6:], data)
	return pdu
}

// parseUpstreamWriteResponse 는 쓰기 응답 PDU 를 검사한다.
// 예외 응답(fc&0x80)·손상 프레임은 ErrGatewayTargetFailed 로 반환한다.
func parseUpstreamWriteResponse(resp []byte) error {
	if len(resp) < 1 {
		return ErrGatewayTargetFailed
	}
	if resp[0]&0x80 != 0 {
		return ErrGatewayTargetFailed
	}
	return nil
}
