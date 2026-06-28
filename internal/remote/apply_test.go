// apply_test.go 는 M3 클라이언트 측 명령 적용 라우팅을 검증한다
// (@SPEC:SPEC-REMOTE-001 M3, REQ-D02/D03/D04/D09).
//
// 검증:
//   - 각 도메인(flow/agent/device)이 올바른 DomainCommander 로 라우팅(REQ-D02/D03/D04).
//   - commander 오류 → Apply 오류 전파(부분 적용 없음 — REQ-D09).
//   - 미구성 도메인/알 수 없는 도메인 거부.
package remote

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCommander 는 DomainCommander 의 테스트 구현이다. 마지막 action/args 를 기록하고
// 미리 설정된 결과/오류를 반환한다.
type fakeCommander struct {
	name       string
	lastAction string
	lastArgs   json.RawMessage
	result     json.RawMessage
	err        error
	calls      int
}

func (f *fakeCommander) Do(_ context.Context, action string, args json.RawMessage) (json.RawMessage, error) {
	f.calls++
	f.lastAction = action
	f.lastArgs = args
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

// TestApply_RoutesPerDomain 는 각 도메인이 올바른 commander 로 라우팅되고 다른
// commander 는 호출되지 않는지 검증한다(REQ-D02/D03/D04).
func TestApply_RoutesPerDomain(t *testing.T) {
	tests := []struct {
		name   string
		domain string
		action string
	}{
		{"flow create", DomainFlow, "create"},
		{"flow deploy", DomainFlow, "deploy"},
		{"flow start", DomainFlow, "start"},
		{"flow stop", DomainFlow, "stop"},
		{"agent create", DomainAgent, "create"},
		{"agent start", DomainAgent, "start"},
		{"agent stop", DomainAgent, "stop"},
		{"device update", DomainDevice, "update"},
		{"device delete-metadata", DomainDevice, "delete_metadata"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			flow := &fakeCommander{name: "flow", result: json.RawMessage(`"flow-ok"`)}
			agent := &fakeCommander{name: "agent", result: json.RawMessage(`"agent-ok"`)}
			device := &fakeCommander{name: "device", result: json.RawMessage(`"device-ok"`)}
			applier := NewApplier(flow, agent, device)

			args := json.RawMessage(`{"id":"x"}`)
			res, err := applier.Apply(context.Background(), tc.domain, tc.action, args)
			require.NoError(t, err)

			switch tc.domain {
			case DomainFlow:
				assert.Equal(t, 1, flow.calls)
				assert.Equal(t, 0, agent.calls)
				assert.Equal(t, 0, device.calls)
				assert.Equal(t, tc.action, flow.lastAction)
				assert.JSONEq(t, string(args), string(flow.lastArgs))
				assert.JSONEq(t, `"flow-ok"`, string(res))
			case DomainAgent:
				assert.Equal(t, 1, agent.calls)
				assert.Equal(t, 0, flow.calls)
				assert.Equal(t, 0, device.calls)
				assert.Equal(t, tc.action, agent.lastAction)
				assert.JSONEq(t, `"agent-ok"`, string(res))
			case DomainDevice:
				assert.Equal(t, 1, device.calls)
				assert.Equal(t, 0, flow.calls)
				assert.Equal(t, 0, agent.calls)
				assert.Equal(t, tc.action, device.lastAction)
				assert.JSONEq(t, `"device-ok"`, string(res))
			}
		})
	}
}

// TestApply_CommanderErrorPropagates 는 commander 오류가 그대로 전파되는지 검증한다
// (REQ-D09 — 적용 실패는 오류로 보고, 부분 적용 없음).
func TestApply_CommanderErrorPropagates(t *testing.T) {
	wantErr := errors.New("conflict: flow already running")
	flow := &fakeCommander{err: wantErr}
	applier := NewApplier(flow, nil, nil)

	res, err := applier.Apply(context.Background(), DomainFlow, "start", nil)
	assert.Nil(t, res)
	assert.ErrorIs(t, err, wantErr)
}

// TestApply_UnknownDomain 는 알 수 없는 도메인이 거부되는지 검증한다.
func TestApply_UnknownDomain(t *testing.T) {
	applier := NewApplier(&fakeCommander{}, &fakeCommander{}, &fakeCommander{})
	_, err := applier.Apply(context.Background(), "bogus", "x", nil)
	assert.ErrorIs(t, err, ErrUnknownDomain)
}

// TestApply_DomainNotConfigured 는 nil commander 도메인이 거부되는지 검증한다(노출
// 안 한 도메인 보호).
func TestApply_DomainNotConfigured(t *testing.T) {
	applier := NewApplier(nil, nil, nil)
	for _, domain := range []string{DomainFlow, DomainAgent, DomainDevice} {
		_, err := applier.Apply(context.Background(), domain, "x", nil)
		assert.ErrorIs(t, err, ErrApplierUnavailable, "도메인=%s", domain)
	}
}

// TestApply_SystemDomainRouting 은 WithSystem 으로 바인딩된 system 도메인이 라우팅되고,
// 미바인딩 시 거부되는지 검증한다(버전 관리 Phase 2).
func TestApply_SystemDomainRouting(t *testing.T) {
	// 미바인딩: system 도메인 거부.
	applier := NewApplier(nil, nil, nil)
	_, err := applier.Apply(context.Background(), DomainSystem, ActionSystemUpdate, nil)
	assert.ErrorIs(t, err, ErrApplierUnavailable)

	// WithSystem 바인딩: system 커맨더로 라우팅.
	sys := &fakeCommander{name: "system", result: json.RawMessage(`"sys-ok"`)}
	applier = NewApplier(nil, nil, nil).WithSystem(sys)
	res, err := applier.Apply(context.Background(), DomainSystem, ActionSystemUpdate, nil)
	assert.NoError(t, err)
	assert.Equal(t, json.RawMessage(`"sys-ok"`), res)
	assert.Equal(t, ActionSystemUpdate, sys.lastAction)
}
