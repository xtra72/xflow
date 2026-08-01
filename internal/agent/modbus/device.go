package modbus

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// ModbusDevice 는 단일 MODBUS 디바이스의 상태와 통신을 관리한다.
// 트랜잭션 ID 관리는 ADU(MBAP) 관심사이므로 트랜스포트가 소유한다.
//
// UnitID 불변식(M9 → SPEC-MODBUS-006 unit_id 런타임 가변화):
//
//	unitID(atomic)는 unit_id 의 런타임 단일 소스(SSOT)이다. 폴링 goroutine 이
//	락 없이 읽는 핫 패스(ReadRegisters/SendPDU)와 응답 빌더가 모두 이 원자값을
//	UnitID() 로 읽고, set_config 는 setUnitID() 로 원자적으로 갱신한다.
//	config.UnitID 는 생성 시점의 시드일 뿐이며 런타임에는 절대 갱신·판독하지
//	않는다(살아있는 값은 항상 UnitID() 를 통해 읽어야 한다). 이로써 set_config
//	가 unit_id 를 바꾸는 도중에도 폴링 goroutine 과의 데이터 경합이 발생하지 않는다.
type ModbusDevice struct {
	config               DeviceConfig
	transport            ModbusTransport
	mu                   sync.RWMutex
	unitID               atomic.Uint32 // 런타임 가변 unit_id 의 SSOT (byte 값을 저장). 위 불변식 참조.
	online               bool
	lastError            error
	consecutiveErr       int
	lastReconnectAttempt time.Time // 마지막 재연결 시도 시각
	logger               *slog.Logger
}

// NewModbusDevice 는 새로운 ModbusDevice 를 생성한다.
// 내부적으로 ModbusTCPTransport 를 생성하여 사용한다.
func NewModbusDevice(cfg DeviceConfig, requestTimeout time.Duration, logger *slog.Logger) *ModbusDevice {
	transport := NewModbusTCPTransport(cfg.Host, cfg.Port, requestTimeout, logger)
	d := &ModbusDevice{
		config:    cfg,
		transport: transport,
		logger:    logger,
	}
	d.unitID.Store(uint32(cfg.UnitID)) // 생성 시 config.UnitID 로 원자값 시드(이후 런타임 SSOT).
	return d
}

// newModbusDeviceWithTransport 는 주입된 트랜스포트를 사용하는 ModbusDevice 를 생성한다.
// 테스트에서 mock 트랜스포트를 주입할 때 사용한다.
func newModbusDeviceWithTransport(cfg DeviceConfig, transport ModbusTransport, logger *slog.Logger) *ModbusDevice {
	d := &ModbusDevice{
		config:    cfg,
		transport: transport,
		logger:    logger,
	}
	d.unitID.Store(uint32(cfg.UnitID)) // 생성 시 config.UnitID 로 원자값 시드(이후 런타임 SSOT).
	return d
}

// UnitID 는 디바이스의 현재 unit_id 를 원자적으로 읽어 반환한다.
// 폴링 goroutine 의 락-프리 핫 패스(ReadRegisters/SendPDU)와 응답 빌더는
// config.UnitID 가 아니라 반드시 이 접근자를 통해 살아있는 값을 읽어야 한다(위 불변식 참조).
func (d *ModbusDevice) UnitID() byte {
	return byte(d.unitID.Load())
}

// setUnitID 는 디바이스의 unit_id 를 원자적으로 갱신한다(set_config 런타임 변경 경로).
// 호출자는 a.mu.Lock() 하에서 호출하여 다른 설정 변경과의 원자적 적용(부분 적용 없음)을 보장한다.
// 원자 저장 자체는 락 없이 읽는 폴링 goroutine 과 경합하지 않는다.
func (d *ModbusDevice) setUnitID(v byte) {
	d.unitID.Store(uint32(v))
}

// Connect 는 디바이스에 TCP 연결을 수립한다.
func (d *ModbusDevice) Connect(ctx context.Context) error {
	if err := d.transport.Connect(ctx); err != nil {
		d.mu.Lock()
		d.lastError = err
		d.consecutiveErr++
		d.mu.Unlock()
		return err
	}

	d.mu.Lock()
	d.online = true
	d.lastError = nil
	d.consecutiveErr = 0
	d.mu.Unlock()

	return nil
}

// Close 는 디바이스 연결을 종료한다.
func (d *ModbusDevice) Close() error {
	err := d.transport.Close()

	d.mu.Lock()
	d.online = false
	d.mu.Unlock()

	return err
}

// ReadRegisters 는 레지스터 그룹 설정에 따라 읽기 요청을 전송하고 응답 데이터를 반환한다.
// protocol.go 의 buildReadPDU/parseReadResponse(순수 PDU)를 사용하며,
// ADU 프레이밍(MBAP/CRC)은 트랜스포트가 담당한다.
func (d *ModbusDevice) ReadRegisters(ctx context.Context, rg RegisterGroupConfig) ([]byte, error) {
	if !d.transport.IsConnected() {
		return nil, ErrDeviceOffline
	}

	pdu := buildReadPDU(rg.FunctionCode, rg.StartAddress, rg.Quantity)

	respPDU, err := d.transport.SendAndReceive(ctx, d.UnitID(), pdu)
	if err != nil {
		d.recordError(err)
		return nil, fmt.Errorf("modbus: device %s read failed: %w", d.config.ID, err)
	}

	_, values, err := parseReadResponse(respPDU)
	if err != nil {
		d.recordError(err)
		return nil, fmt.Errorf("modbus: device %s parse response failed: %w", d.config.ID, err)
	}

	d.recordSuccess()
	return values, nil
}

// SendPDU 는 순수 요청 PDU 를 전송하고 응답 PDU 를 반환한다.
// unitID 부착과 ADU 프레이밍은 트랜스포트가 담당한다.
// 연결 상태 확인, 에러 기록, 성공 기록을 처리한다.
func (d *ModbusDevice) SendPDU(ctx context.Context, pdu []byte) ([]byte, error) {
	if !d.transport.IsConnected() {
		return nil, ErrDeviceOffline
	}
	respPDU, err := d.transport.SendAndReceive(ctx, d.UnitID(), pdu)
	if err != nil {
		d.recordError(err)
		return nil, fmt.Errorf("modbus: device %s send failed: %w", d.config.ID, err)
	}
	d.recordSuccess()
	return respPDU, nil
}

// IsOnline 은 디바이스가 온라인 상태인지 반환한다.
func (d *ModbusDevice) IsOnline() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.online
}

// TryReconnect 는 오프라인 디바이스의 재연결을 시도한다.
// 이미 온라인이면 true 를 반환한다.
// interval 이내에 이전 시도가 있었으면 false 를 반환하여 재연결을 건너뛴다.
// 재연결 성공 시 true, 실패 시 false 를 반환한다.
func (d *ModbusDevice) TryReconnect(ctx context.Context, interval time.Duration) bool {
	d.mu.RLock()
	if d.online {
		d.mu.RUnlock()
		return true
	}
	lastAttempt := d.lastReconnectAttempt
	d.mu.RUnlock()

	// 재연결 간격 미충족 시 스킵
	if time.Since(lastAttempt) < interval {
		return false
	}

	d.mu.Lock()
	d.lastReconnectAttempt = time.Now()
	d.mu.Unlock()

	if err := d.Connect(ctx); err != nil {
		return false
	}
	return true
}

// recordError 는 연속 에러 횟수를 증가시킨다.
func (d *ModbusDevice) recordError(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastError = err
	d.consecutiveErr++
	// 연속 에러가 3회 이상이면 오프라인으로 표시
	if d.consecutiveErr >= 3 {
		d.online = false
		d.logger.Warn("modbus: 디바이스 오프라인 전환",
			"device", d.config.ID,
			"consecutiveErrors", d.consecutiveErr,
		)
	}
}

// recordSuccess 는 에러 카운터를 초기화한다.
func (d *ModbusDevice) recordSuccess() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastError = nil
	d.consecutiveErr = 0
	d.online = true
}
