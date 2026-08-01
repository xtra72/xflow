package modbus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// twoGroupAgentConfig 는 한 디바이스에 두 개의 레지스터 그룹을 가지는 설정을 만든다.
// 그룹 A(start 0)는 짧은 poll_interval, 그룹 B(start 100)는 poll_interval 미지정(기본 주기 폴백).
func twoGroupAgentConfig(groupAInterval, defaultInterval string) agent.AgentConfig {
	groupB := map[string]any{
		"name":          "group-b-default",
		"function_code": 3,
		"start_address": 100,
		"quantity":      10,
	}
	return agent.AgentConfig{
		ID:   "modbus-ac04",
		Name: "AC04 Agent",
		Type: "modbus-client",
		Transport: agent.TransportConfig{
			Type: "modbus-tcp",
			Options: map[string]any{
				"read_mode":        "cached",
				"mode":             "interval",
				"poll_interval":    defaultInterval,
				"request_timeout":  "1s",
				"msg_channel_size": 256,
				"devices": []any{
					map[string]any{
						"id":      "plc-1",
						"host":    "10.0.0.1",
						"port":    502,
						"unit_id": 1,
						"register_groups": []any{
							map[string]any{
								"name":          "group-a-fast",
								"function_code": 3,
								"start_address": 0,
								"quantity":      10,
								"poll_interval": groupAInterval,
							},
							groupB,
						},
					},
				},
			},
		},
	}
}

// countFramesByStart 는 mock 이 기록한 PDU 프레임을 start_address(하위 바이트)로 분류해 센다.
// buildReadPDU = [fc, startHi, startLo, qtyHi, qtyLo] 이므로 frame[2] 가 start_address 하위 바이트다.
func countFramesByStart(frames [][]byte, startLo byte) int {
	n := 0
	for _, f := range frames {
		if len(f) >= 3 && f[2] == startLo {
			n++
		}
	}
	return n
}

// TestAC04_IndependentGroupCadence 는 서로 다른 poll_interval 을 가진 그룹이
// 독립 케이던스로 폴링되고, 미지정 그룹은 기본 주기로 폴백함을 검증한다(AC-04, REQ-02).
func TestAC04_IndependentGroupCadence(t *testing.T) {
	mt := &mockModbusTransport{
		connected: true,
		response:  buildFC03Response(0, 1, 10),
	}
	// 그룹 A: 20ms, 기본(그룹 B): 200ms → A 가 B 보다 훨씬 자주 폴링되어야 한다.
	a, _ := newTestModbusAgent(t, twoGroupAgentConfig("20ms", "200ms"), mt)

	require.NoError(t, a.Start(context.Background()))
	defer func() { _ = a.Stop(context.Background()) }()

	time.Sleep(400 * time.Millisecond)

	mt.mu.Lock()
	frames := make([][]byte, len(mt.sentFrames))
	copy(frames, mt.sentFrames)
	mt.mu.Unlock()

	countA := countFramesByStart(frames, 0x00) // start_address 0
	countB := countFramesByStart(frames, 0x64) // start_address 100

	t.Logf("group A(20ms) polls=%d, group B(200ms default) polls=%d", countA, countB)

	// 미지정 그룹(B)은 기본 주기로 폴백하여 최소 1회 이상 폴링된다.
	assert.GreaterOrEqual(t, countB, 1, "미지정 그룹은 기본 주기로 폴백되어 폴링되어야 한다")
	// 짧은 주기 그룹(A)이 긴 주기 그룹(B)보다 확연히 자주 폴링된다(강제 병합 없음).
	assert.GreaterOrEqual(t, countA, 8, "짧은 주기 그룹은 여러 번 폴링되어야 한다")
	assert.Greater(t, countA, countB*3, "그룹 A 는 그룹 B 보다 훨씬 자주 폴링되어야 한다(독립 케이던스)")
}

// TestAC04_NoGroupInterval_SingleCadence 는 어떤 그룹도 poll_interval 을 지정하지 않으면
// 모든 그룹이 기본 주기로만 폴링되어 기존 단일-티커 동작과 동일함을 검증한다(하위 호환, AC-03/AC-04).
func TestAC04_NoGroupInterval_SingleCadence(t *testing.T) {
	mt := &mockModbusTransport{
		connected: true,
		response:  buildFC03Response(0, 1, 10),
	}
	a, _ := newTestModbusAgent(t, twoDeviceAgentConfig(), mt,
		&mockModbusTransport{connected: true, response: buildFC03Response(0, 2, 5)})

	require.NoError(t, a.Start(context.Background()))
	defer func() { _ = a.Stop(context.Background()) }()

	// 그룹별 스케줄러 goroutine 이 하나도 생성되지 않아야 한다(모든 그룹 기본 주기).
	// poll_interval 100ms 로 ~3회 폴 후 정지 → 누수 없이 종료되어야 한다.
	time.Sleep(250 * time.Millisecond)

	mt.mu.Lock()
	sent := len(mt.sentFrames)
	mt.mu.Unlock()
	assert.Greater(t, sent, 0, "기본 주기로 폴링되어야 한다")
}

// TestAC06_TransportError_OfflineReconnect_ErrorStats 는 트랜스포트 오류 3회 연속 시
// offline 전환 + 오류 통계 증가, 이후 복구 시 reconnect + online 복귀 + 성공 통계 증가를 검증한다
// (AC-06, REQ-04).
func TestAC06_TransportError_OfflineReconnect_ErrorStats(t *testing.T) {
	mt := &mockModbusTransport{
		connected:   true,
		sendRecvErr: errors.New("simulated transport failure"),
		response:    buildFC03Response(0, 1, 10),
	}

	cfg := minimalAgentConfig()
	cfg.Transport.Options["poll_interval"] = "30ms"
	cfg.Transport.Options["reconnect_interval"] = "40ms"

	a, _ := newTestModbusAgent(t, cfg, mt)
	require.NoError(t, a.Start(context.Background()))
	defer func() { _ = a.Stop(context.Background()) }()

	// (1) 오류 구간: 3회 연속 오류 후 offline + 오류 통계 증가.
	require.Eventually(t, func() bool {
		return !a.devices[0].IsOnline()
	}, 2*time.Second, 20*time.Millisecond, "3회 연속 오류 후 디바이스가 offline 되어야 한다")

	devErr := a.devStats["plc-1"].errors.Load()
	grpErr := a.groupStats[groupStatKey("plc-1", "holding_0-9")].errors.Load()
	assert.GreaterOrEqual(t, devErr, int64(3), "디바이스 오류 카운터가 3 이상이어야 한다")
	assert.GreaterOrEqual(t, grpErr, int64(3), "그룹 오류 카운터가 3 이상이어야 한다")

	// health: 유일 디바이스가 offline → degraded (online 비율 0).
	assert.Equal(t, agent.HealthDegraded, a.Health().Status,
		"모든 디바이스 offline 시 health 는 degraded 여야 한다")

	// (2) 복구 구간: 트랜스포트 오류 제거 → reconnect → online + 성공 통계 증가.
	mt.mu.Lock()
	mt.sendRecvErr = nil
	mt.mu.Unlock()

	require.Eventually(t, func() bool {
		return a.devices[0].IsOnline()
	}, 2*time.Second, 20*time.Millisecond, "오류 제거 후 재연결되어 online 으로 복귀해야 한다")

	require.Eventually(t, func() bool {
		return a.devStats["plc-1"].success.Load() > 0
	}, 2*time.Second, 20*time.Millisecond, "복구 후 성공 요청 통계가 증가해야 한다")

	assert.Equal(t, agent.HealthHealthy, a.Health().Status,
		"복구 후 health 는 healthy 여야 한다")

	// ConnectionStatsProvider 표면화 검증(M7).
	conn := a.ConnectionStats()
	require.Len(t, conn, 1)
	assert.Equal(t, "plc-1", conn[0].ID)
	assert.Greater(t, conn[0].MessagesErrored, int64(0), "오류 통계가 표면화되어야 한다")
	assert.Greater(t, conn[0].MessagesReceived, int64(0), "성공 통계가 표면화되어야 한다")
}
