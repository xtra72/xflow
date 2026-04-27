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
}

// parseInfluxDBConfig 는 AgentConfig 에서 InfluxDBConfig 를 파싱한다.
func parseInfluxDBConfig(cfg agent.AgentConfig) (InfluxDBConfig, error) {
	// 기본값 설정
	ic := InfluxDBConfig{
		TimeoutSec:      10,
		BufferSize:      256,
		BatchSize:       1000,
		FlushIntervalMs: 1000,
		Precision:       "ns",
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

	return ic, nil
}
