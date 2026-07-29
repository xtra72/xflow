package xsfm

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// B4 — 셀렉터 fan-out (acceptance.md Module 4 + Scenario 1B.6)
//
// 순수 방출 검증 테스트는 control_response_timeout=0s(fire-and-forget)를 사용해 응답 대기 없이
// 즉시 반환한다. 멤버별 응답 대기 집계(4.7)만 짧은 timeout(200ms) + 에코 주입을 사용한다.
// ---------------------------------------------------------------------------

// groupDevices 는 device_id + 속성 맵으로 config 디바이스 시드 슬라이스를 만든다.
func groupDevices(specs ...map[string]any) []any {
	out := make([]any, 0, len(specs))
	for _, s := range specs {
		out = append(out, s)
	}
	return out
}

// decodeFanOut 는 fan-out 집계 응답을 파싱한다.
func decodeFanOut(t *testing.T, raw []byte) fanOutResponse {
	t.Helper()
	var out fanOutResponse
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

// Scenario 4.1: 그룹 명령이 모든 멤버로 fan-out (3 멤버 → 3 발행 + 3 멤버 집계).
func TestGroup_FanOutAllMembers(t *testing.T) {
	opts := directOpts()
	opts["control_response_timeout"] = "0s"
	opts["devices"] = groupDevices(
		map[string]any{"device_id": "ap-101", "group_id": "concourse-b1"},
		map[string]any{"device_id": "ap-102", "group_id": "concourse-b1"},
		map[string]any{"device_id": "ap-103", "group_id": "concourse-b1"},
	)
	ap, mock := directAgentWithMock(t, opts)

	resp, err := ap.Process([]byte(`{"command":"set_power","group_id":"concourse-b1","params":{"power":true}}`))
	require.NoError(t, err)

	// 멤버별 개별 발행 (3건, 동시 fan-out 이라 순서는 비결정적).
	require.Equal(t, 3, mock.publishedLen())
	assert.ElementsMatch(t,
		[]string{"xsfm/ap-101/cmd", "xsfm/ap-102/cmd", "xsfm/ap-103/cmd"},
		mock.publishedTopics(),
	)

	// 집계 응답: selector=group_id, 3 멤버 status ok, 최상위 ok (결과는 device_id 정렬).
	out := decodeFanOut(t, resp)
	assert.Equal(t, selectorRef{Type: "group_id", Value: "concourse-b1"}, out.Selector)
	assert.Equal(t, "ok", out.Status)
	require.Len(t, out.Results, 3)
	assert.Equal(t, "ap-101", out.Results[0].DeviceID)
	assert.Equal(t, "ap-103", out.Results[2].DeviceID)
	for _, r := range out.Results {
		assert.Equal(t, "ok", r.Status)
	}
}

// Scenario 4.2: 부분 실패 best-effort (한 멤버 발행 실패 → 나머지 계속, 최상위 partial).
func TestGroup_PartialFailureBestEffort(t *testing.T) {
	opts := directOpts()
	opts["control_response_timeout"] = "0s"
	opts["devices"] = groupDevices(
		map[string]any{"device_id": "ap-101", "group_id": "concourse-b1"},
		map[string]any{"device_id": "ap-102", "group_id": "concourse-b1"},
		map[string]any{"device_id": "ap-103", "group_id": "concourse-b1"},
	)
	ap, mock := directAgentWithMock(t, opts)
	mock.failPublish("xsfm/ap-102/cmd") // ap-102 발행 실패.

	resp, err := ap.Process([]byte(`{"command":"set_power","group_id":"concourse-b1","params":{"power":true}}`))
	require.NoError(t, err) // fan-out 자체는 성공; 부분 실패는 집계에 표기.

	// ap-101, ap-103 은 발행 계속(중단 없음), ap-102 만 실패.
	assert.ElementsMatch(t,
		[]string{"xsfm/ap-101/cmd", "xsfm/ap-103/cmd"},
		mock.publishedTopics(),
	)

	out := decodeFanOut(t, resp)
	assert.Equal(t, "partial", out.Status)
	byID := resultsByID(out.Results)
	assert.Equal(t, "ok", byID["ap-101"].Status)
	assert.Equal(t, "error", byID["ap-102"].Status)
	assert.NotEmpty(t, byID["ap-102"].Error)
	assert.Equal(t, "ok", byID["ap-103"].Status)
}

// Scenario 4.3: 빈 그룹 no-op (ErrEmptyGroup, 발행 없음).
func TestGroup_EmptyGroupNoOp(t *testing.T) {
	opts := directOpts()
	opts["control_response_timeout"] = "0s"
	opts["devices"] = groupDevices(map[string]any{"device_id": "ap-101", "group_id": "other"})
	ap, mock := directAgentWithMock(t, opts)

	_, err := ap.Process([]byte(`{"command":"set_power","group_id":"empty-grp","params":{"power":true}}`))
	assert.ErrorIs(t, err, ErrEmptyGroup)
	assert.Equal(t, 0, mock.publishedLen(), "빈 그룹은 어떠한 발행도 하지 않아야 한다")
}

// Scenario 4.4: device_id + group_id 동시 지정 → device_id 단일 (fan-out 미수행).
func TestGroup_DeviceIDPriorityOverGroup(t *testing.T) {
	opts := directOpts()
	opts["control_response_timeout"] = "0s"
	opts["devices"] = groupDevices(
		map[string]any{"device_id": "ap-101", "group_id": "concourse-b1"},
		map[string]any{"device_id": "ap-102", "group_id": "concourse-b1"},
	)
	ap, mock := directAgentWithMock(t, opts)

	resp, err := ap.Process([]byte(`{"command":"set_power","device_id":"ap-101","group_id":"concourse-b1","params":{"power":true}}`))
	require.NoError(t, err)

	// 단일 디바이스 ap-101 에만 발행 (그룹 fan-out 미수행).
	require.Equal(t, 1, mock.publishedLen())
	assert.Equal(t, "xsfm/ap-101/cmd", mock.published[0].topic)
	// 단일 경로 응답(집계 응답이 아님).
	assert.Contains(t, string(resp), `"status":"ok"`)
	assert.NotContains(t, string(resp), `"selector"`)
}

// Scenario 4.5: station 셀렉터 → 해당 역사 멤버만 (다른 역사 제외).
func TestGroup_StationSelector(t *testing.T) {
	opts := directOpts()
	opts["control_response_timeout"] = "0s"
	opts["devices"] = groupDevices(
		map[string]any{"device_id": "ap-101", "station": "ST-101"},
		map[string]any{"device_id": "ap-102", "station": "ST-101"},
		map[string]any{"device_id": "ap-201", "station": "ST-201"},
	)
	ap, mock := directAgentWithMock(t, opts)

	resp, err := ap.Process([]byte(`{"command":"set_power","station":"ST-101","params":{"power":true}}`))
	require.NoError(t, err)

	// ST-101 소속 ap-101, ap-102 만 발행 (ap-201 제외).
	assert.ElementsMatch(t,
		[]string{"xsfm/ap-101/cmd", "xsfm/ap-102/cmd"},
		mock.publishedTopics(),
	)
	out := decodeFanOut(t, resp)
	assert.Equal(t, selectorRef{Type: "station", Value: "ST-101"}, out.Selector)
	require.Len(t, out.Results, 2)
	assert.Equal(t, "ok", out.Status)
}

// Scenario 4.6: line(호선) 셀렉터 → StationsByLine→devices union (다른 호선 제외).
func TestGroup_LineSelector(t *testing.T) {
	opts := directOpts()
	opts["control_response_timeout"] = "0s"
	opts["station_registry"] = map[string]any{
		"ST-101": map[string]any{"line": "line-2"},
		"ST-102": map[string]any{"line": "line-2"},
		"ST-301": map[string]any{"line": "line-3"},
	}
	opts["devices"] = groupDevices(
		map[string]any{"device_id": "ap-101", "station": "ST-101"},
		map[string]any{"device_id": "ap-102", "station": "ST-102"},
		map[string]any{"device_id": "ap-301", "station": "ST-301"},
	)
	ap, mock := directAgentWithMock(t, opts)

	resp, err := ap.Process([]byte(`{"command":"set_fan_speed","line":"line-2","params":{"fan_speed":1}}`))
	require.NoError(t, err)

	// line-2 = {ST-101, ST-102} → ap-101, ap-102 만 발행 (ap-301 line-3 제외).
	assert.ElementsMatch(t,
		[]string{"xsfm/ap-101/cmd", "xsfm/ap-102/cmd"},
		mock.publishedTopics(),
	)
	out := decodeFanOut(t, resp)
	assert.Equal(t, selectorRef{Type: "line", Value: "line-2"}, out.Selector)
	require.Len(t, out.Results, 2)
	assert.Empty(t, out.Excluded)
}

// Scenario 4.7: 셀렉터 fan-out 멤버별 응답 대기 집계 (ok/timeout/error 혼합, best-effort).
func TestGroup_SelectorResponseWaitAggregation(t *testing.T) {
	opts := directOpts()
	opts["control_response_timeout"] = "200ms"
	opts["station_registry"] = map[string]any{
		"ST-101": map[string]any{"line": "line-2"},
		"ST-102": map[string]any{"line": "line-2"},
		"ST-103": map[string]any{"line": "line-2"},
	}
	opts["devices"] = groupDevices(
		map[string]any{"device_id": "ap-101", "station": "ST-101"}, // 에코 → ok
		map[string]any{"device_id": "ap-102", "station": "ST-102"}, // 에코 없음 → timeout
		map[string]any{"device_id": "ap-103", "station": "ST-103"}, // 발행 실패 → error
	)
	ap, mock := directAgentWithMock(t, opts)
	mock.failPublish("xsfm/ap-103/cmd")

	// ap-101 발행 관측 후 매칭 에코 주입 (register→emit 순서상 pending 이미 등록됨).
	go func() {
		for i := 0; i < 3000; i++ {
			if mock.hasPublished("xsfm/ap-101/cmd") {
				ap.FeedState("ap-101", []byte(`{"power":true}`))
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	resp, err := ap.Process([]byte(`{"command":"set_power","line":"line-2","params":{"power":true}}`))
	require.NoError(t, err)

	out := decodeFanOut(t, resp)
	assert.Equal(t, "partial", out.Status)
	byID := resultsByID(out.Results)
	assert.Equal(t, "ok", byID["ap-101"].Status)
	assert.Equal(t, "timeout", byID["ap-102"].Status)
	assert.Equal(t, "ErrControlTimeout", byID["ap-102"].Error)
	assert.Equal(t, "error", byID["ap-103"].Status)
	assert.NotEmpty(t, byID["ap-103"].Error)
	assert.Equal(t, 0, ap.pendings.len(), "모든 멤버 종결 후 pending 없음")
}

// Scenario 4.8: 셀렉터 우선순위 (device_id > station > line > group_id) — 전부 지정 시 device_id 단일.
func TestGroup_SelectorPriority(t *testing.T) {
	opts := directOpts()
	opts["control_response_timeout"] = "0s"
	opts["station_registry"] = map[string]any{"ST-101": map[string]any{"line": "line-2"}}
	opts["devices"] = groupDevices(
		map[string]any{"device_id": "ap-101", "station": "ST-101", "group_id": "concourse-b1"},
		map[string]any{"device_id": "ap-102", "station": "ST-101", "group_id": "concourse-b1"},
	)
	ap, mock := directAgentWithMock(t, opts)

	resp, err := ap.Process([]byte(`{"command":"set_power","device_id":"ap-101","station":"ST-101","line":"line-2","group_id":"concourse-b1","params":{"power":true}}`))
	require.NoError(t, err)

	// device_id 최우선 → ap-101 단일 대상만 (station/line/group fan-out 미수행).
	require.Equal(t, 1, mock.publishedLen())
	assert.Equal(t, "xsfm/ap-101/cmd", mock.published[0].topic)
	assert.NotContains(t, string(resp), `"selector"`)
}

// Scenario 4.9: line 대상 중 미등록 station 디바이스 제외 + excluded 표기.
func TestGroup_LineExcludesUnregisteredStation(t *testing.T) {
	opts := directOpts()
	opts["control_response_timeout"] = "0s"
	opts["station_registry"] = map[string]any{"ST-101": map[string]any{"line": "line-2"}}
	opts["devices"] = groupDevices(
		map[string]any{"device_id": "ap-101", "station": "ST-101"}, // 등록 → 대상
		map[string]any{"device_id": "ap-102", "station": "ST-999"}, // 미등록 station → 제외
	)
	ap, mock := directAgentWithMock(t, opts)

	resp, err := ap.Process([]byte(`{"command":"set_power","line":"line-2","params":{"power":true}}`))
	require.NoError(t, err)

	// ap-101 에만 발행, ap-102 는 미등록 station 참조로 대상 제외.
	require.Equal(t, 1, mock.publishedLen())
	assert.Equal(t, "xsfm/ap-101/cmd", mock.published[0].topic)

	out := decodeFanOut(t, resp)
	require.Len(t, out.Results, 1)
	assert.Equal(t, "ap-101", out.Results[0].DeviceID)
	assert.Equal(t, []string{"ap-102"}, out.Excluded, "미등록 station 디바이스가 excluded 에 표기되어야 한다")
}

// Scenario 1B.6: port 모드 — 그룹 명령이 멤버별 N개 메시지를 제어 출력 포트로 방출.
func TestGroup_PortModeFanOut(t *testing.T) {
	opts := map[string]any{
		"transport_mode":           "port",
		"payload_mapping":          validPayloadMapping(),
		"control_response_timeout": "0s",
		"devices": groupDevices(
			map[string]any{"device_id": "ap-101", "group_id": "concourse-b1"},
			map[string]any{"device_id": "ap-102", "group_id": "concourse-b1"},
			map[string]any{"device_id": "ap-103", "group_id": "concourse-b1"},
		),
	}
	a, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	resp, err := ap.Process([]byte(`{"command":"set_power","group_id":"concourse-b1","params":{"power":true}}`))
	require.NoError(t, err)

	// 제어 출력 포트로 3개의 멤버별 메시지 방출 (N 멤버 → N 메시지).
	got := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		select {
		case msg := <-ap.ControlPort():
			assert.JSONEq(t, `{"power":true}`, string(msg.Payload))
			got = append(got, msg.DeviceID)
		case <-time.After(time.Second):
			t.Fatal("expected 3 control messages on port")
		}
	}
	assert.ElementsMatch(t, []string{"ap-101", "ap-102", "ap-103"}, got)

	// 집계 응답에 3 멤버 status 포함.
	out := decodeFanOut(t, resp)
	require.Len(t, out.Results, 3)
	assert.Equal(t, "ok", out.Status)
}

// device_id 도 셀렉터도 없는 제어 명령은 대상 미지정으로 ErrInvalidCommand.
func TestGroup_NoTargetInvalid(t *testing.T) {
	opts := directOpts()
	opts["control_response_timeout"] = "0s"
	opts["devices"] = groupDevices(map[string]any{"device_id": "ap-101"})
	ap, mock := directAgentWithMock(t, opts)

	_, err := ap.Process([]byte(`{"command":"set_power","params":{"power":true}}`))
	assert.ErrorIs(t, err, ErrInvalidCommand)
	assert.Equal(t, 0, mock.publishedLen())
}

// resultsByID 는 멤버 결과를 device_id 로 인덱싱한다 (순서 비의존 검증용).
func resultsByID(results []groupResult) map[string]groupResult {
	m := make(map[string]groupResult, len(results))
	for _, r := range results {
		m[r.DeviceID] = r
	}
	return m
}
