---
id: SPEC-STORE-003
title: Store 에이전트 정적 키 정의 및 태그 메타데이터 — 구현 계획
version: 0.1.0
status: draft
created: 2026-04-24
updated: 2026-04-24
author: xtra
priority: medium
---

# SPEC-STORE-003: 구현 계획 (Implementation Plan)

## 기술 스택 (Technical Stack)

### 언어 및 런타임

- **Go 1.22+** (기존 프로젝트 표준)
- 신규 외부 의존성 **없음**

### 라이브러리

- `gopkg.in/yaml.v3` (기존) — 설정 YAML 파싱
- `encoding/json` (stdlib) — API 응답 직렬화
- `net/url` + `net/http` (stdlib) — 쿼리 파라미터 파싱 및 HTTP 핸들링
- `regexp` (stdlib) — 태그 key 형식 검증

### 영향 범위

- `internal/agent/system/store.go` (config struct, defaults, parseStoreConfig)
- `internal/agent/system/store_user_agent.go` (쓰기 경로: Set, SetWithTTL)
- `internal/agent/system/store_query.go` (조회 경로: SeriesKeys, ListStoreKeys, GetEntries)
- `internal/api/handlers/store_*.go` (핸들러: keys, tags, keys?tag=)
- 테스트 파일: `internal/agent/system/store_*_test.go`, `internal/api/handlers/store_*_test.go`

## 기술적 접근 (Technical Approach)

### 1. Config 스키마 확장

`StoreConfig` struct에 다음 필드를 추가한다.

```go
type StoreConfig struct {
    // 기존 운영 필드 (변경 없음)
    scanInterval   time.Duration
    maxKeyLength   int
    maxHistorySize int
    historyTTL     time.Duration

    // 신규 데이터 필드
    allowDynamicKeys bool                           // 기본값 true
    staticKeys       map[string]map[string]string   // key → tags map
}
```

`parseStoreConfig` 확장:
- `allow_dynamic_keys` (bool) → 미설정 시 `true`
- `keys` ([]map[string]any) → 각 엔트리는 `{"key": string, "tags": map[string]string}`
- 파싱 시 중복 검증: 동일 key가 두 번 나타나면 `ErrDuplicateStaticKey`
- 태그 key 검증: `^[a-zA-Z0-9_-]+$` 위반 시 `ErrInvalidTagKey`

### 2. 에러 선언

```go
var (
    ErrKeyNotAllowed      = errors.New("store: key not allowed (not in static keys and allow_dynamic_keys=false)")
    ErrDuplicateStaticKey = errors.New("store: duplicate static key definition")
    ErrInvalidTagKey      = errors.New("store: invalid tag key (must match ^[a-zA-Z0-9_-]+$)")
)
```

### 3. 쓰기 경로 수정

모든 쓰기 진입점(`Set`, `SetWithTTL`, 기타 WriteOp 핸들러)의 최상단에 검증 추가:

```go
func (as *agentStore) Set(ctx context.Context, key string, value any) error {
    if !as.config.allowDynamicKeys {
        if _, ok := as.config.staticKeys[key]; !ok {
            return ErrKeyNotAllowed
        }
    }
    // 기존 로직 (max_key_length 검증 등)
    ...
}
```

거부된 쓰기는 엔트리에도 히스토리에도 기록되지 않는다(검증이 모든 저장 로직 앞에 위치).

### 4. 조회 경로에서 태그 병합

`ListStoreKeys`, `GetEntries` 등의 응답 빌더에서:
- 정적 키에 해당하면 `config.staticKeys[key]`에서 태그를 가져와 응답에 포함
- 동적 키(정적 목록에 없음)는 tags 필드 생략 또는 `null`

### 5. API 핸들러 변경

#### 5.1 `GET /api/v1/store/{name}/keys` 응답 확장

```json
{
  "count": 3,
  "keys": ["indoor:1:room_temp", "outdoor:temperature", "dynamic_key"],
  "tags": {
    "indoor:1:room_temp": {"room": "1", "type": "temperature"},
    "outdoor:temperature": {"location": "outside", "type": "temperature"}
  }
}
```
- `tags` 객체는 정적 키만 포함 (동적 키는 생략)
- 정적 키가 하나도 없으면 `tags` 필드 자체를 생략 (하위호환)

#### 5.2 신규 `GET /api/v1/store/{name}/tags`

```json
{
  "pairs": [
    {"key": "room", "values": ["1", "2", "3"]},
    {"key": "type", "values": ["temperature", "humidity"]},
    {"key": "location", "values": ["outside"]}
  ]
}
```
- 정적 키의 모든 태그를 유니크하게 수집
- `values` 배열은 정렬되어 반환 (안정적인 UI 렌더링)

#### 5.3 `?tag=key:value` 필터링

```
GET /api/v1/store/{name}/keys?tag=room:1&tag=type:temperature
```

- 각 `tag` 파라미터를 `strings.SplitN(v, ":", 2)`로 파싱
- 콜론 없으면 `400 Bad Request` + `{"error":"invalid tag format: expected key:value"}`
- 다중 태그는 AND 조건: 모든 지정된 태그를 포함해야 매칭

### 6. 하위호환 보장

- 기존 설정 파일(신규 필드 없음) → `parseStoreConfig`가 기본값(`allow_dynamic_keys=true`, `staticKeys=nil`)으로 초기화
- 기존 저장 데이터(엔트리 + 히스토리) → 정적 키 정의와 무관하게 보존 (검증은 신규 쓰기에만 적용)
- Characterization test로 기존 동작을 고정하여 회귀 방지

## 마일스톤 (Milestones)

### Primary Goal: 핵심 기능 구현
- Config 스키마 및 파싱 확장
- 쓰기 경로의 동적 키 거부 로직
- 조회 응답에 태그 포함

### Secondary Goal: API 확장
- `GET /api/v1/store/{name}/tags` 신규 엔드포인트
- `?tag=key:value` 필터링 구현
- 에러 응답 포맷 표준화

### Final Goal: 품질 확보
- 백엔드 테스트 커버리지 85% 이상 (DDD characterization + TDD)
- CHANGELOG 및 관련 문서 갱신
- SPEC-WEB-005 v0.4.0 연동 검증(API 호환성)

## Task Decomposition (10 작업)

1. **DTO/설정 파싱 확장** — `parseStoreConfig`에 `keys`, `allow_dynamic_keys` 파싱 로직 추가
2. **`StoreConfig` 구조체 필드 추가** — `allowDynamicKeys`, `staticKeys` 필드 + 기본값 초기화
3. **에러 선언 및 상수 정의** — `ErrKeyNotAllowed`, `ErrDuplicateStaticKey`, `ErrInvalidTagKey`, 태그 key 정규식
4. **쓰기 경로에서 동적 키 거부 로직** — `Set`, `SetWithTTL` 등에 사전 검증
5. **조회 경로 태그 병합** — `ListStoreKeys`, `SeriesKeys`, `GetEntries` 응답 빌더에 태그 첨부
6. **핸들러 확장: `/store/{name}/keys`** — 응답에 `tags` 맵 포함, 정적 키 없으면 필드 생략
7. **핸들러 신규: `/store/{name}/tags`** — 유니크 (key, values) 쌍 집계 및 정렬 반환
8. **핸들러 확장: `/store/{name}/keys?tag=`** — 쿼리 파싱, 검증(400), AND 필터링
9. **백엔드 테스트** — characterization (기존 동작 보존) + TDD (신규 기능) 세트 작성, 커버리지 85%+
10. **문서 및 CHANGELOG 갱신** — 설정 예시 추가, API 변경점 기록, 관련 SPEC 링크

## 아키텍처 영향 (Architecture Impact)

### 레이어 영향

- **설정 레이어** (`parseStoreConfig`): 신규 필드 파싱 추가, 기존 경로 보존
- **에이전트 코어** (`store.go`, `store_user_agent.go`): 쓰기 검증 훅 추가
- **쿼리 레이어** (`store_query.go`): 응답에 태그 병합
- **HTTP 핸들러 레이어**: 신규 엔드포인트 1개, 기존 엔드포인트 2개 확장
- **프로토콜 호환성**: 기존 클라이언트는 추가 필드를 무시하므로 호환

### 데이터 흐름

```
YAML Config → parseStoreConfig → StoreConfig{staticKeys, allowDynamicKeys}
                                           ↓
Write Request → [검증: allowDynamicKeys && key not in staticKeys → reject]
                                           ↓
                                   existing write pipeline
                                           ↓
Read Request → existing pipeline → [태그 병합: staticKeys[key]] → Response
```

## 리스크 및 완화 (Risks & Mitigations)

| 리스크 | 영향 | 완화 전략 |
|--------|------|-----------|
| 설정 파일 규모 증가 (100+ 키) | YAML 가독성 저하 | 향후 별도 SPEC에서 `keys_file: "./keys.yaml"` 외부 파일 지원 검토 |
| 사용자 실수로 중복 키 정의 | 설정 로드 실패 | 로드 시점 검증 + 명확한 에러 메시지(`ErrDuplicateStaticKey`) |
| 태그 key 특수문자 충돌 (URL 쿼리 파싱) | `?tag=a:b:c` 같은 모호한 파싱 | 태그 key 정규식 제한 + `SplitN(":", 2)`로 value 쪽 콜론 허용 |
| 하위호환 파손 | 기존 프로덕션 설정 로드 실패 | characterization test로 `parseStoreConfig` 기존 동작 고정, 신규 필드는 모두 optional |
| strict 모드 전환 시 운영 혼란 | 유효한 쓰기가 갑자기 거부 | 기본값 `allow_dynamic_keys=true` 유지, 운영자가 명시적으로 활성화해야 함 |

## 테스트 전략 (Testing Strategy)

### 개발 방법론: Hybrid (DDD + TDD)

**DDD Characterization (기존 코드 변경분):**
- `parseStoreConfig`에 신규 필드가 없는 기존 설정을 로드하는 테스트 → 동작 보존 확인
- 기존 Set/Get 경로가 strict 모드 OFF 상태에서 이전과 동일하게 동작
- 기존 `GET /keys` 응답 포맷이 정적 키 없을 때 기존과 동일

**TDD (신규 기능):**
- 신규 필드 파싱 (정상 케이스 + 중복 + 태그 key 위반)
- 동적 키 거부 로직 (strict 모드)
- 태그 포함 응답 (정적 키 + 동적 키 혼재 상황)
- 태그 필터 쿼리 (단일/다중/잘못된 형식)
- `/tags` 엔드포인트 유니크 집계

### 커버리지 목표

- `internal/agent/system/store.go`: 85%+
- `internal/agent/system/store_user_agent.go`: 85%+
- `internal/agent/system/store_query.go`: 85%+
- `internal/api/handlers/store_*.go`: 85%+

### Race 테스트

- `go test -race ./internal/agent/system/...` 통과 필수
- strict 모드에서 동시 쓰기 시도의 거부 일관성 확인

### 통합 테스트

- 실 YAML 파일 로드 → agent 부팅 → HTTP 요청 → 응답 검증 전체 파이프라인
- SPEC-WEB-005 v0.4.0 UI 소비자와의 계약 테스트 (API 응답 스키마 스냅샷)
