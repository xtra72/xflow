package airpurifier

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// params_backfill_test.go 는 HTTP exec 엔드포인트(POST /agents/{id}/exec)가 요청 본문을
// 표준 {command, params} 계약으로 재직렬화하여 top-level 주소지정 필드를 소실시키는 회귀를
// 재현·검증한다. Process 진입부의 fillFromParams 가 params 의 주소지정 값을 구조체 필드로
// 승격하여 UI 구동 명령이 서버측에서 정상 동작해야 한다 (SPEC-AIRPURIFIER-001).

// paramsAgent 는 params-only 경로 검증용 direct 에이전트를 생성한다 (config 시드 없음).
func paramsAgent(t *testing.T) *AirPurifierAgent {
	t.Helper()
	opts := directOpts()
	opts["control_response_timeout"] = "0s" // 제어 방출 검증이 응답 대기 없이 즉시 반환.
	a, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	return asAP(t, a)
}

// 핵심 회귀 테스트: add_device 를 params 만 담은 본문(HTTP exec 경로 모사)으로 처리 →
// 디바이스가 params 의 값으로 등록되어야 한다.
func TestProcess_ParamsOnly_AddDevice(t *testing.T) {
	ap := paramsAgent(t)

	body := `{"command":"add_device","params":{"device_id":"ap-101","station":"ST1","place":"PL-A","index":3,"group_id":"g1","name":"n"}}`
	_, err := ap.Process([]byte(body))
	require.NoError(t, err)

	dev, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.Equal(t, "ap-101", dev.DeviceID)
	assert.Equal(t, "ST1", dev.Station)
	assert.Equal(t, "PL-A", dev.Place)
	assert.Equal(t, 3, dev.Index)
	assert.Equal(t, "g1", dev.GroupID)
	assert.Equal(t, "n", dev.Name)
	assert.Equal(t, "bridge", dev.Source)
}

// add_station 을 params 만으로 처리 → 역사 레지스트리에 등록되어야 한다.
func TestProcess_ParamsOnly_AddStation(t *testing.T) {
	ap := paramsAgent(t)

	body := `{"command":"add_station","params":{"station":"ST1","line":"L2","display_name":"강남","order":2}}`
	_, err := ap.Process([]byte(body))
	require.NoError(t, err)

	stations := ap.stations.ListStations()
	require.Len(t, stations, 1)
	assert.Equal(t, "ST1", stations[0].Station)
	assert.Equal(t, "L2", stations[0].Line)
	assert.Equal(t, "강남", stations[0].DisplayName)
	assert.Equal(t, 2, stations[0].Order)
}

// add_place 를 params 만으로 처리 → 역사 내 위치가 등록되어야 한다 (선행 add_station 필요).
func TestProcess_ParamsOnly_AddPlace(t *testing.T) {
	ap := paramsAgent(t)

	_, err := ap.Process([]byte(`{"command":"add_station","params":{"station":"ST1"}}`))
	require.NoError(t, err)

	body := `{"command":"add_place","params":{"station":"ST1","place":"PL-A","display_name":"1번출구","order":1}}`
	_, err = ap.Process([]byte(body))
	require.NoError(t, err)

	stations := ap.stations.ListStations()
	require.Len(t, stations, 1)
	require.Len(t, stations[0].Places, 1)
	assert.Equal(t, "PL-A", stations[0].Places[0].Place)
	assert.Equal(t, "1번출구", stations[0].Places[0].DisplayName)
	assert.Equal(t, 1, stations[0].Places[0].Order)
}

// set_power 를 params 만으로 처리 → device_id(주소지정)는 params 에서, power(제어 값)도
// params 에서 읽어 대상 디바이스를 제어해야 한다.
func TestProcess_ParamsOnly_SetPower(t *testing.T) {
	ap, mock := directAgentWithMock(t, oneDeviceOpts(nil)) // config 시드 ap-101.

	body := `{"command":"set_power","params":{"device_id":"ap-101","power":true}}`
	out, err := ap.Process([]byte(body))
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(out, &resp))
	assert.Equal(t, "ok", resp["status"])
	assert.Equal(t, "ap-101", resp["device_id"])

	// 브로커로 전원 명령이 발행되었는지 확인 (device_id 가 params 에서 정상 해석됨).
	require.NotEmpty(t, mock.published, "set_power 는 명령을 발행해야 한다")
}

// set_fan_speed 를 params 만으로 처리 → device_id 는 params, fan_speed 도 params 에서 읽는다.
func TestProcess_ParamsOnly_SetFanSpeed(t *testing.T) {
	ap, mock := directAgentWithMock(t, oneDeviceOpts(nil))
	ap.FeedState("ap-101", []byte(`{"power":true}`)) // 로스터 전원 ON 관측 (기본 reject 정책 회피).

	body := `{"command":"set_fan_speed","params":{"device_id":"ap-101","fan_speed":2}}`
	out, err := ap.Process([]byte(body))
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(out, &resp))
	assert.Equal(t, "ok", resp["status"])
	require.Len(t, mock.published, 1, "set_fan_speed 는 명령을 발행해야 한다")
	assert.JSONEq(t, `{"fan_speed":2}`, string(mock.published[0].payload))
}

// 하위 호환: top-level 필드는 여전히 동작해야 한다 (기존 노드/유닛테스트 경로).
func TestProcess_TopLevel_StillWorks(t *testing.T) {
	ap := paramsAgent(t)

	body := `{"command":"add_device","device_id":"ap-201","station":"ST2","index":5}`
	_, err := ap.Process([]byte(body))
	require.NoError(t, err)

	dev, err := ap.GetDevice("ap-201")
	require.NoError(t, err)
	assert.Equal(t, "ST2", dev.Station)
	assert.Equal(t, 5, dev.Index)
}

// top-level 우선 규칙: top-level 과 params 가 같은 필드를 모두 담으면 top-level 이 이긴다.
func TestProcess_TopLevelWinsOverParams(t *testing.T) {
	ap := paramsAgent(t)

	body := `{"command":"add_device","device_id":"ap-301","station":"TOP","index":9,"params":{"device_id":"ap-999","station":"PARAM","index":1}}`
	_, err := ap.Process([]byte(body))
	require.NoError(t, err)

	// top-level device_id 로 등록되어야 하고, params 의 ap-999 는 무시되어야 한다.
	dev, err := ap.GetDevice("ap-301")
	require.NoError(t, err)
	assert.Equal(t, "TOP", dev.Station, "top-level station 이 우선해야 한다")
	assert.Equal(t, 9, dev.Index, "top-level index 가 우선해야 한다")

	_, err = ap.GetDevice("ap-999")
	assert.Error(t, err, "params 의 device_id 는 top-level 이 존재하면 무시되어야 한다")
}

// set_device 를 params 만으로 처리 → 다섯 필드(name/group_id/station/place/index)가 모두
// 갱신되어야 한다. handleSetDevice 의 부분 갱신 존재 판별기가 params 키도 인식해야 한다.
func TestProcess_ParamsOnly_SetDevice(t *testing.T) {
	ap := paramsAgent(t)

	// 선행 등록 (top-level 경로).
	_, err := ap.Process([]byte(`{"command":"add_device","device_id":"ap-101","name":"old","station":"ST1","place":"PL-A","index":1,"group_id":"g1"}`))
	require.NoError(t, err)

	// params-only set_device (HTTP exec 경로 모사).
	body := `{"command":"set_device","params":{"device_id":"ap-101","name":"new","station":"ST2","place":"PL-B","index":5,"group_id":"g2"}}`
	_, err = ap.Process([]byte(body))
	require.NoError(t, err)

	dev, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.Equal(t, "new", dev.Name)
	assert.Equal(t, "g2", dev.GroupID)
	assert.Equal(t, "ST2", dev.Station)
	assert.Equal(t, "PL-B", dev.Place)
	assert.Equal(t, 5, dev.Index)
}

// set_device 부분 갱신: params 에 담긴 필드만 갱신되고 나머지는 보존되어야 한다.
func TestProcess_ParamsOnly_SetDevice_Partial(t *testing.T) {
	ap := paramsAgent(t)

	_, err := ap.Process([]byte(`{"command":"add_device","device_id":"ap-102","name":"keep","station":"KEEP","index":7,"group_id":"gk"}`))
	require.NoError(t, err)

	// name 만 params 로 전달 → name 만 갱신, 나머지 보존.
	_, err = ap.Process([]byte(`{"command":"set_device","params":{"device_id":"ap-102","name":"renamed"}}`))
	require.NoError(t, err)

	dev, err := ap.GetDevice("ap-102")
	require.NoError(t, err)
	assert.Equal(t, "renamed", dev.Name)
	assert.Equal(t, "KEEP", dev.Station, "미전달 필드는 보존되어야 한다")
	assert.Equal(t, 7, dev.Index, "미전달 필드는 보존되어야 한다")
	assert.Equal(t, "gk", dev.GroupID, "미전달 필드는 보존되어야 한다")
}

// 하위 호환: top-level set_device 부분 갱신은 여전히 동작해야 한다.
func TestProcess_TopLevel_SetDevice_StillWorks(t *testing.T) {
	ap := paramsAgent(t)

	_, err := ap.Process([]byte(`{"command":"add_device","device_id":"ap-103","name":"old","station":"S1","index":2}`))
	require.NoError(t, err)

	// top-level 로 station 만 갱신.
	_, err = ap.Process([]byte(`{"command":"set_device","device_id":"ap-103","station":"S9"}`))
	require.NoError(t, err)

	dev, err := ap.GetDevice("ap-103")
	require.NoError(t, err)
	assert.Equal(t, "S9", dev.Station)
	assert.Equal(t, "old", dev.Name, "미전달 필드는 보존되어야 한다")
	assert.Equal(t, 2, dev.Index, "미전달 필드는 보존되어야 한다")
}

// index/order 가 params 에 JSON 숫자(float64) 또는 숫자 문자열로 와도 올바른 int 로 변환된다.
func TestProcess_ParamsIndexOrder_NumberAndString(t *testing.T) {
	// JSON 숫자(float64) 경로.
	ap := paramsAgent(t)
	_, err := ap.Process([]byte(`{"command":"add_device","params":{"device_id":"ap-401","index":7}}`))
	require.NoError(t, err)
	dev, err := ap.GetDevice("ap-401")
	require.NoError(t, err)
	assert.Equal(t, 7, dev.Index)

	// 숫자 문자열 경로 (견고성: "3" → 3).
	ap2 := paramsAgent(t)
	_, err = ap2.Process([]byte(`{"command":"add_device","params":{"device_id":"ap-402","index":"3"}}`))
	require.NoError(t, err)
	dev2, err := ap2.GetDevice("ap-402")
	require.NoError(t, err)
	assert.Equal(t, 3, dev2.Index)

	// add_station 의 order 도 동일하게 검증 (숫자 문자열).
	_, err = ap2.Process([]byte(`{"command":"add_station","params":{"station":"STX","order":"4"}}`))
	require.NoError(t, err)
	stations := ap2.stations.ListStations()
	require.Len(t, stations, 1)
	assert.Equal(t, 4, stations[0].Order)
}
