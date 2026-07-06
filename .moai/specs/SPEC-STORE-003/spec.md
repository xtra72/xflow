---
id: SPEC-STORE-003
title: Store 에이전트 정적 키 정의 및 태그 메타데이터
version: 0.3.0
status: in_progress
created: 2026-04-24
updated: 2026-05-04
author: xtra
priority: medium
---

# SPEC-STORE-003: Store 에이전트 정적 키 정의 및 태그 메타데이터

## HISTORY

- **0.3.0** (2026-05-04): Store 키 메타데이터 모델 진화 (BREAKING CHANGE):
  (1) `allow_dynamic_keys` (bool) → `registration_type` (enum: `manual` | `auto`) **clean rename**. 하위호환 shim 없음. 기존 yaml 파일은 마이그레이션 필수.
  (2) **`data_type` 1급 필드 신설** — 6종 enum (`int` | `float` | `string` | `boolean` | `bytes` | `json`). manual 모드는 명시 선언 필수, auto 모드는 첫 쓰기에서 추론·고정.
  (3) **`metric_type` 1급 필드 신설** — semantic classification (예: `temperature`, `humidity`). 정규식 `^[a-zA-Z0-9_-]+$`, 기본값 `"unknown"`.
  (4) **API 응답 진화 (BREAKING)** — `GET /api/v1/store/{name}/keys` 응답의 `keys` 필드가 string 배열에서 object 배열 `[{key, registration, data_type, metric_type, tags}]`로 변경.
  (5) **신규 필터 쿼리** — `?data_type=`, `?metric_type=`, `?registration=`. 기존 `?tag=`와 AND 조합.
  (6) **신규 에러** — `ErrTypeMismatch`, `ErrUnsupportedValueType`, `ErrInvalidDataType`, `ErrInvalidMetricType`.
- **0.2.0** (2026-04-27): Store 에이전트 확장 기능:
  (1) 동적 키 → 정적 변환 UI 및 PromoteToStaticDialog (Configure API 재사용, 백엔드 변경 0).
  (2) 저장소 전체/개별 키 초기화: `DELETE /api/v1/store/{name}/keys/{key}` (정적: history만, 동적: entry 완전 삭제), `DELETE /api/v1/store/{name}/keys` (bulk). 신규 백엔드 `Store.ClearHistory()` 메서드.
  (3) 버킷 타임스탬프 벽시계 경계 정렬 (`floor(tsMs / intervalMs) * intervalMs`).
  (4) 정적 키 태그 입력 blur 자동 commit (TagChipsEditor pending input 유실 버그 수정).
  (5) UI 개선: StoreKeysEditor 키:태그 1:3 비율, readOnly 가시성 복원 (FormField + StoreKeysEditor 외 8종 property 편집기), 저장소 탭 행별 액션 분리 및 전체 초기화 버튼 색상 중립화.

| Version | Date       | Author | Change                                                                      |
| ------- | ---------- | ------ | --------------------------------------------------------------------------- |
| 0.3.0   | 2026-05-04 | xtra   | **BREAKING**: allow_dynamic_keys → registration_type, data_type/metric_type 1급 필드, API keys 응답 객체 배열 진화, 신규 필터 쿼리, 신규 에러 4종 |
| 0.2.0   | 2026-04-27 | xtra   | 동적→정적 변환 UI, 키 초기화 (DELETE 2개 엔드포인트), 버킷 벽시계 정렬, 태그 blur commit, UI 개선 |
| 0.1.0   | 2026-04-24 | xtra   | 최초 작성 — 정적 키 목록, 태그 메타데이터, 동적 키 제어, 태그 필터링 API 도입 |

## 개요 (Overview)

Store 에이전트(SPEC-STORE-001, SPEC-STORE-002 기반)는 v0.2.0에서 정적 키 정의와 태그 메타데이터를 도입했다. **v0.3.0은 키 메타데이터 모델을 본격적으로 강화**하여 다음 능력을 추가한다.

1. **등록 방식 명시화** (`registration_type`): bool `allow_dynamic_keys`를 enum `manual` | `auto`로 clean rename. 의도 표현이 명확해지고 향후 확장(예: `mixed` 모드) 여지 확보.
2. **데이터 타입 1급화** (`data_type`): 키마다 6종 타입 중 하나를 선언 또는 추론하여 strict type checking. manual 모드는 yaml 명시 선언 필수, auto 모드는 첫 쓰기 값에서 자동 추론·고정.
3. **메트릭 타입 분류** (`metric_type`): semantic 분류(예: `temperature`, `humidity`)를 1급 필드로 표현. UI 그룹화·필터·시각화 단위 매핑 등에 활용.
4. **API 응답 객체화 (BREAKING)**: `keys` 응답이 `[{key, registration, data_type, metric_type, tags}]` 객체 배열로 진화. 풍부한 메타데이터를 단일 라운드트립으로 노출.
5. **다축 필터링**: `?data_type=`, `?metric_type=`, `?registration=` 신규 필터를 기존 `?tag=`와 AND 조합 가능.

**Breaking Change Notice**: v0.2.0의 `allow_dynamic_keys` 필드와 string 배열 `keys` 응답은 v0.3.0에서 제거된다. 기존 yaml 파일은 부팅에 실패하며 운영자는 명시적 마이그레이션이 필요하다. 하위호환 shim은 의도적으로 제공하지 않는다(설계 단순성 우선).

## 배경 (Background)

### v0.2.0의 한계

- `allow_dynamic_keys=false`라는 부정형 bool은 의도 표현이 모호하다. "허용 안 함"보다 "수동 등록 모드"가 자연스럽다.
- 동일 키에 대해 `22` (int)와 `"22"` (string)가 혼재 저장되면 다운스트림(쿼리·시각화·집계)에서 타입 캐스팅 부담.
- 태그(`map[string]string`) 안에 `type=temperature`처럼 의미 분류를 넣는 방식은 1급 시민이 아니어서 필터·집계가 약하다.

### v0.3.0의 목표

- **명시적 의도**: `registration_type` enum으로 등록 정책을 표현
- **타입 안정성**: `data_type` 선언/추론 + 후속 쓰기 타입 일치 검증으로 데이터 일관성 보장
- **시맨틱 1급화**: `metric_type`을 별도 필드로 승격하여 도메인 분류를 표현

### SDD 2025 Constitution 정합성

- Go 1.22+ 기존 기술 스택 (신규 의존성 없음)
- `reflect` (stdlib) 또는 type switch 사용 (auto 추론용)
- 하위호환은 의도적으로 포기 (clean rename) — 명확한 마이그레이션 가이드로 보완

## EARS 요구사항 (EARS Requirements)

본 SPEC은 9개 EARS 모듈로 구성된다 (v0.2.0 M1~M5 + v0.3.0 신규 M6~M9).

---

### M1: 정적 키 정의 (Static Key Definition)

- **Ubiquitous**: 시스템은 Store 에이전트 설정에 정적 키 목록(`keys`) 필드를 optional로 지원해야 한다.
- **Ubiquitous**: 각 정적 키 엔트리는 `key` (string, 필수), `data_type` (enum, manual 모드 필수), `metric_type` (string, optional, default `"unknown"`), `tags` (map[string]string, optional)를 포함한다.
- **State-driven**: IF `keys` 필드가 설정되지 않거나 빈 배열인 경우, THEN 시스템은 기존 동작(모든 키 자유 쓰기)을 `registration_type`에 따라 결정해야 한다.
- **Unwanted**: 시스템은 동일한 `key` 값을 갖는 중복 엔트리가 `keys` 목록에 존재하면 설정 로드 시 에러(`ErrDuplicateStaticKey`)를 반환해야 한다.
- **Unwanted**: 시스템은 태그 key가 정규식 `^[a-zA-Z0-9_-]+$`를 만족하지 않으면 설정 로드 시 에러(`ErrInvalidTagKey`)를 반환해야 한다.

---

### M2: 등록 방식과 미등록 키 거부 (Registration Type and Unregistered Key Rejection)

> v0.3.0에서 `allow_dynamic_keys` (bool) → `registration_type` (enum)로 **clean rename**. 의미는 보존되나 yaml 키와 코드 식별자 모두 변경된다.

- **Ubiquitous**: 시스템은 `registration_type` enum 필드를 Store 에이전트 설정에 지원해야 한다. 허용 값은 `"manual"`과 `"auto"`이며, 기본값은 `"auto"`이다.
- **State-driven**: WHILE `registration_type == "auto"`, 시스템은 정적 키 목록에 없는 키 쓰기 요청을 허용하고 첫 쓰기 시점에 키를 자동 등록해야 한다 (data_type은 M7에 따라 추론).
- **Unwanted**: WHEN `registration_type == "manual"` AND 쓰기 요청의 키가 정적 키 목록에 없으면, THEN 시스템은 쓰기를 거부하고 `ErrKeyNotAllowed`를 반환해야 한다.
- **Unwanted**: 거부된 쓰기는 엔트리에도 히스토리에도 기록되지 않아야 한다.
- **Unwanted**: WHEN 설정에 `allow_dynamic_keys` 필드가 존재하면, THEN 시스템은 설정 로드를 실패시키고 `registration_type`으로 마이그레이션하라는 명시적 에러 메시지를 반환해야 한다 (clean rename, no shim).

---

### M3: 태그 조회 및 응답 포함 (Tag Retrieval and Response Inclusion)

- **Ubiquitous**: 시스템은 엔트리 조회 응답(`GET /api/v1/agents/{id}` → `state.entries`)에 정적 키의 태그를 포함해야 한다.
- **Ubiquitous**: 시스템은 키 목록 조회 응답(`GET /api/v1/store/{name}/keys`)에 각 키의 `data_type`, `metric_type`, `registration`, `tags`를 포함해야 한다 (M9 참조).
- **State-driven**: IF 키가 정적 키 목록에 없고 동적으로 쓰여진 경우 (auto 모드), THEN 응답의 `tags`는 빈 객체(`{}`)로 표현되며 `metric_type`은 `"unknown"`, `data_type`은 추론된 값으로 표현된다.
- **Optional**: WHERE API 호출자가 `GET /api/v1/store/{name}/tags`를 요청하면, 시스템은 전체 정적 키 태그의 유니크한 (key, values) 쌍 목록을 반환한다.

---

### M4: 태그 기반 키 필터링 (Tag-Based Key Filtering)

- **Event-driven**: WHEN `GET /api/v1/store/{name}/keys?tag=key:value` 요청이 오면, THEN 시스템은 해당 태그(key=value)를 포함하는 키만 반환해야 한다.
- **Event-driven**: WHEN 다수의 `tag` 쿼리 파라미터(`?tag=room:1&tag=type:temperature`)가 전달되면, THEN 시스템은 **모든** 태그를 포함하는 키만 반환해야 한다 (AND 조건).
- **State-driven**: IF `tag` 쿼리 파라미터가 없으면, THEN 시스템은 전체 키 목록을 반환한다 (기존 동작 유지).
- **Unwanted**: IF `tag` 쿼리 값이 `key:value` 형식이 아니면 (예: 콜론 없음), THEN 시스템은 HTTP 400 Bad Request를 반환하고 에러 메시지 `"invalid tag format: expected key:value"`를 응답 바디에 포함해야 한다.
- **Event-driven**: WHEN `?tag=`와 신규 필터(`?data_type=`, `?metric_type=`, `?registration=`)가 동시에 사용되면, THEN 모든 필터가 AND 조건으로 결합되어야 한다 (M9 참조).

---

### M5: 설정 마이그레이션 정책 (Configuration Migration Policy)

> v0.3.0에서 의미 변경: 기존 v0.2.0 yaml은 더 이상 호환되지 않는다.

- **Ubiquitous**: 시스템은 `keys` 및 `registration_type` 필드가 모두 없는 설정 파일을 로드할 때, `registration_type=auto`, `keys=[]`로 초기화하고 정상 부팅해야 한다.
- **Unwanted**: WHEN 설정 파일에 `allow_dynamic_keys` 필드가 존재하면, THEN 시스템은 부팅을 실패시키고 다음과 같은 명시적 마이그레이션 에러를 반환해야 한다: `"config: 'allow_dynamic_keys' is removed in v0.3.0; use 'registration_type: manual|auto' instead"`.
- **Unwanted**: WHEN `registration_type == "manual"` AND `keys` 엔트리에 `data_type` 필드가 누락되면, THEN 시스템은 부팅을 실패시키고 `ErrInvalidDataType`을 반환해야 한다 (manual 모드는 명시 선언 필수).
- **Event-driven**: WHEN 새로운 설정이 agent config reload로 적용되면, THEN 정적 키 정의가 변경되더라도 기존 저장된 엔트리 및 히스토리 데이터는 보존되어야 한다.

---

### M6: 등록 방식 (Registration Type) — v0.3.0 신규

- **Ubiquitous**: 시스템은 `registration_type` 필드를 enum으로 지원해야 한다. 허용 값: `"manual"`, `"auto"`. 기본값: `"auto"`.
- **Unwanted**: WHEN `registration_type` 값이 `"manual"`/`"auto"` 외의 임의 문자열이면, THEN 설정 로드를 실패시키고 명시적 에러를 반환해야 한다.
- **Ubiquitous**: 시스템은 키 목록 조회 응답 객체(M9)에서 각 키의 등록 출처를 `registration` 필드로 표현해야 한다. 값은 `"manual"`(yaml에 명시 정의된 키) 또는 `"auto"`(쓰기 시점에 자동 등록된 키)이다.
- **State-driven**: IF `registration_type == "auto"`, THEN yaml에 명시 정의된 키는 `registration="manual"`로, 쓰기 시점에 자동 등록된 키는 `registration="auto"`로 표시된다 (혼재 가능).
- **State-driven**: IF `registration_type == "manual"`, THEN 모든 키는 `registration="manual"`이다 (auto 등록 경로 차단).

---

### M7: 데이터 타입 (Data Type) — v0.3.0 신규

- **Ubiquitous**: 시스템은 `data_type` 필드를 enum으로 지원해야 한다. 허용 값: `"int"`, `"float"`, `"string"`, `"boolean"`, `"bytes"`, `"json"` (총 6종).
- **State-driven**: IF `registration_type == "manual"`, THEN 모든 정적 키 엔트리는 `data_type` 필드를 yaml에 명시 선언해야 하며, 누락 시 `ErrInvalidDataType`로 부팅 실패한다.
- **State-driven**: IF `registration_type == "auto"` AND yaml의 키 엔트리에 `data_type`이 명시되면, THEN 해당 선언이 우선하며 후속 쓰기는 그 타입에 맞아야 한다.
- **Event-driven**: WHEN `registration_type == "auto"` AND 쓰기 요청의 키가 정적 키 목록에 없으면, THEN 시스템은 쓰기 값에서 `data_type`을 다음 매핑에 따라 추론하여 키를 등록해야 한다:
  - Go `bool` → `"boolean"`
  - Go `int` / `int8`..`int64` / `uint`..`uint64` → `"int"`
  - Go `float32` / `float64` → `"float"`
  - Go `string` → `"string"`
  - Go `[]byte` → `"bytes"`
  - Go `map`, `slice`, struct (json-marshalable) → `"json"`
- **Unwanted**: WHEN 쓰기 값이 `nil`이면, THEN 시스템은 쓰기를 거부하고 `ErrUnsupportedValueType`을 반환해야 한다 (auto 모드 추론 불가).
- **Unwanted**: WHEN 쓰기 값의 Go 타입이 등록된 `data_type`과 일치하지 않으면 (manual 또는 auto 등록 후 후속 쓰기), THEN 시스템은 쓰기를 거부하고 `ErrTypeMismatch`를 반환해야 한다.
- **Unwanted**: 한번 결정된 키의 `data_type`은 변경 불가하다 (auto 모드의 첫 쓰기 타입은 영구 고정). 타입 변경이 필요하면 키 삭제 후 재등록한다.
- **Unwanted**: WHEN yaml의 `data_type` 값이 6종 enum 외의 임의 문자열이면, THEN 설정 로드를 실패시키고 `ErrInvalidDataType`을 반환해야 한다.

---

### M8: 메트릭 타입 (Metric Type) — v0.3.0 신규

- **Ubiquitous**: 시스템은 `metric_type` 필드를 free string으로 지원하되, 정규식 `^[a-zA-Z0-9_-]+$`를 만족해야 한다.
- **Ubiquitous**: 시스템은 yaml에 `metric_type`이 누락되었거나 빈 문자열이면 기본값 `"unknown"`을 적용해야 한다.
- **Unwanted**: WHEN yaml의 `metric_type` 값이 정규식을 위반하면 (예: 공백, `.`, `:` 포함), THEN 설정 로드를 실패시키고 `ErrInvalidMetricType`을 반환해야 한다.
- **Ubiquitous**: 시스템은 키 목록 조회 응답 객체(M9)에서 각 키의 `metric_type`을 항상 노출해야 한다 (auto 등록 키는 `"unknown"`).
- **Optional**: WHERE 운영자가 `metric_type`을 활용한 시맨틱 분류(예: `temperature`, `humidity`, `pressure`)를 원하면, 자유롭게 도메인 어휘를 정의할 수 있다 (시스템은 값을 강제하지 않음).

---

### M9: API 응답 진화 (API Response Evolution) — v0.3.0 신규 (BREAKING)

> v0.2.0의 `keys: ["k1", "k2"]` string 배열은 v0.3.0에서 객체 배열로 **breaking change**된다. UI 소비자(SPEC-WEB-005)는 동시 업데이트가 필요하다.

- **Ubiquitous**: 시스템은 `GET /api/v1/store/{name}/keys` 응답의 `keys` 필드를 다음 객체 배열 스키마로 반환해야 한다:
  ```json
  {
    "count": 2,
    "keys": [
      {
        "key": "indoor:1:room_temp",
        "registration": "manual",
        "data_type": "float",
        "metric_type": "temperature",
        "tags": {"room": "1"}
      },
      {
        "key": "sensor_dyn",
        "registration": "auto",
        "data_type": "int",
        "metric_type": "unknown",
        "tags": {}
      }
    ]
  }
  ```
- **Event-driven**: WHEN `?data_type=<value>` 쿼리 파라미터가 전달되면, THEN 시스템은 해당 `data_type`을 갖는 키만 반환해야 한다.
- **Event-driven**: WHEN `?metric_type=<value>` 쿼리 파라미터가 전달되면, THEN 시스템은 해당 `metric_type`을 갖는 키만 반환해야 한다.
- **Event-driven**: WHEN `?registration=<value>` 쿼리 파라미터가 전달되면 (값: `manual` 또는 `auto`), THEN 시스템은 해당 등록 출처의 키만 반환해야 한다.
- **Event-driven**: WHEN 다수 필터가 동시 사용되면 (`?registration=manual&metric_type=temperature&tag=room:1`), THEN 모든 필터가 **AND 조건**으로 결합되어 모두 만족하는 키만 반환되어야 한다.
- **State-driven**: IF 필터 값이 빈 문자열(`?metric_type=`)이면, THEN 시스템은 매칭되는 키가 없는 것으로 간주하고 빈 결과를 반환한다 (전체 반환이 아님).
- **Ubiquitous**: 응답의 `keys` 배열은 `key` 알파벳 오름차순으로 정렬되어 안정적인 클라이언트 렌더링을 보장해야 한다.
- **Ubiquitous**: v0.2.0의 string 배열 형식 `keys: ["k1", "k2"]`와 분리된 `tags` 맵은 v0.3.0에서 **제거**된다 (BREAKING). 모든 메타데이터는 객체 배열 안으로 통합된다.

## 명세 (Specifications)

### Config 스키마 (v0.3.0)

```yaml
agents:
  - type: store
    name: "system-store"
    config:
      # 운영 필드 (변경 없음)
      scan_interval: "30s"
      max_key_length: 512
      history_ttl: "1h"
      max_history_size: 1000

      # v0.3.0 신규 / 변경 필드
      registration_type: "manual"     # enum: manual | auto, default auto
      keys:
        - key: "indoor:1:room_temp"
          data_type: "float"          # 필수 (manual 모드)
          metric_type: "temperature"  # optional, default "unknown"
          tags:
            room: "1"
        - key: "indoor:1:humidity"
          data_type: "float"
          metric_type: "humidity"
          tags:
            room: "1"
        - key: "device:event"
          data_type: "json"
          metric_type: "event"
          tags:
            kind: "telemetry"
```

**v0.2.0 → v0.3.0 마이그레이션 예시:**

```yaml
# Before (v0.2.0)
allow_dynamic_keys: false
keys:
  - key: "indoor:1:room_temp"
    tags:
      room: "1"
      type: "temperature"

# After (v0.3.0)
registration_type: "manual"
keys:
  - key: "indoor:1:room_temp"
    data_type: "float"
    metric_type: "temperature"   # tags의 type=temperature를 1급 필드로 승격
    tags:
      room: "1"                  # 나머지 태그는 그대로
```

### API 변경 (v0.3.0)

1. **`GET /api/v1/store/{name}/keys` 응답 BREAKING 변경**
   - 기존: `{"count": N, "keys": ["k1", "k2"], "tags": {...}}`
   - 신규: `{"count": N, "keys": [{key, registration, data_type, metric_type, tags}, ...]}`
2. **신규 필터 쿼리 (M9)**
   - `?data_type=<int|float|string|boolean|bytes|json>`
   - `?metric_type=<value>`
   - `?registration=<manual|auto>`
   - 기존 `?tag=key:value`와 AND 결합
3. **`GET /api/v1/store/{name}/tags` (변경 없음, M3 유지)**

### 에러 모델

**v0.2.0 보존 (시맨틱 일부 변경):**
- `ErrKeyNotAllowed`: `registration_type=manual`에서 미등록 키 쓰기 요청 (이전: `allow_dynamic_keys=false`)
- `ErrDuplicateStaticKey`: 설정 로드 시 중복 키 정의
- `ErrInvalidTagKey`: 태그 key가 `^[a-zA-Z0-9_-]+$`를 위반

**v0.3.0 신규:**
- `ErrTypeMismatch`: 쓰기 값의 Go 타입이 등록된 `data_type`과 불일치
- `ErrUnsupportedValueType`: `nil` 또는 추론 불가 타입의 쓰기 시도 (auto 모드)
- `ErrInvalidDataType`: yaml의 `data_type` 값이 6종 enum 외이거나 manual 모드에서 누락
- `ErrInvalidMetricType`: yaml의 `metric_type`이 `^[a-zA-Z0-9_-]+$` 정규식 위반

## 관련 SPEC (Related SPECs)

- **SPEC-STORE-001**: Store 에이전트 기본 설계 (전제)
- **SPEC-STORE-002**: Store 엔트리/히스토리 쿼리 API (전제)
- **SPEC-WEB-005 v0.5.0 (예정)**: Store 설정 UI에서 `data_type`/`metric_type` 편집 + 객체 배열 응답 적응 (BREAKING change 동시 업데이트 필요)

## TAG Traceability

- @SPEC:SPEC-STORE-003 → spec.md (이 문서)
- @PLAN:SPEC-STORE-003 → plan.md
- @ACCEPTANCE:SPEC-STORE-003 → acceptance.md
- 구현 경로 (v0.2.0 보존 + v0.3.0 추가/변경):
  - `internal/agent/system/store_options.go` (storeConfig: registrationType, staticKeys 진화)
  - `internal/agent/system/store.go` (data_type 추론, type 검증, runtime setters)
  - `internal/agent/system/store_data_type.go` (신규 후보: inferDataType helper, DataType enum)
  - `internal/agent/system/store_user_agent.go`
  - `internal/agent/system/store_query.go`
  - `internal/api/handler/store_*.go` (객체 배열 응답, 신규 필터)

## Implementation Notes

### v0.2.0 보존 사항 (Divergence from Original Plan)

- **`registration_type` 게이트 위치**: 정적 키 이름이 namespace prefix 가 붙기 전(user-facing) 형식이므로 `NamespacedStore` 경계에 `keyGatekeeper` 인터페이스 + 게이트 검사로 배치 (v0.2.0 채택, v0.3.0 보존)
- **`parseStoreConfig` 시그니처**: `([]StoreOption, error)` 형태로 검증 에러 명시적 전파 (v0.2.0 채택, v0.3.0 보존)
- **`api.Context.QueryValues(name)`**: 다중 쿼리 파라미터 지원 (v0.2.0 채택, v0.3.0 추가 필터에서 재사용)

### v0.2.0 Post-implementation fixes

- **`UserStoreAgent.Configure` runtime 적용** (`e114781`): `SetAllowDynamicKeys` / `SetStaticKeys` runtime setter. **v0.3.0에서 `SetRegistrationType` / `SetStaticKeys` (signature 변경)으로 진화.**
- **`NodeStoreAdapter` 재시작 후 고립 수정** (`329a3d9`): resolver 함수 패턴.

### v0.2.0 Notes (보존)

- `Store.ClearHistory(ctx, key) error` 인터페이스, DELETE 엔드포인트 2개, 버킷 벽시계 정렬, 태그 입력 blur 자동 commit, ConfirmDialog/PromoteToStaticDialog, 저장소 탭 액션 컬럼 분리 — 모두 v0.3.0에서 그대로 유지된다.

### v0.3.0 Implementation Strategy

- **Clean rename**: `allowDynamicKeys` 식별자 / `allow_dynamic_keys` yaml 키 모두 코드베이스에서 완전 제거. grep으로 잔존 참조 체크.
- **`StaticKeyMeta` struct 도입**: `staticKeys map[string]map[string]string` (tag만 보유) → `staticKeys map[string]StaticKeyMeta { DataType, MetricType, Tags }`로 진화.
- **`inferDataType(value any) (DataType, error)` helper**: type switch 또는 `reflect.TypeOf` 사용. v0.3.0 핵심 신규 함수.
- **타입 검증 hot path**: `Set`/`SetWithTTL`에서 `staticKeys[key].DataType` 조회는 O(1) map lookup. 검증 오버헤드 무시 가능.
- **API 핸들러 책임 분리**: 응답 빌더는 `[]KeyResponseObject` 생성 책임만 가짐. 필터링은 별도 helper에서 AND 결합.

### Status: in_progress (v0.3.0 진행 중)

이전 v0.2.0의 status=completed에서 v0.3.0 진화로 status=in_progress로 전환. v0.3.0 완료 후 다시 completed로 회귀 예정.

### v0.4.0 필터 UI 제거 (UI 단순화, 2026-07-06)

- v0.4.0에서 저장소 데이터 테이블 상단에 도입했던 **"메트릭 타입" 필터 드롭다운과 태그 필터 칩(TagFilterChips) UI 를 저장소 탭에서 제거**했다(후속 UI 단순화). 상단 필터 두 블록이 중복 UI 로 판단된 데 따른 조치다.
- **대체 수단**: 유지되는 키워드 검색 상자(키 / metric_type / 태그 매칭)와 컬럼별 Excel 스타일 헤더 필터가 동일한 필터링 요구를 그대로 커버한다.
- 제거 범위: 위 두 필터 UI 와 연동 상태/핸들러/파생값, 그리고 대응 단위 테스트 블록("메트릭 타입 필터"). 백엔드 API·데이터 스키마·필터 매칭 의미에는 변경이 없다.
- 위 원본 요구사항 텍스트(메트릭 타입 분류, 태그 메타데이터/필터링 등)는 그대로 유효하며, 본 노트는 UI 노출 방식만의 변경을 기록한다.
