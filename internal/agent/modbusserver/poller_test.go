package modbusserver

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	modbus "github.com/xtra/xflow/internal/agent/modbus"
)

// ---------------------------------------------------------------------------
// M4 — Indirect 폴러 (REQ-MODBUS-010-04)
// ---------------------------------------------------------------------------

// indirectBackingConfig 는 indirect 폴러 테스트용 BackingConfig 를 만든다.
func indirectBackingConfig(interval, timeout time.Duration) *BackingConfig {
	return &BackingConfig{
		Mode:         BackingModeIndirect,
		UnitID:       2,
		PollInterval: interval,
		Timeout:      timeout,
	}
}

// pollTargetsHolding 는 holding 0..count 단일 영역 폴 대상을 만든다.
func pollTargetsHolding(count uint16) []pollTarget {
	return []pollTarget{{fc: modbus.FC03ReadHoldingRegisters, start: 0, count: count}}
}

// AC-07 — 폴러 pollOnce 가 upstream 을 조회하여 RegisterMap 을 갱신하고 성공 시각을 기록한다.
func TestDevicePoller_PollOnce_UpdatesRegisterMapAndRecordsSuccess(t *testing.T) {
	inner := backedTestInner()
	fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) {
		return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{11, 22, 33}), nil
	}}
	bs := newBackedStore(inner, fake, indirectBackingConfig(time.Second, 50*time.Millisecond))
	p := &devicePoller{store: bs, targets: pollTargetsHolding(3)}

	// 폴 이전에는 stale(성공 이력 없음).
	assert.True(t, bs.isStale())

	require.NoError(t, p.pollOnce(time.Now()))

	// upstream 조회 결과가 RegisterMap 에 저장되었다.
	stored, err := inner.ReadHoldingRegisters(0, 3)
	require.NoError(t, err)
	assert.Equal(t, []uint16{11, 22, 33}, stored)

	// 마지막 성공 시각이 기록되어 더 이상 stale 이 아니다.
	assert.False(t, bs.isStale())
	assert.Equal(t, 1, fake.calls())
}

// AC-07 세부: 폴러가 모든 영역(coils/di/holding/input)을 대응 FC 로 폴링한다.
func TestDevicePoller_PollOnce_AllAreas(t *testing.T) {
	inner := backedTestInner()
	fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) {
		switch pdu[0] {
		case modbus.FC01ReadCoils:
			return []byte{modbus.FC01ReadCoils, 0x01, 0x05}, nil // bit0=1,bit1=0,bit2=1
		case modbus.FC02ReadDiscreteInputs:
			return []byte{modbus.FC02ReadDiscreteInputs, 0x01, 0x03}, nil // bit0=1,bit1=1
		case modbus.FC03ReadHoldingRegisters:
			return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{100, 200}), nil
		case modbus.FC04ReadInputRegisters:
			return regReadResp(modbus.FC04ReadInputRegisters, []uint16{7, 8}), nil
		}
		return nil, errors.New("unexpected fc")
	}}
	bs := newBackedStore(inner, fake, indirectBackingConfig(time.Second, time.Second))
	targets := pollTargetsFromConfig(RegisterMapConfig{
		Coils:            []*RegisterAreaConfig{{StartAddress: 0, Count: 3}},
		DiscreteInputs:   []*RegisterAreaConfig{{StartAddress: 0, Count: 2}},
		HoldingRegisters: []*RegisterAreaConfig{{StartAddress: 0, Count: 2}},
		InputRegisters:   []*RegisterAreaConfig{{StartAddress: 0, Count: 2}},
	})
	p := &devicePoller{store: bs, targets: targets}

	require.NoError(t, p.pollOnce(time.Now()))

	c, _ := inner.ReadCoils(0, 3)
	assert.Equal(t, []bool{true, false, true}, c)
	di, _ := inner.ReadDiscreteInputs(0, 2)
	assert.Equal(t, []bool{true, true}, di)
	hr, _ := inner.ReadHoldingRegisters(0, 2)
	assert.Equal(t, []uint16{100, 200}, hr)
	ir, _ := inner.ReadInputRegisters(0, 2)
	assert.Equal(t, []uint16{7, 8}, ir)
	assert.Equal(t, 4, fake.calls())
}

// pollTargetsFromConfig 가 공유 세그먼트를 제외하고 로컬 세그먼트만 대상으로 만든다.
func TestPollTargetsFromConfig_ExcludesSharedAndEmpty(t *testing.T) {
	targets := pollTargetsFromConfig(RegisterMapConfig{
		HoldingRegisters: []*RegisterAreaConfig{
			{StartAddress: 0, Count: 10},
			{StartAddress: 100, Count: 5, IsShared: true, SharedAddress: 0}, // 제외
		},
		Coils: []*RegisterAreaConfig{{StartAddress: 0, Count: 8}},
	})
	require.Len(t, targets, 2)
	assert.Equal(t, pollTarget{fc: modbus.FC01ReadCoils, start: 0, count: 8}, targets[0])
	assert.Equal(t, pollTarget{fc: modbus.FC03ReadHoldingRegisters, start: 0, count: 10}, targets[1])
}

// AC-08 — Indirect 읽기는 upstream 을 호출하지 않고 폴러가 갱신한 저장값으로 서빙한다.
func TestDevicePoller_IndirectRead_ServesStoredValue(t *testing.T) {
	inner := backedTestInner()
	fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) {
		return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{555}), nil
	}}
	bs := newBackedStore(inner, fake, indirectBackingConfig(time.Second, time.Second))
	p := &devicePoller{store: bs, targets: pollTargetsHolding(1)}

	// 폴러가 갱신했다고 가정.
	require.NoError(t, p.pollOnce(time.Now()))
	callsAfterPoll := fake.calls()

	// 마스터 읽기 → 저장값 서빙, upstream 추가 호출 없음.
	vals, err := bs.ReadHoldingRegisters(0, 1)
	require.NoError(t, err)
	assert.Equal(t, []uint16{555}, vals)
	assert.Equal(t, callsAfterPoll, fake.calls(), "indirect 읽기는 upstream 을 직접 호출하지 않아야 한다")
}

// AC-09 — stale 경계: timeout 이내(경계 == timeout 포함)면 서빙, 초과면 0x0B.
func TestDevicePoller_Indirect_StaleBoundary(t *testing.T) {
	inner := backedTestInner()
	fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) {
		return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{42}), nil
	}}
	timeout := 100 * time.Millisecond
	bs := newBackedStore(inner, fake, indirectBackingConfig(time.Second, timeout))
	p := &devicePoller{store: bs, targets: pollTargetsHolding(1)}
	require.NoError(t, p.pollOnce(time.Now()))

	// 경계 <= timeout: 마지막 성공이 정확히 timeout 만큼 과거여도 서빙(stale 허용).
	bs.recordPollSuccess(time.Now().Add(-timeout + 5*time.Millisecond))
	vals, err := bs.ReadHoldingRegisters(0, 1)
	require.NoError(t, err, "timeout 이내면 stale 저장값을 서빙해야 한다")
	assert.Equal(t, []uint16{42}, vals)

	// 초과: 마지막 성공이 timeout 보다 더 과거 → 0x0B.
	bs.recordPollSuccess(time.Now().Add(-2 * timeout))
	_, err = bs.ReadHoldingRegisters(0, 1)
	assert.ErrorIs(t, err, ErrGatewayTargetFailed, "timeout 초과면 0x0B 를 반환해야 한다")
}

// AC-09 세부: 폴이 실패하면 lastOK 를 갱신하지 않아 staleness 가 자란다.
func TestDevicePoller_PollFailure_DoesNotRefreshLastOK(t *testing.T) {
	inner := backedTestInner()
	// 최초 성공 후 이후 폴은 실패하도록 전환하는 페이크.
	var mu sync.Mutex
	fail := false
	fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		if fail {
			return nil, errors.New("upstream down")
		}
		return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{1}), nil
	}}
	bs := newBackedStore(inner, fake, indirectBackingConfig(time.Second, time.Second))
	p := &devicePoller{store: bs, targets: pollTargetsHolding(1)}

	require.NoError(t, p.pollOnce(time.Now()))
	before := bs.lastOK.Load()

	mu.Lock()
	fail = true
	mu.Unlock()

	// 폴 실패는 오류를 반환하고 lastOK 를 갱신하지 않는다.
	err := p.pollOnce(time.Now().Add(time.Hour))
	assert.ErrorIs(t, err, ErrGatewayTargetFailed)
	assert.Equal(t, before, bs.lastOK.Load(), "폴 실패 시 마지막 성공 시각은 유지되어야 한다")
}

// pollOnce 는 폴 대상이 없으면 성공을 기록하지 않는다(비정상 설정 방어).
func TestDevicePoller_PollOnce_NoTargets_NoRecord(t *testing.T) {
	inner := backedTestInner()
	fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) { return nil, nil }}
	bs := newBackedStore(inner, fake, indirectBackingConfig(time.Second, time.Second))
	p := &devicePoller{store: bs, targets: nil}

	require.NoError(t, p.pollOnce(time.Now()))
	assert.True(t, bs.isStale(), "폴 대상이 없으면 stale 로 유지되어야 한다")
	assert.Equal(t, 0, fake.calls())
}

// AC-07/AC-11 — 실행 중인 폴러 goroutine: 주기적 갱신 + 취소 시 누수 없이 종료.
func TestDevicePoller_Running_UpdatesThenStopsCleanly(t *testing.T) {
	inner := backedTestInner()
	fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) {
		return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{7}), nil
	}}
	bs := newBackedStore(inner, fake, indirectBackingConfig(5*time.Millisecond, time.Second))

	p := startPoller(context.Background(), bs, pollTargetsHolding(1), 5*time.Millisecond, backedTestLogger())

	// 폴러가 최소 1회 폴링하여 RegisterMap 을 갱신할 때까지 대기한다.
	require.Eventually(t, func() bool {
		v, err := inner.ReadHoldingRegisters(0, 1)
		return err == nil && len(v) == 1 && v[0] == 7 && !bs.isStale()
	}, time.Second, 2*time.Millisecond, "폴러가 poll_interval 경과 후 RegisterMap 을 갱신해야 한다")

	// 취소 → 폴러 goroutine 이 종료되어야 한다(누수 없음).
	// stop() 은 done 채널을 닫힘까지 대기하므로, 반환 시점에 goroutine 은 이미 종료되었다.
	p.stop()
	select {
	case <-p.done:
		// done 이 닫힘 = goroutine 종료 확정(누수 없음).
	default:
		t.Fatal("stop 후 폴러 goroutine 이 종료되지 않았다(done 미close)")
	}
}

// AC-09(running) — 실행 중 폴러가 계속 실패하면 stale 로 유지되며 루프는 살아있다가
// 취소 시 깨끗이 종료된다(run 오류 분기 포함).
func TestDevicePoller_Running_UpstreamDown_StaysStale(t *testing.T) {
	inner := backedTestInner()
	fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) {
		return nil, errors.New("upstream down")
	}}
	bs := newBackedStore(inner, fake, indirectBackingConfig(2*time.Millisecond, 50*time.Millisecond))
	p := startPoller(context.Background(), bs, pollTargetsHolding(1), 2*time.Millisecond, backedTestLogger())

	// 폴이 수 회 실패하도록 대기하면서, 실패 폴은 lastOK 를 갱신하지 않아 stale 로 남는다.
	require.Eventually(t, func() bool {
		return fake.calls() >= 3
	}, time.Second, 2*time.Millisecond, "폴러는 실패해도 계속 폴링을 시도해야 한다")
	assert.True(t, bs.isStale(), "폴이 계속 실패하면 stale 로 유지되어야 한다")

	p.stop()
	select {
	case <-p.done:
	default:
		t.Fatal("stop 후 폴러가 종료되지 않았다")
	}
}

// stop() 은 여러 번 호출해도 안전하다(멱등).
func TestDevicePoller_Stop_Idempotent(t *testing.T) {
	inner := backedTestInner()
	fake := &fakeTransport{resp: func(pdu []byte) ([]byte, error) {
		return regReadResp(modbus.FC03ReadHoldingRegisters, []uint16{1}), nil
	}}
	bs := newBackedStore(inner, fake, indirectBackingConfig(time.Millisecond, time.Second))
	p := startPoller(context.Background(), bs, pollTargetsHolding(1), time.Millisecond, backedTestLogger())
	p.stop()
	assert.NotPanics(t, func() { p.stop() }, "stop 은 중복 호출에도 안전해야 한다")
}
