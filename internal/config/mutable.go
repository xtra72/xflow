package config

import "strings"

// mutableKeys - 런타임에 변경 가능한 키 목록
var mutableKeys = map[string]bool{
	"engine.backpressure_threshold":         true,
	"engine.max_concurrent_flows":           true,
	"engine.execution_policy":               true,
	"observe.default_level":                 true,
	"observe.metrics.enabled":               true,
	"observe.trace.enabled":                 true,
	"observe.id_style":                      true,
	"server.cors.allowed_origins":           true,
	"server.rate_limit.requests_per_second": true,
	"auth.jwt.access_ttl":                   true,
	"auth.jwt.refresh_ttl":                  true,
	// 원격 관리 핫리로드 키 (@SPEC:SPEC-REMOTE-001 M1, REQ-A06/A07).
	// mode/server_url/exposure 변경을 런타임에 감지·반영하기 위해 mutable 로 둔다.
	"remote_management.mode":             true,
	"remote_management.server_url":       true,
	"remote_management.exposure.flows":   true,
	"remote_management.exposure.agents":  true,
	"remote_management.exposure.devices": true,
}

// immutablePrefixes - 와일드카드 패턴으로 immutable 처리되는 접두사
var immutablePrefixes = []string{
	"server.tls.",
}

// IsMutable - 키가 런타임에 변경 가능한지 확인
// mutableKeys 맵에 있으면 true, immutable 접두사에 해당하면 false,
// 알 수 없는 키는 기본적으로 immutable(false)
func IsMutable(key string) bool {
	if mutableKeys[key] {
		return true
	}
	return false
}

// IsImmutable - 키가 런타임에 변경 불가능한지 확인
// IsMutable의 반대 결과를 반환
func IsImmutable(key string) bool {
	if mutableKeys[key] {
		return false
	}
	for _, prefix := range immutablePrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	// 알 수 없는 키는 기본적으로 immutable
	return true
}
