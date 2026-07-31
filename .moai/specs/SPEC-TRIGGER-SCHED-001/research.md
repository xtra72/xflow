# SPEC-TRIGGER-SCHED-001 — 정찰/연구 (research.md)

> Tier L 연구 문서. 코드베이스 정찰 결과, 재사용 자산, 제약, 모드 축 코드 점검.
> 버전: 0.3.0 (spec.md 동기) — 구현 완료 + 3-phase close. 정찰 기준선(trigger.go 발화 경로·xsfm 2축 제어·선행 재사용 자산)이 구현으로 검증됨: 발화 게이트는 generation-token 뒤 삽입(§1.3 확정), ACTION 은 2축 유지·`mode` 무방출(§2.3 → spec.md §8 IN-5), dual-write 는 선행 자산 재사용(§3 → IN-7). 구현 커밋: 백엔드 `03e8827b`, 프런트 `fb40c4b2`.
> 이전 버전: 0.2.0 — OQ-1~6 확정(RD-4~9) 반영. 모드 축 코드 점검 결론은 RD-4(v1 2축 한정, 모드 도입 유보)로 확정.

## 1. Trigger 노드 (internal/node/trigger.go)

### 1.1 현행 스케줄 모델 (67-83)

`TriggerSchedule` 현재 필드: `Type`, `Value`(any), `Days`([]string, weekly), `Times`([]string), `Day`(any, monthly), `Payload`(any), `PayloadTmpl`(map). → **name/유효기간/priority/enabled 부재** — 본 SPEC 이 확장.

스케줄 타입 6종(47-65): `interval`/`cron`/`once`/`times`/`weekly`/`monthly`.

### 1.2 등록 경로 (390-467)

- `registerSchedules`(390) → `registerSingleSchedule`(400, switch by type) → `register{Interval,Cron,Once,Times,Weekly,Monthly}`.
- `newEntry`(424): 스케줄 → `triggerTimerEntry` 생성, 등록 시점 `rearmGen` 캡처(432). ← 본 SPEC 은 여기서 enabled/validFrom/validTo 추가 캡처.
- `addEntry`(436): `timerMu` 잠금 하 append.
- 타이머 ID 는 노드 UUID 기반(454-458) — flow 간 충돌 방지.

### 1.3 발화 경로 (712-759) — 게이팅 삽입 지점

`makeHandler` 클로저 게이트 순서:
1. `entry.gen != n.rearmGen.Load()` → return (719, stale in-flight drop, SPEC-TRIGGER-PANEL-001 RD-6)
2. `n.paused`(pauseMu RLock) → return (724-729)
3. `entry.lastDayGate` && not last day → return (733-738, monthly last)
4. `buildMessage`(744) → `sourceCh <- msg`(747, non-blocking, 버퍼 풀 시 drop+warn)

→ **본 SPEC 의 enabled/유효기간 게이트는 2와 3 사이**(또는 3 직전)에 삽입. 게이팅 스칼라는 entry 사전 캡처 → 클로저에서 노드 lock 미획득(재귀 RLock deadlock 회피, 프로젝트 기지식 `project_hvac_agent_lock_pattern`).

### 1.4 payload 해결 (761-)

`buildMessage`: (1) 스케줄 payload_template → payload → (2) 노드 레벨 → (3) 기본 `{trigger_time}`. per-schedule payload(`Payload`/`PayloadTmpl`)가 규칙별 제어 명령을 나르는 경로.

### 1.5 live 재무장 (SPEC-TRIGGER-PANEL-001 M1)

`Configure` 가 started-gate + StateRunning 조건에서 `rearmGen` 증가 → cancel+re-register. `payloadMu` race fix. → 본 SPEC 의 규칙 메타 live 변경도 동일 규약으로 무결.

## 2. xsfm 제어 모델 (internal/agent/xsfm/)

### 2.1 셀렉터 (agent.go)

- 라우팅 우선순위(874): `device_id > station > line > group_id`.
- 필드(agent.go:113-123): `GroupID`, `Station`, `Line`, `DeviceID`, `DeviceName`, `GroupName`.
- 이름 셀렉터 승격(166-172, SPEC-XSFM-NAMESEL-001): params 의 `device_name`/`group_name`/`station`/`line` 을 top-level 로 승격.
- **명시적 "all"/"전체" 셀렉터는 부재** → **RD-6 확정**: "전체" 는 `line` 셀렉터(호선 전체, 예 `line: "2"`)로 매핑. 전역 "all" 미도입.

### 2.2 제어 명령 (control.go) — 2축만

- `handleSetPower`(272): `params.power`(bool).
- `handleSetFanSpeed`(285): `params.fan_speed`(int), 범위 밖 `ErrInvalidFanSpeed`, 전원 OFF 시 정책(기본 `ErrPowerOff` 또는 ON 선방출).
- `handleSetMultiple`(322): `params.{power, fan_speed}`, power ON 을 fan 보다 먼저(338).
- `parseSetMultipleParams`(645): power/fan_speed 파싱, 둘 다 없으면 오류.

### 2.3 모드 축 코드 점검 결론 (OQ-5 근거)

- `control.go:269` 주석: **"Module 3 — 개별 제어 (2-축: power on/off, fan_speed 1/2/3)"**.
- `provider.go:24`: **"Every facility is controllable (2-axis: set_power + set_fan_speed)"**.
- grep(`"mode"`, `set_multiple`, params) 결과: 제어 params 축은 `power`/`fan_speed` **뿐**. `mode`(Auto/Sleep) 키/핸들러/인코딩 **부재**.
- **결론(RD-4 확정)**: 이미지의 "전원 ON · Auto" / "모드 Sleep · 풍량 1" 이 시사하는 모드 축(Auto/Sleep)은 **현재 xsfm 제어 모델에 존재하지 않는다.** **v1 ACTION 은 전원/풍량 2축으로 한정**하고, 모드 도입은 xsfm 제어 모델 변경을 요하는 **별도 SPEC 으로 유보(deferred)**. ACTION 라벨에서 Auto/Sleep 표기는 2축 현실(전원 ON/OFF, 풍량 N)로 대체한다.

## 3. 재사용 자산 (SPEC-TRIGGER-PANEL-001, 완료)

- `trigger-config` 패널 + `TriggerConfigPanel.tsx`: 범용 스케줄 CRUD + payload 카탈로그.
- `useNodeTypeInstances('trigger')`: 노드 피커 + running/stopped 배지.
- dual-write: `nodeService.configureNode`(live) + `flowService.getFlow→patch→updateFlow`(persist), 404→persist-only, last-write-wins 통지(RD-8).
- `triggerPanelUtils.ts`: 스케줄 ↔ config 직렬화 유틸.
- **재사용 판단(단순성 사다리)**: 노드 피커·dual-write·payload 경로 전부 재사용 → 신규 API/저장소 불필요. 본 SPEC 은 (a) 백엔드 필드 5개 + 게이트 2개, (b) 프런트 특화 패널(테이블/모달/TARGET·ACTION 편집기)만 신규.

## 4. trigger → xsfm 배선 가정

trigger 노드는 payload 를 downstream 으로 방출하고, 하위 노드(xsfm 제어 노드 또는 mqtt)를 통해 xsfm 에이전트로 전달된다. 배선(어떤 노드가 하위인지)은 flow 작성자 책임이며, 본 패널은 payload 형태(제어 명령)만 보장한다. (SPEC 가정 A-6)

## 5. 제약·교훈 반영

- **lock 재귀 RLock 금지**(`project_hvac_agent_lock_pattern`): 발화 게이트 값은 entry 사전 캡처, 클로저에서 노드 lock-holding 중 노드 메서드 호출 금지.
- **timestamp 규약**(`project_timestamp_convention`): 메시지 payload 타임스탬프는 epoch ms(int64). 유효기간은 UI 표시용 `YYYY-MM-DD` 문자열(발화 게이트는 날짜 단위 비교) — payload 타임스탬프와 별개.
- **device_id 키잉**(`project_device_id_agent_keying`): TARGET 개별 셀렉터의 device_id 는 에이전트 컨텍스트에서 고유해야 함 — 피커가 유효 device_id 를 제시.

## 6. 확정 결정 (RD — 구 OQ, 전부 해소됨)

| RD (구 OQ) | 확정 내용 | 코드 근거 |
| -- | -- | -- |
| RD-5 (OQ-1) | 범용 패널 대체 아님 → 신규 `facility-schedule` 패널 공존 | uiStore PanelType 추가 |
| RD-8 (OQ-2) | 유효기간: 서버 로컬·날짜 단위 양끝 inclusive, 빈=경계 무제한, 발화=enabled AND 범위 | makeHandler 게이트 |
| RD-7 (OQ-3) | priority 는 v1 표시/정렬 전용(런타임 충돌 해소 유보) | 테이블 정렬 메타 |
| RD-6 (OQ-4) | "전체" = `line` 셀렉터(호선 전체). 전역 "all" 미도입 | agent.go:874 셀렉터에 all 부재 |
| RD-4 (OQ-5) | ACTION 2축(power/fan_speed) 한정, 모드 축 없음 → 별도 SPEC 유보 | control.go:269, provider.go:24 (2축) |
| RD-9 (OQ-6) | 확장 필드는 노드 config 에 dual-write 지속 | nodeService/flowService 재사용 |
