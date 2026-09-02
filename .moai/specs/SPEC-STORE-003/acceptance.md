---
id: SPEC-STORE-003
title: Store 에이전트 정적 키 정의 및 태그 메타데이터 — 수용 기준
version: 0.3.1
status: in_progress
created: 2026-04-24
updated: 2026-08-12
author: xtra
priority: medium
---

# SPEC-STORE-003: 수용 기준 (Acceptance Criteria)

## 시나리오 (Given-When-Then)

> v0.3.0 갱신: 시나리오 1~5는 v0.2.0 동작을 v0.3.0 모델(`registration_type`, `data_type`, `metric_type`)로 표현. 시나리오 6~10은 v0.3.0 신규.

### Scenario 1: 정적 키 + 데이터 타입 + 태그 정상 동작 (M1, M3, M7, M8)

**Given**
- Store 에이전트 설정에 다음이 정의되어 있다:
  ```yaml
  registration_type: "auto"
  keys:
    - key: "indoor:1:room_temp"
      data_type: "float"
      metric_type: "temperature"
      tags:
        room: "1"
  ```
- 에이전트가 정상 부팅되었다

**When**
- 클라이언트가 `SET indoor:1:room_temp = 22.5` (float64) 쓰기 요청을 전송한다
- 이후 `GET /api/v1/agents/{id}`를 호출하여 `state.entries`를 조회한다

**Then**
- 쓰기가 성공한다 (HTTP 200 또는 프로토콜별 성공 코드)
- 응답의 `entries` 배열에 해당 키 엔트리가 존재한다
- 해당 엔트리의 `tags` 필드가 `{"room": "1"}`와 일치한다
- 엔트리의 `value`가 `22.5`로 저장되어 있다
- `GET /api/v1/store/{name}/keys` 응답의 해당 키 객체에 `data_type="float"`, `metric_type="temperature"`, `registration="manual"`이 포함된다

---

### Scenario 2: Manual 모드에서 미등록 키 거부 (M2, M6)

**Given**
- Store 에이전트 설정에 `registration_type: "manual"`이 설정되어 있다
- `keys: [{key: "indoor:1:room_temp", data_type: "float", metric_type: "temperature", tags: {room: "1"}}]`만 정의되어 있다
- 에이전트가 정상 부팅되었다

**When**
- 클라이언트가 `SET outdoor:temperature = 35.0` 쓰기 요청을 전송한다 (미등록 키)

**Then**
- 쓰기가 거부된다 (`ErrKeyNotAllowed` 에러 반환)
- `state.entries`에 `outdoor:temperature` 키가 존재하지 않는다
- 히스토리에도 해당 쓰기 기록이 없다
- 기존 정적 키 `indoor:1:room_temp`에 대한 타입 일치 쓰기는 정상적으로 허용된다

---

### Scenario 3: Auto 모드에서 미등록 키 자동 등록 (M2, M6, M7)

**Given**
- Store 에이전트 설정에 `registration_type: "auto"` (또는 미설정 → 기본값)
- `keys: [{key: "indoor:1:room_temp", data_type: "float", metric_type: "temperature", tags: {room: "1"}}]`이 정의되어 있다

**When**
- 클라이언트가 `SET outdoor:temperature = 35.0` (float64) 쓰기 요청을 전송한다 (미등록 키)
- 이후 `GET /api/v1/store/{name}/keys`를 호출한다

**Then**
- 쓰기가 성공한다
- 응답의 `keys` 배열에 두 객체가 포함된다 (key 알파벳 정렬):
  - `{key: "indoor:1:room_temp", registration: "manual", data_type: "float", metric_type: "temperature", tags: {room: "1"}}`
  - `{key: "outdoor:temperature", registration: "auto", data_type: "float", metric_type: "unknown", tags: {}}`
- 정적 키의 메타데이터는 보존되며, 동적 등록된 키는 `registration="auto"`, `metric_type="unknown"`, 빈 태그를 갖는다

---

### Scenario 4: 태그 기반 키 필터링 보존 (M4)

**Given**
- Store 에이전트 설정에 다음 정적 키가 정의되어 있다 (`registration_type: "auto"`):
  - `indoor:1:room_temp` with `data_type=float`, `metric_type=temperature`, tags `{room: "1"}`
  - `indoor:2:room_temp` with `data_type=float`, `metric_type=temperature`, tags `{room: "2"}`
  - `outdoor:temperature` with `data_type=float`, `metric_type=temperature`, tags `{location: "outside"}`
- 모든 키에 값이 쓰여져 있다

**When**
- 클라이언트가 `GET /api/v1/store/{name}/keys?tag=room:1`을 호출한다

**Then**
- 응답 HTTP 상태 코드는 200이다
- 응답 JSON에서 `count == 1`이다
- `keys` 배열은 정확히 한 객체를 포함한다: `{key: "indoor:1:room_temp", ..., tags: {room: "1"}}`

**When (follow-up — multi-tag AND)**
- 클라이언트가 `GET /api/v1/store/{name}/keys?tag=room:2`를 호출한다

**Then**
- `count == 1`, `keys`는 `[{key: "indoor:2:room_temp", ...}]`이다

**When (edge case — invalid tag format)**
- 클라이언트가 `GET /api/v1/store/{name}/keys?tag=invalidformat`을 호출한다 (콜론 없음)

**Then**
- 응답 HTTP 상태 코드는 400이다
- 응답 바디에 에러 메시지 `"invalid tag format: expected key:value"`가 포함된다

---

### Scenario 5: 빈 설정 부팅 (M5)

**Given**
- Store 에이전트 설정 파일에 `keys`, `registration_type` 필드가 모두 없다
- 다른 운영 필드(`history_ttl`, `max_history_size`, `max_key_length`, `scan_interval`)만 존재한다
- `allow_dynamic_keys` 필드도 없다 (v0.3.0 깨끗한 신규 설정)

**When**
- 에이전트를 시작하고, 임의의 키 `arbitrary_key = "some_value"` (string)를 쓴다
- 이후 `GET /api/v1/store/{name}/keys`를 호출한다

**Then**
- 에이전트가 에러 없이 정상 부팅된다 (`registration_type` default `"auto"` 적용)
- 쓰기가 성공한다 (auto 모드, 첫 쓰기에서 `data_type="string"` 추론·등록)
- 응답의 `keys` 배열에 `{key: "arbitrary_key", registration: "auto", data_type: "string", metric_type: "unknown", tags: {}}`가 포함된다

---

### Scenario 6: v0.2.0 yaml 마이그레이션 실패 (M5, BREAKING) — v0.3.0 신규

**Given**
- Store 에이전트 설정 파일에 v0.2.0 형식의 `allow_dynamic_keys: false`가 존재한다:
  ```yaml
  agents:
    - type: store
      name: "system-store"
      config:
        allow_dynamic_keys: false
        keys:
          - key: "indoor:1:room_temp"
            tags:
              room: "1"
  ```

**When**
- 에이전트를 v0.3.0 바이너리로 시작한다

**Then**
- 에이전트는 부팅을 거부한다 (config load 실패)
- 에러 메시지에 다음이 포함된다: `"'allow_dynamic_keys' is removed in v0.3.0; use 'registration_type: manual|auto' instead"`
- 프로세스는 비정상 종료 코드를 반환한다 (운영 모니터링이 즉시 감지 가능)
- shim이나 자동 변환은 발생하지 않는다 (clean rename 의도)

---

### Scenario 7: Manual 모드 명시 data_type 검증 (M7) — v0.3.0 신규

**Given**
- Store 에이전트 설정에 다음이 정의되어 있다:
  ```yaml
  registration_type: "manual"
  keys:
    - key: "indoor:1:room_temp"
      data_type: "float"
      metric_type: "temperature"
      tags: {room: "1"}
  ```

**When (정상 케이스)**
- 클라이언트가 `SET indoor:1:room_temp = 22.5` (float64)를 전송한다

**Then**
- 쓰기가 성공한다

**When (타입 불일치)**
- 클라이언트가 `SET indoor:1:room_temp = "hot"` (string)를 전송한다

**Then**
- 쓰기가 거부된다
- `ErrTypeMismatch` 에러가 반환된다
- 엔트리·히스토리에 기록되지 않는다

**When (manual 모드 data_type 누락)**
- 다른 SPEC 설정에서 `registration_type: "manual"`인 키 엔트리가 `data_type` 없이 정의된다

**Then**
- 부팅이 실패한다
- `ErrInvalidDataType` 에러가 반환된다

---

### Scenario 8: Auto 모드 data_type 추론 + 후속 mismatch 거부 (M7) — v0.3.0 신규

**Given**
- Store 에이전트 설정에 `registration_type: "auto"`만 설정되어 있고 `keys`는 비어 있다
- 에이전트가 정상 부팅되었다

**When (첫 쓰기)**
- 클라이언트가 `SET sensor1 = 42` (Go int)를 전송한다

**Then**
- 쓰기가 성공한다
- 키 `sensor1`이 `data_type="int"`, `metric_type="unknown"`, `registration="auto"`로 자동 등록된다
- `GET /keys` 응답에서 확인 가능하다

**When (후속 동일 타입 쓰기)**
- 클라이언트가 `SET sensor1 = 100` (Go int)를 전송한다

**Then**
- 쓰기가 성공한다 (타입 일치)

**When (후속 다른 타입 쓰기)**
- 클라이언트가 `SET sensor1 = "broken"` (Go string)를 전송한다

**Then**
- 쓰기가 거부된다
- `ErrTypeMismatch` 에러가 반환된다 (첫 쓰기에서 결정된 `int` 타입은 영구 고정)
- 엔트리·히스토리에 기록되지 않는다

---

### Scenario 9: metric_type default 적용 + 필터 (M8, M9) — v0.3.0 신규

**Given**
- Store 에이전트 설정에 다음이 정의되어 있다:
  ```yaml
  registration_type: "manual"
  keys:
    - key: "k1"
      data_type: "float"
      metric_type: "temperature"
      tags: {}
    - key: "k2"
      data_type: "float"
      # metric_type 누락 → default "unknown"
      tags: {}
    - key: "k3"
      data_type: "float"
      metric_type: "humidity"
      tags: {}
  ```

**When**
- 클라이언트가 `GET /api/v1/store/{name}/keys?metric_type=temperature`를 호출한다

**Then**
- 응답 HTTP 상태 코드는 200이다
- `count == 1`
- `keys` 배열에 `{key: "k1", ..., metric_type: "temperature"}`만 포함된다
- `k2`(metric_type=`unknown`)와 `k3`(metric_type=`humidity`)는 제외된다

**When (default 확인)**
- 클라이언트가 `GET /api/v1/store/{name}/keys`를 호출한다 (필터 없음)

**Then**
- `keys` 배열에 세 객체가 모두 포함된다
- `k2` 객체의 `metric_type` 필드가 `"unknown"`으로 표시된다 (default 자동 적용)

---

### Scenario 10: API 응답 객체 배열 BREAKING (M9) — v0.3.0 신규

**Given**
- Store 에이전트 설정에 다음이 정의되어 있다 (`registration_type: "auto"`):
  ```yaml
  keys:
    - key: "alpha"
      data_type: "int"
      metric_type: "count"
      tags: {kind: "a"}
    - key: "beta"
      data_type: "string"
      tags: {kind: "b"}
  ```
- 그리고 클라이언트가 `SET gamma = true` (bool)을 호출하여 동적 등록되었다

**When**
- 클라이언트가 `GET /api/v1/store/{name}/keys`를 호출한다

**Then**
- 응답 JSON 구조는 다음 형태이다 (`keys`가 객체 배열):
  ```json
  {
    "count": 3,
    "keys": [
      {"key": "alpha", "registration": "manual", "data_type": "int",     "metric_type": "count",   "tags": {"kind": "a"}},
      {"key": "beta",  "registration": "manual", "data_type": "string",  "metric_type": "unknown", "tags": {"kind": "b"}},
      {"key": "gamma", "registration": "auto",   "data_type": "boolean", "metric_type": "unknown", "tags": {}}
    ]
  }
  ```
- `keys` 배열은 key 알파벳 오름차순으로 정렬되어 있다
- v0.2.0의 `keys: ["alpha", "beta", "gamma"]` 형식 + 별도 `tags` 맵은 응답에 **존재하지 않는다** (BREAKING 검증)
- 모든 객체는 5개 필드(`key`, `registration`, `data_type`, `metric_type`, `tags`)를 항상 포함한다 (빈 tags라도 `{}`로 명시)

**When (multi-filter AND)**
- 클라이언트가 `GET /api/v1/store/{name}/keys?registration=manual&data_type=int`을 호출한다

**Then**
- `count == 1`, `keys`는 `[{key: "alpha", ...}]`이다 (모든 필터 만족)

---

### Scenario 11: store-write `data_type: "auto"` 가변 타입 저장 (M7) — v0.3.1 신규

**Given**
- Store 에이전트가 `registration_type: "auto"`, 정적 키 없음으로 구성되어 있다
- store-write 노드가 `key_template: "{$.metadata.device.name}-{$.metadata.measurement}"`, `metrics[].data_type: "auto"`, `metrics[].value_key: "$.payload.value"`, `metrics[].metric_type: "$.metadata.measurement"` 로 설정되어 있다

**When**
- 다음 측정 메시지들이 순서대로 유입된다:
  - `measurement=temperature`, `value=25.4` (float)
  - `measurement=humidity`, `value=57` (int)
  - `measurement=power`, `value=false` (bool)

**Then**
- 각 시리즈 키가 값 타입대로 등록된다: `dev-temperature → float`, `dev-humidity → int`, `dev-power → boolean`
- 세 쓰기 모두 성공하며 `ErrTypeMismatch` 가 발생하지 않는다 (고정 리터럴 `float` 였다면 `power=false` 에서 실패했을 시나리오)
- 값이 문자열로 변환되지 않고 원래 숫자/불리언 타입으로 보존된다 (동적 string coercion 우회)

**When (같은 키 후속 쓰기)**
- `measurement=power`, `value=true` 가 다시 유입된다

**Then**
- 이미 boolean 으로 고정된 `dev-power` 에 boolean 값이므로 정상 저장된다 (키 단위 타입 일정성)

**When (추론 불가 값)**
- `value=nil` 인 쓰기가 유입된다

**Then**
- nil 은 auto 여부와 무관하게 거부된다 (일반 nil 쓰기와 동일; auto 가 nil 거부를 우회하지 않음)

---

## Edge Case Checklist

### v0.2.0 보존 (v0.3.0 모델로 표현)

- [ ] 동일 key가 `keys` 목록에 두 번 정의 → 설정 로드 에러 (`ErrDuplicateStaticKey`)
- [ ] 태그 key에 특수문자 포함 (예: `room.1`, `room 1`, `room:sub`) → 설정 로드 에러 (`ErrInvalidTagKey`)
- [ ] `keys` 배열이 빈 배열(`[]`) + `registration_type=auto` → 모든 쓰기 허용 (auto 등록)
- [ ] `keys` 배열이 빈 배열(`[]`) + `registration_type=manual` → 모든 쓰기 거부 (잠금 모드)
- [ ] 태그 value에 빈 문자열(`""`) → 허용 (key 검증만 수행)
- [ ] `?tag=key:` (value 없음) → 빈 value로 매칭 시도, 매칭되는 키가 없으면 빈 결과 반환
- [ ] `?tag=key:value:extra` (콜론 두 개 이상) → `SplitN(":", 2)`로 `value:extra`를 value로 취급
- [ ] URL 인코딩된 태그 (`?tag=room%3A1`) → 정상 디코딩 후 `room:1`로 파싱
- [ ] 대소문자 구분: `?tag=Room:1`과 `?tag=room:1`은 다른 태그 (case-sensitive)
- [ ] 동시성: manual 모드에서 다수 고루틴이 미등록 키 쓰기 시도 → 모두 거부 (`go test -race` 통과)
- [ ] 설정 reload 중 진행 중인 쓰기 → 데이터 일관성 유지
- [ ] `/tags` 엔드포인트에서 정적 키가 하나도 없을 때 → `{"pairs": []}` 반환
- [ ] 매우 많은 정적 키(1000+) + 태그 필터링 → 성능 저하 없이 반환

### v0.3.0 신규

- [ ] **Manual 모드 data_type 누락** — yaml에 `registration_type: manual`인데 `keys[].data_type`이 없음 → 부팅 실패 (`ErrInvalidDataType`)
- [ ] **Auto 모드 nil 값 쓰기** — `SET key = nil` → 거부 (`ErrUnsupportedValueType`)
- [ ] **Auto 모드 []byte 쓰기** — `SET key = []byte{0x01, 0x02}` → 등록 (`data_type="bytes"`)
- [ ] **Auto 모드 map 쓰기** — `SET key = map[string]any{"a": 1}` → 등록 (`data_type="json"`)
- [ ] **Auto 모드 slice 쓰기** — `SET key = []int{1, 2, 3}` → 등록 (`data_type="json"`)
- [ ] **Auto 모드 struct 쓰기** — json-marshalable struct → 등록 (`data_type="json"`)
- [ ] **Auto 모드 channel 쓰기** — `SET key = make(chan int)` → 거부 (`ErrUnsupportedValueType`)
- [ ] **타입 진화 시도 차단** — auto 등록 키가 int, 후속 float64 쓰기 → `ErrTypeMismatch` (자동 진화 없음)
- [ ] **metric_type 정규식 위반** — `metric_type: "room.temp"` (점 포함) → 부팅 실패 (`ErrInvalidMetricType`)
- [ ] **metric_type 빈 문자열** — yaml에 `metric_type: ""` → default `"unknown"` 적용
- [ ] **data_type enum 외 값** — `data_type: "double"` → 부팅 실패 (`ErrInvalidDataType`)
- [ ] **registration_type enum 외 값** — `registration_type: "automatic"` → 부팅 실패
- [ ] **`allow_dynamic_keys` 잔존** — yaml에 v0.2.0 필드가 있으면 명시적 마이그레이션 에러로 부팅 실패
- [ ] **Multi-filter AND 결합** — `?registration=manual&metric_type=temperature&tag=room:1` → 모든 조건 만족 키만
- [ ] **`?metric_type=` 빈 값** — 빈 결과 반환 (전체 반환 아님)
- [ ] **`?data_type=invalid` (enum 외)** — 빈 결과 반환 (HTTP 200 + 빈 keys 배열, 또는 400 — 핸들러 정책에 명시)
- [ ] **응답 정렬 안정성** — `keys` 배열은 항상 key 알파벳 오름차순 정렬
- [ ] **빈 tags 객체 표시** — auto 등록 키의 `tags` 필드는 `{}`로 명시 (생략 안 함)
- [ ] **Auto 등록의 동시성 안전성** — 동일 키에 대한 동시 첫 쓰기 → mutex 보호로 한 번만 등록, 다른 호출자는 등록된 타입에 따라 처리
- [ ] **int8/uint64 등 다양한 정수 타입** — 모두 `data_type="int"`로 추론 (단일 enum)
- [ ] **float32 vs float64** — 모두 `data_type="float"`로 추론

---

## TRUST 5 품질 게이트

### T — Tested (테스트)

- [ ] `internal/agent/system/store_options.go` 커버리지 ≥ 90%
- [ ] `internal/agent/system/store.go` 커버리지 ≥ 85%
- [ ] `internal/agent/system/store_data_type.go` (신규) 커버리지 **= 100%** (모든 type switch 분기 + nil + json-marshalable)
- [ ] `internal/agent/system/store_user_agent.go` 커버리지 ≥ 85%
- [ ] `internal/agent/system/store_query.go` 커버리지 ≥ 85%
- [ ] `internal/api/handler/store_*.go` 커버리지 ≥ 85%
- [ ] DDD characterization test: v0.2.0 보존 가능 동작(태그 필터, DELETE 엔드포인트, ClearHistory, 빈 설정 부팅) 회귀 없음
- [ ] DDD migration test: v0.2.0 yaml(`allow_dynamic_keys`)는 v0.3.0에서 부팅 실패 + 명시적 메시지
- [ ] TDD test: 신규 4개 에러(`ErrTypeMismatch`, `ErrUnsupportedValueType`, `ErrInvalidDataType`, `ErrInvalidMetricType`) 모두 케이스 커버
- [ ] TDD test: API 객체 배열 응답 + 신규 필터 AND 결합 모든 조합
- [ ] `go test -race ./...` 통과 (auto 등록 동시성 포함)

### R — Readable (가독성)

- [ ] 신규 struct (`StaticKeyMeta`), enum (`RegistrationType`, `DataType`, `RegistrationSource`), 함수 (`inferDataType`, `matchesDataType`)에 Go doc 코멘트 추가
- [ ] type switch 분기에 의미 주석 (특히 json-marshalable 판별)
- [ ] 한국어 주석 가능 (프로젝트 언어 설정 `code_comments=ko`)
- [ ] `gofmt`, `goimports` 준수

### U — Unified (통일성)

- [ ] 기존 에러 네이밍 패턴(`ErrXxx`) 준수 (신규 4종 포함)
- [ ] 기존 config 파싱 패턴(`parseXxxConfig`) 준수
- [ ] HTTP 핸들러의 에러 응답 포맷 기존 convention과 일치 (`{"error": "..."}`)
- [ ] JSON 필드 네이밍 (snake_case) 기존 API와 일치 (`data_type`, `metric_type`, `registration`)
- [ ] enum 상수 네이밍: `RegistrationManual`, `DataTypeFloat` 등 일관된 prefix

### S — Secured (보안)

- [ ] URL 쿼리 파싱 시 부적절한 입력 방어 (매우 긴 metric_type, null byte, 매우 긴 data_type 값)
- [ ] 설정 파일 파싱 시 타입 캐스팅 에러 안전 처리 (panic 없음)
- [ ] manual 모드 거부 시 스택 트레이스나 내부 경로 누출 없음
- [ ] metric_type 정규식으로 injection 가능성 제거 (`^[a-zA-Z0-9_-]+$`만 허용)
- [ ] `inferDataType`이 untrusted input(특히 json) 처리 시 무한 재귀나 DoS 회피
- [ ] auto 등록 폭주 방지: 미등록 키 무제한 등록은 메모리 압박 → 향후 SPEC에서 max_static_keys 검토 (현재 SPEC 범위 외)

### T — Trackable (추적성)

- [ ] SPEC @TAG 주석: `// @SPEC:SPEC-STORE-003 v0.3.0` 신규 코드 블록(특히 `store_data_type.go`)에 삽입
- [ ] Conventional commit 메시지 (`feat(store)!: rename allow_dynamic_keys to registration_type, add data_type and metric_type [SPEC-STORE-003]`) — `!` BREAKING 표식
- [ ] CHANGELOG.md에 변경 항목 기재:
  - BREAKING: `allow_dynamic_keys` → `registration_type` clean rename
  - BREAKING: `keys` API 응답 객체 배열 진화
  - 신규 필드: `data_type`, `metric_type`
  - 신규 필터: `?data_type=`, `?metric_type=`, `?registration=`
  - 신규 에러: 4종
- [ ] **마이그레이션 가이드** 문서 별도 섹션으로 CHANGELOG 또는 `docs/migration/v0.3.0.md`에 포함:
  - yaml before/after 예시
  - 각 신규 필드 의미 설명
  - 운영 체크리스트 (grep으로 잔존 `allow_dynamic_keys` 검출)
- [ ] 설정 예시 문서(`references/design/` 또는 관련 도큐먼트)에 v0.3.0 예시로 갱신

---

## Definition of Done (DoD)

본 SPEC v0.3.0이 완료되려면 다음 조건을 모두 만족해야 한다.

1. **기능 완전성**
   - [ ] 9개 EARS 모듈(M1~M9)의 모든 요구사항이 구현됨
   - [ ] 10개 Given-When-Then 시나리오가 자동화 테스트로 통과
   - [ ] Edge case 체크리스트 항목이 모두 검증됨 (v0.2.0 보존 + v0.3.0 신규)

2. **품질 기준**
   - [ ] TRUST 5 품질 게이트 모든 항목 통과
   - [ ] `go test -race ./...` 전체 통과
   - [ ] `go vet` 및 `golangci-lint` 경고 0
   - [ ] `inferDataType` 분기 커버리지 100%

3. **마이그레이션 준수**
   - [ ] v0.2.0 yaml(`allow_dynamic_keys` 포함)은 v0.3.0에서 명확한 에러 메시지로 부팅 실패
   - [ ] **마이그레이션 가이드**가 CHANGELOG에 포함됨 (yaml before/after, 운영 체크리스트)
   - [ ] 코드베이스에서 `allowDynamicKeys` / `allow_dynamic_keys` 식별자 잔존 0건 (grep 검증)
   - [ ] 코드베이스에서 v0.2.0의 `staticKeys map[string]map[string]string` (tag만) 시그니처 잔존 0건

4. **API 변경 검증**
   - [ ] `keys` 응답이 객체 배열로만 제공됨 (string 배열 잔존 없음)
   - [ ] 모든 신규 필터(`?data_type=`, `?metric_type=`, `?registration=`)가 `?tag=`와 AND 결합으로 동작
   - [ ] 응답 객체의 5개 필드(`key`, `registration`, `data_type`, `metric_type`, `tags`)가 항상 포함됨

5. **문서화**
   - [ ] spec.md, plan.md, acceptance.md 3개 파일 v0.3.0 일관 갱신
   - [ ] CHANGELOG 갱신 (BREAKING 표식 포함)
   - [ ] 설정 예시 문서가 v0.3.0 형식으로 갱신
   - [ ] 신규 에러 4종이 운영자 문서에 기재

6. **통합**
   - [ ] **SPEC-WEB-005 v0.5.0과 계약 동기화** — UI는 객체 배열 응답에 적응 + `data_type`/`metric_type` 편집 UI 제공
   - [ ] 기존 SPEC-STORE-001, SPEC-STORE-002 기능 회귀 없음
   - [ ] DELETE 엔드포인트(v0.2.0)는 v0.3.0 모델에서도 동일하게 동작

7. **릴리즈 준비**
   - [ ] 변경 요약 + 마이그레이션 가이드를 포함한 PR 생성 (BREAKING 표식)
   - [ ] 코드 리뷰 1회 이상 승인
   - [ ] 병합 가능 상태 (merge-ready)
   - [ ] 운영팀 사전 공지 (BREAKING 변경 임박 알림)
