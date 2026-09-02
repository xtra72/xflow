# SPEC-TRIGGER-SCHED-001 — 기술 설계 (design.md)

> Tier L 설계 문서. 아키텍처·데이터 모델·인터페이스 계약·발화 게이팅 상세.
> 버전: 0.3.0 (spec.md 동기) — 구현 완료 + 3-phase close. 설계→구현 실현 매핑은 spec.md §8 IN-1~IN-7 참조: §2 데이터 모델 `Enabled *bool`(IN-1), §3 `withinValidity` RFC3339 수용·잘못된 날짜 경계 무제한(IN-2), §4.1 TARGET 열거 `agentId` 기반·이름 폴백(IN-4), §4.2 ACTION `{power,fanSpeed}` null 인코딩·`mode` 무방출(IN-5), §5 dual-write 단일 `persist()` 재사용(IN-7). 구현 커밋: 백엔드 `03e8827b`, 프런트 `fb40c4b2`.
> 이전 버전: 0.2.0 — OQ-1~6 확정(RD-4~9) 반영: RD-4(ACTION 2축·모드 축 없음), RD-5(`facility-schedule` 패널 공존), RD-6("전체"=line 셀렉터), RD-7(priority 표시/정렬 전용), RD-8(유효기간 서버 로컬·양끝 inclusive), RD-9(dual-write 지속).

## 1. 컴포넌트 개요

```
[대시보드]
  └─ facility-schedule 패널 (신규)
       ├─ 노드 피커  ── useNodeTypeInstances('trigger')  (재사용)
       ├─ 요약 테이블 (SCHEDULE/TARGET/PLAN/ACTION/PRIO/STATE, 읽기 전용)
       │     └─ 행별 EDIT 버튼 + STATE 토글
       ├─ 규칙 모달 (생성/편집)
       │     ├─ TARGET 피커  → 셀렉터 payload 조립
       │     └─ ACTION 편집기 → 제어 명령 payload 조립
       └─ dual-write ── nodeService.configureNode(live)
                        flowService.getFlow→patch→updateFlow(persist)   (재사용)
                             │
                             ▼ (flow 정의 지속 + live 재무장)
[백엔드 TriggerNode] internal/node/trigger.go
   ├─ TriggerSchedule (+name/valid_from/valid_to/priority/enabled)
   ├─ parseScheduleConfig (신규 5키 파싱 + 하위호환 기본값)
   ├─ newEntry → triggerTimerEntry (게이팅 값 캡처)
   └─ makeHandler (발화 게이트: gen → paused → [enabled] → [유효기간] → lastDayGate → emit)
        │ payload = 제어 명령 (per-schedule payload 경로)
        ▼
[downstream 노드] → xsfm 제어 노드/mqtt → xsfm 에이전트
   selector: device_id > station > line > group_id (+ device_name/group_name)
   command : set_power / set_fan_speed / set_multiple (params: power bool, fan_speed 1~3)
```

## 2. 데이터 모델 — TriggerSchedule 확장

```go
// internal/node/trigger.go — TriggerSchedule 에 추가 (제안)
type TriggerSchedule struct {
    // ... 기존 필드 (Type/Value/Days/Times/Day/Payload/PayloadTmpl) ...

    // SPEC-TRIGGER-SCHED-001: 설비 제어 예약 메타
    Name      string // 규칙 표시 이름 (config: "name")
    ValidFrom string // "YYYY-MM-DD" 또는 "" (하한 무제한) (config: "valid_from")
    ValidTo   string // "YYYY-MM-DD" 또는 "" (무기한)       (config: "valid_to")
    Priority  int    // 표시/정렬 우선순위                    (config: "priority")
    Enabled   *bool  // 활성 여부. nil/부재 = true (하위호환) (config: "enabled")
}
```

`Enabled` 를 포인터로 두는 이유: config 에서 `enabled` 키가 부재할 때 "true(기본 활성)"로 하위 호환 해석하기 위함. 값이 명시된 경우에만 역참조.

`triggerTimerEntry`(발화 클로저가 참조)에 게이팅 스칼라 캡처:

```go
type triggerTimerEntry struct {
    // ... 기존 (timerID/scheduleType/payload/payloadTmpl/lastDayGate/gen) ...
    enabled   bool   // newEntry 시점 캡처 (nil → true)
    validFrom string // "" = 하한 무제한
    validTo   string // "" = 무기한
}
```

priority/name 은 발화에 불필요 → entry 에 캡처하지 않고, 필요 시 방출 메시지 메타(buildMessage)로만 전달.

## 3. 발화 게이팅 상세 (makeHandler)

기존 게이트 순서(trigger.go:714-738)에 2개 게이트를 삽입한다:

```
handler(trigger):
    if entry.gen != rearmGen         → return   // (기존) stale drop
    if paused                        → return   // (기존)
    if !entry.enabled                → return   // (신규) REQ-SCHED-01-03
    if !withinValidity(now, entry)   → return   // (신규) REQ-SCHED-01-04
    if entry.lastDayGate && !isLastDay(now) → return  // (기존) monthly last
    msg = buildMessage(...)          // (기존) payload = 제어 명령
    sourceCh <- msg                  // (기존) non-blocking
```

`withinValidity` 판정(RD-8 확정):

```
withinValidity(now, entry):
    d := now.In(serverLocal).Date()          // 서버 로컬·날짜 단위
    if entry.validFrom != "" && d < parse(validFrom) → false   // 하한(inclusive)
    if entry.validTo   != "" && d > parse(validTo)   → false   // 상한(inclusive)
    return true
```

- **서버 로컬 시간** 기준, 빈 `validFrom`=하한 무제한 / 빈 `validTo`=무기한(상한 무제한).
- **양끝 inclusive**(날짜 단위). 시각 파싱 실패는 방어적으로 "게이트 통과"가 아니라 "미발화 + 경고 로그" 처리(잘못된 config 로 인한 오발화 방지).
- 최종 발화 조건: `entry.enabled` **AND** `withinValidity(now, entry)`.
- **동시성**: `now()` 는 선행 `nowFunc` 훅 재사용(테스트 주입 가능). 게이팅 값은 `newEntry` 시점 캡처된 immutable 스칼라이므로 발화 클로저에서 노드 lock 을 잡지 않는다 → 재귀 RLock deadlock 회피(프로젝트 기지식 준수).

## 4. payload 계약 (TARGET / ACTION → 제어 명령)

패널이 조립하는 규칙 payload 는 xsfm exec 계약과 정합한다.

### 4.1 TARGET → 셀렉터 키

| UI 선택 | 셀렉터 키 | 비고 |
| -- | -- | -- |
| 개별 | `device_id` 또는 `device_name` | 이름 셀렉터 승격(NAMESEL) |
| 그룹 | `group_id` 또는 `group_name` | 커스텀 그룹/이름 |
| 전체 | `line` | **RD-6**: 호선 전체(예 `line: "2"` = "2호선 전체"). 전역 "all" 셀렉터 미도입 |

셀렉터 우선순위(agent.go:874): `device_id > station > line > group_id`. 피커는 단일 종류만 기록해 모호성 방지.

### 4.2 ACTION → 제어 명령

| UI 축 | command | params |
| -- | -- | -- |
| 전원만 | `set_power` | `{ power: bool }` |
| 풍량만 | `set_fan_speed` | `{ fan_speed: 1..3 }` |
| 전원+풍량 | `set_multiple` | `{ power: bool, fan_speed: 1..3 }` |

- `fan_speed` 범위 1~3(control.go: `ErrInvalidFanSpeed`). 편집기는 범위 밖 조립 차단.
- `set_multiple` 은 백엔드에서 power ON 을 fan 보다 먼저 방출(control.go:338) — 패널은 축만 지정, 순서는 에이전트 보장.
- **모드 축 없음(RD-4)**: `mode`(Auto/Sleep) 는 xsfm 제어 모델에 없음. v1 payload 에 mode 키를 넣지 않으며 ACTION 은 전원/풍량 2축으로 한정한다. 모드 도입은 별도 xsfm 제어 모델 SPEC 으로 유보.

### 4.3 예시

```json
{ "command": "set_power",    "device_id": "GN-HALL:P1:2", "params": { "power": true } }
{ "command": "set_multiple", "group_id": "hall-group",    "params": { "power": true, "fan_speed": 1 } }
{ "command": "set_power",    "line": "2",                 "params": { "power": false } }
```

## 5. 프런트 인터페이스 계약

- **패널 타입 등록(RD-5)**: `uiStore` PanelType 열거에 신규 `facility-schedule` 추가(범용 `trigger-config` 과 공존, 대체 아님). `renderDashboardPanel` 분기 + `AddPanelDialog` 항목.
- **노드 피커**: `useNodeTypeInstances('trigger')` 재사용 — running/stopped 배지 포함.
- **규칙 모델(프런트)**:

```ts
interface FacilityRule {
  name: string;
  validFrom: string;   // "YYYY-MM-DD" | ""
  validTo: string;     // "YYYY-MM-DD" | ""
  priority: number;
  enabled: boolean;
  schedule: TriggerScheduleConfig;   // PLAN (선행 스케줄 타입 재사용)
  target: TargetSelector;            // { kind: 'all'|'group'|'device', key, value }
  action: ControlCommand;            // { command, params }
}
```

- **dual-write**: 선행 `triggerPanelUtils.ts` 의 configureNode + getFlow→patch→updateFlow 경로 재사용. 규칙 집합 ↔ config 스케줄 배열 직렬화/역직렬화 유틸 추가.

## 6. 테스트 설계 개요

| 레이어 | 대상 | 도구 |
| -- | -- | -- |
| 백엔드 | 파싱 하위호환, enabled/유효기간 게이트, 경계값, `-race` | `go test`, `nowFunc` 훅 |
| 프런트 | 테이블 6컬럼, 모달 왕복, 토글, 피커/편집기 조립, dual-write mock | vitest |
| 회귀 | 선행 `trigger-config` + trigger 노드 | 기존 스위트 |

## 7. 확정 결정 반영 (RD 요약 — 구 OQ)

- **RD-5**(OQ-1): 신규 `facility-schedule` 패널을 범용 `trigger-config` 과 공존 등록.
- **RD-8**(OQ-2): `withinValidity` = 서버 로컬·날짜 단위 양끝 inclusive·빈=경계 무제한, 발화는 enabled AND 유효기간 논리곱.
- **RD-7**(OQ-3): priority 는 v1 표시/정렬 전용(런타임 충돌 해소 유보).
- **RD-6**(OQ-4): TARGET "전체" = `line` 셀렉터(호선 전체), 전역 "all" 미도입.
- **RD-4**(OQ-5): ACTION 2축(power/fan_speed) 한정, 모드 축 없음(별도 SPEC 유보).
- **RD-9**(OQ-6): 확장 필드는 dual-write 로 노드 config 에 지속.
