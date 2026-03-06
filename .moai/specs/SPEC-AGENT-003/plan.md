---
id: SPEC-AGENT-003
version: "1.0.0"
status: completed
created: "2026-03-06"
updated: "2026-03-06"
author: xtra
---

# SPEC-AGENT-003 Implementation Plan

## 1. 개요

Agent Message Buffer Metrics Exposure 기능의 구현 계획을 정의한다.
`BufferInfoProvider` 선택적 인터페이스를 통해 6개 에이전트의 내부 메시지 버퍼 상태를 `StatsSnapshot`에 노출하고, API/CLI에서 확인 가능하게 한다.

## 2. 마일스톤

### Primary Goal: 코어 인터페이스 및 구조체 확장 (Module 1)

| 태스크 | 파일 | 변경 내용 |
|--------|------|----------|
| T1 | `internal/agent/agent.go` | `BufferInfoProvider` 인터페이스 정의 추가 |
| T2 | `internal/agent/info.go` | `StatsSnapshot`에 `MsgBufferPending`, `MsgBufferCapacity` 필드 추가 |

**의존성**: 없음 (이 마일스톤이 모든 후속 작업의 선행 조건)

### Secondary Goal: 에이전트 구현체 적용 (Module 2)

| 태스크 | 파일 | 변경 내용 |
|--------|------|----------|
| T3 | `internal/agent/modbus/agent.go` | `BufferInfo()` 메서드 추가, `Stats()` 오버라이드, 컴파일 타임 체크 |
| T4 | `internal/agent/modbusserver/agent.go` | `BufferInfo()` 메서드 추가, `Stats()` 오버라이드, 컴파일 타임 체크 |
| T5 | `internal/agent/samsung/agent.go` | `BufferInfo()` 메서드 추가, `Stats()` 오버라이드, 컴파일 타임 체크 |
| T6 | `internal/agent/system/http_receiver.go` | `BufferInfo()` 메서드 추가, `Stats()` 오버라이드, 컴파일 타임 체크 |
| T7 | `internal/agent/system/influxdb_agent.go` | `BufferInfo()` 메서드 추가, `Stats()` 오버라이드, 컴파일 타임 체크 |
| T8 | `internal/agent/system/mqtt_subscriber.go` | `BufferInfo()` 메서드 추가, `Stats()` 오버라이드, 컴파일 타임 체크 |

**의존성**: T1, T2 완료 필요
**병렬 실행**: T3~T8은 서로 독립적이므로 병렬 수행 가능

### Final Goal: API/CLI 출력 반영 (Module 3)

| 태스크 | 파일 | 변경 내용 |
|--------|------|----------|
| T9 | `internal/api/handler/agent.go` | `AgentStatsResponse`와 `AgentStatsInfo`에 `BufferPending`, `BufferCapacity` 필드 추가 |
| T10 | `internal/api/service/agent_adapter.go` | `agentToHandlerInfo()` 변환 시 버퍼 필드 매핑 추가, `AgentStats()` 메서드 반영 |
| T11 | `internal/cli/agent.go` | `agent get` DetailFormatter에 Buffer 행 추가, `agent stats` 출력에 Buffer 포함 |

**의존성**: T1, T2 완료 필요 (T3~T8과 병렬 가능하나, 통합 테스트 시 구현체 필요)

### Optional Goal: 테스트 작성

| 태스크 | 파일 | 변경 내용 |
|--------|------|----------|
| T12 | `internal/agent/info_test.go` | `StatsSnapshot` 새 필드 검증 |
| T13 | `internal/agent/modbus/agent_test.go` | `BufferInfo()` 반환값 검증 |
| T14 | `internal/agent/samsung/agent_test.go` | `BufferInfo()` 반환값 검증 |
| T15 | `internal/api/service/agent_adapter_test.go` | 버퍼 필드 변환 검증 |

## 3. 태스크 의존성 그래프

```
T1 (인터페이스 정의) ─┐
                     ├──> T3~T8 (에이전트 구현체, 병렬)
T2 (StatsSnapshot)  ─┤
                     ├──> T9 (API DTO 확장)
                     ├──> T10 (어댑터 변환)
                     └──> T11 (CLI 출력)

T3~T8 + T9~T11 ──────> T12~T15 (테스트)
```

## 4. 기술 접근

### 4.1 BufferInfoProvider 구현 패턴

각 에이전트에 동일한 패턴 적용:

1. `BufferInfo() (int, int)` 메서드 추가 - 채널의 `len()`과 `cap()` 반환
2. `Stats() agent.StatsSnapshot` 메서드 오버라이드 - 기존 `a.stats.Snapshot()` 결과에 버퍼 정보 추가
3. 컴파일 타임 체크 `var _ agent.BufferInfoProvider = (*Agent)(nil)` 추가

### 4.2 Stats() 오버라이드 vs 어댑터 레이어 감지

**선택: Stats() 오버라이드 방식**

이유:
- `Info()` 내부에서 `ba.stats.Snapshot()`을 호출하므로, 에이전트가 `Stats()`를 오버라이드하면 `Info().Stats` 필드에도 자동 반영
- 어댑터 레이어에서 감지하면 `Stats()` 직접 호출과 `Info().Stats` 간 불일치 발생 가능
- 기존 에이전트들의 `Info()` 메서드가 이미 자체 스냅샷을 생성하므로, 해당 위치에서 버퍼 정보 추가가 자연스러움

주의: `BaseAgent.Info()`는 `ba.stats.Snapshot()`을 호출하지만, 각 구현체(`ModbusAgent`, `NASAAgent` 등)는 자체 `Info()` 메서드를 가지고 있으므로 `ba.stats.Snapshot()` 대신 구현체의 `Stats()` 사용으로 통일해야 할 수 있다. 구현 시 각 에이전트의 `Info()` 메서드 내부를 확인하여 일관성을 보장한다.

### 4.3 CLI 출력 조건부 표시

`buffer_capacity == 0`이면 Buffer 행을 생략하는 조건 추가:
- `agent get`: DetailFormatter의 fieldOrder에 "buffer" 키를 조건부 추가
- `agent stats`: 출력 로직에서 capacity 체크 후 표시

## 5. 리스크 분석

### 리스크 1: 에이전트별 Info() 구현 불일치

- **설명**: 일부 에이전트는 `BaseAgent.Info()`를 호출하고, 다른 에이전트는 자체 `Info()`를 완전히 구현한다
- **영향**: 버퍼 정보가 일부 에이전트에서 누락될 수 있다
- **대응**: 구현 전 각 에이전트의 `Info()` 메서드 체인을 분석하여 `Stats()` 호출 경로 확인

### 리스크 2: 채널 nil 참조

- **설명**: 에이전트가 Stop 이후 채널이 nil 또는 closed 상태일 때 `len(ch)`, `cap(ch)` 호출
- **영향**: `len(nil)`, `cap(nil)`은 Go에서 0을 반환하므로 안전하나, 명시적 nil 체크 고려
- **대응**: Go 언어 스펙상 nil 채널에 `len()`, `cap()` 호출은 0을 반환하므로 추가 방어 코드 불필요

### 리스크 3: JSON 직렬화 역호환성

- **설명**: `StatsSnapshot`에 필드 추가 시 기존 클라이언트에 영향
- **영향**: 새 필드는 기본값 0으로 나타나므로 기존 JSON 파싱에 문제 없음
- **대응**: API DTO에서 `json:"buffer_pending"`, `json:"buffer_capacity"` 태그로 명시적 이름 지정

### 리스크 4: 기존 테스트 깨짐

- **설명**: `StatsSnapshot` 리터럴로 초기화하는 기존 테스트가 새 필드 누락으로 경고 발생 가능
- **영향**: Go는 구조체 리터럴에서 누락 필드를 기본값으로 처리하므로 컴파일 에러 없음. 단, `go vet`에서 경고 가능
- **대응**: 기존 테스트가 필드명 기반 초기화(`StatsSnapshot{MessagesReceived: 10}`)를 사용하면 문제 없음. 순서 기반 초기화 사용 시 수정 필요

## 6. 영향받는 파일 요약

| 파일 | 변경 유형 | 예상 변경량 |
|------|----------|------------|
| `internal/agent/agent.go` | 인터페이스 추가 | +8줄 |
| `internal/agent/info.go` | 필드 추가 | +2줄 |
| `internal/agent/modbus/agent.go` | 메서드 추가 | +12줄 |
| `internal/agent/modbusserver/agent.go` | 메서드 추가 | +12줄 |
| `internal/agent/samsung/agent.go` | 메서드 추가 | +12줄 |
| `internal/agent/system/http_receiver.go` | 메서드 추가 | +12줄 |
| `internal/agent/system/influxdb_agent.go` | 메서드 추가 | +12줄 |
| `internal/agent/system/mqtt_subscriber.go` | 메서드 추가 | +12줄 |
| `internal/api/handler/agent.go` | DTO 필드 추가 | +4줄 |
| `internal/api/service/agent_adapter.go` | 변환 로직 확장 | +6줄 |
| `internal/cli/agent.go` | CLI 출력 반영 | +15줄 |

**총 예상 변경량**: 약 107줄 추가, 기존 코드 수정 최소화
