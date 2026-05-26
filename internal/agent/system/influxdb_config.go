package system

import (
	"fmt"

	"github.com/xtra/xflow/internal/agent"
)

// InfluxDBConfig 는 InfluxDB 에이전트의 설정이다.
type InfluxDBConfig struct {
	// URL 은 InfluxDB 서버 주소이다 (예: "http://localhost:8086").
	URL string `json:"url"`

	// Token 은 인증 토큰이다.
	Token string `json:"token"`

	// Org 는 조직명이다 (v2 에서 필수, v3 에서 선택).
	Org string `json:"org"`

	// Bucket 은 버킷명(v2) 또는 데이터베이스명(v3)이다.
	Bucket string `json:"bucket"`

	// Version 은 InfluxDB 버전이다 ("2" 또는 "3").
	Version string `json:"version"`

	// QueryLanguage 는 기본 쿼리 언어이다 ("flux", "influxql", "sql").
	QueryLanguage string `json:"query_language"`

	// TimeoutSec 은 작업 타임아웃(초)이다.
	TimeoutSec int `json:"timeout_sec"`

	// BufferSize 는 수신 버퍼 크기이다.
	BufferSize int `json:"buffer_size"`

	// BatchSize 는 배치 쓰기 최대 크기이다.
	BatchSize int `json:"batch_size"`

	// FlushIntervalMs 는 플러시 간격(밀리초)이다.
	FlushIntervalMs int `json:"flush_interval_ms"`

	// Precision 은 타임스탬프 정밀도이다 ("ns", "us", "ms", "s").
	Precision string `json:"precision"`

	// Debug 는 InfluxDB 로 전송되는 메시지를 DEBUG 레벨로 출력할지 여부이다 (v0.16.4).
	// 운영 환경에서는 false 권장 (로그 부하).
	Debug bool `json:"debug"`

	// DualTagEmit 은 SPEC-DEVICE-IDENTITY-001 Phase C § C3 의 dual-tag 부착
	// 동작 활성화 여부이다 (default: true — Soft Deprecation 호환 기간 동안 ON).
	//
	// true 일 때 WriteData.Tags 에 composite (device_id) tag value 가 발견되면
	// device.Registry.ResolveDevice 로 UUID 를 조회하여 "uid" tag 를 함께 부착한다.
	// 외부 쿼리/대시보드는 composite tag 를 그대로 사용 (Soft Deprecation 보장).
	//
	// false 로 설정 시 Phase A/B 동작 그대로 — 부착 없음 (긴급 opt-out 경로).
	DualTagEmit bool `json:"dual_tag_emit"`

	// DualTagEmitSourceKeys 는 dual-tag 부착 시 composite tag value 를 찾을 tag
	// key 우선순위 목록이다 (default: ["device_id"]).
	//
	// 첫 번째로 발견된 key 의 값을 ResolveDevice 에 전달한다. 운영자가 다른 tag
	// key (예: "id" 또는 "device") 를 사용하는 경우 yaml 에서 명시적 지정 가능.
	//
	// 빈 슬라이스 또는 nil 일 때 default 가 자동 적용된다.
	DualTagEmitSourceKeys []string `json:"dual_tag_emit_source_keys"`
}

// parseInfluxDBConfig 는 AgentConfig 에서 InfluxDBConfig 를 파싱한다.
func parseInfluxDBConfig(cfg agent.AgentConfig) (InfluxDBConfig, error) {
	// 기본값 설정.
	// v0.16.5: Precision 기본값을 "ms" 로 변경 — influxdb-write 노드가
	// msg.Timestamp().UnixMilli() 를 보내기 때문 (이전 "ns" 는 1000× 오차 발생).
	//
	// SPEC-DEVICE-IDENTITY-001 Phase C § C3: DualTagEmit 기본값 true (Soft
	// Deprecation 호환 기간 동안 ON), DualTagEmitSourceKeys 기본값 ["device_id"].
	ic := InfluxDBConfig{
		TimeoutSec:            10,
		BufferSize:            256,
		BatchSize:             1000,
		FlushIntervalMs:       1000,
		Precision:             "ms",
		DualTagEmit:           true,
		DualTagEmitSourceKeys: []string{"device_id"},
	}

	opts := cfg.Transport.Options

	// 필수 필드 파싱
	if v, ok := opts["url"].(string); ok && v != "" {
		ic.URL = v
	} else {
		return ic, fmt.Errorf("influxdb config: url 은 필수입니다")
	}

	if v, ok := opts["token"].(string); ok && v != "" {
		ic.Token = v
	} else {
		return ic, fmt.Errorf("influxdb config: token 은 필수입니다")
	}

	if v, ok := opts["bucket"].(string); ok && v != "" {
		ic.Bucket = v
	} else {
		return ic, fmt.Errorf("influxdb config: bucket 은 필수입니다")
	}

	if v, ok := opts["version"].(string); ok && v != "" {
		ic.Version = v
	} else {
		return ic, fmt.Errorf("influxdb config: version 은 필수입니다")
	}

	// 버전 유효성 검사
	if ic.Version != "2" && ic.Version != "3" {
		return ic, fmt.Errorf("influxdb config: version 은 '2' 또는 '3' 이어야 합니다 (입력값: %q)", ic.Version)
	}

	// 버전 기반 기본 쿼리 언어 설정
	if ic.Version == "2" {
		ic.QueryLanguage = "flux"
	} else {
		ic.QueryLanguage = "sql"
	}

	// v2 에서는 org 가 필수
	if ic.Version == "2" {
		if v, ok := opts["org"].(string); ok && v != "" {
			ic.Org = v
		} else {
			return ic, fmt.Errorf("influxdb config: InfluxDB 2.x 에서 org 는 필수입니다")
		}
	} else {
		// v3 에서는 org 가 선택
		if v, ok := opts["org"].(string); ok {
			ic.Org = v
		}
	}

	// 선택 필드 파싱
	if v, ok := opts["query_language"].(string); ok && v != "" {
		ic.QueryLanguage = v
	}
	if v, ok := opts["timeout_sec"]; ok {
		ic.TimeoutSec = toInt(v)
	}
	if v, ok := opts["buffer_size"]; ok {
		ic.BufferSize = toInt(v)
	}
	if v, ok := opts["batch_size"]; ok {
		ic.BatchSize = toInt(v)
	}
	if v, ok := opts["flush_interval_ms"]; ok {
		ic.FlushIntervalMs = toInt(v)
	}
	if v, ok := opts["precision"].(string); ok && v != "" {
		ic.Precision = v
	}
	// v0.16.4: debug 옵션 — true 면 전송되는 WriteData / QueryRequest 를 DEBUG 로그.
	if v, ok := opts["debug"].(bool); ok {
		ic.Debug = v
	}

	// SPEC-DEVICE-IDENTITY-001 Phase C § C3: dual-tag emit 설정.
	// DualTagEmit default true 는 위 초기화에서 적용 — 옵션 미지정 시 ON.
	if v, ok := opts["dual_tag_emit"].(bool); ok {
		ic.DualTagEmit = v
	}

	// DualTagEmitSourceKeys: yaml 에서 []any 또는 []string 으로 들어올 수 있다.
	// 패키지 내 toStringSlice 헬퍼 (mqtt_agent.go) 재사용. 빈 문자열 원소는
	// 추가로 필터링하여 잘못된 yaml 입력 ([""] 등) 으로부터 보호한다.
	// 빈 슬라이스 또는 비-string 원소만 있으면 default ["device_id"] 유지.
	if raw, ok := opts["dual_tag_emit_source_keys"]; ok {
		keys := filterNonEmpty(toStringSlice(raw))
		if len(keys) > 0 {
			ic.DualTagEmitSourceKeys = keys
		}
		// 빈 슬라이스 입력 → default 유지 (위 초기화 값).
	}

	return ic, nil
}

// filterNonEmpty 는 빈 문자열 원소를 제거한 슬라이스를 반환한다.
//
// SPEC-DEVICE-IDENTITY-001 Phase C § C3 — dual_tag_emit_source_keys 유효성
// 검증 보조. nil 슬라이스는 nil 그대로 반환 (default 유지를 위해).
func filterNonEmpty(s []string) []string {
	if len(s) == 0 {
		return s
	}
	out := make([]string, 0, len(s))
	for _, v := range s {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
