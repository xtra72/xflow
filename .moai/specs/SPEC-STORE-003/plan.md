---
id: SPEC-STORE-003
title: Store 에이전트 정적 키 정의 및 태그 메타데이터 — 구현 계획
version: 0.3.0
status: in_progress
created: 2026-04-24
updated: 2026-05-04
author: xtra
priority: medium
---

# SPEC-STORE-003: 구현 계획 (Implementation Plan)

## v0.3.0 Scope Note

본 계획서는 v0.3.0 진화에 맞추어 갱신되었다. 핵심 변경:

- `allow_dynamic_keys` (bool) → `registration_type` (enum) **clean rename** (no backward shim)
- `data_type` 6종 enum 1급 필드 도입 (manual 명시 / auto 추론)
- `metric_type` semantic 1급 필드 도입 (default `"unknown"`)
- API `keys` 응답을 string 배열 → 객체 배열로 BREAKING 변경
- 신규 필터 `?data_type=`, `?metric_type=`, `?registration=` (기존 `?tag=`와 AND 결합)
- 신규 에러 4종: `ErrTypeMismatch`, `ErrUnsupportedValueType`, `ErrInvalidDataType`, `ErrInvalidMetricType`

v0.2.0의 구현(정적 키, 태그, 동적 키 거부, DELETE 엔드포인트, 버킷 벽시계 정렬, UI 통합)은 모두 보존되며 v0.3.0의 새 모델 위에서 재정렬된다.

## 기술 스택 (Technical Stack)

### 언어 및 런타임

- **Go 1.22+** (기존 프로젝트 표준)
- 신규 외부 의존성 **없음**

### 라이브러리

- `gopkg.in/yaml.v3` (기존) — 설정 YAML 파싱
- `encoding/json` (stdlib) — API 응답 직렬화 + auto 모드 `json` 타입 검증 후보
- `net/url` + `net/http` (stdlib) — 쿼리 파라미터 파싱 및 HTTP 핸들링
- `regexp` (stdlib) — 태그 key, metric_type 형식 검증
- `reflect` (stdlib, 선택) — auto 모드 data_type 추론 (또는 type switch로 충분)

### 영향 범위 (v0.3.0)

- `internal/agent/system/store_options.go` — storeConfig 진화: `registrationType`, `staticKeys map[string]StaticKeyMeta`
- `internal/agent/system/store.go` — `Set`/`SetWithTTL` 타입 검증, runtime setters rename
- `internal/agent/system/store_data_type.go` (신규) — `DataType` enum, `inferDataType` helper, 검증 함수
- `internal/agent/system/store_user_agent.go` — Configure 흐름에서 새 필드 반영
- `internal/agent/system/store_query.go` — 응답 빌더 객체 배열로 진화
- `internal/api/handler/store_*.go` — keys 응답 BREAKING 변경, 신규 필터 핸들러
- 테스트: `internal/agent/system/store_static_keys_test.go` (마이그레이션), 신규 `store_data_type_test.go`, 신규 `store_metric_type_test.go`, 신규 `store_registration_test.go`, `internal/api/handler/store_*_test.go` 갱신

## 기술적 접근 (Technical Approach)

### 1. storeConfig 구조체 진화

```go
// v0.2.0 (제거 대상)
type storeConfig struct {
    // ...
    allowDynamicKeys bool
    staticKeys       map[string]map[string]string // key → tags
}

// v0.3.0 (목표)
type storeConfig struct {
    // ... 운영 필드 (변경 없음)
    registrationType RegistrationType                // enum: "manual" | "auto"
    staticKeys       map[string]StaticKeyMeta        // key → meta
}

type StaticKeyMeta struct {
    DataType   DataType          // enum: int|float|string|boolean|bytes|json
    MetricType string            // free string ^[a-zA-Z0-9_-]+$, default "unknown"
    Tags       map[string]string // 기존 태그 맵
    Source     RegistrationSource // "manual" (yaml) or "auto" (런타임 등록)
}

type RegistrationType string
const (
    RegistrationManual RegistrationType = "manual"
    RegistrationAuto   RegistrationType = "auto"
)

type RegistrationSource string
const (
    SourceManual RegistrationSource = "manual" // yaml 명시
    SourceAuto   RegistrationSource = "auto"   // 런타임 등록
)

type DataType string
const (
    DataTypeInt     DataType = "int"
    DataTypeFloat   DataType = "float"
    DataTypeString  DataType = "string"
    DataTypeBoolean DataType = "boolean"
    DataTypeBytes   DataType = "bytes"
    DataTypeJSON    DataType = "json"
)
```

### 2. parseStoreConfig 진화

- `registration_type` (string) → enum 검증, 미설정 시 `RegistrationAuto`
- `allow_dynamic_keys` 키 발견 시 즉시 명시적 마이그레이션 에러 (`ErrAllowDynamicKeysRemoved`로 별도 sentinel 또는 `ErrInvalidConfig`로 wrapping)
- `keys[].data_type` (string) → 6종 enum 검증
  - `registration_type == "manual"` → 누락 시 `ErrInvalidDataType`
  - `registration_type == "auto"` → 누락 허용 (런타임 추론), 명시 시 enum 검증
- `keys[].metric_type` (string, optional) → 정규식 검증, 누락/빈 문자열 시 `"unknown"`
- 중복 key 검증 + 태그 key 정규식 (v0.2.0 보존)

### 3. 신규 에러 선언

```go
var (
    // v0.2.0 보존 (시맨틱 일부 변경)
    ErrKeyNotAllowed      = errors.New("store: key not allowed (registration_type=manual and key not in static keys)")
    ErrDuplicateStaticKey = errors.New("store: duplicate static key definition")
    ErrInvalidTagKey      = errors.New("store: invalid tag key (must match ^[a-zA-Z0-9_-]+$)")

    // v0.3.0 신규
    ErrTypeMismatch         = errors.New("store: value type does not match registered data_type")
    ErrUnsupportedValueType = errors.New("store: unsupported value type for auto data_type inference")
    ErrInvalidDataType      = errors.New("store: invalid or missing data_type (must be one of: int, float, string, boolean, bytes, json)")
    ErrInvalidMetricType    = errors.New("store: invalid metric_type (must match ^[a-zA-Z0-9_-]+$)")
)
```

### 4. inferDataType helper (v0.3.0 핵심)

```go
// inferDataType infers the DataType from a Go value for auto-registration mode.
// Returns ErrUnsupportedValueType for nil and unrecognized types.
func inferDataType(value any) (DataType, error) {
    if value == nil {
        return "", ErrUnsupportedValueType
    }
    switch v := value.(type) {
    case bool:
        return DataTypeBoolean, nil
    case int, int8, int16, int32, int64,
         uint, uint8, uint16, uint32, uint64:
        return DataTypeInt, nil
    case float32, float64:
        return DataTypeFloat, nil
    case string:
        return DataTypeString, nil
    case []byte:
        return DataTypeBytes, nil
    default:
        // map, slice, struct → json (json-marshalable check)
        if isJSONMarshalable(v) {
            return DataTypeJSON, nil
        }
        return "", ErrUnsupportedValueType
    }
}

// matchesDataType checks whether a Go value's type matches the declared DataType.
func matchesDataType(value any, dt DataType) bool {
    inferred, err := inferDataType(value)
    if err != nil {
        return false
    }
    return inferred == dt
}
```

### 5. 쓰기 경로 (Set / SetWithTTL) 진화

```go
func (as *agentStore) Set(ctx context.Context, key string, value any) error {
    cfg := as.config
    meta, exists := cfg.staticKeys[key]

    if !exists {
        // 키 미등록
        if cfg.registrationType == RegistrationManual {
            return ErrKeyNotAllowed
        }
        // auto 모드: 추론하여 등록
        dt, err := inferDataType(value)
        if err != nil {
            return err // ErrUnsupportedValueType
        }
        cfg.staticKeys[key] = StaticKeyMeta{
            DataType:   dt,
            MetricType: "unknown",
            Tags:       map[string]string{},
            Source:     SourceAuto,
        }
    } else {
        // 키 등록됨: 타입 검증
        if !matchesDataType(value, meta.DataType) {
            return ErrTypeMismatch
        }
    }
    // ... 기존 max_key_length 검증 + 저장 로직
}
```

**Locking**: `staticKeys` map 갱신은 기존 store mutex 안에서 수행되어야 한다 (auto 등록의 동시성 안전성).

### 6. API 응답 객체 배열 (BREAKING)

`internal/api/handler/store_keys.go` (또는 동등 위치):

```go
type StoreKeyResponse struct {
    Key          string            `json:"key"`
    Registration string            `json:"registration"`  // "manual" | "auto"
    DataType     string            `json:"data_type"`
    MetricType   string            `json:"metric_type"`
    Tags         map[string]string `json:"tags"`          // 빈 맵이라도 항상 포함 (생략 안 함)
}

type StoreKeysListResponse struct {
    Count int                `json:"count"`
    Keys  []StoreKeyResponse `json:"keys"`
}
```

핸들러는 `staticKeys`를 순회하며 객체 배열을 빌드하고, 필터 쿼리에 따라 결과를 좁힌다. **`tags` 맵은 객체 외부의 별도 필드가 아니라 각 객체 내부**에 있다.

### 7. 신규 필터 핸들러

```go
type keyFilter struct {
    Tags         []TagFilter // []{Key, Value}, 다중 ?tag= AND
    DataType     string      // "" = no filter
    MetricType   string      // "" = no filter
    Registration string      // "" = no filter
}

func (f keyFilter) Matches(meta StaticKeyMeta) bool {
    if f.DataType != "" && string(meta.DataType) != f.DataType { return false }
    if f.MetricType != "" && meta.MetricType != f.MetricType { return false }
    if f.Registration != "" && string(meta.Source) != f.Registration { return false }
    for _, tf := range f.Tags {
        if v, ok := meta.Tags[tf.Key]; !ok || v != tf.Value { return false }
    }
    return true
}
```

**빈 문자열 시맨틱**: `?metric_type=`은 `MetricType=""`로 필드 활성화 → `meta.MetricType == ""`인 키만 매칭. 실제로는 `"unknown"` default가 적용되므로 빈 결과 반환된다 (직관적).

### 8. Runtime Setters Rename

```go
// v0.2.0
func (s *userStoreAgent) SetAllowDynamicKeys(allow bool)
func (s *userStoreAgent) SetStaticKeys(keys map[string]map[string]string)

// v0.3.0
func (s *userStoreAgent) SetRegistrationType(rt RegistrationType)
func (s *userStoreAgent) SetStaticKeys(keys map[string]StaticKeyMeta)
```

`Configure` 흐름에서 `parseStoreConfig` 결과를 두 setter로 적용 (기존 패턴 보존).

### 9. 마이그레이션 가드 (clean rename)

```go
// parseStoreConfig 초기 단계
if _, hasOld := raw["allow_dynamic_keys"]; hasOld {
    return nil, fmt.Errorf("config: 'allow_dynamic_keys' is removed in v0.3.0; use 'registration_type: manual|auto' instead")
}
```

명시적 에러 메시지로 운영자에게 마이그레이션 경로를 안내. **shim 없음** (clean rename 의도).

## 마일스톤 (Milestones)

### Primary Goal: 모델 진화 핵심

- storeConfig 구조체 진화 (`StaticKeyMeta`, `RegistrationType`, `DataType` 도입)
- parseStoreConfig 갱신 (registration_type, data_type, metric_type 파싱 + 마이그레이션 에러)
- inferDataType helper 구현
- 쓰기 경로 타입 검증 + auto 등록 로직

### Secondary Goal: API 진화

- keys 응답 객체 배열로 BREAKING 변경
- 신규 필터 쿼리 (`?data_type=`, `?metric_type=`, `?registration=`) AND 결합
- 응답 정렬 (key 알파벳)

### Final Goal: 품질 + 통합

- 백엔드 테스트 커버리지 85% 이상 (DDD + TDD 병행)
- inferDataType 100% 분기 커버리지
- 마이그레이션 가이드 (CHANGELOG)
- SPEC-WEB-005 v0.5.0 UI 팀 동시 업데이트 조율

## Task Decomposition (14 작업)

1. **storeConfig 진화** — `allowDynamicKeys` 제거, `registrationType` 추가 + enum 상수 정의
2. **`StaticKeyMeta` struct 도입** — `DataType`, `MetricType`, `Tags`, `Source` 필드 + `staticKeys map[string]StaticKeyMeta`로 변경
3. **신규 에러 변수 선언** — `ErrTypeMismatch`, `ErrUnsupportedValueType`, `ErrInvalidDataType`, `ErrInvalidMetricType`
4. **`inferDataType` helper 구현** — 6종 enum 분기 + nil 거부 + json-marshalable 판별 (신규 `store_data_type.go`)
5. **parseStoreConfig 갱신** — registration_type enum 파싱, allow_dynamic_keys 마이그레이션 가드, keys[].data_type/metric_type 파싱·검증, 정규식 검증
6. **쓰기 경로 진화 (`Set`/`SetWithTTL`)** — manual 모드 타입 검증, auto 모드 추론 + 등록, mismatch 거부 (`ErrTypeMismatch`)
7. **`keyGatekeeper` / `NamespacedStore` 정렬** — registration_type 시맨틱에 맞춰 게이트 검사 갱신 (v0.2.0 패턴 보존)
8. **API 핸들러 BREAKING 변경 (`/keys`)** — string 배열 → 객체 배열 응답 스키마, 알파벳 정렬
9. **신규 필터 핸들러** — `?data_type=`, `?metric_type=`, `?registration=` 파싱 + AND 결합 (`keyFilter` 구조체)
10. **Runtime setters rename** — `SetAllowDynamicKeys` → `SetRegistrationType`, `SetStaticKeys` 시그니처 진화 + Configure 흐름 적용
11. **마이그레이션 테스트 (DDD characterization)** — v0.2.0 yaml에 `allow_dynamic_keys`가 있으면 명시적 에러로 부팅 실패 보장
12. **신규 TDD 테스트** — data_type 명시/추론, manual/auto 등록, mismatch 거부, metric_type 검증/default, API 객체 배열 + 필터 AND 조합
13. **SPEC-WEB-005 계약 노트** — UI 측이 객체 배열 응답에 적응해야 함을 명시 (별도 SPEC 또는 본 SPEC `Related` 섹션 갱신)
14. **CHANGELOG + 마이그레이션 가이드** — v0.3.0 BREAKING 변경 항목, yaml before/after 예시, 신규 에러 표

## 아키텍처 영향 (Architecture Impact)

### 레이어 영향

- **설정 레이어** (`parseStoreConfig`): 신규 필드 + 마이그레이션 가드 + 정규식/enum 검증
- **에이전트 코어**: 쓰기 검증 훅이 type-aware로 진화, auto 등록 시 staticKeys 동시 갱신
- **신규 검증 레이어** (`store_data_type.go`): 추론 + 타입 매칭 helper 분리
- **HTTP 핸들러 레이어**: 응답 스키마 BREAKING 변경, 필터 결합 로직 추가
- **프로토콜 호환성**: 기존 v0.2.0 클라이언트는 객체 배열 응답을 파싱할 수 없음 → SPEC-WEB-005 동시 업데이트 필수

### 데이터 흐름 (v0.3.0)

```
YAML Config → parseStoreConfig
              ├─ allow_dynamic_keys 존재? → 명시적 마이그레이션 에러
              └─ storeConfig{registrationType, staticKeys map[string]StaticKeyMeta}
                              ↓
Write Request → key 존재 여부
              ├─ 미등록 + manual → ErrKeyNotAllowed
              ├─ 미등록 + auto → inferDataType → 등록 (Source=auto)
              └─ 등록됨 → matchesDataType?
                          ├─ no → ErrTypeMismatch
                          └─ yes → 기존 저장 파이프라인
                              ↓
Read Request → existing pipeline → buildKeyResponse(meta) → []StoreKeyResponse
                                                          → keyFilter AND 결합 → response
```

### 신규 검증 레이어

`store_data_type.go`로 분리되어 단위 테스트 격리 + 재사용성 확보. `inferDataType`, `matchesDataType`, `validateDataTypeEnum`, `validateMetricType` helpers 모두 이 파일에 응집.

## 리스크 및 완화 (Risks & Mitigations)

| 리스크 | 영향 | 완화 전략 |
|--------|------|-----------|
| **운영자 yaml 마이그레이션 부담** | v0.3.0 부팅 시 기존 yaml 실패 | (1) 명시적 에러 메시지로 마이그레이션 경로 안내. (2) CHANGELOG에 before/after 예시. (3) 운영자 사전 공지. |
| **SPEC-WEB-005 UI 동기 업데이트 필요** | UI가 string 배열을 기대하면 키 표시 불가 | (1) 본 SPEC v0.3.0과 SPEC-WEB-005 v0.5.0 묶어서 PR. (2) UI 측 계약 테스트 (객체 배열 스키마 스냅샷). (3) BREAKING 노트 PR description. |
| **auto 모드 첫 쓰기 타입 고착** | 잘못 추론된 타입으로 후속 쓰기 거부 | (1) 첫 쓰기 신중성 강조 (문서). (2) 키 삭제 후 재쓰기 경로 (DELETE 엔드포인트 v0.2.0) 활용 가능. (3) 모니터링 알림 권장 (ErrTypeMismatch 빈도). |
| **`reflect` 또는 type switch의 엣지 케이스** | nil pointer, custom type, channel 등 | (1) type switch 우선 (성능 + 명확성), 6종 외 타입은 `ErrUnsupportedValueType`. (2) 단위 테스트로 분기 100% 커버. (3) custom type은 underlying type으로 평가. |
| **Set 호출당 검증 오버헤드** | hot path 성능 저하 | (1) `staticKeys` map lookup은 O(1). (2) 타입 매칭은 type switch (~ns 수준). (3) 벤치마크로 회귀 모니터링. |
| **`metric_type=""` 빈 결과 시맨틱 혼란** | 운영자가 전체 반환을 기대할 수 있음 | (1) M9에 명시 ("빈 값 = 빈 결과"). (2) 응답 헤더로 적용된 필터 echo (debug 보조). (3) 문서 예시 강화. |
| **`allow_dynamic_keys` 마이그레이션 누락** | 프로덕션 부팅 실패 | (1) 부팅 실패 시 stderr에 명시적 메시지 + 종료 코드. (2) 운영자 체크리스트. (3) Helm chart / 배포 스크립트에 grep 가드 추가 권장. |

## 테스트 전략 (Testing Strategy)

### 개발 방법론: Hybrid (DDD + TDD)

**DDD Characterization (v0.2.0 동작 보존 가능 영역):**
- v0.2.0의 `?tag=key:value` 단일/다중 필터 동작 → v0.3.0에서도 동일 결과 보장 (회귀 방지)
- DELETE 엔드포인트(`/keys/{key}`, `/keys` bulk) 동작 보존 — `staticKeys` 자료구조 변경에도 영향 없도록
- `Store.ClearHistory` 시맨틱 보존
- 빈 yaml 부팅 (`registration_type` 미설정 → `auto`, `keys` 미설정 → `[]`)

**DDD Migration (BREAKING 변경 검증):**
- v0.2.0 yaml (`allow_dynamic_keys: false`) 로드 시 명시적 에러 메시지 보장
- v0.2.0 응답 스키마 (`keys: []string`)는 v0.3.0에서 더 이상 발생하지 않음

**TDD (v0.3.0 신규 기능):**
- `inferDataType`: 6종 enum 매핑 + nil 거부 + json-marshalable + custom type 분기
- `matchesDataType`: 매칭/불일치 모든 조합
- parseStoreConfig: registration_type enum, data_type enum, metric_type 정규식, 마이그레이션 가드
- Set 경로: manual 모드 정상/거부, auto 모드 추론 등록 + 후속 mismatch 거부
- 신규 필터: 단일/다중/AND 결합/빈 값 시맨틱
- API 응답 객체 배열: 정렬, registration 필드, 동적 등록 키의 metric_type=unknown

### 커버리지 목표

- `internal/agent/system/store_options.go`: 90%+
- `internal/agent/system/store.go`: 85%+
- `internal/agent/system/store_data_type.go` (신규): 100% (모든 분기)
- `internal/agent/system/store_user_agent.go`: 85%+
- `internal/agent/system/store_query.go`: 85%+
- `internal/api/handler/store_*.go`: 85%+

### Race 테스트

- `go test -race ./internal/agent/system/...` 통과 필수
- auto 모드 동시 쓰기에서 staticKeys map 갱신 일관성 (mutex 보호 검증)

### 통합 테스트

- 실 YAML 파일 로드 → agent 부팅 → HTTP 요청 → 응답 검증 전체 파이프라인
- 마이그레이션 시나리오: v0.2.0 yaml → 부팅 실패 + 적절한 에러 메시지
- SPEC-WEB-005 v0.5.0과 객체 배열 응답 스키마 계약 테스트
