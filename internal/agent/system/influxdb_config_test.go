package system

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// newInfluxDBTestConfig는 테스트용 InfluxDB AgentConfig를 생성하는 헬퍼이다.
// version에 따라 v2는 org 필드를 포함하고, v3는 포함하지 않는다.
func newInfluxDBTestConfig(version string) agent.AgentConfig {
	opts := map[string]any{
		"url":     "http://localhost:8086",
		"token":   "my-token",
		"bucket":  "my-bucket",
		"version": version,
	}
	if version == "2" {
		opts["org"] = "my-org"
	}
	return agent.AgentConfig{
		ID:   "agent-influxdb-test",
		Name: "test-influxdb",
		Type: "influxdb",
		Transport: agent.TransportConfig{
			Type:    "influxdb",
			Options: opts,
		},
	}
}

func TestParseInfluxDBConfig_정상_v2(t *testing.T) {
	// v2 설정의 모든 필드가 올바르게 파싱되는지 확인한다.
	cfg := newInfluxDBTestConfig("2")
	cfg.Transport.Options["query_language"] = "influxql"
	cfg.Transport.Options["timeout_sec"] = 30
	cfg.Transport.Options["buffer_size"] = 512
	cfg.Transport.Options["batch_size"] = 2000
	cfg.Transport.Options["flush_interval_ms"] = 5000
	cfg.Transport.Options["precision"] = "ms"

	ic, err := parseInfluxDBConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "http://localhost:8086", ic.URL)
	assert.Equal(t, "my-token", ic.Token)
	assert.Equal(t, "my-org", ic.Org)
	assert.Equal(t, "my-bucket", ic.Bucket)
	assert.Equal(t, "2", ic.Version)
	assert.Equal(t, "influxql", ic.QueryLanguage)
	assert.Equal(t, 30, ic.TimeoutSec)
	assert.Equal(t, 512, ic.BufferSize)
	assert.Equal(t, 2000, ic.BatchSize)
	assert.Equal(t, 5000, ic.FlushIntervalMs)
	assert.Equal(t, "ms", ic.Precision)
}

func TestParseInfluxDBConfig_정상_v3(t *testing.T) {
	// v3 설정의 모든 필드가 올바르게 파싱되는지 확인한다.
	cfg := newInfluxDBTestConfig("3")
	cfg.Transport.Options["org"] = "optional-org"
	cfg.Transport.Options["query_language"] = "influxql"
	cfg.Transport.Options["timeout_sec"] = 20
	cfg.Transport.Options["buffer_size"] = 128
	cfg.Transport.Options["batch_size"] = 500
	cfg.Transport.Options["flush_interval_ms"] = 3000
	cfg.Transport.Options["precision"] = "us"

	ic, err := parseInfluxDBConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "http://localhost:8086", ic.URL)
	assert.Equal(t, "my-token", ic.Token)
	assert.Equal(t, "optional-org", ic.Org)
	assert.Equal(t, "my-bucket", ic.Bucket)
	assert.Equal(t, "3", ic.Version)
	assert.Equal(t, "influxql", ic.QueryLanguage)
	assert.Equal(t, 20, ic.TimeoutSec)
	assert.Equal(t, 128, ic.BufferSize)
	assert.Equal(t, 500, ic.BatchSize)
	assert.Equal(t, 3000, ic.FlushIntervalMs)
	assert.Equal(t, "us", ic.Precision)
}

func TestParseInfluxDBConfig_필수필드누락_url(t *testing.T) {
	// url 필드가 누락되면 에러를 반환해야 한다.
	cfg := newInfluxDBTestConfig("3")
	delete(cfg.Transport.Options, "url")

	_, err := parseInfluxDBConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "url")
}

func TestParseInfluxDBConfig_필수필드누락_token(t *testing.T) {
	// token 필드가 누락되면 에러를 반환해야 한다.
	cfg := newInfluxDBTestConfig("3")
	delete(cfg.Transport.Options, "token")

	_, err := parseInfluxDBConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token")
}

func TestParseInfluxDBConfig_필수필드누락_bucket(t *testing.T) {
	// bucket 필드가 누락되면 에러를 반환해야 한다.
	cfg := newInfluxDBTestConfig("3")
	delete(cfg.Transport.Options, "bucket")

	_, err := parseInfluxDBConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bucket")
}

func TestParseInfluxDBConfig_필수필드누락_version(t *testing.T) {
	// version 필드가 누락되면 에러를 반환해야 한다.
	cfg := newInfluxDBTestConfig("3")
	delete(cfg.Transport.Options, "version")

	_, err := parseInfluxDBConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version")
}

func TestParseInfluxDBConfig_잘못된버전(t *testing.T) {
	// 유효하지 않은 version 값이면 에러를 반환해야 한다.
	cfg := newInfluxDBTestConfig("3")
	cfg.Transport.Options["version"] = "4"

	_, err := parseInfluxDBConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version")
}

func TestParseInfluxDBConfig_v2_org필수(t *testing.T) {
	// v2에서 org 필드가 누락되면 에러를 반환해야 한다.
	cfg := newInfluxDBTestConfig("2")
	delete(cfg.Transport.Options, "org")

	_, err := parseInfluxDBConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "org")
}

func TestParseInfluxDBConfig_v3_org선택(t *testing.T) {
	// v3에서 org 필드가 없어도 에러 없이 파싱되어야 한다.
	cfg := newInfluxDBTestConfig("3")
	// v3 기본 설정에는 org가 없으므로 그대로 사용

	ic, err := parseInfluxDBConfig(cfg)
	require.NoError(t, err)
	assert.Equal(t, "", ic.Org)
}

func TestParseInfluxDBConfig_기본값_v2(t *testing.T) {
	// v2의 기본값이 올바르게 설정되는지 확인한다.
	cfg := newInfluxDBTestConfig("2")

	ic, err := parseInfluxDBConfig(cfg)
	require.NoError(t, err)

	// v2 기본 쿼리 언어는 "flux"
	assert.Equal(t, "flux", ic.QueryLanguage)
	assert.Equal(t, 10, ic.TimeoutSec)
	assert.Equal(t, 256, ic.BufferSize)
	assert.Equal(t, 1000, ic.BatchSize)
	assert.Equal(t, 1000, ic.FlushIntervalMs)
	assert.Equal(t, "ns", ic.Precision)
}

func TestParseInfluxDBConfig_기본값_v3(t *testing.T) {
	// v3의 기본값이 올바르게 설정되는지 확인한다.
	cfg := newInfluxDBTestConfig("3")

	ic, err := parseInfluxDBConfig(cfg)
	require.NoError(t, err)

	// v3 기본 쿼리 언어는 "sql"
	assert.Equal(t, "sql", ic.QueryLanguage)
	assert.Equal(t, 10, ic.TimeoutSec)
	assert.Equal(t, 256, ic.BufferSize)
	assert.Equal(t, 1000, ic.BatchSize)
	assert.Equal(t, 1000, ic.FlushIntervalMs)
	assert.Equal(t, "ns", ic.Precision)
}

func TestParseInfluxDBConfig_사용자지정_옵션(t *testing.T) {
	// 사용자 지정 옵션이 기본값을 올바르게 덮어쓰는지 확인한다.
	cfg := newInfluxDBTestConfig("3")
	cfg.Transport.Options["query_language"] = "influxql"
	cfg.Transport.Options["timeout_sec"] = 60
	cfg.Transport.Options["buffer_size"] = 1024
	cfg.Transport.Options["batch_size"] = 5000
	cfg.Transport.Options["flush_interval_ms"] = 2000
	cfg.Transport.Options["precision"] = "s"

	ic, err := parseInfluxDBConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "influxql", ic.QueryLanguage)
	assert.Equal(t, 60, ic.TimeoutSec)
	assert.Equal(t, 1024, ic.BufferSize)
	assert.Equal(t, 5000, ic.BatchSize)
	assert.Equal(t, 2000, ic.FlushIntervalMs)
	assert.Equal(t, "s", ic.Precision)
}
