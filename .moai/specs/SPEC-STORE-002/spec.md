---
id: SPEC-STORE-002
version: "1.0.0"
status: draft
created: "2026-03-30"
updated: "2026-03-30"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-03-30 | 1.0.0 | 초기 SPEC 작성 |

---

# SPEC-STORE-002: Store Value History - 값 히스토리 추적 시스템

## 1. Environment (환경)

### 1.1 시스템 개요

SPEC-STORE-001에서 정의한 키-값 저장소 시스템에 **값 변경 히스토리 추적 기능**을 추가한다. Store에 저장된 값이 갱신될 때마다 이전 값을 히스토리로 보존하여, 특정 키의 값 변경 이력을 시간순으로 조회할 수 있도록 한다.

본 SPEC은 다음을 다룬다:
- `historyEntry` 내부 구조체 및 `HistoryEntry` 공개 구조체 정의
- `storeItem`에 히스토리 슬라이스 필드 추가
- 에이전트별 히스토리 설정 (`max_history_size`, `history_ttl`)
- Set/SetWithTTL 호출 시 히스토리 축적 및 FIFO 방식 제거
- `GetHistory()` 인터페이스 메서드 추가
- `StoreEntry`에 히스토리 메타데이터 필드 추가
- TTL 매니저의 히스토리 정리 통합
- 웹 UI 히스토리 표시
- 노드 및 브릿지 연동

핵심 사용자 요구사항:
- 히스토리 갯수 제한 설정 가능
- 히스토리 시간 제한 설정 가능
- 갯수 또는 시간 초과 시 오래된 값부터 자동 제거
- 현재 히스토리 수와 최대 히스토리 수 조회 가능

### 1.2 기술 환경

- **언어**: Go 1.23+
- **핵심 파일**: `internal/agent/system/store_volatile.go`, `internal/agent/system/store.go`, `internal/agent/system/store_options.go`
- **의존성**: 표준 라이브러리 (`sync`, `time`, `context`) + 기존 Store 시스템 전체
- **웹 UI**: React + TypeScript (`web/src/`)
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **Tier**: internal (비공개 패키지)

### 1.3 설계 원칙

- **하위 호환성**: `max_history_size=0` (기본값)이면 히스토리를 추적하지 않으며, 기존 동작과 완전히 동일하다. 기존 코드 변경 없이 업그레이드 가능하다.
- **에이전트별 설정**: 히스토리 설정은 Store 에이전트 단위로 적용되며, Agent YAML 설정에서 `max_history_size`와 `history_ttl`을 지정한다.
- **FIFO 제거**: 히스토리가 최대 갯수를 초과하면 가장 오래된 항목부터 제거한다. 시간 제한 초과 시에도 오래된 항목부터 제거한다.
- **storeItem 내장**: 히스토리 데이터는 `storeItem` 구조체 내부에 `[]historyEntry` 슬라이스로 저장되어, 별도 자료구조 없이 키와 함께 관리된다.
- **최신순 반환**: `GetHistory()` 호출 시 가장 최근 히스토리부터 반환한다 (newest-first).
- **동시성 안전**: 기존 VolatileStore의 동시성 보장 메커니즘을 그대로 활용한다.

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- `historyEntry` 내부 구조체 및 `HistoryEntry` 공개 구조체 정의
- `storeItem`에 `history []historyEntry` 필드 추가
- `storeConfig`에 `maxHistorySize int`, `historyTTL time.Duration` 추가
- `StoreOption` 함수: `WithMaxHistorySize()`, `WithHistoryTTL()`
- Set/SetWithTTL 시 히스토리 축적 로직
- 갯수 기반 제거 (maxHistorySize 초과 시)
- 시간 기반 제거 (historyTTL 초과 시)
- `Store` 인터페이스에 `GetHistory()` 메서드 추가
- `VolatileStore.GetHistory()` 구현
- `NamespacedStore.GetHistory()` 위임 구현
- `agentStore.GetHistory()` 위임 구현
- `StoreEntry`에 `HistoryCount`, `MaxHistorySize` 필드 추가
- TTL 매니저의 히스토리 정리 통합
- Agent YAML 설정 파싱 (`max_history_size`, `history_ttl`)
- `UserStoreAgent.State()`에 히스토리 메타데이터 포함
- 웹 UI StoreTab 히스토리 표시
- `node.StoreReader`에 `GetHistory()` 추가 및 `NodeStoreAdapter` 구현
- `StoreReadNode`의 `include_history` 설정
- `BridgeHandler`의 `"history"` 오퍼레이션

**OUT OF SCOPE (별도 SPEC)**:
- PersistentStore 히스토리 구현 (DB 스키마 변경 필요, 향후 SPEC)
- 히스토리 데이터의 영속적 저장
- 히스토리 변경 이벤트/콜백 시스템
- 히스토리 데이터의 집계/통계 기능 (평균, 최대, 최소 등)
- 히스토리 데이터의 시각화 차트 (향후 대시보드 SPEC)

### 1.5 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-STORE-001 | 확장 대상 | Store 인터페이스, storeItem, storeConfig, VolatileStore, NamespacedStore, TTL 매니저의 원본 정의 |
| SPEC-SYSAGENT-001 | 소비자 | UserStoreAgent의 State() 응답에 히스토리 메타데이터 추가 |
| SPEC-NODE-001 | 소비자 | node.StoreReader 인터페이스 확장, StoreReadNode 설정 추가 |
| SPEC-WEB-001 | 소비자 | StoreTab에 히스토리 갯수 컬럼 및 확장 뷰 추가 |
| SPEC-BRIDGE-001 | 소비자 | BridgeHandler에 "history" 오퍼레이션 추가 |

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: 히스토리 데이터는 `storeItem` 구조체 내부 `[]historyEntry` 슬라이스에 저장되므로, 키 삭제 시 히스토리도 자동으로 GC된다.
- A2: `historyEntry`는 내부(unexported) 타입이며, 외부에는 `HistoryEntry` 공개 구조체로 변환하여 반환한다.
- A3: 히스토리 슬라이스는 prepend 방식으로 추가하여 인덱스 0이 가장 최근 값이다. 이를 통해 `GetHistory()` 호출 시 추가 정렬 없이 newest-first 순서를 보장한다.
- A4: `maxHistorySize`와 `historyTTL`은 에이전트 수준 설정이며, 개별 키별로는 설정할 수 없다.
- A5: 히스토리 축적은 `maxHistorySize > 0`일 때만 동작하며, `maxHistorySize=0`이면 히스토리 슬라이스를 생성하지 않는다.
- A6: TTL 매니저의 주기적 스캔에서 `historyTTL` 기반 정리를 함께 수행하므로, 별도 고루틴이 불필요하다.
- A7: VolatileStore의 기존 동시성 보장 메커니즘(`sync.Map` 또는 `sync.Mutex`)이 히스토리 접근에도 동일하게 적용된다.
- A8: 히스토리 데이터는 메모리에만 저장되므로, 프로세스 재시작 시 소멸된다.

### 2.2 도메인 가정

- A9: 일반적인 사용 시나리오에서 `max_history_size`는 10~1000 범위로 설정될 것으로 예상한다.
- A10: `history_ttl`이 0이면 시간 기반 제거를 하지 않으며, 갯수 기반 제거만 적용된다.
- A11: 히스토리는 값이 실제로 변경될 때만 축적된다. 동일한 값으로 Set해도 히스토리에 기록된다 (값 동일성 비교는 하지 않음).
- A12: 웹 UI에서 히스토리 조회는 전체 히스토리를 한 번에 로드하며, 페이지네이션은 클라이언트 측에서 처리한다.
- A13: 브릿지의 "history" 오퍼레이션은 `store.history_limit` 메타데이터로 반환 갯수를 제한할 수 있다.

---

## 3. Requirements (요구사항)

### Module 1: 히스토리 데이터 구조 (P0)

#### REQ-STORE-002-01-01 (Ubiquitous) historyEntry 내부 구조체 정의

시스템은 **항상** 다음 필드를 포함하는 `historyEntry` 내부(unexported) 구조체를 제공해야 한다:

- `Value any` - 히스토리에 저장된 이전 값
- `Timestamp time.Time` - 해당 값이 히스토리로 이동된 시각

#### REQ-STORE-002-01-02 (Ubiquitous) HistoryEntry 공개 구조체 정의

시스템은 **항상** 다음 필드를 포함하는 `HistoryEntry` 공개(exported) 구조체를 제공해야 한다:

- `Value any` - 히스토리에 저장된 이전 값
- `Timestamp time.Time` - 해당 값이 히스토리로 이동된 시각

`HistoryEntry`는 `GetHistory()` 메서드의 반환 타입으로 사용되며, 내부 `historyEntry`를 외부에 노출할 때 변환하여 사용한다.

#### REQ-STORE-002-01-03 (Ubiquitous) storeItem에 history 필드 추가

시스템은 **항상** `storeItem` 구조체에 `history []historyEntry` 필드를 추가해야 한다. 이 슬라이스는 해당 키의 값 변경 이력을 저장한다.

#### REQ-STORE-002-01-04 (Ubiquitous) StoreEntry에 히스토리 메타데이터 필드 추가

시스템은 **항상** `StoreEntry` 구조체에 다음 필드를 추가해야 한다:

- `HistoryCount int` - 해당 키에 저장된 현재 히스토리 항목 수
- `MaxHistorySize int` - 에이전트에 설정된 최대 히스토리 크기

---

### Module 2: 히스토리 설정 (P0)

#### REQ-STORE-002-02-01 (Ubiquitous) storeConfig에 maxHistorySize 추가

시스템은 **항상** `storeConfig` 구조체에 `maxHistorySize int` 필드를 추가해야 한다. 기본값은 `0`이며, 0은 히스토리 추적 비활성을 의미한다.

#### REQ-STORE-002-02-02 (Ubiquitous) storeConfig에 historyTTL 추가

시스템은 **항상** `storeConfig` 구조체에 `historyTTL time.Duration` 필드를 추가해야 한다. 기본값은 `0`이며, 0은 시간 기반 히스토리 제거 비활성을 의미한다.

#### REQ-STORE-002-02-03 (Ubiquitous) StoreOption 함수 정의

시스템은 **항상** 다음 `StoreOption` 함수를 제공해야 한다:

- `WithMaxHistorySize(n int) StoreOption` - 최대 히스토리 항목 수 설정
- `WithHistoryTTL(d time.Duration) StoreOption` - 히스토리 항목 유효 시간 설정

#### REQ-STORE-002-02-04 (Event-Driven) UserStoreAgent parseStoreConfig 파싱

**WHEN** UserStoreAgent가 Agent YAML 설정을 파싱할 때, **THEN** 다음 설정 키를 인식하고 `storeConfig`에 반영해야 한다:

- `max_history_size` (int) - 최대 히스토리 크기 (기본값 0)
- `history_ttl` (duration string, 예: `"1h"`, `"30m"`) - 히스토리 TTL (기본값 0)

Agent YAML 설정 예시:
```yaml
config:
  backend: volatile
  default_ttl: 5m
  max_history_size: 100
  history_ttl: 1h
```

#### REQ-STORE-002-02-05 (State-Driven) Configure()에서 런타임 변경 지원

**IF** StoreAgent가 Running 또는 Paused 상태일 때, **THEN** `Configure(ctx, cfg)`를 통해 다음 설정을 런타임에 변경할 수 있어야 한다:

- `max_history_size` - 최대 히스토리 크기
- `history_ttl` - 히스토리 TTL

변경 시 기존 히스토리 데이터에 즉시 새로운 제한을 적용하여 초과분을 제거한다.

---

### Module 3: 히스토리 축적 및 제거 (P0)

#### REQ-STORE-002-03-01 (Event-Driven) Set/SetWithTTL 시 히스토리 축적

**WHEN** `Store.Set(ctx, key, value)` 또는 `Store.SetWithTTL(ctx, key, value, ttl)` 호출 시 해당 키가 이미 존재하고 `maxHistorySize > 0`이면, **THEN** 현재 값을 `historyEntry{Value: currentValue, Timestamp: time.Now()}`로 생성하여 히스토리 슬라이스의 앞(인덱스 0)에 prepend해야 한다.

히스토리 축적 후 새로운 값으로 기존 값을 덮어쓴다.

#### REQ-STORE-002-03-02 (Event-Driven) 갯수 기반 제거

**WHEN** 히스토리 prepend 후 히스토리 슬라이스 길이가 `maxHistorySize`를 초과하면, **THEN** 슬라이스 뒤쪽(가장 오래된 항목)부터 초과분을 제거하여 길이를 `maxHistorySize` 이하로 유지해야 한다.

#### REQ-STORE-002-03-03 (State-Driven) 시간 기반 제거

**IF** `historyTTL > 0`이면, **THEN** 히스토리 prepend 시 `Timestamp`가 `time.Now().Add(-historyTTL)` 이전인 항목을 슬라이스에서 제거해야 한다. 시간 기반 제거는 갯수 기반 제거와 함께 적용된다.

#### REQ-STORE-002-03-04 (Event-Driven) Delete 시 히스토리 삭제

**WHEN** `Store.Delete(ctx, key)` 호출 시, **THEN** 해당 키의 히스토리 데이터도 함께 삭제되어야 한다. (`storeItem` 삭제 시 내부 `history` 슬라이스가 GC됨)

#### REQ-STORE-002-03-05 (Event-Driven) Clear 시 모든 히스토리 삭제

**WHEN** `Store.Clear(ctx)` 호출 시, **THEN** 모든 키의 히스토리 데이터도 함께 삭제되어야 한다.

---

### Module 4: 히스토리 조회 (P0)

#### REQ-STORE-002-04-01 (Ubiquitous) Store 인터페이스에 GetHistory 추가

시스템은 **항상** `Store` 인터페이스에 다음 메서드를 추가해야 한다:

```go
GetHistory(ctx context.Context, key string) ([]HistoryEntry, error)
```

존재하지 않는 키에 대해 호출 시 `ErrKeyNotFound`를 반환한다. 히스토리가 없으면 빈 슬라이스를 반환한다.

#### REQ-STORE-002-04-02 (Ubiquitous) VolatileStore.GetHistory 구현

시스템은 **항상** `VolatileStore.GetHistory()`를 구현하여 내부 `[]historyEntry`를 `[]HistoryEntry`로 변환하여 반환해야 한다. 반환 순서는 **최신순(newest-first)**이다 (인덱스 0이 가장 최근).

#### REQ-STORE-002-04-03 (Ubiquitous) NamespacedStore.GetHistory 위임

시스템은 **항상** `NamespacedStore.GetHistory(ctx, key)`를 구현하여 네임스페이스 접두사를 적용한 키로 내부 스토어의 `GetHistory()`에 위임해야 한다.

#### REQ-STORE-002-04-04 (Ubiquitous) agentStore.GetHistory 위임

시스템은 **항상** `agentStore.GetHistory(ctx, key)`를 구현하여 스토어 닫힘(closed) 여부를 확인한 후 내부 스토어의 `GetHistory()`에 위임해야 한다. 스토어가 닫혀있으면 `ErrStoreClosed`를 반환한다.

#### REQ-STORE-002-04-05 (Ubiquitous) Get() 반환값에 히스토리 메타데이터 포함

시스템은 **항상** `Store.Get()` 반환 `StoreEntry`에 다음 필드를 채워야 한다:

- `HistoryCount` - 해당 키의 현재 히스토리 항목 수
- `MaxHistorySize` - 에이전트에 설정된 최대 히스토리 크기

---

### Module 5: TTL 매니저 히스토리 정리 (P1)

#### REQ-STORE-002-05-01 (State-Driven) scan()에서 히스토리 정리

**IF** `historyTTL > 0`이면, **THEN** `ttlManager.scan()` 주기적 실행 시 모든 키의 히스토리를 순회하여 `Timestamp`가 `time.Now().Add(-historyTTL)` 이전인 항목을 제거해야 한다.

이를 통해 Set이 호출되지 않는 키의 히스토리도 시간 경과에 따라 자동 정리된다.

#### REQ-STORE-002-05-02 (Event-Driven) 만료 키 삭제 시 히스토리 삭제

**WHEN** TTL 매니저가 만료된 키를 삭제할 때, **THEN** 해당 키의 히스토리 데이터도 함께 삭제되어야 한다. (`storeItem` 전체 삭제이므로 자동 적용)

---

### Module 6: 웹 UI 히스토리 표시 (P1)

#### REQ-STORE-002-06-01 (Event-Driven) State()에 히스토리 갯수 포함

**WHEN** `UserStoreAgent.State()` 호출 시, **THEN** 각 엔트리에 `history_count` 필드를 포함하여 해당 키의 현재 히스토리 항목 수를 반환해야 한다.

#### REQ-STORE-002-06-02 (Event-Driven) State()에 전체 히스토리 수 포함

**WHEN** `UserStoreAgent.State()` 호출 시, **THEN** 응답의 요약 정보에 `total_history_entries` 필드를 포함하여 스토어 전체의 히스토리 항목 총 수를 반환해야 한다.

#### REQ-STORE-002-06-03 (Ubiquitous) StoreTab 히스토리 갯수 컬럼

시스템은 **항상** 웹 UI StoreTab의 엔트리 테이블에 히스토리 갯수 컬럼을 표시해야 한다. `max_history_size=0`이면 컬럼을 숨기거나 "-"로 표시한다.

#### REQ-STORE-002-06-04 (Event-Driven) 엔트리 확장 시 히스토리 목록 표시

**WHEN** 사용자가 StoreTab에서 특정 엔트리를 확장(expand)할 때, **THEN** 해당 키의 히스토리 목록을 최신순으로 표시해야 한다. 각 히스토리 항목은 **값**과 **시각**을 포함한다.

---

### Module 7: 노드 연동 (P1)

#### REQ-STORE-002-07-01 (Ubiquitous) node.StoreReader에 GetHistory 추가

시스템은 **항상** `node.StoreReader` 인터페이스에 다음 메서드를 추가해야 한다:

```go
GetHistory(ctx context.Context, key string) ([]any, error)
```

반환값은 `[]any` 타입으로, 각 요소는 `map[string]any{"value": v, "timestamp": t}` 형태이다.

#### REQ-STORE-002-07-02 (Ubiquitous) NodeStoreAdapter.GetHistory 구현

시스템은 **항상** `NodeStoreAdapter`가 `node.StoreReader.GetHistory()`를 구현하여, 내부 `system.Store.GetHistory()`를 호출하고 `[]HistoryEntry`를 `[]any`로 변환해야 한다.

#### REQ-STORE-002-07-03 (Ubiquitous) StoreReadNode에 include_history 설정 추가

시스템은 **항상** StoreReadNode에 `include_history` 설정(bool, 기본값 `false`)을 추가해야 한다.

#### REQ-STORE-002-07-04 (State-Driven) include_history 시 payload에 히스토리 포함

**IF** `include_history=true`로 설정되어 있으면, **THEN** StoreReadNode의 출력 payload에 `history` 배열을 포함해야 한다. 배열의 각 요소는 `{"value": ..., "timestamp": "..."}` 형태이다.

---

### Module 8: 브릿지 연동 (P2)

#### REQ-STORE-002-08-01 (Event-Driven) BridgeHandler에 "history" 오퍼레이션 추가

**WHEN** BridgeHandler가 `store.operation: "history"` 메시지를 수신하면, **THEN** `store.key`에 해당하는 키의 히스토리를 조회하여 응답 메시지의 Payload에 포함하여 반환해야 한다.

응답 Payload 형식:
```json
{
  "history": [
    {"value": "...", "timestamp": "2026-03-30T12:00:00Z"},
    {"value": "...", "timestamp": "2026-03-30T11:59:00Z"}
  ],
  "count": 2,
  "max_size": 100
}
```

#### REQ-STORE-002-08-02 (Optional) history_limit 메타데이터 지원

**가능하면** "history" 오퍼레이션 메시지에 `store.history_limit` 메타데이터를 지원하여, 반환되는 히스토리 항목 수를 제한할 수 있어야 한다. 미지정 시 전체 히스토리를 반환한다.

---

## 4. Acceptance Criteria (인수 조건)

### 4.1 핵심 시나리오

#### AC-01: 히스토리 비활성 상태 (하위 호환성)

```
Given max_history_size=0 (기본값)인 StoreAgent
When Set(ctx, "key1", "value1") 후 Set(ctx, "key1", "value2") 호출
Then GetHistory(ctx, "key1") 반환값은 빈 슬라이스 []
And Get(ctx, "key1").HistoryCount == 0
And Get(ctx, "key1").MaxHistorySize == 0
```

#### AC-02: 히스토리 축적

```
Given max_history_size=5인 StoreAgent
When Set(ctx, "temp", 20.0) → Set(ctx, "temp", 21.0) → Set(ctx, "temp", 22.0) 순서로 호출
Then GetHistory(ctx, "temp") 반환값은 [HistoryEntry{21.0, t2}, HistoryEntry{20.0, t1}]
And Get(ctx, "temp").Value == 22.0
And Get(ctx, "temp").HistoryCount == 2
And Get(ctx, "temp").MaxHistorySize == 5
```

#### AC-03: 갯수 기반 제거

```
Given max_history_size=3인 StoreAgent
When "key1"에 대해 값을 v1, v2, v3, v4, v5 순서로 Set 호출 (총 5회)
Then GetHistory(ctx, "key1") 반환값은 길이 3
And 가장 오래된 v1은 제거됨
And 반환 순서: [HistoryEntry{v4, t4}, HistoryEntry{v3, t3}, HistoryEntry{v2, t2}]
And 현재 값: v5
```

#### AC-04: 시간 기반 제거

```
Given max_history_size=100, history_ttl=10s인 StoreAgent
When "key1"에 값을 1초 간격으로 15회 Set 후, 12초 경과
Then GetHistory(ctx, "key1")에서 Timestamp가 12초 이전인 항목은 제거됨
And 최근 10초 이내 항목만 남음
```

#### AC-05: Delete 시 히스토리 삭제

```
Given 히스토리가 축적된 "key1"
When Delete(ctx, "key1") 호출
Then GetHistory(ctx, "key1") 반환값은 ErrKeyNotFound
```

#### AC-06: Clear 시 전체 히스토리 삭제

```
Given 여러 키에 히스토리가 축적된 상태
When Clear(ctx) 호출
Then 모든 키의 GetHistory() 반환값은 ErrKeyNotFound
```

#### AC-07: NamespacedStore 히스토리 위임

```
Given NamespacedStore("ns1")과 NamespacedStore("ns2")
When ns1에서 Set(ctx, "key", "a") → Set(ctx, "key", "b") 호출
Then ns1.GetHistory(ctx, "key") == [HistoryEntry{"a", t1}]
And ns2.GetHistory(ctx, "key") == ErrKeyNotFound
```

#### AC-08: Agent YAML 설정 파싱

```
Given Agent YAML에 max_history_size: 50, history_ttl: 30m 설정
When UserStoreAgent가 설정을 파싱
Then storeConfig.maxHistorySize == 50
And storeConfig.historyTTL == 30 * time.Minute
```

#### AC-09: Configure()를 통한 런타임 변경

```
Given max_history_size=100, 각 키에 80개 히스토리 축적
When Configure(ctx, {"max_history_size": 50}) 호출
Then 이후 각 키의 히스토리가 50개 이하로 트리밍됨
```

#### AC-10: State() 히스토리 메타데이터

```
Given 3개 키에 각각 10, 20, 30개 히스토리 축적
When UserStoreAgent.State() 호출
Then 각 엔트리에 history_count 포함 (10, 20, 30)
And total_history_entries == 60
```

#### AC-11: TTL 매니저 히스토리 정리

```
Given max_history_size=100, history_ttl=5s인 StoreAgent
And "key1"에 히스토리 10개 축적 후 Set 호출 없이 6초 경과
When ttlManager.scan() 실행
Then "key1"의 5초 이전 히스토리 항목이 제거됨
```

#### AC-12: 동시성 안전

```
Given max_history_size=100인 StoreAgent
When 10개 goroutine에서 동시에 같은 키에 Set과 GetHistory를 반복 호출
Then 데이터 레이스 없음 (go test -race 통과)
And 히스토리 길이는 항상 maxHistorySize 이하
```

### 4.2 노드 연동 시나리오

#### AC-13: StoreReadNode include_history

```
Given include_history=true로 설정된 StoreReadNode
And "sensor1" 키에 히스토리 [25.0, 24.0, 23.0] 축적
When StoreReadNode 실행
Then 출력 payload에 history 배열 포함
And history[0].value == 25.0
```

#### AC-14: BridgeHandler history 오퍼레이션

```
Given "key1"에 히스토리 축적
When store.operation="history", store.key="key1" 메시지 수신
Then 응답 Payload에 history 배열, count, max_size 포함
```

### 4.3 웹 UI 시나리오

#### AC-15: StoreTab 히스토리 컬럼

```
Given max_history_size > 0인 StoreAgent
When StoreTab 로딩
Then 히스토리 갯수 컬럼 표시
And 각 엔트리에 현재 히스토리 수 표시
```

#### AC-16: 엔트리 확장 히스토리 뷰

```
Given "temp" 키에 히스토리 [22.0(t3), 21.0(t2), 20.0(t1)] 축적
When 사용자가 "temp" 엔트리를 확장
Then 히스토리 목록 표시: 22.0 (t3), 21.0 (t2), 20.0 (t1) 순서
```

---

## 5. Specifications (명세)

### 5.1 타입 시그니처

```go
// historyEntry - 내부 히스토리 항목 (unexported)
type historyEntry struct {
    Value     any
    Timestamp time.Time
}

// HistoryEntry - 공개 히스토리 항목 (exported)
type HistoryEntry struct {
    Value     any
    Timestamp time.Time
}

// storeItem 확장 (기존 필드 + history)
type storeItem struct {
    value     any
    createdAt time.Time
    updatedAt time.Time
    expiresAt time.Time
    namespace string
    history   []historyEntry  // NEW: 값 변경 히스토리
}

// StoreEntry 확장 (기존 필드 + 히스토리 메타데이터)
type StoreEntry struct {
    Value          any
    TTL            time.Duration
    CreatedAt      time.Time
    UpdatedAt      time.Time
    Namespace      string
    ExpiresAt      time.Time
    HistoryCount   int  // NEW: 현재 히스토리 항목 수
    MaxHistorySize int  // NEW: 최대 히스토리 크기
}

// storeConfig 확장 (기존 필드 + 히스토리 설정)
type storeConfig struct {
    backend        string
    defaultTTL     time.Duration
    scanInterval   time.Duration
    repository     StoreRepository
    maxKeyLength   int
    maxHistorySize int           // NEW: 최대 히스토리 크기 (0=비활성)
    historyTTL     time.Duration // NEW: 히스토리 TTL (0=시간 제한 없음)
}

// Store 인터페이스 확장
type Store interface {
    Get(ctx context.Context, key string) (StoreEntry, error)
    Set(ctx context.Context, key string, value any) error
    SetWithTTL(ctx context.Context, key string, value any, ttl time.Duration) error
    Delete(ctx context.Context, key string) error
    Has(ctx context.Context, key string) (bool, error)
    Keys(ctx context.Context, pattern string) ([]string, error)
    Clear(ctx context.Context) error
    GetHistory(ctx context.Context, key string) ([]HistoryEntry, error)  // NEW
}

// 새로운 StoreOption 함수
func WithMaxHistorySize(n int) StoreOption
func WithHistoryTTL(d time.Duration) StoreOption

// node.StoreReader 확장
type StoreReader interface {
    Read(ctx context.Context, key string) (any, error)
    GetHistory(ctx context.Context, key string) ([]any, error)  // NEW
}
```

### 5.2 히스토리 축적 의사코드

```
function Set(ctx, key, value):
    item = store.getItem(key)
    if item exists AND config.maxHistorySize > 0:
        // 현재 값을 히스토리에 prepend
        entry = historyEntry{Value: item.value, Timestamp: now()}
        item.history = prepend(entry, item.history)

        // 갯수 기반 트리밍
        if len(item.history) > config.maxHistorySize:
            item.history = item.history[:config.maxHistorySize]

        // 시간 기반 트리밍 (historyTTL > 0인 경우)
        if config.historyTTL > 0:
            cutoff = now() - config.historyTTL
            item.history = filter(item.history, entry.Timestamp >= cutoff)

    // 새 값 설정
    item.value = value
    item.updatedAt = now()
    store.setItem(key, item)
```

### 5.3 파일 변경 목록

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `internal/agent/system/store.go` | 수정 | `Store` 인터페이스에 `GetHistory()` 추가, `StoreEntry` 필드 추가, `HistoryEntry` 구조체 정의 |
| `internal/agent/system/store_volatile.go` | 수정 | `storeItem`에 `history` 필드 추가, `Set()`/`SetWithTTL()`에 히스토리 축적 로직 추가, `GetHistory()` 구현, `historyEntry` 구조체 정의 |
| `internal/agent/system/store_options.go` | 수정 | `storeConfig`에 `maxHistorySize`, `historyTTL` 필드 추가, `WithMaxHistorySize()`, `WithHistoryTTL()` 함수 추가 |
| `internal/agent/system/store_namespace.go` | 수정 | `NamespacedStore.GetHistory()` 위임 구현 |
| `internal/agent/system/store_ttl.go` | 수정 | `scan()`에 `historyTTL` 기반 정리 로직 추가 |
| `internal/agent/system/store_agent.go` 또는 관련 파일 | 수정 | `UserStoreAgent.State()`에 `history_count`, `total_history_entries` 포함, `parseStoreConfig`에서 `max_history_size`/`history_ttl` 파싱 |
| `internal/node/bridge_adapter.go` | 수정 | `BridgeHandler`에 "history" 오퍼레이션 추가 |
| `internal/api/service/node_adapter.go` 또는 관련 파일 | 수정 | `NodeStoreAdapter.GetHistory()` 구현 |
| `web/src/` (StoreTab 관련) | 수정 | 히스토리 갯수 컬럼, 확장 뷰 추가 |

### 5.4 Agent YAML 설정 예시

```yaml
# 히스토리 비활성 (기본값, 하위 호환)
name: my-store
type: store
config:
  backend: volatile
  default_ttl: 5m

# 히스토리 활성 (갯수 제한만)
name: sensor-store
type: store
config:
  backend: volatile
  default_ttl: 5m
  max_history_size: 100

# 히스토리 활성 (갯수 + 시간 제한)
name: log-store
type: store
config:
  backend: volatile
  default_ttl: 10m
  max_history_size: 1000
  history_ttl: 1h
```

---

## 6. Traceability (추적성)

| 요구사항 ID | 모듈 | 우선순위 | 대상 파일 |
|------------|------|---------|----------|
| REQ-STORE-002-01-01 ~ 01-04 | 히스토리 데이터 구조 | P0 | store.go, store_volatile.go |
| REQ-STORE-002-02-01 ~ 02-05 | 히스토리 설정 | P0 | store_options.go, store_agent 관련 |
| REQ-STORE-002-03-01 ~ 03-05 | 히스토리 축적 및 제거 | P0 | store_volatile.go |
| REQ-STORE-002-04-01 ~ 04-05 | 히스토리 조회 | P0 | store.go, store_volatile.go, store_namespace.go |
| REQ-STORE-002-05-01 ~ 05-02 | TTL 매니저 히스토리 정리 | P1 | store_ttl.go |
| REQ-STORE-002-06-01 ~ 06-04 | 웹 UI 히스토리 표시 | P1 | store_agent 관련, web/src/ |
| REQ-STORE-002-07-01 ~ 07-04 | 노드 연동 | P1 | node 관련, node_adapter 관련 |
| REQ-STORE-002-08-01 ~ 08-02 | 브릿지 연동 | P2 | bridge_adapter.go |
