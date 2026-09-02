package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// modbus-read periodic(source) 모드 테스트
// ---------------------------------------------------------------------------
// setupReadNode(modbus_cmdset_test.go)는 Configure+Init 후 mock/agentType을 주입한다.
// periodic 모드 goroutine 은 agent 주입 전 틱은 skip 하고 주입 후 틱부터 emit 하므로
// 짧은 interval + SourceCh 수신 대기로 검증한다.

// recvSource 는 SourceCh 에서 최대 d 동안 1건을 수신한다.
func recvSource(t *testing.T, ch <-chan message.Message, d time.Duration) (message.Message, bool) {
	t.Helper()
	select {
	case m, ok := <-ch:
		return m, ok
	case <-time.After(d):
		return nil, false
	}
}

// (a) periodic 모드: poll_interval>0 + config command_set 있음 → SourceCh 로 주기 emit.
func TestModbusReadPoll_Periodic_EmitsOnSourceCh(t *testing.T) {
	mock := &mockModbusAgent{processResp: []byte(`{"values":[1,2,3]}`)}
	cfg := map[string]any{
		"agent_ref":     "mb-server",
		"poll_interval": "10ms",
		"command_set": []any{
			map[string]any{"area": "holding_registers", "address": float64(0), "count": float64(3)},
		},
	}
	n := setupReadNode(t, cfg, agentTypeServer, mock)
	defer func() { _ = n.Shutdown(context.Background()) }()

	// source 모드 활성 → SourceCh 는 non-nil
	ch := n.SourceCh()
	require.NotNil(t, ch, "periodic 모드에서 SourceCh 는 non-nil 이어야 한다")

	msg, ok := recvSource(t, ch, 2*time.Second)
	require.True(t, ok, "periodic 틱이 최소 1건 emit 해야 한다")
	require.NotNil(t, msg)

	success, _ := msg.Payload().Get("success")
	assert.Equal(t, true, success)
	assert.Equal(t, agentTypeServer, mustGet(t, msg, "agent_type"))

	values := resultEntries(t, msg, "values")
	require.Len(t, values, 1)
	assert.Equal(t, "holding_registers", values[0]["area"])
	_, hasRaw := values[0]["raw"]
	assert.True(t, hasRaw, "server uint16 읽기는 raw 로 담긴다")

	ts, hasTs := msg.Payload().Get("timestamp")
	require.True(t, hasTs, "periodic emit 은 epoch-ms timestamp 를 포함한다")
	_, tsIsInt64 := ts.(int64)
	assert.True(t, tsIsInt64, "timestamp 는 int64(epoch-ms)")
}

// (b) poll_interval 설정됐지만 config command_set 비어있음 → source 모드 아님.
func TestModbusReadPoll_NoSource_WhenCommandSetEmpty(t *testing.T) {
	mock := &mockModbusAgent{processResp: []byte(`{"values":[1]}`)}
	cfg := map[string]any{
		"agent_ref":     "mb-server",
		"poll_interval": "10ms",
		// command_set 없음
	}
	n := setupReadNode(t, cfg, agentTypeServer, mock)
	defer func() { _ = n.Shutdown(context.Background()) }()

	assert.Nil(t, n.SourceCh(), "command_set 비어있으면 SourceCh 는 nil")
	assert.False(t, n.sourceMode, "source 모드 비활성")

	// nil 채널 → 어떤 emit 도 없음(수신 시도는 타임아웃).
	_, ok := recvSource(t, n.SourceCh(), 100*time.Millisecond)
	assert.False(t, ok, "source 모드가 아니면 emit 없음")
}

// (c) poll_interval 미설정 → source 모드 아님(command_set 있어도).
func TestModbusReadPoll_NoSource_WhenPollIntervalUnset(t *testing.T) {
	mock := &mockModbusAgent{processResp: []byte(`{"values":[1]}`)}
	cfg := map[string]any{
		"agent_ref": "mb-server",
		"command_set": []any{
			map[string]any{"area": "holding_registers", "address": float64(0), "count": float64(1)},
		},
		// poll_interval 없음
	}
	n := setupReadNode(t, cfg, agentTypeServer, mock)
	defer func() { _ = n.Shutdown(context.Background()) }()

	assert.Nil(t, n.SourceCh(), "poll_interval 미설정이면 SourceCh 는 nil")
	assert.False(t, n.sourceMode)
}

// (d) Process on-demand 는 두 모드 모두에서 동작한다.
func TestModbusReadPoll_ProcessWorks_InBothModes(t *testing.T) {
	readOp := []any{
		map[string]any{"area": "holding_registers", "address": float64(0), "count": float64(2)},
	}

	t.Run("on-demand", func(t *testing.T) {
		mock := &mockModbusAgent{processResp: []byte(`{"values":[9,9]}`)}
		n := setupReadNode(t, map[string]any{"agent_ref": "s", "command_set": readOp}, agentTypeServer, mock)
		out, err := n.Process(context.Background(), message.New())
		require.NoError(t, err)
		require.Len(t, out, 1)
		success, _ := out[0].Payload().Get("success")
		assert.Equal(t, true, success)
	})

	t.Run("periodic-mode-process-still-works", func(t *testing.T) {
		mock := &mockModbusAgent{processResp: []byte(`{"values":[9,9]}`)}
		cfg := map[string]any{"agent_ref": "s", "poll_interval": "50ms", "command_set": readOp}
		n := setupReadNode(t, cfg, agentTypeServer, mock)
		defer func() { _ = n.Shutdown(context.Background()) }()

		// poll 루프가 도는 중에도 입력 메시지 기반 on-demand Process 는 정상 동작.
		out, err := n.Process(context.Background(), message.New())
		require.NoError(t, err)
		require.Len(t, out, 1)
		success, _ := out[0].Payload().Get("success")
		assert.Equal(t, true, success)
	})
}

// (e) Shutdown 은 깨끗하게 종료된다(패닉/이중 close 없음, sourceCh EOF).
func TestModbusReadPoll_Shutdown_Clean(t *testing.T) {
	mock := &mockModbusAgent{processResp: []byte(`{"values":[1]}`)}
	cfg := map[string]any{
		"agent_ref":     "mb-server",
		"poll_interval": "10ms",
		"command_set": []any{
			map[string]any{"area": "holding_registers", "address": float64(0), "count": float64(1)},
		},
	}
	n := setupReadNode(t, cfg, agentTypeServer, mock)
	ch := n.SourceCh()
	require.NotNil(t, ch)

	// 최소 1틱 대기(루프가 실제로 돌고 있음을 보장).
	_, _ = recvSource(t, ch, 2*time.Second)

	require.NotPanics(t, func() {
		require.NoError(t, n.Shutdown(context.Background()))
	}, "Shutdown 은 패닉 없이 종료")

	// Shutdown 후 sourceCh 는 닫혀 EOF(ok=false)로 배수된다.
	drained := false
	deadline := time.After(2 * time.Second)
	for !drained {
		select {
		case _, ok := <-ch:
			if !ok {
				drained = true // 채널 close 확인
			}
		case <-deadline:
			t.Fatal("Shutdown 후 sourceCh 가 닫히지 않음")
		}
	}
}
