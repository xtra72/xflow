package modbusserver

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	modbus "github.com/xtra/xflow/internal/agent/modbus"
)

// ---------------------------------------------------------------------------
// 테스트 더블: fake upstream 트랜스포트 (하드웨어 없이 응답/끊김/지연 주입)
// ---------------------------------------------------------------------------

// fakeTransport 는 modbus.ModbusTransport 를 만족하는 인메모리 테스트 더블이다.
type fakeTransport struct {
	mu           sync.Mutex
	connected    bool
	connectCalls int
	closeCalls   int
	sendCalls    int
	lastUnitID   byte
	lastPDU      []byte
	delay        time.Duration
	// resp 는 요청 PDU 에 대한 응답 PDU 를 반환한다. err != nil 이면 SendAndReceive 가 이를 반환한다.
	resp func(pdu []byte) ([]byte, error)
}

var _ modbus.ModbusTransport = (*fakeTransport)(nil)

func (f *fakeTransport) Connect(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connectCalls++
	f.connected = true
	return nil
}

func (f *fakeTransport) SendAndReceive(ctx context.Context, unitID byte, pdu []byte) ([]byte, error) {
	f.mu.Lock()
	f.sendCalls++
	f.lastUnitID = unitID
	f.lastPDU = append([]byte(nil), pdu...)
	delay := f.delay
	resp := f.resp
	f.mu.Unlock()

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return resp(pdu)
}

func (f *fakeTransport) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closeCalls++
	f.connected = false
	return nil
}

func (f *fakeTransport) IsConnected() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connected
}

func (f *fakeTransport) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sendCalls
}

// ---------------------------------------------------------------------------
// 응답 PDU 빌더 헬퍼
// ---------------------------------------------------------------------------

// regReadResp 는 레지스터 읽기 응답 PDU [fc][byteCount][data...] 를 생성한다.
func regReadResp(fc byte, values []uint16) []byte {
	data := encodeRegisters(values)
	resp := make([]byte, 2+len(data))
	resp[0] = fc
	resp[1] = byte(len(data))
	copy(resp[2:], data)
	return resp
}

// writeEchoResp 는 쓰기 응답 PDU [fc][addr(2)][value/qty(2)] 를 요청에서 에코 생성한다.
func writeEchoResp(pdu []byte) []byte {
	resp := make([]byte, 5)
	resp[0] = pdu[0]
	copy(resp[1:5], pdu[1:5])
	return resp
}

func backedTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// backedTestInner 는 백킹 store 의 내부 RegisterMap(holding/coils/di/input 0..100)을 만든다.
func backedTestInner() *RegisterMap {
	return NewRegisterMap(RegisterMapConfig{
		Coils:            []*RegisterAreaConfig{{StartAddress: 0, Count: 100}},
		DiscreteInputs:   []*RegisterAreaConfig{{StartAddress: 0, Count: 100}},
		HoldingRegisters: []*RegisterAreaConfig{{StartAddress: 0, Count: 100}},
		InputRegisters:   []*RegisterAreaConfig{{StartAddress: 0, Count: 100}},
	})
}

// writeSingleRegisterPDU 는 마스터의 FC06 요청 PDU 를 만든다(테스트 전용).
func writeSingleRegisterPDU(addr, value uint16) []byte {
	return []byte{modbus.FC06WriteSingleRegister,
		byte(addr >> 8), byte(addr), byte(value >> 8), byte(value)}
}

// ---------------------------------------------------------------------------
// AC-03 — 트랜스포트 재사용 + 연결 수립
// ---------------------------------------------------------------------------

func TestNewUpstreamTransport_ReusesModbusTransports(t *testing.T) {
	// TCP 백킹은 internal/agent/modbus 의 ModbusTCPTransport 를 재사용한다.
	tcpCfg := &BackingConfig{
		Transport: TransportTCP, Host: "127.0.0.1", Port: 1502,
		Mode: BackingModeDirect, Timeout: time.Second,
	}
	tr, err := newUpstreamTransport(tcpCfg, backedTestLogger())
	require.NoError(t, err)
	require.NotNil(t, tr)
	_, ok := tr.(*modbus.ModbusTCPTransport)
	assert.True(t, ok, "tcp 백킹은 *modbus.ModbusTCPTransport 를 재사용해야 한다")
	assert.False(t, tr.IsConnected(), "생성 직후에는 아직 연결되지 않아야 한다")

	// RTU 백킹은 ModbusRTUTransport 를 재사용한다.
	rtuCfg := &BackingConfig{
		Transport: TransportRTU,
		Serial:    SerialConfig{Port: "/dev/ttyUSB0", BaudRate: 9600, DataBits: 8, StopBits: 1, Parity: "none"},
		Mode:      BackingModeDirect, Timeout: time.Second,
	}
	tr2, err := newUpstreamTransport(rtuCfg, backedTestLogger())
	require.NoError(t, err)
	_, ok = tr2.(*modbus.ModbusRTUTransport)
	assert.True(t, ok, "rtu 백킹은 *modbus.ModbusRTUTransport 를 재사용해야 한다")
}

func TestBackedStore_ConnectLifecycle_ServingReady(t *testing.T) {
	inner := backedTestInner()
	fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) {
		return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{42}), nil
	}}
	bs := newBackedStore(inner, fake, &BackingConfig{Mode: BackingModeDirect, UnitID: 2, Timeout: time.Second})

	// 에이전트 Start 가 수행할 Connect 를 시뮬레이션한다(트랜스포트 소유는 상위, M5).
	require.NoError(t, fake.Connect(context.Background()))
	assert.True(t, fake.IsConnected())
	assert.Equal(t, 1, fake.connectCalls)

	// 연결 후 서빙이 동작한다.
	vals, err := bs.ReadHoldingRegisters(0, 1)
	require.NoError(t, err)
	assert.Equal(t, []uint16{42}, vals)
}

// ---------------------------------------------------------------------------
// AC-04 — Direct 읽기 = 실시간 조회 + 저장 + 전달
// ---------------------------------------------------------------------------

func TestBackedStore_DirectRead_QueriesStoresServes(t *testing.T) {
	inner := backedTestInner()
	fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) {
		return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{111, 222}), nil
	}}
	bs := newBackedStore(inner, fake, &BackingConfig{Mode: BackingModeDirect, UnitID: 7, Timeout: time.Second})
	handler := newRequestHandlerWithStore(bs, nil)

	// 마스터 FC03 읽기 → 서빙 경로 전체를 통과한다.
	resp := handler.HandleRequest(buildReadPDU(modbus.FC03ReadHoldingRegisters, 0, 2))
	require.NotNil(t, resp)
	assert.Equal(t, modbus.FC03ReadHoldingRegisters, resp[0])
	assert.Equal(t, byte(4), resp[1]) // byteCount = 2 regs * 2
	assert.Equal(t, []byte{0x00, 0x6F, 0x00, 0xDE}, resp[2:6])

	// upstream 이 즉시 조회되었고, 요청 unit_id 가 backing unit_id 로 전달되었다.
	assert.Equal(t, 1, fake.calls())
	assert.Equal(t, byte(7), fake.lastUnitID)

	// 조회 결과가 RegisterMap 에 저장되었다.
	stored, err := inner.ReadHoldingRegisters(0, 2)
	require.NoError(t, err)
	assert.Equal(t, []uint16{111, 222}, stored)
}

// AC-04 세부: 코일/이산입력/입력레지스터 direct 읽기도 조회+저장+서빙한다.
func TestBackedStore_DirectRead_AllReadAreas(t *testing.T) {
	// FC01 ReadCoils
	t.Run("coils", func(t *testing.T) {
		inner := backedTestInner()
		fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) {
			// 코일 3개: bit0=1,bit1=0,bit2=1 → 0x05, byteCount=1
			return []byte{modbus.FC01ReadCoils, 0x01, 0x05}, nil
		}}
		bs := newBackedStore(inner, fake, &BackingConfig{Mode: BackingModeDirect, UnitID: 2, Timeout: time.Second})
		vals, err := bs.ReadCoils(0, 3)
		require.NoError(t, err)
		assert.Equal(t, []bool{true, false, true}, vals)
		stored, _ := inner.ReadCoils(0, 3)
		assert.Equal(t, []bool{true, false, true}, stored)
	})

	// FC02 ReadDiscreteInputs
	t.Run("discrete_inputs", func(t *testing.T) {
		inner := backedTestInner()
		fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) {
			return []byte{modbus.FC02ReadDiscreteInputs, 0x01, 0x03}, nil
		}}
		bs := newBackedStore(inner, fake, &BackingConfig{Mode: BackingModeDirect, UnitID: 2, Timeout: time.Second})
		vals, err := bs.ReadDiscreteInputs(0, 2)
		require.NoError(t, err)
		assert.Equal(t, []bool{true, true}, vals)
		stored, _ := inner.ReadDiscreteInputs(0, 2)
		assert.Equal(t, []bool{true, true}, stored)
	})

	// FC04 ReadInputRegisters
	t.Run("input_registers", func(t *testing.T) {
		inner := backedTestInner()
		fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) {
			return regReadResp(modbus.FC04ReadInputRegisters, []uint16{7, 8}), nil
		}}
		bs := newBackedStore(inner, fake, &BackingConfig{Mode: BackingModeDirect, UnitID: 2, Timeout: time.Second})
		vals, err := bs.ReadInputRegisters(0, 2)
		require.NoError(t, err)
		assert.Equal(t, []uint16{7, 8}, vals)
		stored, _ := inner.ReadInputRegisters(0, 2)
		assert.Equal(t, []uint16{7, 8}, stored)
	})
}

// AC-05 세부: 코일 쓰기(FC05 단일 / FC15 다중)와 레지스터 다중(FC16) 전달 + 미러.
func TestBackedStore_DirectWrite_CoilsAndMulti(t *testing.T) {
	t.Run("single_coil_fc05", func(t *testing.T) {
		inner := backedTestInner()
		fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) { return writeEchoResp(pdu), nil }}
		bs := newBackedStore(inner, fake, &BackingConfig{Mode: BackingModeDirect, UnitID: 2, Timeout: time.Second})
		cs, err := bs.WriteCoils(1, []bool{true})
		require.NoError(t, err)
		require.NotNil(t, cs)
		assert.Equal(t, modbus.FC05WriteSingleCoil, fake.lastPDU[0])
		assert.Equal(t, []byte{0xFF, 0x00}, fake.lastPDU[3:5]) // true → 0xFF00
		v, _ := inner.ReadCoils(1, 1)
		assert.Equal(t, []bool{true}, v)
	})

	t.Run("multi_coils_fc15", func(t *testing.T) {
		inner := backedTestInner()
		fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) { return writeEchoResp(pdu), nil }}
		bs := newBackedStore(inner, fake, &BackingConfig{Mode: BackingModeDirect, UnitID: 2, Timeout: time.Second})
		cs, err := bs.WriteCoils(0, []bool{true, false, true})
		require.NoError(t, err)
		require.NotNil(t, cs)
		assert.Equal(t, modbus.FC15WriteMultipleCoils, fake.lastPDU[0])
		v, _ := inner.ReadCoils(0, 3)
		assert.Equal(t, []bool{true, false, true}, v)
	})

	t.Run("multi_registers_fc16", func(t *testing.T) {
		inner := backedTestInner()
		fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) { return writeEchoResp(pdu), nil }}
		bs := newBackedStore(inner, fake, &BackingConfig{Mode: BackingModeDirect, UnitID: 2, Timeout: time.Second})
		cs, err := bs.WriteHoldingRegisters(0, []uint16{10, 20, 30})
		require.NoError(t, err)
		require.NotNil(t, cs)
		assert.Equal(t, modbus.FC16WriteMultipleRegisters, fake.lastPDU[0])
		v, _ := inner.ReadHoldingRegisters(0, 3)
		assert.Equal(t, []uint16{10, 20, 30}, v)
	})

	t.Run("coil_write_disconnect_no_mutation", func(t *testing.T) {
		inner := backedTestInner()
		_, _ = inner.WriteCoils(1, []bool{true})
		fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) { return nil, errors.New("down") }}
		bs := newBackedStore(inner, fake, &BackingConfig{Mode: BackingModeDirect, UnitID: 2, Timeout: time.Second})
		cs, err := bs.WriteCoils(1, []bool{false})
		assert.ErrorIs(t, err, ErrGatewayTargetFailed)
		assert.Nil(t, cs)
		v, _ := inner.ReadCoils(1, 1)
		assert.Equal(t, []bool{true}, v, "실패한 코일 쓰기는 저장값을 변형하지 않아야 한다")
	})
}

// ---------------------------------------------------------------------------
// AC-05 — Direct 쓰기 = 전달 + 미러
// ---------------------------------------------------------------------------

func TestBackedStore_DirectWrite_ForwardsAndMirrors(t *testing.T) {
	inner := backedTestInner()
	fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) {
		return writeEchoResp(pdu), nil
	}}
	bs := newBackedStore(inner, fake, &BackingConfig{Mode: BackingModeDirect, UnitID: 2, Timeout: time.Second})
	handler := newRequestHandlerWithStore(bs, nil)

	// 마스터 FC06 쓰기 (addr=5, value=4242)
	resp, cs := handler.HandleWriteRequest(writeSingleRegisterPDU(5, 4242), "")
	require.NotNil(t, resp)
	require.NotNil(t, cs, "성공 쓰기는 ChangeSet 을 반환해야 한다")

	// upstream 으로 FC06 이 전달되었다.
	assert.Equal(t, modbus.FC06WriteSingleRegister, fake.lastPDU[0])
	assert.Equal(t, 1, fake.calls())

	// RegisterMap 에 미러 저장되었다.
	v, err := inner.ReadHoldingRegisters(5, 1)
	require.NoError(t, err)
	assert.Equal(t, []uint16{4242}, v)
}

// ---------------------------------------------------------------------------
// AC-06 — Direct 끊김 = 즉각 0x0B (+ 쓰기 실패 시 RegisterMap 미변형)
// ---------------------------------------------------------------------------

func TestBackedStore_DirectRead_Disconnect_Returns0x0B(t *testing.T) {
	inner := backedTestInner()
	fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) {
		return nil, errors.New("upstream unreachable")
	}}
	bs := newBackedStore(inner, fake, &BackingConfig{Mode: BackingModeDirect, UnitID: 2, Timeout: time.Second})
	handler := newRequestHandlerWithStore(bs, nil)

	resp := handler.HandleRequest(buildReadPDU(modbus.FC03ReadHoldingRegisters, 0, 2))
	// FC03 예외 = 0x03|0x80 = 0x83, code = 0x0B
	assert.Equal(t, []byte{0x83, ExceptionGatewayTargetFailed}, resp)
}

func TestBackedStore_DirectWrite_Disconnect_Returns0x0B_NoMutation(t *testing.T) {
	inner := backedTestInner()
	// 사전값 설정
	_, _ = inner.WriteHoldingRegisters(5, []uint16{7})

	fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) {
		return nil, errors.New("upstream unreachable")
	}}
	bs := newBackedStore(inner, fake, &BackingConfig{Mode: BackingModeDirect, UnitID: 2, Timeout: time.Second})
	handler := newRequestHandlerWithStore(bs, nil)

	resp, cs := handler.HandleWriteRequest(writeSingleRegisterPDU(5, 4242), "")
	// FC06 예외 = 0x86, code 0x0B
	assert.Equal(t, []byte{0x86, ExceptionGatewayTargetFailed}, resp)
	assert.Nil(t, cs)

	// 실패한 쓰기는 RegisterMap 을 변형하지 않는다.
	v, err := inner.ReadHoldingRegisters(5, 1)
	require.NoError(t, err)
	assert.Equal(t, []uint16{7}, v, "실패한 쓰기는 저장값을 변형하지 않아야 한다")
}

// AC-06 세부: upstream 지연 시 timeout 데드라인으로 대기 없이 즉각 반환한다.
func TestBackedStore_DirectRead_SlowUpstream_TimesOutFast(t *testing.T) {
	inner := backedTestInner()
	fake := &fakeTransport{
		delay: 500 * time.Millisecond, // upstream 이 느리게 응답
		resp: func(pdu []byte) ([]byte, error) {
			return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{1}), nil
		},
	}
	bs := newBackedStore(inner, fake, &BackingConfig{Mode: BackingModeDirect, UnitID: 2, Timeout: 20 * time.Millisecond})

	start := time.Now()
	_, err := bs.ReadHoldingRegisters(0, 1)
	elapsed := time.Since(start)

	assert.ErrorIs(t, err, ErrGatewayTargetFailed)
	assert.Less(t, elapsed, 400*time.Millisecond, "timeout 데드라인으로 upstream delay 이전에 반환해야 한다")
}

// ---------------------------------------------------------------------------
// AC-11 — 연결 종료 수명 (기본: 트랜스포트 Close 가능 + 핸들 접근)
// ---------------------------------------------------------------------------

func TestBackedStore_Close_Lifecycle(t *testing.T) {
	inner := backedTestInner()
	fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) { return nil, nil }}
	bs := newBackedStore(inner, fake, &BackingConfig{Mode: BackingModeDirect, UnitID: 2, Timeout: time.Second})

	require.NoError(t, fake.Connect(context.Background()))
	require.True(t, fake.IsConnected())

	// remove_device / Stop 시 상위(M5)가 접근할 트랜스포트 핸들이 노출되어야 한다.
	got, ok := bs.transport.(*fakeTransport)
	require.True(t, ok)
	assert.Same(t, fake, got, "backedStore 는 M5 가 관리할 트랜스포트 핸들을 보유해야 한다")

	require.NoError(t, got.Close())
	assert.False(t, fake.IsConnected())
	assert.Equal(t, 1, fake.closeCalls)
}

// ---------------------------------------------------------------------------
// AC-12 — 0x0B 예외 매핑 + 순수 slave 회귀 없음
// ---------------------------------------------------------------------------

func TestExceptionMapping_BackingVsPureSlave(t *testing.T) {
	// 순수 slave: 범위 밖 읽기 → 기존과 동일하게 0x02 (회귀 없음).
	plainRM := NewRegisterMap(RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{{StartAddress: 0, Count: 10}},
	})
	plainHandler := NewRequestHandler(plainRM, nil)
	respSlave := plainHandler.HandleRequest(buildReadPDU(modbus.FC03ReadHoldingRegisters, 5000, 1))
	assert.Equal(t, []byte{0x83, modbus.ExceptionIllegalDataAddress}, respSlave,
		"순수 slave 범위 밖 오류는 여전히 0x02 여야 한다")

	// 백킹: upstream 실패 → 0x0B.
	inner := backedTestInner()
	fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) {
		return nil, errors.New("unreachable")
	}}
	bs := newBackedStore(inner, fake, &BackingConfig{Mode: BackingModeDirect, UnitID: 2, Timeout: time.Second})
	backedHandler := newRequestHandlerWithStore(bs, nil)
	respBacked := backedHandler.HandleRequest(buildReadPDU(modbus.FC03ReadHoldingRegisters, 0, 1))
	assert.Equal(t, []byte{0x83, ExceptionGatewayTargetFailed}, respBacked,
		"백킹 실패는 0x0B 로 매핑되어야 한다")
}

// AC-12 세부: upstream 이 MODBUS 예외를 반환해도 게이트웨이 실패(0x0B)로 매핑한다.
func TestBackedStore_UpstreamException_MapsTo0x0B(t *testing.T) {
	inner := backedTestInner()
	fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) {
		// upstream 예외 응답: [fc|0x80, exCode]
		return []byte{pdu[0] | 0x80, modbus.ExceptionSlaveDeviceFailure}, nil
	}}
	bs := newBackedStore(inner, fake, &BackingConfig{Mode: BackingModeDirect, UnitID: 2, Timeout: time.Second})

	_, err := bs.ReadHoldingRegisters(0, 1)
	assert.ErrorIs(t, err, ErrGatewayTargetFailed)
}

// ---------------------------------------------------------------------------
// AC-01 — 하위 호환 특성화: 순수 slave 서빙 바이트 동일
// ---------------------------------------------------------------------------

// TestBackwardCompat_PureSlave_ByteIdentical 는 백킹 도입(mapStoreError 리팩터)이
// 순수 slave 경로의 응답 바이트를 변경하지 않았음을 특성화한다.
func TestBackwardCompat_PureSlave_ByteIdentical(t *testing.T) {
	rm := NewRegisterMap(RegisterMapConfig{
		Coils:            []*RegisterAreaConfig{{StartAddress: 0, Count: 10}},
		HoldingRegisters: []*RegisterAreaConfig{{StartAddress: 0, Count: 10}},
	})
	_, _ = rm.WriteHoldingRegisters(0, []uint16{0x1234, 0x5678})
	_, _ = rm.WriteCoils(0, []bool{true, false, true})
	handler := NewRequestHandler(rm, nil)

	// 정상 읽기: [fc][byteCount][data...]
	assert.Equal(t,
		[]byte{0x03, 0x04, 0x12, 0x34, 0x56, 0x78},
		handler.HandleRequest(buildReadPDU(modbus.FC03ReadHoldingRegisters, 0, 2)))

	// 범위 밖 읽기 → 0x02 (Illegal Data Address, 불변)
	assert.Equal(t,
		[]byte{0x83, modbus.ExceptionIllegalDataAddress},
		handler.HandleRequest(buildReadPDU(modbus.FC03ReadHoldingRegisters, 500, 1)))

	// 수량 0 → 0x03 (Illegal Data Value, 불변)
	assert.Equal(t,
		[]byte{0x83, modbus.ExceptionIllegalDataValue},
		handler.HandleRequest(buildReadPDU(modbus.FC03ReadHoldingRegisters, 0, 0)))

	// 미지원 FC → 0x01 (Illegal Function, 불변)
	assert.Equal(t,
		[]byte{0xFF, modbus.ExceptionIllegalFunction},
		handler.HandleRequest([]byte{0x7F, 0x00, 0x00, 0x00, 0x01}))

	// 쓰기 성공 echo (불변)
	wresp, cs := handler.HandleWriteRequest(writeSingleRegisterPDU(1, 0xABCD), "")
	assert.Equal(t, writeSingleRegisterPDU(1, 0xABCD), wresp)
	require.NotNil(t, cs)

	// 범위 밖 쓰기 → 0x02 (불변)
	wresp2, cs2 := handler.HandleWriteRequest(writeSingleRegisterPDU(500, 1), "")
	assert.Equal(t, []byte{0x86, modbus.ExceptionIllegalDataAddress}, wresp2)
	assert.Nil(t, cs2)
}

// ---------------------------------------------------------------------------
// Indirect store-level seam (M4 배선 대비 — 폴러 goroutine 은 M4)
// ---------------------------------------------------------------------------

func TestBackedStore_Indirect_StaleSeam(t *testing.T) {
	inner := backedTestInner()
	fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) { return nil, nil }}
	bs := newBackedStore(inner, fake, &BackingConfig{
		Mode: BackingModeIndirect, UnitID: 2, PollInterval: time.Second, Timeout: 50 * time.Millisecond,
	})

	// 폴 성공 이력이 없으면(lastOK==0) stale → 0x0B, upstream 을 호출하지 않는다.
	_, err := bs.ReadHoldingRegisters(0, 1)
	assert.ErrorIs(t, err, ErrGatewayTargetFailed)
	assert.Equal(t, 0, fake.calls(), "indirect 읽기는 upstream 을 직접 호출하지 않는다")

	// 폴러(M4)가 저장값 갱신 + 성공 시각 기록을 시뮬레이션한다.
	_, _ = inner.WriteHoldingRegisters(0, []uint16{999})
	bs.recordPollSuccess(time.Now())

	// timeout 이내 → 저장값 서빙(stale 허용).
	vals, err := bs.ReadHoldingRegisters(0, 1)
	require.NoError(t, err)
	assert.Equal(t, []uint16{999}, vals)

	// timeout 초과 시뮬레이션 → 0x0B.
	bs.recordPollSuccess(time.Now().Add(-time.Second))
	_, err = bs.ReadHoldingRegisters(0, 1)
	assert.ErrorIs(t, err, ErrGatewayTargetFailed)
}

// PDU 파싱/조립 헬퍼의 방어적 분기 커버.
func TestUpstreamPDUHelpers_DefensiveBranches(t *testing.T) {
	// parseUpstreamReadResponse: 손상 프레임(짧음/byteCount 초과)은 게이트웨이 실패.
	_, err := parseUpstreamReadResponse([]byte{0x03})
	assert.ErrorIs(t, err, ErrGatewayTargetFailed)
	_, err = parseUpstreamReadResponse([]byte{0x03, 0x04, 0x00}) // byteCount=4 인데 data 부족
	assert.ErrorIs(t, err, ErrGatewayTargetFailed)

	// parseUpstreamWriteResponse: 빈 응답은 게이트웨이 실패.
	assert.ErrorIs(t, parseUpstreamWriteResponse([]byte{}), ErrGatewayTargetFailed)
	// 정상 에코는 nil.
	assert.NoError(t, parseUpstreamWriteResponse([]byte{0x06, 0x00, 0x05, 0x10, 0x92}))

	// newUpstreamTransport: 미지원 transport 는 오류.
	_, err = newUpstreamTransport(&BackingConfig{Transport: "udp", Mode: BackingModeDirect}, backedTestLogger())
	assert.ErrorIs(t, err, ErrInvalidBackingConfig)
}

// ---------------------------------------------------------------------------
// AC-13(부분) — direct backedStore vs 서빙 동시 접근 race 클린
// ---------------------------------------------------------------------------

func TestBackedStore_ConcurrentDirect_RaceClean(t *testing.T) {
	inner := backedTestInner()
	fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) {
		if pdu[0] == modbus.FC03ReadHoldingRegisters {
			return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{5}), nil
		}
		return writeEchoResp(pdu), nil
	}}
	bs := newBackedStore(inner, fake, &BackingConfig{Mode: BackingModeDirect, UnitID: 2, Timeout: time.Second})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = bs.ReadHoldingRegisters(0, 4)
		}()
		go func() {
			defer wg.Done()
			_, _ = bs.WriteHoldingRegisters(0, []uint16{1, 2})
		}()
	}
	wg.Wait()
}
