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
	assert.Equal(t, "ms", ic.Precision) // v0.16.5: default 변경
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
	assert.Equal(t, "ms", ic.Precision) // v0.16.5: default 변경
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

// TestParseInfluxDBConfig_Debug 는 debug 옵션 파싱을 검증한다 (v0.16.4).
func TestParseInfluxDBConfig_Debug(t *testing.T) {
	t.Run("기본값_false", func(t *testing.T) {
		cfg := newInfluxDBTestConfig("3")
		ic, err := parseInfluxDBConfig(cfg)
		require.NoError(t, err)
		assert.False(t, ic.Debug, "기본 debug 값은 false")
	})
	t.Run("true_지정", func(t *testing.T) {
		cfg := newInfluxDBTestConfig("3")
		cfg.Transport.Options["debug"] = true
		ic, err := parseInfluxDBConfig(cfg)
		require.NoError(t, err)
		assert.True(t, ic.Debug)
	})
}

// TestParseInfluxDBConfig_DualTagEmit 는 SPEC-DEVICE-IDENTITY-001 Phase C § C3
// 의 dual-tag 설정 파싱을 검증한다.
func TestParseInfluxDBConfig_DualTagEmit(t *testing.T) {
	t.Run("기본값_true_ON", func(t *testing.T) {
		// 옵션 미지정 시 dual_tag_emit 은 true (Soft Deprecation 호환 기간 ON).
		cfg := newInfluxDBTestConfig("3")
		ic, err := parseInfluxDBConfig(cfg)
		require.NoError(t, err)
		assert.True(t, ic.DualTagEmit, "DualTagEmit 기본값은 true 여야 한다")
	})

	t.Run("명시적_false_OFF", func(t *testing.T) {
		// 운영자 opt-out — false 명시 시 동작 OFF.
		cfg := newInfluxDBTestConfig("3")
		cfg.Transport.Options["dual_tag_emit"] = false
		ic, err := parseInfluxDBConfig(cfg)
		require.NoError(t, err)
		assert.False(t, ic.DualTagEmit)
	})

	t.Run("명시적_true_ON", func(t *testing.T) {
		// 명시적 ON.
		cfg := newInfluxDBTestConfig("3")
		cfg.Transport.Options["dual_tag_emit"] = true
		ic, err := parseInfluxDBConfig(cfg)
		require.NoError(t, err)
		assert.True(t, ic.DualTagEmit)
	})
}

// TestParseInfluxDBConfig_DualTagEmitSourceKeys 는 source key 목록 파싱을 검증한다.
func TestParseInfluxDBConfig_DualTagEmitSourceKeys(t *testing.T) {
	t.Run("기본값_device_id", func(t *testing.T) {
		cfg := newInfluxDBTestConfig("3")
		ic, err := parseInfluxDBConfig(cfg)
		require.NoError(t, err)
		assert.Equal(t, []string{"device_id"}, ic.DualTagEmitSourceKeys,
			"DualTagEmitSourceKeys 기본값은 [\"device_id\"]")
	})

	t.Run("yaml_string_slice", func(t *testing.T) {
		// yaml 디코더가 []any 로 전달하는 일반적 경로.
		cfg := newInfluxDBTestConfig("3")
		cfg.Transport.Options["dual_tag_emit_source_keys"] = []any{"device_id", "id", "device"}
		ic, err := parseInfluxDBConfig(cfg)
		require.NoError(t, err)
		assert.Equal(t, []string{"device_id", "id", "device"}, ic.DualTagEmitSourceKeys)
	})

	t.Run("native_string_slice", func(t *testing.T) {
		// 프로그래밍 경로에서 []string 직접 전달.
		cfg := newInfluxDBTestConfig("3")
		cfg.Transport.Options["dual_tag_emit_source_keys"] = []string{"id"}
		ic, err := parseInfluxDBConfig(cfg)
		require.NoError(t, err)
		assert.Equal(t, []string{"id"}, ic.DualTagEmitSourceKeys)
	})

	t.Run("빈_슬라이스_default_유지", func(t *testing.T) {
		// 빈 슬라이스 입력 시 default 가 유지되어야 한다.
		cfg := newInfluxDBTestConfig("3")
		cfg.Transport.Options["dual_tag_emit_source_keys"] = []any{}
		ic, err := parseInfluxDBConfig(cfg)
		require.NoError(t, err)
		assert.Equal(t, []string{"device_id"}, ic.DualTagEmitSourceKeys)
	})

	t.Run("빈_문자열_원소_제거", func(t *testing.T) {
		// 빈 문자열만 있으면 default 유지, 일부만 빈 문자열이면 필터링.
		cfg := newInfluxDBTestConfig("3")
		cfg.Transport.Options["dual_tag_emit_source_keys"] = []any{"", "device_id", ""}
		ic, err := parseInfluxDBConfig(cfg)
		require.NoError(t, err)
		assert.Equal(t, []string{"device_id"}, ic.DualTagEmitSourceKeys)
	})

	t.Run("비-string_원소_무시", func(t *testing.T) {
		// 비-string 원소는 toStringSlice 가 silently skip.
		cfg := newInfluxDBTestConfig("3")
		cfg.Transport.Options["dual_tag_emit_source_keys"] = []any{"device_id", 42, "id"}
		ic, err := parseInfluxDBConfig(cfg)
		require.NoError(t, err)
		assert.Equal(t, []string{"device_id", "id"}, ic.DualTagEmitSourceKeys)
	})
}

// TestTimestampToTime 은 timestampToTime 헬퍼가 precision 에 따라 epoch 값을
// 올바르게 변환하는지 검증한다 (v0.16.5 — 이전 버그: 항상 ns 로 해석).
func TestTimestampToTime(t *testing.T) {
	// 2026-05-22T00:00:00 UTC 의 각 precision 표현.
	const epochSec = int64(1779062400)
	cases := []struct {
		name      string
		precision string
		ts        int64
	}{
		{"ms (default)", "ms", epochSec * 1000},
		{"ms (empty=ms)", "", epochSec * 1000},
		{"s", "s", epochSec},
		{"us", "us", epochSec * 1_000_000},
		{"ns", "ns", epochSec * 1_000_000_000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := timestampToTime(tc.ts, tc.precision)
			if got.Unix() != epochSec {
				t.Errorf("timestampToTime(%d, %q).Unix() = %d; want %d",
					tc.ts, tc.precision, got.Unix(), epochSec)
			}
		})
	}
}
