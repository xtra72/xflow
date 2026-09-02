---
id: SPEC-NODE-004
version: "1.3.0"
status: in-progress
created: "2026-04-15"
updated: "2026-07-28"
author: xtra
priority: high
amendment_of: SPEC-NODE-004
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-04-15 | 1.0.0 | 초기 SPEC 작성 (draft) |
| 2026-04-16 | 1.1.0 | 구현 완료: (1) Timer Agent 주입 경로 확정 — `AgentResolver`는 user agent manager 소속 에이전트만 해석하므로, 시스템 Timer Agent는 `node.WithTimer(system.Timer)` NodeOption 으로 엔진 구성 시점에 직접 주입한다. `resolveTimer()` 스텁 로직을 `n.timer` 기반 통과 + config fallback 패턴으로 재작성. (2) 웹 UI — `trigger` 노드 스키마 신규 등록, `TriggerScheduleEditor` 전용 컴포넌트 추가 (interval/cron/once/times 4종 전용 위젯 + 프리셋 + 인라인 검증), `payload_mode` UI 전용 가상 필드로 payload/payload_template 토글, `source_ch_size` 고급 설정 섹션 분리 |
| 2026-07-28 | 1.2.0 | 인플레이스 개정 (amendment, plan-phase): 트리거 노드에 3종 신규 기능 추가 — (1) `weekly` 스케줄 타입 (특정 요일×시각 다중 조합 → `MM HH * * DOW` cron 등록), (2) `monthly` 스케줄 타입 (특정 일자/`first`/`last` × 시각; `last`는 매일 cron 등록 후 핸들러에서 월 마지막 날에만 emit 게이트), (3) 스케줄별 페이로드 (per-schedule payload — 항목별 선택 + 노드 레벨 폴백, 전 스케줄 타입 적용, 완전 하위 호환). 신규 REQ: 02-09(weekly), 02-10(monthly date/first), 02-11(monthly last-day 게이트), 03-04(per-schedule payload), 08-05(웹 UI weekly/monthly 위젯 + 스케줄별 페이로드 편집). 코드 구현은 후속 run-phase에서 진행. |
| 2026-07-28 | 1.3.0 | 인플레이스 개정 (amendment, plan-phase): **페이로드 모델 통합** — 기존 `static`(정적 리터럴 복사) vs `payload_template`(4종 `$.` 변수 전용 맵)의 이원 구조를 **단일 통합 템플릿 엔진**으로 통합. 문자열 값 평가 규칙 도입: (a) 문자열 전체가 알려진 변수 토큰 `$.<name>`과 정확히 일치 → 변수의 **네이티브 타입 값**으로 치환(예: `$.tick_count` → 숫자 42), (b) 그 외에는 문자 단위 스캔(interpolation) — `$$`→리터럴 `$`, 알려진 `$.<name>`→변수의 **문자열 형태**, 미지의 `$.<name>`→에러 기록 + 해당 키 값 `null`(오타 탐지), 그 외 문자(단독 `$` 포함)→리터럴. 비문자열 값(숫자/불리언/배열/중첩 오브젝트)은 리터럴 패스스루(중첩 맵 미재귀 — 알려진 한계). `static` 개념 제거하되 config 키(`payload` any / `payload_template` map)는 하위호환 유지하여 **양쪽 모두 통합 엔진으로 라우팅**(비-map 스칼라/배열은 `{"value": <v>}` 래핑 후 평가). 프론트엔드 `payload_mode`(static/template) 토글 제거 → 단일 JSON 페이로드 에디터 + `$.<var>`/`$$` 인라인 힌트. 개정 REQ: 03-01(통합 문자열 평가), 03-03(비문자열 패스스루 + config 하위호환 라우팅), 03-04(per-schedule 통합 평가), 06-04(미지 변수 에러+null), 08-04·08-05(단일 페이로드 에디터). 코드 구현은 후속 run-phase에서 진행. |

### Amendments

- **1.2.0 (2026-07-28) — 인플레이스 개정 (in-place amendment)**
  - **직전 상태**: v1.1.0, `status: implemented`
  - **prior_completed_sha**: 미기록 (v1.1.0은 sync-close 이전 `implemented` 상태로, 별도 완료 SHA가 추적되지 않음)
  - **개정 근거 (rationale)**: 기존 4종 스케줄(interval/cron/once/times)만으로는 "특정 요일 반복", "특정 월일 반복", "월말 반복" 및 "스케줄별 서로 다른 페이로드" 요구를 표현할 수 없어, 사용자 확정 요구사항 3종(R1 weekly / R2 monthly / R3 per-schedule payload)을 추가한다.
  - **범위 (scope)**: Module 2(스케줄 설정)에 weekly/monthly 타입 추가, Module 3(페이로드)에 스케줄별 페이로드 해결 순서 추가, Module 4(메타데이터) `schedule_type` enum 확장, 4.x 명세/웹 UI 확장. 기존 interval/cron/once/times 동작 및 노드 레벨 payload 동작은 **변경 없음**(하위 호환).

- **1.3.0 (2026-07-28) — 인플레이스 개정 (in-place amendment)**
  - **직전 상태**: v1.2.0, `status: in-progress` (v1.2.0 개정의 amendment_of 상태를 계속 이어감)
  - **prior_completed_sha**: 미기록 (v1.2.0은 completed로 close되지 않은 in-progress 상태의 연속 개정)
  - **개정 근거 (rationale)**: 기존 페이로드 모델은 `static`(임의 값 리터럴 복사)과 `payload_template`(문자열 값이 각각 정확히 4종 `$.` 변수 중 하나여야 하며 아니면 에러)의 **이원 구조**였다. 이 이원 구조는 (1) 사용자가 상황마다 static/template 중 무엇을 쓸지 결정해야 하고, (2) "리터럴 텍스트와 변수를 한 문자열에 혼합"하거나 "변수를 네이티브 타입으로 삽입"하는 자연스러운 요구를 표현하지 못했다. 사용자 확정 결정에 따라 static/template 이원성을 제거하고 **이스케이프(`$$`)를 지원하는 단일 템플릿 엔진**으로 통합한다.
  - **범위 (scope)**: Module 3(페이로드)의 문자열 값 평가 규칙 통합(REQ-03-01), 비문자열 패스스루 + config 하위호환 라우팅(REQ-03-03), per-schedule 페이로드의 통합 엔진 적용(REQ-03-04), 미지 변수 에러+null 처리(REQ-06-04), 4.3 Go 시그니처 주석, 4.5 메시지 흐름, 4.6 웹 UI 단일 페이로드 에디터(REQ-08-04/08-05). 스케줄 타입(interval/cron/once/times/weekly/monthly), 스케줄 등록 경로, 메타데이터, per-schedule 페이로드 **해결 순서**(항목→노드→기본)는 **변경 없음**.
  - **하위 호환 (backward compatibility)**: config 키 `payload`(any)/`payload_template`(map)는 그대로 수용되어 양쪽 모두 통합 엔진으로 라우팅된다. 비-map 스칼라/배열(구 static 스칼라)은 `{"value": <v>}`로 래핑 후 평가된다. 구 static 맵(예: `{"cmd":"open"}`)은 동일하게 평가되나 **두 가지 희귀 경계 케이스**만 의미가 바뀐다 — (1) 리터럴 `$.trigger_time` 등을 포함하던 static 문자열은 이제 interpolation 되고, (2) 리터럴 `$`는 이제 `$$`로 작성해야 한다(마이그레이션 노트 참조).

# SPEC-NODE-004: Trigger Node - 스케줄 기반 데이터 생성 소스 노드

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼에 스케줄 기반 데이터 생성 기능을 제공하는 Trigger Node를 정의한다. Trigger Node는 지정된 시간 간격, cron 표현식, 특정 시각, 매일 반복 시각, 특정 요일 반복(weekly), 또는 특정 월일/월말 반복(monthly)에 따라 설정된 페이로드(숫자, 문자열, 오브젝트 등)를 자동 생성하여 플로우에 전송하는 SourceNode이다.

본 SPEC은 다음을 포함한다:

- **TriggerNode** (`trigger.go`): BaseNode + SourceNode 인터페이스 구현, 스케줄 기반 메시지 생성
- **Schedule Configuration**: 다중 스케줄 동시 지원 (interval, cron, once, times, weekly, monthly)
- **Payload Generation**: 단일 통합 템플릿 엔진(`$.<var>` 변수 치환 + `$$` 이스케이프)을 통한 페이로드 생성, 스케줄별 페이로드(per-schedule payload) 오버라이드
- **Timer Agent Integration**: Timer System Agent를 AgentResolver 패턴으로 참조

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/node/`
- **Tier**: internal (비공개 패키지)
- **의존 패키지**:
  - `pkg/lifecycle/` (SPEC-LIFE-001): `BaseLifecycle`, `State` 임베딩
  - `pkg/flow/` (SPEC-FLOW-001): `NodeDef`, `Port` 데이터 구조
  - `pkg/message/` (SPEC-MSG-001): `Message`, `Payload`, `Metadata` 인터페이스
  - `internal/node/` (SPEC-NODE-001): `BaseNode`, `SourceNode`, `Node` 인터페이스, `AgentResolver`
  - `internal/agent/system/` (SPEC-TIMER-001): `Timer` 인터페이스 (AgentResolver를 통해 간접 참조)
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **동시성 모델**: sourceCh 채널 기반 메시지 생성, 내부 상태 `sync.Mutex` 보호

### 1.3 설계 원칙

- **SourceNode 패턴 준수**: Engine이 `SourceCh()` 채널에서 메시지를 읽어 출력 와이어로 전달
- **AgentResolver 패턴**: Timer Agent에 직접 의존하지 않고, AgentResolver 인터페이스로 간접 참조
- **다중 스케줄 동시 실행**: 하나의 Trigger Node에서 여러 스케줄을 병렬 실행
- **통합 페이로드 엔진**: 모든 페이로드는 단일 템플릿 엔진으로 평가된다. 문자열 값은 통째 변수 토큰(네이티브 타입 치환) 또는 문자 단위 interpolation(`$.<var>`/`$$`)으로 처리되고, 비문자열 값은 리터럴 패스스루된다
- **Graceful Shutdown**: 모든 타이머 취소 후 sourceCh 정상 닫기

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- TriggerNode 구조체 (BaseNode 임베딩 + SourceNode 인터페이스)
- 6종 스케줄 타입: interval, cron, once, times, weekly (v1.2.0), monthly (v1.2.0)
- 다중 스케줄 동시 설정 및 실행 (weekly/monthly는 요일·시각·일자 다중 조합 지원)
- 단일 통합 템플릿 엔진 기반 페이로드 생성 (`$.<var>` 치환 + `$$` 이스케이프, v1.3.0)
- config 하위호환 라우팅: `payload`(any) / `payload_template`(map) 키 모두 통합 엔진으로 평가 (v1.3.0)
- 스케줄별 페이로드(per-schedule payload) 오버라이드 및 노드 레벨 폴백 (v1.2.0)
- monthly `"last"` 월말 emit 게이트 (v1.2.0)
- 메시지 메타데이터 자동 첨부
- Timer Agent 통합 (AgentResolver 패턴)
- sourceCh 채널 기반 메시지 출력
- 생명주기 통합 (Init, Pause, Resume, Shutdown)
- Node Registry 등록 ("trigger" 타입)
- Sentinel 에러 정의

**OUT OF SCOPE (별도 SPEC)**:
- Timer Agent 자체 구현 (SPEC-TIMER-001: `internal/agent/system/`)
- Engine의 SourceNode 처리 루프 (SPEC-ENGINE-001)
- Flow YAML 파싱 (SPEC-FLOW-001)
- Message 데이터 구조 (SPEC-MSG-001)

### 1.5 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-NODE-001 | 의존 | `BaseNode`, `SourceNode`, `Node` 인터페이스, `AgentResolver`, Node Registry |
| SPEC-LIFE-001 | 의존 | `BaseLifecycle`, `State` 임베딩 |
| SPEC-FLOW-001 | 의존 | `NodeDef`, `Port` 데이터 구조 |
| SPEC-MSG-001 | 의존 | `Message`, `Payload`, `Metadata` 인터페이스 |
| SPEC-ENGINE-001 | 소비자 | Engine이 `SourceCh()`에서 메시지를 읽어 출력 와이어로 전달 |
| SPEC-TIMER-001 | 협력 | Timer Agent를 AgentResolver로 참조하여 스케줄 등록 |

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: Timer System Agent가 시스템 시작 시 자동 활성화되어 있으며, AgentResolver를 통해 조회 가능하다
- A2: Timer Agent의 `SetInterval()`, `SetCron()`, `SetTimeout()` 메서드는 goroutine-safe하다
- A3: Timer Agent의 핸들러 함수(`TimerHandler`)는 독립 goroutine에서 실행된다 (타이머 간 간섭 없음)
- A4: Engine이 SourceNode 인터페이스를 확인하고 `SourceCh()`에서 메시지를 읽는 goroutine을 관리한다
- A5: sourceCh 채널이 닫히면 Engine의 SourceNode 읽기 goroutine이 정상 종료된다
- A6: `_agent_resolver` 키로 config에 주입되는 AgentResolver가 Timer 인터페이스를 구현하는 Agent를 반환한다
- A13 (v1.2.0): `weekly`/`monthly` 스케줄은 기존 `times` 타입과 **동일한 cron 스케줄러 경로**(`SetCron()`)를 사용한다. 별도의 타임존 처리 로직을 새로 도입하지 않으며, 현재 cron 스케줄러가 사용하는 타임존을 그대로 상속한다 (동작이 놀랍지 않도록 명시).

### 2.2 도메인 가정

- A7: 하나의 Trigger Node에 1개 이상의 스케줄이 반드시 설정되어야 한다
- A8: 스케줄 미설정 시 Init에서 에러를 반환한다
- A9: "times" 스케줄 타입의 시각은 "HH:MM" 형식이며, 매일 해당 시각에 반복 실행된다
- A10: 페이로드 미설정 시 기본 페이로드 `{"trigger_time": <현재시각>}`을 생성한다
- A11: sourceCh 버퍼가 가득 찬 경우(backpressure), 메시지를 드롭하고 경고 로그를 기록한다
- A12: 통합 템플릿 엔진의 알려진 변수는 `$.trigger_time`, `$.tick_count`, `$.schedule_id`, `$.trigger_id` 4종이다 (v1.3.0에서 변수 목록은 변경 없음)
- A14 (v1.2.0): `weekly`/`monthly`는 배열(`days[]`, `times[]`)을 받아 다중 조합을 지원한다. 단일 값도 원소 1개의 배열로 취급한다. weekday 토큰은 소문자 3자 약어(`sun`~`sat`) 및 정수 `0`~`6`(0=일요일)을 허용하며, `monthly.day`는 정수 `1`~`31`, `"first"`(=1일), `"last"`(월말)를 허용한다.
- A15 (v1.2.0): 각 스케줄 항목은 선택적으로 자체 `payload`/`payload_template`를 가질 수 있다. 발화 시 페이로드 해결 순서는 (1) 스케줄 항목 페이로드 → (2) 노드 레벨 페이로드 → (3) 기본 페이로드 이며, 이는 모든 스케줄 타입에 적용되고 기존 설정과 **완전 하위 호환**된다.
- A16 (v1.2.0): 표준 cron은 "마지막 날(L)"을 표현하지 못하므로, `monthly.day = "last"`는 매일 cron 등록 후 핸들러에서 `time.Now().Day()`가 해당 월의 마지막 날과 같을 때만 메시지를 emit한다. 또한 존재하지 않는 일자(예: 31)를 정수로 지정하면 해당 날이 없는 달에는 발화하지 않는다(표준 cron 동작, 의도됨).
- A17 (v1.3.0): 페이로드는 **단일 통합 템플릿 엔진**으로 평가된다. 문자열 값 규칙 — (a) 문자열 **전체**가 알려진 변수 토큰 `$.<name>`과 정확히 일치하면 변수의 **네이티브 타입 값**으로 치환하고(예: `$.tick_count` → 숫자), (b) 그 외에는 문자 단위 스캔으로 `$$`→리터럴 `$`, 알려진 `$.<name>`→변수의 **문자열 형태**, 미지의 `$.<name>`→에러 기록 + 해당 키 값 `null`(오타 탐지), 그 외 문자(단독 `$` 포함)→리터럴로 처리한다. 비문자열 값(숫자/불리언/배열/중첩 오브젝트)은 **리터럴 패스스루**되며, 중첩 맵은 재귀 평가하지 않는다(알려진 한계).
- A18 (v1.3.0): config 키 `payload`(any)/`payload_template`(map)는 하위호환을 위해 모두 수용되며 **양쪽 다 통합 엔진으로 라우팅**된다. 페이로드 값이 map이면 그대로 평가하고, 비-map 스칼라/배열(구 static 스칼라)이면 `{"value": <v>}`로 래핑한 뒤 평가한다. 구 static 맵(`{"cmd":"open"}` 등)은 동일하게 평가되나, 리터럴 `$.` 포함 문자열은 이제 interpolation 되고 리터럴 `$`는 `$$`로 작성해야 하는 두 경계 케이스만 의미가 달라진다.

---

## 3. Requirements (요구사항)

### Module 1: TriggerNode Core - 트리거 노드 핵심 (P0)

#### REQ-NODE-004-01-01 (Ubiquitous) TriggerNode 구조체

시스템은 **항상** `BaseNode`를 임베딩하고 `SourceNode` 인터페이스를 구현하는 `TriggerNode` 구조체를 제공해야 한다:

- `sourceCh chan message.Message`: 메시지 출력 채널 (Engine이 읽기)
- `SourceCh() <-chan message.Message`: SourceNode 인터페이스 메서드
- 노드 타입: `"trigger"`
- 카테고리: `"input"`

#### REQ-NODE-004-01-02 (Ubiquitous) NewTriggerNode 팩토리 함수

시스템은 **항상** `NewTriggerNode(def flow.NodeDef, opts ...NodeOption) (Node, error)` 팩토리 함수를 제공해야 한다:

- `flow.NodeDef`에서 스케줄 설정과 페이로드 설정을 추출
- `config["_agent_resolver"]`에서 `AgentResolver`를 추출
- sourceCh 채널 생성 (버퍼 크기 설정 가능, 기본 64)

#### REQ-NODE-004-01-03 (Ubiquitous) Node Registry 등록

시스템은 **항상** Node Registry의 `RegisterDefaults()`에서 `"trigger"` 타입으로 `NewTriggerNode` 팩토리 함수를 등록해야 한다.

#### REQ-NODE-004-01-04 (Event-Driven) Process 메서드

**WHEN** `TriggerNode.Process(ctx, msg)` 호출 시, **THEN** SourceNode는 외부 입력을 받지 않으므로 입력 메시지를 무시하고 빈 결과를 반환해야 한다.

---

### Module 2: Schedule Configuration - 스케줄 설정 (P0)

#### REQ-NODE-004-02-01 (Ubiquitous) 다중 스케줄 설정

시스템은 **항상** 하나의 TriggerNode에 1개 이상의 스케줄을 설정할 수 있어야 한다. 설정 키는 `"schedules"`이며, 각 스케줄은 `type`과 `value`를 포함한다.

#### REQ-NODE-004-02-02 (State-Driven) interval 스케줄 타입

**IF** 스케줄 타입이 `"interval"`이면, **THEN** `value` 필드를 `time.Duration` 문자열로 파싱하고 Timer Agent의 `SetInterval()`로 주기적 타이머를 등록해야 한다.

예시: `{"type": "interval", "value": "5s"}`

#### REQ-NODE-004-02-03 (State-Driven) cron 스케줄 타입

**IF** 스케줄 타입이 `"cron"`이면, **THEN** `value` 필드를 cron 표현식으로 Timer Agent의 `SetCron()`으로 크론 타이머를 등록해야 한다.

예시: `{"type": "cron", "value": "0 */5 * * * *"}`

#### REQ-NODE-004-02-04 (State-Driven) once 스케줄 타입

**IF** 스케줄 타입이 `"once"`이면, **THEN** `value` 필드를 ISO 8601 타임스탬프로 파싱하여, 현재 시각과의 차이를 계산하고 Timer Agent의 `SetTimeout()`으로 1회성 타이머를 등록해야 한다.

예시: `{"type": "once", "value": "2026-04-15T10:00:00Z"}`

#### REQ-NODE-004-02-05 (State-Driven) times 스케줄 타입

**IF** 스케줄 타입이 `"times"`이면, **THEN** `value` 필드를 `"HH:MM"` 형식의 시각 배열로 파싱하여, 각 시각에 대해 cron 표현식(`MM HH * * *`)으로 변환하고 Timer Agent의 `SetCron()`으로 매일 반복 타이머를 등록해야 한다.

예시: `{"type": "times", "value": ["09:00", "12:00", "18:00"]}`

#### REQ-NODE-004-02-06 (Unwanted) 스케줄 미설정 거부

시스템은 `schedules` 설정이 비어있거나 누락된 경우 Init 시 `ErrTriggerNoSchedules` 에러를 반환**해야 한다**.

#### REQ-NODE-004-02-07 (Unwanted) 잘못된 스케줄 타입 거부

시스템은 지원하지 않는 스케줄 타입이 설정된 경우 Init 시 `ErrTriggerInvalidScheduleType` 에러를 반환**해야 한다**.

#### REQ-NODE-004-02-08 (Unwanted) 잘못된 스케줄 값 거부

시스템은 스케줄 값이 유효하지 않은 경우(파싱 실패, 과거 시각 등) Init 시 `ErrTriggerInvalidScheduleValue` 에러를 반환**해야 한다**.

#### REQ-NODE-004-02-09 (State-Driven) weekly 스케줄 타입 (v1.2.0)

**IF** 스케줄 타입이 `"weekly"`이면, **THEN** `days` 배열의 각 요일과 `times` 배열의 각 시각 **조합마다** cron 표현식(`MM HH * * DOW`)으로 변환하여 Timer Agent의 `SetCron()`으로 매주 반복 타이머를 등록해야 한다.

설정 스키마:

```json
{"type": "weekly", "days": ["mon", "wed", "fri"], "times": ["09:00", "18:00"]}
```

- **`days`**: 요일 토큰 배열. 허용 토큰 = 소문자 3자 약어(`sun`, `mon`, `tue`, `wed`, `thu`, `fri`, `sat`), 정수 `0`~`6`도 허용. cron Dow 필드 매핑: `sun`/0 → 0, `mon`/1 → 1, `tue`/2 → 2, `wed`/3 → 3, `thu`/4 → 4, `fri`/5 → 5, `sat`/6 → 6.
- **`times`**: `"HH:MM"` 형식(00:00~23:59) 시각 배열.
- 등록 타이머 수 = `len(days) × len(times)` (다중 조합).
- 잘못된 요일 토큰 또는 시각은 `ErrTriggerInvalidScheduleValue` 에러를 반환한다.

예시: `days=["mon","wed","fri"]`, `times=["09:00","18:00"]` → 6개 cron 등록 (`0 9 * * 1`, `0 18 * * 1`, `0 9 * * 3`, `0 18 * * 3`, `0 9 * * 5`, `0 18 * * 5`)

#### REQ-NODE-004-02-10 (State-Driven) monthly 스케줄 타입 - 특정 일자/first (v1.2.0)

**IF** 스케줄 타입이 `"monthly"`이고 `day`가 정수(1~31) 또는 `"first"`이면, **THEN** 해당 일자(`"first"` = 1일)와 `times` 배열의 각 시각마다 cron 표현식(`MM HH N * *`)으로 변환하여 Timer Agent의 `SetCron()`으로 매월 반복 타이머를 등록해야 한다.

설정 스키마:

```json
{"type": "monthly", "day": 15, "times": ["08:30"]}
```

- **`day`**: 정수 `1`~`31`, 또는 `"first"`(=1일), 또는 `"last"`(REQ-NODE-004-02-11 참조).
- **`times`**: `"HH:MM"` 형식 시각 배열.
- **표준 cron 특성**: 존재하지 않는 날짜(예: `day=31`)는 그 날이 없는 달(2월, 4월 등)에는 **발화하지 않는다**. 이는 의도된 동작이며, 항상 월말에 발화하려면 `"last"`를 사용해야 한다.
- 잘못된 `day` 값(0, 32 이상, 알 수 없는 문자열) 또는 시각은 `ErrTriggerInvalidScheduleValue` 에러를 반환한다.

예시: `day=15`, `times=["08:30"]` → `30 8 15 * *` (매월 15일 08:30)

#### REQ-NODE-004-02-11 (Event-Driven) monthly `"last"` 월말 emit 게이트 (v1.2.0)

**WHEN** 스케줄 타입이 `"monthly"`이고 `day`가 `"last"`인 경우, **THEN** robfig/cron이 마지막 날(L)을 표현하지 못하므로 `times` 배열의 각 시각마다 **매일** cron(`MM HH * * *`)을 등록하되, 핸들러 진입 시 현재 날짜가 해당 월의 마지막 날일 때만(`time.Now().Day() == <해당 월의 마지막 날>`) 메시지를 emit해야 한다.

- 타이머는 매일 발화하지만, 월말이 아닌 날에는 메시지를 생성하지 않는다 (게이트).
- 2월(28/29일), 30일 달(4·6·9·11월), 31일 달 모두 각 달의 **실제 마지막 날**에만 발화한다.

예시: `{"type": "monthly", "day": "last", "times": ["23:59"]}` → 매일 `59 23 * * *` 등록 + 핸들러 월말 게이트

---

### Module 3: Payload Generation - 페이로드 생성 (P0)

#### REQ-NODE-004-03-01 (Ubiquitous) 통합 페이로드 - 문자열 값 평가 (v1.3.0)

시스템은 **항상** 모든 페이로드를 **단일 통합 템플릿 엔진**으로 평가해야 한다. (v1.2.0까지의 `static`(정적 리터럴 복사) vs `payload_template`(변수 전용 맵) 이원 구조는 제거되고, 하나의 페이로드 개념으로 통합된다.)

페이로드 맵의 각 **문자열 값**에 대한 평가 규칙:

1. **통째 변수 치환 (whole-value)**: 문자열 값 **전체**가 알려진 변수 토큰 `$.<name>`과 **정확히 일치**하면, 변수의 **네이티브 타입 값**으로 치환한다.
   - 예: `$.tick_count` → 문자열 `"42"`가 아닌 **숫자** `42`. (v1.2.0까지의 타입 보존 치환 동작을 유지한다.)
2. **문자 단위 스캔 (interpolation)**: 그 외의 문자열은 문자 단위로 스캔하여 다음과 같이 처리한다:
   - `$$` → 리터럴 `$` 한 글자
   - `$.<name>` (알려진 변수) → 변수의 **문자열 형태**로 치환
   - `$.<name>` (미지의 변수) → **에러 기록**(에러 메시지) 후 해당 키의 값을 `null`로 만든다 (오타 탐지 — v1.2.0까지의 미지 변수 처리와 동일)
   - 그 외 모든 문자 (단독 `$`가 `$$`나 `$.`를 형성하지 않는 경우 포함) → 리터럴

알려진 변수(REQ-NODE-004-03-03 참조)는 4종으로 변경 없다: `$.trigger_time`, `$.tick_count`, `$.schedule_id`, `$.trigger_id`.

#### REQ-NODE-004-03-02 (State-Driven) 기본 페이로드

**IF** 노드 레벨 및 스케줄 항목 페이로드가 모두 없으면(nil), **THEN** 기본 페이로드 `{"trigger_time": <현재 ISO 8601 시각>}`을 생성해야 한다. (해결 순서는 REQ-NODE-004-03-04 참조.)

#### REQ-NODE-004-03-03 (State-Driven) 비문자열 값 패스스루 + config 하위호환 라우팅 (v1.3.0)

**IF** 페이로드 맵의 값이 **비문자열**(숫자, 불리언, 배열, 중첩 오브젝트)이면, **THEN** 통합 엔진은 해당 값을 **리터럴 패스스루**해야 한다 (변수 치환 대상이 아님).

- **중첩 맵 미재귀 (알려진 한계)**: 중첩된 map/오브젝트 값 내부의 문자열은 **재귀적으로 평가하지 않는다**. 오직 최상위 맵의 문자열 값만 REQ-NODE-004-03-01 규칙으로 평가된다.

**알려진 변수** (통합 엔진이 인식하는 4종, 변경 없음):

- `$.trigger_time`: 트리거 발생 ISO 8601 시각
- `$.tick_count`: 해당 스케줄의 누적 트리거 횟수
- `$.schedule_id`: 스케줄 식별자
- `$.trigger_id`: 트리거 노드 이름

**config 하위호환 라우팅 (static 제거 후에도 config 키 유지)**: 노드 레벨 및 스케줄 항목의 페이로드 config 키는 하위호환을 위해 역사적 `payload`(any 값)와 `payload_template`(map) 형식을 **모두 수용**하며, **양쪽 모두 통합 템플릿 엔진으로 라우팅**된다.

- 페이로드 값이 **map** → 그대로 통합 엔진 평가.
- 페이로드 값이 **비-map 스칼라/배열**(구 static 스칼라) → `{"value": <v>}`로 래핑한 뒤 평가. 예: 구 static 스칼라 `5` → `{"value": 5}`.

**마이그레이션 노트 (의미가 바뀌는 경계 케이스)**: 구 static 맵 `{"cmd":"open"}`은 `"open"`에 `$.`/`$$`가 없으므로 리터럴로 통과하여 **동일하게** 평가된다. 단, 다음 두 희귀 케이스만 의미가 달라진다:

1. 리터럴로 전송되던 static 문자열이 `$.trigger_time` 등 알려진 변수 토큰을 포함하면, 이제 **interpolation** 된다.
2. 리터럴 `$`는 이제 `$$`로 작성해야 한다 (단독 `$`는 `$.`/`$$`를 형성하지 않는 한 리터럴이지만, 변수 표기와의 혼동을 피하려면 명시적으로 `$$` 권장).

#### REQ-NODE-004-03-04 (State-Driven) 스케줄별 페이로드 (per-schedule payload) (v1.2.0)

**IF** 개별 스케줄 항목에 자체 페이로드(`payload` 또는 `payload_template`)가 지정되어 있으면, **THEN** 해당 스케줄이 발화할 때 그 스케줄 항목의 페이로드를 우선 사용해야 한다. 선택된 페이로드 소스는 REQ-NODE-004-03-01의 **통합 템플릿 엔진**으로 평가된다. 발화된 스케줄의 페이로드 **해결 순서** (v1.2.0에서 변경 없음):

1. **해당 스케줄 항목**의 페이로드 소스 (`payload` / `payload_template`, 존재 시)
2. **노드 레벨** 페이로드 소스 (`payload` / `payload_template`, 존재 시)
3. **기본 페이로드** `{"trigger_time": <현재 ISO 8601 시각>}`

- **통합 엔진 적용 (v1.3.0)**: 위 (1)/(2)에서 선택된 페이로드 소스는 config 하위호환 라우팅(REQ-NODE-004-03-03)을 거쳐(map은 그대로, 비-map 스칼라/배열은 `{"value": <v>}` 래핑) 통합 엔진으로 평가된다. static/template 구분 없이 동일 엔진을 사용한다.
- **적용 범위**: 본 규칙은 **모든 스케줄 타입**(interval, cron, once, times, weekly, monthly)에 적용된다.
- **하위 호환**: 스케줄 항목에 페이로드가 없는 기존 설정은 노드 레벨 페이로드(또는 기본 페이로드)로 동작하여 **완전 하위 호환**을 보장한다.
- 평가 결과 페이로드는 트리거마다 깊은 복사되어 메시지 간 데이터 격리를 유지한다 (v1.2.0 동일 정책).

설정 스키마 예시:

```json
{
  "type": "weekly",
  "days": ["mon"],
  "times": ["09:00"],
  "payload": {"shift": "morning", "team": "A"}
}
```

---

### Module 4: Message Metadata - 메시지 메타데이터 (P0)

#### REQ-NODE-004-04-01 (Ubiquitous) 트리거 메타데이터 자동 첨부

시스템은 **항상** 생성된 메시지의 Metadata에 다음 정보를 첨부해야 한다:

- `trigger.schedule_type`: 스케줄 타입 (`"interval"` | `"cron"` | `"once"` | `"times"` | `"weekly"` | `"monthly"`)
- `trigger.schedule_id`: 스케줄 고유 식별자 (TimerID)
- `trigger.tick_count`: 해당 스케줄의 누적 트리거 횟수
- `trigger.trigger_time`: 트리거 발생 시각 (ISO 8601)
- `trigger.node_name`: Trigger 노드의 이름 (`Node.Name()`)

---

### Module 5: Lifecycle Integration - 생명주기 통합 (P0)

#### REQ-NODE-004-05-01 (Event-Driven) Init 초기화

**WHEN** `TriggerNode.Init(ctx)` 호출 시, **THEN** 다음을 수행해야 한다:

1. 상태를 `StateInitializing`으로 전이
2. `config["_agent_resolver"]`에서 AgentResolver 추출 및 Timer Agent resolve
3. Timer Agent가 `Timer` 인터페이스를 구현하는지 확인
4. 모든 스케줄을 파싱하고 Timer Agent에 등록
5. 각 스케줄의 핸들러에서 페이로드를 생성하여 sourceCh로 전송
6. 초기화 성공 시 상태를 `StateRunning`으로 전이
7. 초기화 실패 시 등록된 타이머를 모두 취소하고 상태를 `StateError`로 전이

#### REQ-NODE-004-05-02 (Event-Driven) Shutdown 종료

**WHEN** `TriggerNode.Shutdown(ctx)` 호출 시, **THEN** 다음을 수행해야 한다:

1. 상태를 `StateStopping`으로 전이
2. 등록된 모든 타이머를 Timer Agent에서 취소 (`Cancel()`)
3. sourceCh 채널 닫기
4. 상태를 `StateStopped`으로 전이

#### REQ-NODE-004-05-03 (Event-Driven) Pause 일시정지

**WHEN** Trigger Node가 Pause 상태로 전이하면, **THEN** 새로운 메시지 생성을 중단해야 한다. 기존 sourceCh에 버퍼링된 메시지는 Engine이 계속 읽을 수 있다.

#### REQ-NODE-004-05-04 (Event-Driven) Resume 재개

**WHEN** Trigger Node가 Resume 상태로 전이하면, **THEN** 메시지 생성을 재개해야 한다.

---

### Module 6: Error Handling - 에러 처리 (P0)

#### REQ-NODE-004-06-01 (Ubiquitous) Sentinel 에러 정의

시스템은 **항상** 다음 sentinel 에러를 정의해야 한다:

- `ErrTriggerNoSchedules`: 스케줄 미설정
- `ErrTriggerInvalidScheduleType`: 지원하지 않는 스케줄 타입
- `ErrTriggerInvalidScheduleValue`: 유효하지 않은 스케줄 값
- `ErrTriggerTimerNotAvailable`: Timer Agent를 찾을 수 없음
- `ErrTriggerPayloadTemplateFailed`: 템플릿 페이로드 생성 실패

#### REQ-NODE-004-06-02 (Event-Driven) Timer Agent 미사용 가능

**WHEN** AgentResolver를 통해 Timer Agent를 찾을 수 없으면, **THEN** Init 시 `ErrTriggerTimerNotAvailable` 에러를 반환해야 한다.

#### REQ-NODE-004-06-03 (Event-Driven) 채널 만료 시 메시지 드롭

**WHEN** sourceCh 채널 버퍼가 가득 차서 메시지를 전송할 수 없으면, **THEN** 해당 메시지를 드롭하고 경고 로그를 기록해야 한다.

#### REQ-NODE-004-06-04 (Event-Driven) 통합 엔진 평가 실패 - 미지 변수 (v1.3.0)

**WHEN** 통합 템플릿 엔진이 문자열 값 interpolation 중 **미지의 변수** `$.<name>`(알려진 4종에 없음)을 만나면, **THEN** 에러 메시지를 기록하고 **해당 키의 값을 `null`로** 설정한 뒤, 에러 메타데이터(`trigger.error`)를 포함한 메시지를 sourceCh로 전송해야 한다.

- 오타 탐지 목적으로, 미지 변수는 조용히 무시하지 않고 명시적으로 에러로 기록된다 (v1.2.0 미지 변수 처리와 동일한 정책).
- 하나의 페이로드 맵에서 일부 키만 미지 변수 에러인 경우, 정상 키들은 그대로 평가되고 문제 키만 `null`이 된다.

---

## 4. Specifications (명세)

### 4.1 파일 구조

```
internal/node/
  trigger.go              # TriggerNode 구현
  trigger_test.go         # 단위 테스트
```

### 4.2 Flow YAML 설정 예시

```yaml
nodes:
  - name: "my-trigger"
    type: "trigger"
    config:
      schedules:
        - type: "interval"
          value: "5s"
        - type: "cron"
          value: "0 */5 * * * *"
        - type: "once"
          value: "2026-04-15T10:00:00Z"
        - type: "times"
          value: ["09:00", "12:00", "18:00"]
        # weekly (v1.2.0): 특정 요일 × 시각 다중 조합
        - type: "weekly"
          days: ["mon", "wed", "fri"]
          times: ["09:00", "18:00"]
        # monthly (v1.2.0): 특정 일자/first/last × 시각
        - type: "monthly"
          day: 15                 # 정수(1~31) | "first" | "last"
          times: ["08:30"]
        # monthly last-day (v1.2.0): 스케줄별 페이로드 오버라이드 예시
        - type: "monthly"
          day: "last"
          times: ["23:59"]
          payload:                # 이 스케줄만의 페이로드 (노드 레벨보다 우선)
            report: "month-end"
      payload:                    # 노드 레벨(폴백) 페이로드
        temperature: 25.5
        status: "active"
      channel_buffer: 64
    outputs:
      - "out"
```

### 4.3 타입 시그니처

```go
package node

import (
    "context"
    "sync"
    "time"

    "xflow/pkg/flow"
    "xflow/pkg/lifecycle"
    "xflow/pkg/message"
    "xflow/internal/agent/system"
)

// ── Schedule Types ──

type TriggerScheduleType string

const (
    TriggerScheduleInterval TriggerScheduleType = "interval"
    TriggerScheduleCron     TriggerScheduleType = "cron"
    TriggerScheduleOnce     TriggerScheduleType = "once"
    TriggerScheduleTimes    TriggerScheduleType = "times"
    TriggerScheduleWeekly   TriggerScheduleType = "weekly"  // v1.2.0
    TriggerScheduleMonthly  TriggerScheduleType = "monthly" // v1.2.0
)

// TriggerSchedule 은 단일 스케줄 설정을 나타낸다.
type TriggerSchedule struct {
    Type  TriggerScheduleType // 스케줄 타입
    Value any                 // interval: "5s", cron: "0 */5 * * * *", once: "2026-...", times: ["09:00",...]

    // weekly (v1.2.0): 요일 × 시각 다중 조합
    Days  []string // 요일 토큰 (sun~sat 또는 0~6). weekly 전용
    // weekly/monthly (v1.2.0): 시각 배열 ("HH:MM")
    Times []string // weekly/monthly 전용 (times 타입은 Value 사용, 하위 호환)
    // monthly (v1.2.0): 일자 (정수 1~31 | "first" | "last")
    Day   any      // monthly 전용

    // per-schedule payload (v1.2.0): 스케줄 항목별 페이로드 오버라이드 (선택)
    // nil이면 노드 레벨 payload/payloadTmpl 로 폴백한다.
    // v1.3.0: 두 필드 모두 통합 템플릿 엔진으로 평가된다. Payload 가 비-map 스칼라/배열이면
    //         {"value": <v>} 로 래핑 후 평가한다 (config 하위호환 라우팅).
    Payload     any            // 이 스케줄의 페이로드 소스 (config `payload` 키, map|scalar|array, 선택)
    PayloadTmpl map[string]any // 이 스케줄의 페이로드 소스 (config `payload_template` 키, map, 선택)
}

// triggerTimerEntry 는 등록된 타이머의 런타임 정보를 추적한다.
type triggerTimerEntry struct {
    timerID      system.TimerID
    scheduleType TriggerScheduleType
    scheduleID   string      // 사용자 식별용 (예: "interval-0", "cron-1")
    tickCount    int64       // 누적 트리거 횟수 (atomic)
}

// ── TriggerNode ──

type TriggerNode struct {
    *BaseNode
    sourceCh      chan message.Message
    schedules     []TriggerSchedule
    // v1.3.0: 아래 두 필드는 노드 레벨 페이로드 소스로, 모두 통합 템플릿 엔진으로 평가된다.
    payload       any                    // config `payload` 키 (map|scalar|array; nil이면 폴백/기본)
    payloadTmpl   map[string]any         // config `payload_template` 키 (map; nil이면 미사용)
    channelBuffer int                    // sourceCh 버퍼 크기 (기본 64)
    timer         system.Timer           // Timer Agent 인터페이스
    resolver      AgentResolver          // Agent 조회용
    entries       []triggerTimerEntry    // 등록된 타이머 목록
    paused        bool                   // Pause 상태 플래그
    mu            sync.Mutex             // 내부 상태 보호
}

// 컴파일 타임 인터페이스 체크
var _ Node = (*TriggerNode)(nil)
var _ SourceNode = (*TriggerNode)(nil)

func NewTriggerNode(def flow.NodeDef, opts ...NodeOption) (Node, error)

func (n *TriggerNode) Init(ctx context.Context) error
func (n *TriggerNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error)
func (n *TriggerNode) Shutdown(ctx context.Context) error
func (n *TriggerNode) SourceCh() <-chan message.Message

// ── Errors ──

var (
    ErrTriggerNoSchedules          = fmt.Errorf("trigger: %w: no schedules configured", ErrInvalidConfig)
    ErrTriggerInvalidScheduleType  = fmt.Errorf("trigger: %w: unsupported schedule type", ErrInvalidConfig)
    ErrTriggerInvalidScheduleValue = fmt.Errorf("trigger: %w: invalid schedule value", ErrInvalidConfig)
    ErrTriggerTimerNotAvailable    = fmt.Errorf("trigger: %w: timer agent not available", ErrNodeNotInitialized)
    ErrTriggerPayloadTemplateFailed = fmt.Errorf("trigger: %w: payload template evaluation failed", ErrInvalidConfig)
)

// WithTimer 는 TriggerNode 에 Timer 인터페이스를 직접 주입하는 옵션을 반환한다.
// 시스템 Timer Agent 는 AgentResolver(user agent manager)를 통해 접근할 수 없으므로
// 엔진 구성 시점에 이 옵션으로 직접 주입한다.
func WithTimer(timer system.Timer) NodeOption
```

#### 4.3.1 Timer Agent 주입 경로 (v1.1.0)

Trigger 노드는 시스템 Timer Agent 를 필요로 하지만, `AgentResolver` 인터페이스는
`agent.Manager` 소속 user agent 만 해석할 수 있다. 시스템 에이전트(Timer, Logger, Store, Event, File)는
`SystemAgentManager` 가 별도로 관리하므로 resolver 경로로는 접근 불가능하다.

이를 해결하기 위해 **`NodeOption` 기반 직접 주입** 경로를 채택한다:

```
cmd/xflowd/main.go
    sysMgr := system.NewSystemAgentManager()
    sysMgr.Initialize(cfg)
    sysMgr.Start(ctx)
    │
    ▼ sysMgr.Timer() returns *TimerAgent implementing system.Timer
    │
    eng := engine.NewEngine(
        engine.WithNodeOptions(
            node.WithAgentResolver(agentResolver),  // user agents
            node.WithTimer(sysMgr.Timer()),          // system timer (NEW)
        ),
    )
    │
    ▼ Engine.DeployFlow 시 각 노드 생성 시점에 옵션 전달
    │
    NewTriggerNode(def, opts...)
        base := NewBaseNode(def, opts...)
            for _, opt := range opts { opt(b) }    // b.config["_timer_agent"] = timer
        if a, ok := base.config["_timer_agent"]; ok {
            n.timer = a.(system.Timer)              // 노드 필드에 복사 (Configure 보호)
        }
    │
    ▼ Configure(def.Config) 호출 시 b.config 덮어쓰기 되지만 n.timer 는 보존
    │
    Init(ctx)
        resolveTimer()
            if n.timer != nil { return nil }        // 즉시 통과 (production path)
            if a, ok := n.config["_timer_agent"]; ok { ... }  // test path (기존 호환)
            return ErrTriggerTimerNotAvailable
```

**주입 우선순위**:
1. `NodeOption.WithTimer()` → factory 시점에 `n.timer` 설정 (production)
2. `config["_timer_agent"]` → Configure 시점에 `n.timer` 설정 (테스트, backward compat)
3. 둘 다 없음 → `ErrTriggerTimerNotAvailable`

`AgentResolver` 기반 해석 경로는 설계 단계에서 고려되었으나 시스템 에이전트와 user 에이전트의
관리 주체가 다르므로 폐기되었다.

### 4.4 TriggerNode 동작 다이어그램

```
                 Timer Agent
                 (System Agent)
                     │
    ┌────────────────┼─────────────────────┐
    │  TriggerNode   │                     │
    │                │                     │
    │  Schedule 1 ───┤ SetInterval("5s")   │
    │  Schedule 2 ───┤ SetCron("0 */5 *")  │
    │  Schedule 3 ───┤ SetTimeout(delta)   │
    │  Schedule 4 ───┤ SetCron("00 09 *")  │  ← times -> cron 변환
    │                │                     │
    │  TimerHandler ←┘                     │
    │       │                              │
    │       ▼                              │
    │  [Payload 생성]                       │
    │  [Metadata 첨부]                      │
    │       │                              │
    │       ▼                              │
    │  sourceCh ──────────────────────────►│──► Engine ──► 출력 와이어
    │                                      │
    └──────────────────────────────────────┘
```

### 4.5 메시지 생성 흐름

```
Timer Agent 트리거
    │
    ├── 1. TimerHandler 호출 (독립 goroutine)
    │       ├── TimerTrigger 수신 {TimerID, TriggerAt, TickCount, ScheduleID}
    │       ├── Paused 상태 확인 → paused=true이면 리턴 (메시지 미생성)
    │       └── monthly "last" 게이트 (v1.2.0):
    │             해당 entry가 last-day 스케줄이면
    │             time.Now().Day() != <해당 월 마지막 날> 이면 리턴 (메시지 미생성)
    │
    ├── 2. 페이로드 결정 (per-schedule 우선, v1.2.0) → 통합 엔진 평가 (v1.3.0)
    │       ├── 소스 선택 (해결 순서):
    │       │     (1) 스케줄 항목 payload/payload_template 있음 → 우선 선택
    │       │     (2) 노드 레벨 payload/payload_template 있음 → 폴백 선택
    │       │     (3) 모두 없음 → 기본 페이로드 {"trigger_time": now}
    │       └── 선택된 소스 평가 (통합 템플릿 엔진, v1.3.0):
    │             ├── 소스가 map → 그대로 평가
    │             ├── 소스가 비-map 스칼라/배열 → {"value": v} 래핑 후 평가
    │             └── 각 문자열 값: 통째 변수(네이티브 타입) | interpolation($$→$, $.<var>, 미지→에러+null)
    │
    ├── 3. Message 생성
    │       ├── Payload: 결정된 페이로드
    │       └── Metadata:
    │             ├── trigger.schedule_type
    │             ├── trigger.schedule_id
    │             ├── trigger.tick_count
    │             ├── trigger.trigger_time
    │             └── trigger.node_name
    │
    └── 4. sourceCh 전송
            ├── 성공 → 다음 트리거 대기
            └── 버퍼 풀 → 메시지 드롭 + 경고 로그
```

### 4.6 Web UI - Trigger 노드 설정 에디터 (v1.1.0)

트리거 노드의 복잡한 다중 스케줄 + 페이로드 설정을 사용자가 직관적으로 편집할 수 있도록
전용 프론트엔드 컴포넌트를 제공한다.

#### 파일 구조

```
web/src/
  config/
    nodeSchemas.ts                        # trigger 노드 스키마 등록
  components/property/
    TriggerScheduleEditor.tsx             # 스케줄 전용 에디터 (신규)
    FormField.tsx                         # trigger_schedules 타입 분기
    DynamicForm.tsx                       # advanced 필드 섹션 분리
    PropertyPanel.tsx                     # 단일 페이로드 에디터 로드/저장 (v1.3.0: payload_mode 가상 필드 폐기)
  types/
    node.ts                               # ConfigField.type 확장
```

#### 스키마 정의

`trigger` 노드 스키마 (nodeSchemas.ts):

| 필드 | 타입 | 설명 |
|------|------|------|
| `schedules` | `trigger_schedules` (신규) | 필수. 다중 스케줄 편집 전용 위젯 |
| `payload` | `object` (JSON 오브젝트 에디터) | 선택. 단일 페이로드 에디터. `$.<var>` 변수/`$$` 이스케이프 인라인 힌트 노출 (v1.3.0). 미설정 시 기본 페이로드. |
| `source_ch_size` | `number` | **고급 설정** (기본 64, 접힘 섹션) |

> **v1.3.0 변경**: `payload_mode`(static/template) 가상 필드와 `payload_template` 별도 필드가 **제거**되었다. 페이로드는 이제 단일 JSON 오브젝트 에디터 하나로 편집하며, 백엔드는 이 값을 통합 템플릿 엔진(`$.<var>` 치환 + `$$` 이스케이프)으로 평가한다. 하위호환을 위해 백엔드는 여전히 `payload_template` 형식도 수용하나, UI는 단일 `payload` 에디터만 노출한다.

#### TriggerScheduleEditor 컴포넌트

스케줄 타입별 전용 위젯:

| 타입 | 위젯 | 프리셋 |
|------|------|--------|
| `interval` | duration 문자열 + 칩 | 1s/5s/30s/1m/5m/15m/1h |
| `cron` | cron 표현식 + 칩 | 매분/5분마다/매시/매일 자정/매일 9시/평일 9시 |
| `once` | `<input type="datetime-local">` | 로컬 시간 선택 → RFC3339 변환 |
| `times` | `<input type="time">` + 칩 목록 | HH:MM 정렬 추가/삭제 |
| `weekly` (v1.2.0) | 요일 토글 버튼 7개(일~토) + `<input type="time">` 칩 목록 | 평일/주말/매일 요일 프리셋 |
| `monthly` (v1.2.0) | 일자 선택(1~31 드롭다운 \| `first` \| `last`) + `<input type="time">` 칩 목록 | 1일/15일/말일 프리셋 |

실시간 인라인 검증:
- `interval`: duration 정규식 `^\d+(ns|us|µs|ms|s|m|h)$`
- `cron`: 5/6 필드 개수 체크
- `once`: RFC3339 파싱 + 미래 시각 확인
- `times`: `HH:MM` 정규식 + 중복 제거
- `weekly` (v1.2.0): 요일 최소 1개 선택 + 시각 최소 1개(`HH:MM` 정규식) + 요일/시각 중복 제거
- `monthly` (v1.2.0): 일자 값(1~31 \| `first` \| `last`) 유효성 + 시각 최소 1개(`HH:MM`); `day`가 29~31 정수일 때 "해당 일이 없는 달에는 발화하지 않음, 월말은 `last` 사용" 안내 힌트 노출

스케줄별 페이로드 편집 (v1.2.0, v1.3.0 통합 에디터):
- 각 스케줄 항목에 접힘 상태의 "이 스케줄 전용 페이로드(선택)" 서브 섹션 제공.
- v1.3.0: 항목별 `payload_mode`(none/static/template) 서브 토글을 **제거**하고, 노드 레벨과 동일한 **단일 JSON 페이로드 에디터**(`$.<var>`/`$$` 힌트 포함)로 통일한다. 페이로드 유무(설정됨/없음)의 옵션 상태만 유지한다.
- 미입력 시 노드 레벨 페이로드(또는 기본 페이로드)로 폴백함을 안내.

저장 형식은 백엔드와 호환되는 배열 (weekly/monthly는 `days`/`day`/`times` 필드, 스케줄별 `payload` 선택):
```json
[
  {"type": "interval", "value": "5s"},
  {"type": "cron",     "value": "0 */5 * * * *"},
  {"type": "once",     "value": "2026-04-15T10:00:00Z"},
  {"type": "times",    "value": ["09:00", "12:00"]},
  {"type": "weekly",   "days": ["mon", "wed", "fri"], "times": ["09:00", "18:00"]},
  {"type": "monthly",  "day": 15, "times": ["08:30"]},
  {"type": "monthly",  "day": "last", "times": ["23:59"], "payload": {"report": "month-end"}}
]
```

#### payload_mode 가상 필드 처리 (v1.3.0 폐기)

**v1.3.0에서 `payload_mode`(none/static/template) 가상 필드는 제거되었다.** 페이로드 모델이 단일 통합 템플릿 엔진으로 통합되어 static/template 구분이 사라졌기 때문이다. UI는 이제 단일 JSON 페이로드 에디터 하나만 노출한다.

**로드 시** (PropertyPanel useEffect):
- `payload`(map) 또는 하위호환 `payload_template`(map)가 있으면 → 단일 페이로드 에디터에 로드 (두 형식 모두 동일 에디터로 병합 표시).
- 둘 다 없으면 → 페이로드 미설정 상태.

**저장 시** (handleApply):
- 페이로드 에디터에 값이 있으면 → `payload`(map)로 저장. (더 이상 `payload_mode` 유도/정리 및 `payload_template` 삭제 분기가 필요 없다.)
- 페이로드 에디터가 비어있으면 → `delete payload; delete payload_template` (기본 페이로드로 폴백).

**인라인 힌트**: 에디터에 `$.trigger_time`, `$.tick_count`, `$.schedule_id`, `$.trigger_id` 변수 목록과 `$$` 리터럴-달러 이스케이프 설명을 상시 노출한다.

#### advanced 섹션 인프라

`ConfigField` 에 `advanced?: boolean` 필드를 추가하고, `DynamicForm` 이 이를 분리하여
기본 접힘 상태의 "고급 설정" collapsible 섹션으로 렌더링한다. trigger 뿐 아니라
다른 노드(향후 deduplicate, filter 등)에서도 재사용 가능한 공통 인프라이다.

---

## 5. Traceability (추적성)

| 요구사항 ID | 모듈 | 파일 | 우선순위 |
|------------|------|------|---------|
| REQ-NODE-004-01-01 ~ 01-04 | TriggerNode Core | internal/node/trigger.go | P0 |
| REQ-NODE-004-02-01 ~ 02-08 | Schedule Configuration | internal/node/trigger.go | P0 |
| REQ-NODE-004-02-09 (v1.2.0) | weekly 스케줄 타입 | internal/node/trigger.go | P0 |
| REQ-NODE-004-02-10 (v1.2.0) | monthly 스케줄 타입 (date/first) | internal/node/trigger.go | P0 |
| REQ-NODE-004-02-11 (v1.2.0) | monthly last-day emit 게이트 | internal/node/trigger.go | P0 |
| REQ-NODE-004-03-01 (v1.3.0) | 통합 페이로드 - 문자열 값 평가 (통합 템플릿 엔진) | internal/node/trigger.go | P0 |
| REQ-NODE-004-03-02 | 기본 페이로드 | internal/node/trigger.go | P0 |
| REQ-NODE-004-03-03 (v1.3.0) | 비문자열 패스스루 + config 하위호환 라우팅 | internal/node/trigger.go | P0 |
| REQ-NODE-004-03-04 (v1.2.0, v1.3.0 통합 엔진) | 스케줄별 페이로드 (per-schedule payload) | internal/node/trigger.go | P0 |
| REQ-NODE-004-04-01 | Message Metadata | internal/node/trigger.go | P0 |
| REQ-NODE-004-05-01 ~ 05-04 | Lifecycle Integration | internal/node/trigger.go | P0 |
| REQ-NODE-004-06-01 ~ 06-03 | Error Handling | internal/node/trigger.go | P0 |
| REQ-NODE-004-06-04 (v1.3.0) | 통합 엔진 평가 실패 - 미지 변수 에러+null | internal/node/trigger.go | P0 |
| REQ-NODE-004-07-01 (v1.1.0) | Timer Agent 주입 (WithTimer NodeOption) | internal/node/trigger.go, cmd/xflowd/main.go | P0 |
| REQ-NODE-004-08-01 (v1.1.0) | Web UI - 전용 스케줄 에디터 | web/src/components/property/TriggerScheduleEditor.tsx | P1 |
| REQ-NODE-004-08-02 (v1.1.0) | Web UI - 스키마 등록 | web/src/config/nodeSchemas.ts | P1 |
| REQ-NODE-004-08-03 (v1.1.0) | Web UI - advanced 섹션 인프라 | web/src/components/property/DynamicForm.tsx | P1 |
| REQ-NODE-004-08-04 (v1.1.0, v1.3.0 폐기) | Web UI - payload_mode 가상 필드 (v1.3.0 제거, 단일 페이로드 에디터로 대체) | web/src/components/property/PropertyPanel.tsx | P1 |
| REQ-NODE-004-08-05 (v1.2.0, v1.3.0 단일 에디터) | Web UI - weekly/monthly 위젯 + 단일 페이로드 에디터(`$.<var>`/`$$` 힌트) | web/src/components/property/TriggerScheduleEditor.tsx, web/src/config/nodeSchemas.ts, web/src/components/property/PropertyPanel.tsx | P1 |
