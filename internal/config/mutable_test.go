package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsMutable - 알려진 mutable 키들이 true를 반환하는지 검증
func TestIsMutable(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"엔진 백프레셔 임계값", "engine.backpressure_threshold", true},
		{"엔진 실행 정책", "engine.execution_policy", true},
		{"관측 기본 로그 레벨", "observe.default_level", true},
		{"관측 메트릭 활성화", "observe.metrics.enabled", true},
		{"관측 트레이스 활성화", "observe.trace.enabled", true},
		{"서버 CORS 허용 오리진", "server.cors.allowed_origins", true},
		{"서버 속도 제한", "server.rate_limit.requests_per_second", true},
		{"인증 JWT 액세스 TTL", "auth.jwt.access_ttl", true},
		{"인증 JWT 리프레시 TTL", "auth.jwt.refresh_ttl", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsMutable(tt.key),
				"IsMutable(%q)는 %v여야 합니다", tt.key, tt.want)
		})
	}
}

// TestIsImmutable - 알려진 immutable 키들이 true를 반환하는지 검증
func TestIsImmutable(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"서버 포트", "server.port", true},
		{"서버 호스트", "server.host", true},
		{"스토리지 타입", "storage.type", true},
		{"플러그인 디렉토리", "plugin.directory", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsImmutable(tt.key),
				"IsImmutable(%q)는 %v여야 합니다", tt.key, tt.want)
		})
	}
}

// TestIsMutable_WildcardPattern - server.tls.* 와일드카드 패턴이 immutable인지 검증
func TestIsMutable_WildcardPattern(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"TLS 인증서 파일", "server.tls.cert_file"},
		{"TLS 키 파일", "server.tls.key_file"},
		{"TLS 활성화", "server.tls.enabled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, IsMutable(tt.key),
				"IsMutable(%q)는 false여야 합니다 (와일드카드 immutable)", tt.key)
			assert.True(t, IsImmutable(tt.key),
				"IsImmutable(%q)는 true여야 합니다 (와일드카드 immutable)", tt.key)
		})
	}
}

// TestIsMutable_UnknownKey - 알 수 없는 키는 기본적으로 immutable인지 검증
func TestIsMutable_UnknownKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"알 수 없는 키 1", "unknown.key"},
		{"알 수 없는 키 2", "some.random.path"},
		{"알 수 없는 키 3", "foo.bar.baz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, IsMutable(tt.key),
				"IsMutable(%q)는 false여야 합니다 (알 수 없는 키는 immutable)", tt.key)
			assert.True(t, IsImmutable(tt.key),
				"IsImmutable(%q)는 true여야 합니다 (알 수 없는 키는 immutable)", tt.key)
		})
	}
}
