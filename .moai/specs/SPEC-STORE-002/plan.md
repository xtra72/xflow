---
id: SPEC-STORE-002
type: plan
version: "1.0.0"
created: "2026-03-30"
updated: "2026-03-30"
---

# SPEC-STORE-002 구현 계획: Store Value History

## 마일스톤

### Primary Goal: 핵심 히스토리 엔진 (P0)

**Module 1: 데이터 구조** (REQ-STORE-002-01-01 ~ 01-04)
- `historyEntry`, `HistoryEntry` 구조체 정의
- `storeItem`에 `history` 필드 추가
- `StoreEntry`에 `HistoryCount`, `MaxHistorySize` 필드 추가

**Module 2: 설정** (REQ-STORE-002-02-01 ~ 02-05)
- `storeConfig` 확장 (`maxHistorySize`, `historyTTL`)
- `WithMaxHistorySize()`, `WithHistoryTTL()` StoreOption 함수
- `parseStoreConfig`에서 YAML 설정 파싱
- `Configure()` 런타임 변경 지원

**Module 3: 축적 및 제거** (REQ-STORE-002-03-01 ~ 03-05)
- `Set()`/`SetWithTTL()`에 히스토리 prepend 로직
- 갯수 기반 트리밍 (maxHistorySize 초과 시)
- 시간 기반 트리밍 (historyTTL 초과 시)
- `Delete()`/`Clear()` 시 히스토리 자동 삭제

**Module 4: 조회** (REQ-STORE-002-04-01 ~ 04-05)
- `Store` 인터페이스에 `GetHistory()` 추가
- `VolatileStore.GetHistory()` 구현
- `NamespacedStore.GetHistory()` 위임
- `agentStore.GetHistory()` 위임
- `Get()` 반환값에 `HistoryCount`, `MaxHistorySize` 포함

### Secondary Goal: 시스템 통합 (P1)

**Module 5: TTL 매니저** (REQ-STORE-002-05-01 ~ 05-02)
- `scan()`에 히스토리 시간 기반 정리 추가

**Module 6: 웹 UI** (REQ-STORE-002-06-01 ~ 06-04)
- `State()` 응답에 히스토리 메타데이터 추가
- StoreTab 히스토리 컬럼 및 확장 뷰

**Module 7: 노드 연동** (REQ-STORE-002-07-01 ~ 07-04)
- `node.StoreReader.GetHistory()` 추가
- `NodeStoreAdapter` 구현
- `StoreReadNode` `include_history` 설정

### Optional Goal: 브릿지 연동 (P2)

**Module 8: 브릿지** (REQ-STORE-002-08-01 ~ 08-02)
- `BridgeHandler` "history" 오퍼레이션
- `store.history_limit` 메타데이터 지원

## 기술 접근

### 아키텍처 방향

히스토리 데이터는 `storeItem` 내부에 `[]historyEntry` 슬라이스로 저장한다. 별도 자료구조나 인덱스 없이 기존 저장소 구조를 최소한으로 확장하여 복잡도를 낮춘다.

### 구현 전략

1. **Store 인터페이스 변경 우선**: `GetHistory()` 메서드를 먼저 추가하여 컴파일 에러를 해결하고, 모든 구현체에 stub을 배치한다.
2. **VolatileStore 핵심 구현**: `Set()`/`SetWithTTL()` 내부에 히스토리 축적 로직을 추가한다.
3. **테스트 주도**: 각 모듈별 테스트를 먼저 작성하고 구현을 진행한다.
4. **하위 호환성 검증**: 기존 테스트가 모두 통과하는지 확인한다.

### 리스크 및 대응

| 리스크 | 영향 | 대응 |
|--------|------|------|
| 메모리 사용량 증가 | 히스토리 축적으로 인한 메모리 소비 | `maxHistorySize`와 `historyTTL`로 상한 제어 |
| 동시성 경합 | 히스토리 슬라이스 접근 시 경합 | 기존 동시성 메커니즘 (mutex/sync.Map) 활용 |
| Store 인터페이스 변경 영향 | 모든 구현체 수정 필요 | 컴파일 타임에 누락 감지, stub 우선 배치 |
| 성능 저하 | Set마다 히스토리 처리 오버헤드 | prepend + slice truncation으로 O(1) 수준 유지 |
