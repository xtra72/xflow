package chirpstack

import (
	"strings"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// webUIChirpStackOptions 는 web UI(agentSchemas.ts CHIRPSTACK_FIELDS)가 실제로
// PUT /api/v1/agents/{id}/config 로 보내는 payload 를 그대로 재현한다.
//
// 위젯 종류마다 직렬화 형태가 다르다는 것이 이 테스트군의 핵심이다:
//   - select(qos, measurement_emit_mode, timestamp_source) → 문자열
//   - string(topics, offline_threshold)                    → 문자열(토픽은 "쉼표로 구분")
//   - number(keep_alive_sec, buffer_size, ...)             → 진짜 JSON 숫자(디코딩 시 float64)
//   - boolean(auto_reconnect, ...)                         → bool
func webUIChirpStackOptions() map[string]any {
	return map[string]any{
		"broker":                "tcp://mqtt.example:1883",
		"client_id":             "cs-web",
		"username":              "u",
		"password":              "p",
		"topics":                "application/1/#, application/2/#",
		"qos":                   "2",
		"keep_alive_sec":        float64(60),
		"auto_reconnect":        true,
		"clean_session":         true,
		"buffer_size":           float64(1024),
		"connect_timeout_sec":   float64(10),
		"measurement_emit_mode": "per_measurement",
		"timestamp_source":      "uplink",
		"emit_comm_state":       false,
		"offline_threshold":     "300s",
	}
}

func optsConfig(opts map[string]any) agent.AgentConfig {
	return agent.AgentConfig{Transport: agent.TransportConfig{Options: opts}}
}

// TestConfigure_WebUIPayload_Succeeds 는 보고된 장애를 그대로 재현한다.
//
// 증상: PUT /api/v1/agents/{id}/config 가
// "chirpstack configure: qos 는 숫자여야 합니다 (got string)" 로 거부되어
// 사용자가 ChirpStack 설정을 아예 저장할 수 없었다.
func TestConfigure_WebUIPayload_Succeeds(t *testing.T) {
	resetNameRegistryForTest()

	created, err := NewChirpStackAgent(newTestConfig("cfg-web-1", "cfg-web-1"))
	if err != nil {
		t.Fatalf("NewChirpStackAgent: %v", err)
	}
	a, ok := created.(*ChirpStackAgent)
	if !ok {
		t.Fatalf("agent type = %T, want *ChirpStackAgent", created)
	}

	cfg := newTestConfig("cfg-web-1", "cfg-web-1")
	cfg.Transport.Options = webUIChirpStackOptions()

	if err := a.Configure(cfg); err != nil {
		t.Fatalf("Configure(web UI payload) = %v, want nil", err)
	}

	cc := a.cs()
	if cc.QoS != 2 {
		t.Errorf("QoS = %d, want 2", cc.QoS)
	}
	if len(cc.Topics) != 2 || cc.Topics[0] != "application/1/#" || cc.Topics[1] != "application/2/#" {
		t.Errorf("Topics = %v, want [application/1/# application/2/#]", cc.Topics)
	}
	if cc.KeepAliveSec != 60 || cc.BufferSize != 1024 || cc.ConnectTimeoutSec != 10 {
		t.Errorf("numeric knobs = %d/%d/%d, want 60/1024/10",
			cc.KeepAliveSec, cc.BufferSize, cc.ConnectTimeoutSec)
	}
	if cc.OfflineThreshold != 300*time.Second {
		t.Errorf("OfflineThreshold = %v, want 300s", cc.OfflineThreshold)
	}
}

// TestParseChirpStackConfig_QoSStringYieldsValue 는 조용한 QoS 0 결함을 고정한다.
//
// web UI 의 qos 는 select 위젯이라 '0'/'1'/'2' 문자열로 전송된다. toInt 에 문자열
// 분기가 없어 QoS 2 를 고른 사용자가 실제로는 QoS 0 으로 동작해 왔다.
func TestParseChirpStackConfig_QoSStringYieldsValue(t *testing.T) {
	cases := []struct {
		in   any
		want byte
	}{
		{"0", 0},
		{"1", 1},
		{"2", 2},
		{" 2 ", 2}, // 앞뒤 공백 허용
		{2, 2},     // 명시적 숫자(기존 API/YAML 호출자) 회귀 없음
		{float64(2), 2},
	}
	for _, c := range cases {
		cc := parseChirpStackConfig(optsConfig(map[string]any{"qos": c.in}))
		if cc.QoS != c.want {
			t.Errorf("qos=%#v → QoS = %d, want %d", c.in, cc.QoS, c.want)
		}
	}
}

// TestParseChirpStackConfig_CommaTopics 는 "쉼표로 구분" 이라 안내된 토픽 입력이
// 실제로 반영되는지 검증한다. 기존에는 []string/[]any 만 처리해 사용자가 입력한
// 토픽이 통째로 버려지고 기본값 application/# 이 유지되었다.
func TestParseChirpStackConfig_CommaTopics(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want []string
	}{
		{"쉼표+공백", "application/1/#, application/2/#", []string{"application/1/#", "application/2/#"}},
		{"단일", "application/#", []string{"application/#"}},
		{"빈 원소 제거", "a,,b, ,c", []string{"a", "b", "c"}},
		{"슬라이스 유지", []string{"a", "b"}, []string{"a", "b"}},
		{"any 슬라이스 유지", []any{"a", "b"}, []string{"a", "b"}},
	}
	for _, c := range cases {
		cc := parseChirpStackConfig(optsConfig(map[string]any{"topics": c.in}))
		if strings.Join(cc.Topics, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s: Topics = %v, want %v", c.name, cc.Topics, c.want)
		}
	}
}

// TestValidateChirpStackOptions_UnparseableRejected 는 해석 불가능한 값이
// strict 경로에서 조용한 0 이 아니라 명확한 에러가 되는지 검증한다.
func TestValidateChirpStackOptions_UnparseableRejected(t *testing.T) {
	cases := []struct {
		name    string
		opts    map[string]any
		wantSub string
	}{
		{"qos 문자열 오타", map[string]any{"qos": "abc"}, "qos 는 숫자여야 합니다"},
		{"qos 소수 문자열", map[string]any{"qos": "1.0"}, "qos 는 숫자여야 합니다"},
		{"qos 범위 초과", map[string]any{"qos": "3"}, "qos 는 0/1/2 여야 합니다"},
		{"keep_alive_sec 오타", map[string]any{"keep_alive_sec": "60초"}, "keep_alive_sec 는 숫자여야 합니다"},
		{"buffer_size 오타", map[string]any{"buffer_size": "많이"}, "buffer_size 는 숫자여야 합니다"},
		{"connect_timeout_sec 오타", map[string]any{"connect_timeout_sec": "x"}, "connect_timeout_sec 는 숫자여야 합니다"},
		{"topics 혼합 타입", map[string]any{"topics": []any{"a", 1}}, "topics"},
		{"offline_threshold 오타", map[string]any{"offline_threshold": "5분"}, "offline_threshold"},
	}
	for _, c := range cases {
		err := validateChirpStackOptions(c.opts)
		if err == nil {
			t.Errorf("%s: validate = nil, want error", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.wantSub) {
			t.Errorf("%s: err = %q, want substring %q", c.name, err.Error(), c.wantSub)
		}
	}
}

// TestParseChirpStackConfig_UnparseableKeepsDefault 는 관용(생성) 경로가 해석
// 불가능한 값을 만났을 때 조용한 0 이 아니라 구조체 기본값을 유지하는지 검증한다.
// 조용한 0 (QoS 0 / keep-alive 0) 이야말로 이번 결함의 근원이었다.
func TestParseChirpStackConfig_UnparseableKeepsDefault(t *testing.T) {
	cc := parseChirpStackConfig(optsConfig(map[string]any{
		"qos":                 "abc",
		"keep_alive_sec":      "예순",
		"connect_timeout_sec": []any{1},
		"topics":              []any{"a", 1},
		"offline_threshold":   "5분",
	}))

	if cc.QoS != 1 {
		t.Errorf("QoS = %d, want default 1 (never a silent 0)", cc.QoS)
	}
	if cc.KeepAliveSec != 60 {
		t.Errorf("KeepAliveSec = %d, want default 60", cc.KeepAliveSec)
	}
	if cc.ConnectTimeoutSec != 10 {
		t.Errorf("ConnectTimeoutSec = %d, want default 10", cc.ConnectTimeoutSec)
	}
	if len(cc.Topics) != 1 || cc.Topics[0] != defaultChirpStackTopic {
		t.Errorf("Topics = %v, want default [%s]", cc.Topics, defaultChirpStackTopic)
	}
	if cc.OfflineThreshold != defaultOfflineThreshold {
		t.Errorf("OfflineThreshold = %v, want default %v", cc.OfflineThreshold, defaultOfflineThreshold)
	}
}

// TestValidateChirpStackOptions_EmptyStringIsUnspecified 는 폼이 비운 필드를
// 빈 문자열로 보내는 경우를 "미지정" 으로 보아 통과시키는지 검증한다.
// (validateMeasurementEmitMode 가 "" 를 허용하는 이 파일의 기존 관례와 동형이다.)
func TestValidateChirpStackOptions_EmptyStringIsUnspecified(t *testing.T) {
	opts := map[string]any{
		"qos":                  "",
		"keep_alive_sec":       "",
		"topics":               "",
		"comm_report_interval": "",
		"offline_threshold":    "  ",
	}
	if err := validateChirpStackOptions(opts); err != nil {
		t.Fatalf("validate(빈 문자열) = %v, want nil", err)
	}

	cc := parseChirpStackConfig(optsConfig(opts))
	if cc.QoS != 1 || cc.KeepAliveSec != 60 {
		t.Errorf("빈 문자열이 기본값을 남기지 않았다: QoS=%d KeepAliveSec=%d", cc.QoS, cc.KeepAliveSec)
	}
	if cc.OfflineThreshold != defaultOfflineThreshold {
		t.Errorf("OfflineThreshold = %v, want %v", cc.OfflineThreshold, defaultOfflineThreshold)
	}
}

// TestParseChirpStackConfig_ExplicitNumericUnchanged 는 진짜 숫자를 보내는
// 기존 호출자(API/YAML)의 동작이 변하지 않았는지 고정한다.
func TestParseChirpStackConfig_ExplicitNumericUnchanged(t *testing.T) {
	cc := parseChirpStackConfig(optsConfig(map[string]any{
		"qos":                  2,
		"keep_alive_sec":       int64(45),
		"buffer_size":          float64(2048),
		"connect_timeout_sec":  15,
		"comm_report_interval": 60,
		"offline_threshold":    120,
		"topics":               []string{"a/#"},
	}))

	if cc.QoS != 2 {
		t.Errorf("QoS = %d, want 2", cc.QoS)
	}
	if cc.KeepAliveSec != 45 || cc.BufferSize != 2048 || cc.ConnectTimeoutSec != 15 {
		t.Errorf("numeric knobs = %d/%d/%d", cc.KeepAliveSec, cc.BufferSize, cc.ConnectTimeoutSec)
	}
	if cc.CommReportInterval != 60*time.Second {
		t.Errorf("CommReportInterval = %v, want 60s", cc.CommReportInterval)
	}
	if cc.OfflineThreshold != 120*time.Second {
		t.Errorf("OfflineThreshold = %v, want 120s", cc.OfflineThreshold)
	}
	if len(cc.Topics) != 1 || cc.Topics[0] != "a/#" {
		t.Errorf("Topics = %v, want [a/#]", cc.Topics)
	}
}

// TestConstructAndConfigurePathsAgree 는 이번 결함의 실제 불변식을 직접 검증한다:
// 생성(관용) 경로와 Configure(strict) 경로는 "받아들이는 입력"과 "만들어내는 설정"이
// 동일해야 한다. 이 대칭이 깨진 탓에 생성은 성공하는데 저장은 실패했다.
func TestConstructAndConfigurePathsAgree(t *testing.T) {
	resetNameRegistryForTest()

	base := newTestConfig("cfg-sym-1", "cfg-sym-1")
	base.Transport.Options = webUIChirpStackOptions()

	// 생성 경로: 동일 payload 로 에이전트를 만들 수 있어야 한다.
	created, err := NewChirpStackAgent(base)
	if err != nil {
		t.Fatalf("생성 경로가 web UI payload 를 거부했다: %v", err)
	}
	a, ok := created.(*ChirpStackAgent)
	if !ok {
		t.Fatalf("agent type = %T", created)
	}
	fromConstruct := *a.cs()

	// Configure 경로: 동일 payload 를 거부하지 않아야 한다.
	if err := a.Configure(base); err != nil {
		t.Fatalf("Configure 경로가 web UI payload 를 거부했다: %v", err)
	}
	fromConfigure := *a.cs()

	// ClientID 는 미지정 시 UUID 로 자동 생성되지만 여기서는 명시했으므로 동일해야 한다.
	if fromConstruct.QoS != fromConfigure.QoS ||
		fromConstruct.KeepAliveSec != fromConfigure.KeepAliveSec ||
		fromConstruct.BufferSize != fromConfigure.BufferSize ||
		fromConstruct.ConnectTimeoutSec != fromConfigure.ConnectTimeoutSec ||
		fromConstruct.OfflineThreshold != fromConfigure.OfflineThreshold ||
		fromConstruct.Broker != fromConfigure.Broker ||
		fromConstruct.ClientID != fromConfigure.ClientID ||
		!equalStringSlice(fromConstruct.Topics, fromConfigure.Topics) {
		t.Errorf("생성/Configure 결과 불일치:\n  construct = %+v\n  configure = %+v",
			fromConstruct, fromConfigure)
	}
}

// TestCoerceInt 는 공유 강제변환 헬퍼의 계약을 고정한다.
func TestCoerceInt(t *testing.T) {
	cases := []struct {
		in     any
		want   int
		wantOK bool
	}{
		{5, 5, true},
		{int64(5), 5, true},
		{byte(5), 5, true},
		{5.9, 5, true}, // JSON 숫자는 float64 로 오므로 절삭은 불가피하다
		{"5", 5, true},
		{" -3 ", -3, true},
		{"1.0", 0, false}, // 문자열은 사용자가 타이핑한 텍스트 — 절삭을 발명하지 않는다
		{"", 0, false},
		{"abc", 0, false},
		{nil, 0, false},
		{[]any{1}, 0, false},
	}
	for _, c := range cases {
		got, ok := coerceInt(c.in)
		if got != c.want || ok != c.wantOK {
			t.Errorf("coerceInt(%#v) = (%d,%v), want (%d,%v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}

// TestCoerceDuration 은 duration 강제변환 계약을 고정한다.
// 숫자 분기가 "초" 이므로, 숫자 문자열도 동일하게 "초" 로 읽는다.
func TestCoerceDuration(t *testing.T) {
	cases := []struct {
		in     any
		want   time.Duration
		wantOK bool
	}{
		{60, 60 * time.Second, true},
		{float64(60), 60 * time.Second, true},
		{"60", 60 * time.Second, true},
		{"60s", 60 * time.Second, true},
		{"1m30s", 90 * time.Second, true},
		{"", 0, false},
		{"5분", 0, false},
		{nil, 0, false},
	}
	for _, c := range cases {
		got, ok := coerceDuration(c.in)
		if got != c.want || ok != c.wantOK {
			t.Errorf("coerceDuration(%#v) = (%v,%v), want (%v,%v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}

// TestCoerceStringSlice 는 목록 강제변환 계약을 고정한다.
// 부분 성공(일부 원소만 조용히 버리기)은 금지된다 — 이번 결함의 근원 형태이다.
func TestCoerceStringSlice(t *testing.T) {
	cases := []struct {
		in     any
		want   string // "|" join
		wantOK bool
	}{
		{[]string{"a", "b"}, "a|b", true},
		{[]any{"a", "b"}, "a|b", true},
		{"a,b", "a|b", true},
		{"a, b , c", "a|b|c", true},
		{"a,,b", "a|b", true},
		{"", "", true},             // 빈 문자열 → 빈 목록(미지정 판단은 호출자 몫)
		{[]any{"a", 1}, "", false}, // 부분 성공 금지
		{42, "", false},
	}
	for _, c := range cases {
		got, ok := coerceStringSlice(c.in)
		if strings.Join(got, "|") != c.want || ok != c.wantOK {
			t.Errorf("coerceStringSlice(%#v) = (%v,%v), want (%q,%v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}
