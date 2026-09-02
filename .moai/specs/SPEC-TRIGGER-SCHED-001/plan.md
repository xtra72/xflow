# SPEC-TRIGGER-SCHED-001 — 구현 계획 (plan.md)

> 대상 SPEC: 설비 제어 예약 패널 (스케줄 규칙 테이블 + 모달 편집)
> Tier: L — 노드 모델 변경 + 특화 패널 + 테이블/모달 + TARGET/ACTION 편집기 (백엔드+프런트 교차)
> 버전: 0.3.0 (spec.md 동기) — 구현 완료 + 3-phase close. 전 마일스톤(M1~M6) 구현·커밋(백엔드 M1~M2 `03e8827b`, 프런트 M3~M6 `fb40c4b2`). as-implemented 정련 7건은 spec.md §8 IN-1~IN-7 참조.
> 이전 버전: 0.2.0 — OQ-1~6 확정(RD-4~9) 반영. RD-4(ACTION 2축·모드 축 없음), RD-5(신규 `facility-schedule` 패널 공존), RD-6("전체"=line), RD-7(priority 표시/정렬 전용), RD-8(유효기간 서버 로컬·양끝 inclusive), RD-9(dual-write 지속).

## 1. 기술 접근 (Technical Approach)

본 SPEC 은 3개 레이어에 걸친다:

1. **백엔드 모델·발화 게이팅** (`internal/node/trigger.go`): `TriggerSchedule` 5개 필드 확장 + `parseScheduleConfig` 파싱 + `triggerTimerEntry`/`newEntry` 캡처 + `makeHandler` 발화 게이트 삽입. 선행 SPEC-TRIGGER-PANEL-001 M1 의 `rearmGen` live 재무장 규약을 재사용해 live 반영을 보장한다.
2. **프런트 특화 패널** (`web/src/pages/dashboard`): 신규 패널 타입(가칭 `facility-schedule`) + 읽기 전용 요약 테이블 + 모달 생성/편집 + 행별 EDIT + STATE 토글. 노드 피커·dual-write 는 선행 패널 자산(`useNodeTypeInstances`, `nodeService`/`flowService`, `triggerPanelUtils.ts`)을 재사용한다.
3. **TARGET/ACTION 조립 유틸**: TARGET 피커(셀렉터 조립) + ACTION 편집기(제어 명령 조립) — xsfm 셀렉터 우선순위 및 2축(power/fan_speed) 제어 명령과 정합.

핵심 설계 제약:
- 발화 게이팅은 lock-holding 함수 내 `a.Name()` 등 재귀 RLock 유발 호출 금지(프로젝트 기지식 HVAC lock 패턴 교훈). `-race` 클린 유지.
- 확장 필드는 하위 호환(부재 시 enabled=true, 무기한, priority=0)으로 기존 스케줄 무회귀.
- 범용 `trigger-config` 패널과 공존(대체 아님, RD-5). 유효기간 게이팅은 서버 로컬·날짜 단위 양끝 inclusive(RD-8).

## 2. 마일스톤 (우선순위 기반, 시간 예측 없음)

### M1 — 백엔드 모델 확장 + 파싱 (Priority High, Primary Goal)

- `TriggerSchedule` 에 `Name`/`ValidFrom`/`ValidTo`/`Priority`/`Enabled` 추가.
- `parseScheduleConfig` 에서 5개 config 키 파싱 + 하위 호환 기본값.
- `triggerTimerEntry` + `newEntry` 에서 게이팅 값(enabled/validFrom/validTo) 캡처.
- 특성화 테스트: 기존 6종 스케줄 파싱 무회귀.

### M2 — 발화 게이팅 (enabled + 유효기간) (Priority High, Primary Goal)

- `makeHandler` 게이트 순서에 enabled/유효기간 게이트 삽입(§4.2 위치).
- `nowFunc` 훅(선행 monthly last 게이트용) 재사용해 유효기간 경계 테스트.
- 테스트: 비활성 미발화 / 유효기간 밖 미발화 / 유효기간 내 정상 발화 / 경계값.

### M3 — 프런트 패널 스캐폴드 + 요약 테이블 (Priority High, Secondary Goal)

- 신규 패널 타입 등록(`uiStore`/`renderDashboardPanel`/`AddPanelDialog`).
- 노드 피커(`useNodeTypeInstances('trigger')`) + running/stopped 배지 재사용.
- 6컬럼 읽기 전용 테이블 + priority 정렬 + 빈 상태.

### M4 — 모달 생성/편집 + STATE 토글 (Priority High, Secondary Goal)

- 규칙 생성/편집 모달 폼(이름/유효기간/PLAN/TARGET/ACTION/priority/enabled).
- 행별 EDIT 버튼 + "+" 신규 + STATE 배지 토글.
- 필수 값 검증(REQ-SCHED-03-05).

### M5 — TARGET 피커 + ACTION 편집기 (Priority Medium, Secondary Goal)

- TARGET 피커: 전체/그룹/개별 → 셀렉터 payload 조립. "전체" = `line` 셀렉터(호선 전체, RD-6).
- ACTION 편집기: set_power/set_fan_speed/set_multiple 조립 + fan_speed 1~3 검증. 전원/풍량 2축만 — 모드 축 없음(RD-4).

### M6 — Dual-Write 통합 + 회귀 검증 (Priority Medium, Final Goal)

- configureNode(live) + updateFlow(persist) dual-write, 404→persist-only 폴백.
- 확장 메타 + payload 지속화 왕복 검증.
- 회귀: 범용 `trigger-config` 패널 + trigger 노드 기존 동작 무회귀.

## 3. 아키텍처 설계 방향

- **의존 방향**: 백엔드(M1→M2)가 프런트(M3→M6)의 계약(config 키/payload 형태)을 확정하므로 백엔드 선행. TARGET/ACTION 조립(M5)의 셀렉터 키·축은 RD-6(전체=line)/RD-4(2축)로 확정됨.
- **재사용 우선(단순성 사다리)**: 노드 피커·dual-write·payload 경로는 선행 SPEC 자산 재사용 — 신규 저장소/API 도입 없음(RD-1, OQ-6).
- **발화 게이팅 최소 변경**: `makeHandler` 에 조건 2개 추가 — 신규 타이머 타입/경로 없음.

## 4. 리스크 및 대응

| 리스크 | 영향 | 대응 |
| -- | -- | -- |
| "전체" 매핑(RD-6 확정) | M5 TARGET 피커 셀렉터 키 | 확정: "전체" = `line` 셀렉터(호선 전체). 전역 "all" 미도입 |
| 모드 축 기대 불일치(RD-4 확정) | 이미지의 Auto/Sleep 재현 불가 | 확정: v1 2축(power/fan_speed) 한정, 모드 도입은 별도 xsfm 제어 모델 SPEC 으로 유보 |
| 유효기간 경계/타임존(RD-8 확정) | 발화 게이트 경계 오작동 | 확정: 서버 로컬·날짜 단위 양끝 inclusive·빈=경계 무제한 을 테스트로 고정 |
| lock 재귀 RLock | deadlock(v0.18.6 트랩) | 게이팅 값은 entry 에 사전 캡처, 발화 클로저에서 노드 lock-holding 중 노드 메서드 호출 회피 |
| 동시 flow 편집 경합 | 규칙 유실 | last-write-wins + 통지(선행 SPEC-TRIGGER-PANEL-001 RD-8) 계승 |

## 5. 검증 전략 (요약)

- 백엔드: `go test ./internal/node/...` (특성화 + 신규 게이팅), `-race` 클린, 커버리지 85%+.
- 프런트: vitest (테이블 렌더/모달 왕복/토글/피커·편집기 조립/ dual-write mock).
- 회귀: 선행 `trigger-config` 패널 + trigger 노드 기존 테스트 전량 통과.

## 6. 범위 밖 (Out of Scope)

- priority 기반 런타임 충돌 해소(RD-7 유보) — 후속 SPEC.
- xsfm 모드 축(Auto/Sleep) 도입(RD-4 유보) — 별도 xsfm 제어 모델 SPEC.
- 공유 규칙 저장소/서버측 스케줄 카탈로그 — 노드 config 지속(RD-9)으로 충분.
