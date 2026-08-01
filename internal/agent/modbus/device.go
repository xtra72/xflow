package modbus

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ModbusDevice 는 단일 MODBUS 디바이스의 상태와 통신을 관리한다.
// 트랜잭션 ID 관리는 ADU(MBAP) 관심사이므로 트랜스포트가 소유한다.
type ModbusDevice struct {
	config               DeviceConfig
	transport            ModbusTransport
	mu                   sync.RWMutex
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
	return &ModbusDevice{
		config:    cfg,
		transport: transport,
		logger:    logger,
	}
}

// newModbusDeviceWithTransport 는 주입된 트랜스포트를 사용하는 ModbusDevice 를 생성한다.
// 테스트에서 mock 트랜스포트를 주입할 때 사용한다.
func newModbusDeviceWithTransport(cfg DeviceConfig, transport ModbusTransport, logger *slog.Logger) *ModbusDevice {
	return &ModbusDevice{
		config:    cfg,
		transport: transport,
		logger:    logger,
	}
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

	respPDU, err := d.transport.SendAndReceive(ctx, d.config.UnitID, pdu)
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
	respPDU, err := d.transport.SendAndReceive(ctx, d.config.UnitID, pdu)
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
