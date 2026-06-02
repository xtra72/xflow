package handler

import "strings"

// 본 파일은 "보안 민감 config 키" 의 단일 진실 공급원(Single Source of Truth)이다.
//
// 프론트엔드(ImportDialog)와 동일한 키 집합을 공유해야 하며, 변경 시 양쪽을
// 함께 수정해야 한다. 키 비교는 항상 대소문자 무시(case-insensitive)로 수행한다.
//
// 배치 위치 주의:
//   - export 리댁션은 handler 패키지(Export/ExportAll)에서 수행되므로 SoT 는
//     handler 패키지에 둔다. service 패키지는 handler 를 import 하므로 필요 시
//     service 쪽에서도 이 함수들을 재사용할 수 있다(반대 방향은 import cycle).
//
// SPEC: flow-management — export 시 비밀값 제거 (requirement 1)

// sensitiveConfigKeys 는 보안상 민감하다고 간주하는 config 키 집합이다.
// 키는 모두 소문자로 저장하며, 조회 시 입력 키를 소문자로 변환하여 비교한다.
//
// 프론트엔드와 정확히 동일한 집합을 유지해야 한다:
//
//	password, token, secret, api_key, apikey, access_token, auth_token,
//	client_secret, private_key, passphrase, username
var sensitiveConfigKeys = map[string]struct{}{
	"password":      {},
	"token":         {},
	"secret":        {},
	"api_key":       {},
	"apikey":        {},
	"access_token":  {},
	"auth_token":    {},
	"client_secret": {},
	"private_key":   {},
	"passphrase":    {},
	"username":      {},
}

// IsSensitiveConfigKey 는 주어진 키가 보안 민감 키인지 대소문자 무시로 판별한다.
func IsSensitiveConfigKey(key string) bool {
	_, ok := sensitiveConfigKeys[strings.ToLower(key)]
	return ok
}

// RedactSensitiveConfig 는 cfg 의 복사본을 반환하되 보안 민감 키를 제거한다.
//
//   - 입력 맵을 절대 변경하지 않는다(라이브 플로우/에이전트 config 보호).
//   - 중첩된 map[string]any (예: headers, config) 에 대해 재귀적으로 동작한다.
//   - []any 안의 map 요소에 대해서도 재귀적으로 동작한다.
//   - 민감 키가 가리키는 값은 (중첩 맵이라도) 통째로 제거한다.
//
// nil 입력에는 nil 을 반환한다.
func RedactSensitiveConfig(cfg map[string]any) map[string]any {
	if cfg == nil {
		return nil
	}
	out := make(map[string]any, len(cfg))
	for k, v := range cfg {
		if IsSensitiveConfigKey(k) {
			// 민감 키는 키 자체를 제거한다(값은 복사하지 않는다).
			continue
		}
		out[k] = redactValue(v)
	}
	return out
}

// redactValue 는 값이 중첩 맵/슬라이스이면 재귀적으로 리댁션한 복사본을,
// 그 외 스칼라 값은 그대로 반환한다.
func redactValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return RedactSensitiveConfig(t)
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = redactValue(item)
		}
		return out
	default:
		return v
	}
}

// CollectSensitiveKeys 는 cfg(및 중첩 맵)에 실제로 존재하는 민감 키를
// 정규(canonical, 소문자) 이름으로 수집하여 정렬된 슬라이스로 반환한다.
//
// import 다이얼로그가 "어떤 비밀값을 다시 입력받아야 하는지" 판단하는 데 사용한다.
// 값은 포함하지 않으며 키 이름만 반환한다. 대소문자만 다른 키는 한 번만 포함한다.
//
// 정규(소문자) 이름을 반환하는 이유: Go map 순회 순서는 비결정적이므로 "원본
// 케이스 보존" 은 결과를 비결정적으로 만든다. 정규화하면 결정적이며, 프론트의
// 민감 키 집합 또한 소문자 기준 비교이므로 호환된다.
//
// 키가 하나도 없으면 nil 을 반환한다(JSON omitempty 와 결합 시 필드 자체가 생략됨).
func CollectSensitiveKeys(cfg map[string]any) []string {
	if cfg == nil {
		return nil
	}
	seen := make(map[string]struct{}) // 정규(소문자) 키 집합
	collectSensitiveKeys(cfg, seen)
	if len(seen) == 0 {
		return nil
	}
	keys := make([]string, 0, len(seen))
	for canonical := range seen {
		keys = append(keys, canonical)
	}
	sortStrings(keys)
	return keys
}

// collectSensitiveKeys 는 맵을 재귀 순회하며 민감 키의 정규(소문자) 이름을
// seen 에 누적한다.
func collectSensitiveKeys(cfg map[string]any, seen map[string]struct{}) {
	for k, v := range cfg {
		if IsSensitiveConfigKey(k) {
			seen[strings.ToLower(k)] = struct{}{}
			continue // 민감 값 내부는 더 탐색하지 않는다
		}
		switch t := v.(type) {
		case map[string]any:
			collectSensitiveKeys(t, seen)
		case []any:
			for _, item := range t {
				if m, ok := item.(map[string]any); ok {
					collectSensitiveKeys(m, seen)
				}
			}
		}
	}
}

// sortStrings 는 표준 sort 패키지 대신 단순 삽입 정렬로 문자열 슬라이스를
// 오름차순 정렬한다(외부 의존 최소화, 키 개수가 적어 성능 무관).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
