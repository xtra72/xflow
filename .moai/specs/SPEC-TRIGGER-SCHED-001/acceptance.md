# SPEC-TRIGGER-SCHED-001 — 인수 기준 (acceptance.md)

> 형식: Given-When-Then. 각 시나리오는 대응 REQ 를 명시한다.
> 버전: 0.2.0 (spec.md 동기) — OQ-1~6 확정(RD-4~9) 반영. AC-2/AC-3(RD-8 서버 로컬·양끝 inclusive), AC-4(RD-7 PRIO 정렬), AC-9(RD-6 전체=line), AC-10(RD-4 2축·모드 없음), AC-11(RD-9 확장 필드 지속) 구체화. AC-13(RD-5 패널 공존) 신규.

## AC-1 — 비활성 규칙은 발화하지 않는다 (REQ-SCHED-01-03, RD-8)

- **Given** `enabled=false` 인 규칙과 그 스케줄 타이머가 등록된 trigger 노드 (유효기간이 현재를 포함하더라도)
- **When** 해당 스케줄의 타이머가 발화한다
- **Then** downstream(`sourceCh`)으로 메시지가 방출되지 않는다(발화 조건 `enabled AND 유효기간` 의 논리곱 미충족)
- **And** 동일 노드의 다른 활성 규칙 발화에는 영향이 없다

## AC-2 — 규칙은 유효기간 내에서만 발화한다 (REQ-SCHED-01-04, RD-8)

- **Given** `valid_from=2026-07-20`, `valid_to=2026-07-22` 인 활성 규칙 (유효기간 판정은 **서버 로컬 시간** 기준)
- **When** 서버 로컬 현재 시각이 2026-07-19(이전)일 때 타이머가 발화한다
- **Then** 메시지가 방출되지 않는다
- **When** 서버 로컬 현재 시각이 2026-07-21(범위 내)일 때 타이머가 발화한다
- **Then** 메시지가 방출된다
- **When** 서버 로컬 현재 시각이 2026-07-23(이후)일 때 타이머가 발화한다
- **Then** 메시지가 방출되지 않는다
- **And** `valid_to` 가 빈 값(무기한)이면 상한 없이 항상 발화한다(활성·하한 충족 시)
- **And** `valid_from` 이 빈 값(하한 무제한)이면 하한 없이 상한 이내에서 발화한다

## AC-3 — 유효기간 경계값 (양끝 inclusive) (REQ-SCHED-01-04, RD-8)

- **Given** `valid_from=2026-07-20`, `valid_to=2026-07-22` 인 활성 규칙 (**서버 로컬·날짜 단위 양끝 inclusive**)
- **When** 서버 로컬 현재 시각이 2026-07-20 00:00(하한 경계 당일) 및 2026-07-22 23:59(상한 경계 당일)일 때 각각 발화한다
- **Then** 두 경계 당일 모두 메시지가 방출된다
- **And** 2026-07-19 23:59 및 2026-07-23 00:00 에는 방출되지 않는다(경계 밖)

## AC-4 — 요약 테이블 6컬럼 렌더 + PRIO 정렬 (REQ-SCHED-02-01~07, RD-7)

- **Given** 이름/유효기간/TARGET/PLAN/ACTION/priority/enabled 가 지정된 규칙 1건
- **When** 패널이 렌더된다
- **Then** SCHEDULE(이름 + "valid_from ~ valid_to"/무기한), TARGET(전체/그룹/개별 배지 + 설명), PLAN(스케줄 요약), ACTION(제어 명령 요약), PRIO(정수), STATE(활성/비활성 배지) 6컬럼이 모두 표시된다
- **Given** priority 가 각각 `2`, `0`, `1` 인 규칙 3건
- **When** 패널이 렌더된다
- **Then** 행은 priority 오름차순(`0`, `1`, `2`)으로 정렬되고 동률은 안정 정렬로 원래 순서를 유지한다
- **And** priority 는 표시/정렬 전용이며 발화 순서·충돌 해소에는 영향을 주지 않는다

## AC-5 — 모달 생성 왕복 (REQ-SCHED-03-01/03/04, 06)

- **Given** 빈 규칙 목록의 패널
- **When** "+" 를 클릭해 모달을 열고 필수 값(이름/PLAN/TARGET/ACTION)을 입력 후 저장한다
- **Then** 규칙이 테이블에 나타나고 dual-write(configureNode + updateFlow)가 호출된다
- **And** 모달이 닫힌다

## AC-6 — 모달 편집 왕복 (REQ-SCHED-03-02/04)

- **Given** 규칙 1건이 있는 테이블
- **When** 해당 행의 EDIT 버튼을 클릭한다
- **Then** 기존 값이 채워진 편집 모달이 열린다
- **When** 값을 수정 후 저장한다
- **Then** 테이블이 갱신되고 dual-write 가 호출된다

## AC-7 — STATE 토글 활성/비활성 (REQ-SCHED-02-07, 03-06)

- **Given** `enabled=true` 규칙
- **When** STATE 배지를 토글한다
- **Then** 규칙의 `enabled` 가 false 로 반전되고 dual-write persist 가 호출된다
- **And** 배지가 비활성 표시로 갱신된다

## AC-8 — 필수 값 누락 저장 거부 (REQ-SCHED-03-05)

- **Given** 이름 또는 PLAN 또는 TARGET 또는 ACTION 이 비어 있는 모달
- **When** 저장을 시도한다
- **Then** 저장이 거부되고 모달 내 검증 오류가 표시된다

## AC-9 — TARGET 피커가 올바른 셀렉터를 조립한다 (REQ-SCHED-04-02~05, RD-6)

- **Given** TARGET 피커
- **When** "개별" + 특정 디바이스를 선택한다
- **Then** payload 에 `device_id`(또는 `device_name`) 셀렉터가 기록된다
- **When** "그룹" 을 선택한다
- **Then** `group_id`(또는 `group_name`) 셀렉터가 기록된다
- **When** "전체" + 특정 호선(예 2호선)을 선택한다
- **Then** payload 에 `line` 셀렉터가 기록된다(예 `{ "line": "2" }`, "2호선 전체")
- **And** 전역 "all" 셀렉터 키는 어떤 경우에도 조립되지 않는다(xsfm 에 부재)

## AC-10 — ACTION 편집기가 2축 제어 명령만 조립한다 (REQ-SCHED-05-01~06, RD-4)

- **Given** ACTION 편집기 (전원·풍량 2축만 제공, 모드 축 없음)
- **When** 전원만 지정한다 (예 전원 ON)
- **Then** `set_power` + `params.power`(bool) 가 조립된다
- **When** 풍량만 지정한다 (예 풍량 1)
- **Then** `set_fan_speed` + `params.fan_speed`(int 1~3) 가 조립된다
- **When** 전원 + 풍량을 함께 지정한다 (예 전원 ON · 풍량 2)
- **Then** `set_multiple` + `params.{power, fan_speed}` 가 조립된다
- **And** `fan_speed` 1~3 범위 밖 값은 조립되지 않는다
- **And** payload params 에 `mode`(Auto/Sleep 등) 키는 어떤 경우에도 포함되지 않으며, 편집기 UI 에도 모드 선택 컨트롤이 없다
- **And** ACTION 요약 라벨은 모드를 표기하지 않는다("전원 ON", "전원 OFF", "풍량 1", "전원 ON · 풍량 2" — Auto/Sleep 아님)

## AC-11 — Dual-Write persist(확장 필드) + 404 폴백 (REQ-SCHED-06-01/02/04, RD-9)

- **Given** 규칙 생성/편집/토글 동작
- **When** 대상 노드가 기동 중이다
- **Then** configureNode(live) + getFlow→patch→updateFlow(persist) 가 모두 호출된다
- **When** configureNode 가 404(노드 미기동)를 반환한다
- **Then** persist-only 로 폴백하고 사용자에게 통지한다
- **And** 확장 필드(`name`/`valid_from`/`valid_to`/`priority`/`enabled`) 및 TARGET/ACTION payload 가 trigger 스케줄 config 에 저장되어 재배포/재기동 후에도 config 에서 그대로 복원된다(별도 저장소 없음)

## AC-12 — 무회귀 (REQ-SCHED-07-01/02)

- **Given** 기존 범용 `trigger-config` 패널(SPEC-TRIGGER-PANEL-001) 및 trigger 노드
- **When** 본 SPEC 변경이 반영된다
- **Then** 범용 패널의 스케줄 CRUD/카탈로그/dual-write 동작이 회귀하지 않는다
- **And** trigger 노드의 6종 스케줄 타입 + per-schedule payload 해결 순서가 보존된다
- **And** `go test ./...` 및 vitest 전량 통과, `-race` 클린, 커버리지 85%+

## AC-13 — 신규 패널이 범용 패널과 공존한다 (REQ-SCHED-07-01, RD-5)

- **Given** 대시보드 패널 추가 다이얼로그
- **When** 사용자가 패널 타입을 선택한다
- **Then** 신규 `facility-schedule` 패널 타입과 기존 `trigger-config` 패널 타입이 **둘 다** 선택지로 제공된다
- **And** 한 대시보드에 두 패널 타입을 동시에 추가·사용할 수 있다(신규 패널이 범용 패널을 대체하지 않는다)

## Definition of Done

- [ ] AC-1 ~ AC-13 전부 통과
- [ ] `go test ./internal/node/...` + 전체 `go test ./...` exit 0
- [ ] vitest 전량 통과 (신규 테이블/모달/피커/편집기 + 기존)
- [ ] `-race` 클린, 커버리지 85% 이상
- [ ] 범용 `trigger-config` 패널 무회귀 + 신규 패널 공존 확인
- [ ] RD-4~9 확정 반영 (Open Questions 잔여 없음)
