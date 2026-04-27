---
id: SPEC-STORE-003
title: Store 에이전트 정적 키 정의 및 태그 메타데이터
version: 0.2.0
status: completed
created: 2026-04-24
updated: 2026-04-27
author: xtra
priority: medium
---

# SPEC-STORE-003: Store 에이전트 정적 키 정의 및 태그 메타데이터

## HISTORY

- **0.2.0** (2026-04-27): Store 에이전트 확장 기능:
  (1) 동적 키 → 정적 변환 UI 및 PromoteToStaticDialog (Configure API 재사용, 백엔드 변경 0).
  (2) 저장소 전체/개별 키 초기화: `DELETE /api/v1/store/{name}/keys/{key}` (정적: history만, 동적: entry 완전 삭제), `DELETE /api/v1/store/{name}/keys` (bulk). 신규 백엔드 `Store.ClearHistory()` 메서드.
  (3) 버킷 타임스탬프 벽시계 경계 정렬 (`floor(tsMs / intervalMs) * intervalMs`).
  (4) 정적 키 태그 입력 blur 자동 commit (TagChipsEditor pending input 유실 버그 수정).
  (5) UI 개선: StoreKeysEditor 키:태그 1:3 비율, readOnly 가시성 복원 (FormField + StoreKeysEditor 외 8종 property 편집기), 저장소 탭 행별 액션 분리 및 전체 초기화 버튼 색상 중립화.

| Version | Date       | Author | Change                                                                      |
| ------- | ---------- | ------ | --------------------------------------------------------------------------- |
| 0.2.0   | 2026-04-27 | xtra   | 동적→정적 변환 UI, 키 초기화 (DELETE 2개 엔드포인트), 버킷 벽시계 정렬, 태그 blur commit, UI 개선 |
| 0.1.0   | 2026-04-24 | xtra   | 최초 작성 — 정적 키 목록, 태그 메타데이터, 동적 키 제어, 태그 필터링 API 도입 |

## 개요 (Overview)

Store 에이전트(SPEC-STORE-001, SPEC-STORE-002 기반)는 현재 키/값 쓰기 시점에 어떠한 키든 동적으로 수용하며, 각 키에 대한 의미론적 메타데이터가 없다. 본 SPEC은 다음 세 가지 능력을 추가한다.

1. **정적 키 정의**: 설정에 미리 허용된 키 목록과 각 키의 태그(`map[string]string`)를 선언
2. **동적 키 등록 제어**: strict 모드(`allow_dynamic_keys=false`)에서 미등록 키 쓰기 거부
3. **태그 조회 및 태그 기반 필터링**: 엔트리/키 조회 응답에 태그 포함, `?tag=key:value` 쿼리로 AND 필터링

설정 파일은 시각적으로 **운영(operational)** 필드와 **데이터(data)** 필드로 구분될 것이나(관련 UI는 SPEC-WEB-005 v0.4.0에서 처리), 백엔드는 모든 필드를 평평한 구조로 노출한다.

## 배경 (Background)

### 현재 상태

현재 `StoreConfig`는 다음 필드만 가진다:

```yaml
history_ttl: "1h"
max_history_size: 1000
max_key_length: 512
scan_interval: "30s"
```

모든 키는 쓰기 시점에 자유롭게 생성되며, 메타데이터가 전혀 없다.

### 사용자 요구

- **키 스키마 통제**: 운영자가 허용 키 목록을 미리 정의하여 데이터 품질을 보장
- **의미 부여**: 각 키에 태그(room=1, type=temperature 등)를 부여하여 UI 및 분석에서 그룹화·필터링
- **선택적 strict 모드**: 미등록 키를 완전히 거부하여 typo나 잘못된 이름 유입 방지

### SDD 2025 Constitution 정합성

- Go 1.22+ 기존 기술 스택 (신규 의존성 없음)
- YAML 파싱은 기존 `gopkg.in/yaml.v3` 사용
- 하위호환 보장: 신규 필드 미설정 시 기존 동작 100% 보존

## EARS 요구사항 (EARS Requirements)

본 SPEC은 5개 EARS 모듈로 구성된다.

---

### M1: 정적 키 정의 (Static Key Definition)

- **Ubiquitous**: 시스템은 Store 에이전트 설정에 정적 키 목록(`keys`) 필드를 optional로 지원해야 한다.
- **Ubiquitous**: 각 정적 키 엔트리는 `key` (string, 필수)와 `tags` (map[string]string, optional)를 포함한다.
- **State-driven**: IF `keys` 필드가 설정되지 않거나 빈 배열인 경우, THEN 시스템은 기존 동작을 유지해야 한다 (하위호환).
- **Unwanted**: 시스템은 동일한 `key` 값을 갖는 중복 엔트리가 `keys` 목록에 존재하면 설정 로드 시 에러(`ErrDuplicateStaticKey`)를 반환해야 한다.
- **Unwanted**: 시스템은 태그 key가 정규식 `^[a-zA-Z0-9_-]+$`를 만족하지 않으면 설정 로드 시 에러(`ErrInvalidTagKey`)를 반환해야 한다.

---

### M2: 동적 키 등록 제어 (Dynamic Key Registration Control)

- **Ubiquitous**: 시스템은 `allow_dynamic_keys` boolean 필드를 Store 에이전트 설정에 지원하며, 기본값은 `true`이다.
- **State-driven**: WHILE `allow_dynamic_keys == true`, 시스템은 정적 키 목록에 없는 키 쓰기 요청을 허용해야 한다 (해당 키는 태그가 없는 상태로 저장됨).
- **Unwanted**: WHEN `allow_dynamic_keys == false` AND 쓰기 요청의 키가 정적 키 목록에 없으면, THEN 시스템은 쓰기를 거부하고 `ErrKeyNotAllowed`를 반환해야 한다.
- **Unwanted**: 거부된 쓰기는 엔트리에도 히스토리에도 기록되지 않아야 한다.

---

### M3: 태그 조회 및 응답 포함 (Tag Retrieval and Response Inclusion)

- **Ubiquitous**: 시스템은 엔트리 조회 응답(`GET /api/v1/agents/{id}` → `state.entries`)에 정적 키의 태그를 포함해야 한다.
- **Ubiquitous**: 시스템은 키 목록 조회 응답(`GET /api/v1/store/{name}/keys`)에 각 정적 키의 태그 맵을 포함해야 한다.
- **State-driven**: IF 키가 정적 키 목록에 없고 동적으로 쓰여진 경우, THEN `tags`는 JSON에서 생략되거나 빈 객체(`{}`)로 표현된다.
- **Optional**: WHERE API 호출자가 `GET /api/v1/store/{name}/tags`를 요청하면, 시스템은 전체 정적 키 태그의 유니크한 (key, values) 쌍 목록을 반환한다.

---

### M4: 태그 기반 키 필터링 (Tag-Based Key Filtering)

- **Event-driven**: WHEN `GET /api/v1/store/{name}/keys?tag=key:value` 요청이 오면, THEN 시스템은 해당 태그(key=value)를 포함하는 키만 반환해야 한다.
- **Event-driven**: WHEN 다수의 `tag` 쿼리 파라미터(`?tag=room:1&tag=type:temperature`)가 전달되면, THEN 시스템은 **모든** 태그를 포함하는 키만 반환해야 한다 (AND 조건).
- **State-driven**: IF `tag` 쿼리 파라미터가 없으면, THEN 시스템은 전체 키 목록을 반환한다 (기존 동작 유지).
- **Unwanted**: IF `tag` 쿼리 값이 `key:value` 형식이 아니면 (예: 콜론 없음), THEN 시스템은 HTTP 400 Bad Request를 반환하고 에러 메시지 `"invalid tag format: expected key:value"`를 응답 바디에 포함해야 한다.

---

### M5: 설정 하위호환성 (Configuration Backward Compatibility)

- **Ubiquitous**: 시스템은 `keys` 및 `allow_dynamic_keys` 필드가 없는 기존 설정 파일을 정상적으로 로드해야 한다.
- **Ubiquitous**: 미설정 시 기본값은 `keys = []`, `allow_dynamic_keys = true`이며, 결과적으로 기존 동작이 완전히 보존되어야 한다.
- **Event-driven**: WHEN 새로운 설정이 agent config reload로 적용되면, THEN 정적 키 정의가 변경되더라도 기존 저장된 엔트리 및 히스토리 데이터는 보존되어야 한다.

## 명세 (Specifications)

### Config 스키마 확장

```yaml
agents:
  - type: store
    name: "system-store"
    config:
      # 기존 운영 필드
      scan_interval: "30s"
      max_key_length: 512
      history_ttl: "1h"
      max_history_size: 1000

      # 신규 데이터 필드
      allow_dynamic_keys: true          # 기본 true
      keys:                              # optional, 빈 배열이거나 미설정 가능
        - key: "indoor:1:room_temp"
          tags:
            room: "1"
            type: "temperature"
        - key: "outdoor:temperature"
          tags:
            location: "outside"
            type: "temperature"
```

### API 변경

1. **`GET /api/v1/store/{name}/keys` 응답 확장**
   - 기존 응답에 `tags` 필드를 추가 (optional, 정적 키가 없으면 생략)
2. **신규 `GET /api/v1/store/{name}/tags`**
   - 전체 태그 `(key, values[])` 유니크 페어 반환 (UI 필터 chip 생성용)
3. **`GET /api/v1/store/{name}/keys?tag=key:value` 지원**
   - 다중 `tag` 파라미터는 AND 조건

### 에러 모델

- `ErrKeyNotAllowed`: strict 모드에서 미등록 키 쓰기 요청
- `ErrDuplicateStaticKey`: 설정 로드 시 중복 키 정의
- `ErrInvalidTagKey`: 태그 key가 `^[a-zA-Z0-9_-]+$`를 위반

## 관련 SPEC (Related SPECs)

- **SPEC-STORE-001**: Store 에이전트 기본 설계 (전제)
- **SPEC-STORE-002**: Store 엔트리/히스토리 쿼리 API (전제)
- **SPEC-WEB-005 v0.4.0**: Store 설정 UI의 운영/데이터 분리 및 태그 필터 chip (하위 소비자)

## TAG Traceability

- @SPEC:SPEC-STORE-003 → spec.md (이 문서)
- @PLAN:SPEC-STORE-003 → plan.md
- @ACCEPTANCE:SPEC-STORE-003 → acceptance.md
- 구현 경로 (예정):
  - `internal/agent/system/store.go`
  - `internal/agent/system/store_user_agent.go`
  - `internal/agent/system/store_query.go`
  - `internal/api/handlers/store_*.go`

## Implementation Notes

### Divergence from Original Plan

- **`allow_dynamic_keys` 게이트 위치**: 계획에서는 `agentStore.Set/SetWithTTL` 직접 검사였으나, 정적 키 이름이 namespace prefix 가 붙기 전(user-facing) 형식이므로 `NamespacedStore` 경계에 `keyGatekeeper` 인터페이스 + 게이트 검사로 배치하여 키 매칭 정확도 확보
- **`parseStoreConfig` 시그니처 변경**: `[]StoreOption` → `([]StoreOption, error)` 로 변경하여 검증 에러(중복 키, 잘못된 태그 key regex) 명시적 전파
- **`api.Context.QueryValues(name)`**: 다중 `?tag=` 쿼리 파라미터 지원을 위해 Context 인터페이스에 메서드 추가 (단일 구현체 `httpContext` 만 영향)

### Post-implementation fixes

- **`UserStoreAgent.Configure` runtime 적용 누락 수정** (`e114781`): Configure 가 `agentConfig` 만 갱신하고 inner store 에 정책 필드 반영을 안 하던 버그 수정. `SetAllowDynamicKeys` / `SetStaticKeys` runtime setter 추가
- **`NodeStoreAdapter` 재시작 후 고립 수정** (`329a3d9`): 생성 시점 store 스냅샷 → resolver 함수 패턴으로 변경. 에이전트 재시작 시 inner 가 교체되어도 동일 adapter 가 새 inner 로 재라우팅

### v0.2.0 Notes

- **`Store.ClearHistory(ctx, key) error` 인터페이스 추가**: `VolatileStore`/`NamespacedStore`/`PersistentStore` 모두 구현. 정적 키는 history 만 비우고 entry 메타데이터(태그) 보존, 동적 키는 entry 자체 완전 삭제 (`UserStoreAgent.IsStaticKey` / `DeleteEntry` 신규 헬퍼 사용).
- **DELETE 엔드포인트 2개 신설** (`internal/api/handler/store_*.go`):
  - `DELETE /api/v1/store/{name}/keys/{key}` — 단일 키 초기화. 정적/동적 분기 분리.
  - `DELETE /api/v1/store/{name}/keys` — bulk 초기화. 응답에 `cleared_count` / `deleted_count` 포함.
- **버킷 타임스탬프 벽시계 경계 정렬**: `bucketStart = floor(tsMs / intervalMs) * intervalMs` (epoch zero 기준). 사용자 시작점 기반이 아닌 UTC 벽시계 정렬로 변경. 1m → 초=0, 5m → 분 0/5/10..., 1h → 분=초=0. 시멘틱 변경이지만 결과 정렬이 더 직관적. TSDB 엔진은 이미 동일 로직.
- **태그 입력 blur 자동 commit** (`web/src/components/property/TagChipsEditor.tsx`): 키/값 입력 필드에 `onBlur` 핸들러 추가하여 pending input 을 자동으로 chip 으로 변환. "추가" 버튼 누르지 않고 폼 저장 시 입력값 유실되던 문제 해결.
- **신규 Frontend 컴포넌트**:
  - `ConfirmDialog` — 재사용 가능한 확인 다이얼로그 (default/danger variant)
  - `PromoteToStaticDialog` — 동적 키를 정적 키로 변환 (태그 입력 + Configure API 재사용)
- **저장소 탭 액션 컬럼 분리**: 타입 컬럼은 정적/동적 배지만, 마지막 "액션" 컬럼에 [정적변환] [초기화] 버튼 모음. 헤더 "전체 초기화" 버튼 색상은 중립화 (실제 destructive 의도는 ConfirmDialog danger variant 가 담당).

### Status: completed (Level 1 spec-first lifecycle)
