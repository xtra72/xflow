# Plan — SPEC-TRIGGER-PANEL-001

> Trigger 노드 대시보드 패널 (스케줄 설정 + 페이로드 카탈로그 + 노드 맵핑). Tier L. 우선순위 기반 마일스톤(시간 예측 없음).
>
> **버전 노트**: v0.2.0 (2026-07-31) — OQ-1~6 엔지니어링 기본값 확정 → RD-6~11 반영. M1 은 generation 토큰 재무장으로, M5 는 last-write-wins + running/stopped 배지로 구체화. OQ 미해결 항목 없음.

## 기술 접근 (Technical Approach)

두 축을 분리해 다룬다: (1) **백엔드 live 재등록**(노드 핵심 최소 침습 확장) — 기존 `cancelAllTimers()`/`registerSchedules()` 재사용, (2) **프런트 패널**(가산형 대시보드 위젯) — 기존 패널 시스템/타겟팅 훅/dual-write 선례 재사용. 노드 페이로드 모델은 불변, 카탈로그는 프런트 편의 계층으로 국한한다. 저장은 `CustomNode` 듀얼 라이트(live `configureNode` + persist `updateFlow`) 관용구를 미러링한다.

## 마일스톤 (우선순위 순)

### M1 — 백엔드 Trigger live 재등록 (Priority High · 1차 목표)

- `TriggerNode.Configure`(`trigger.go:951-995`) 말미에 재무장 단계 추가: `state()==Running && timer!=nil` 게이트 하에 `cancelAllTimers()` → `rearmGen++` → `registerSchedules()`, 실패 시 `cancelAllTimers()` 롤백. (REQ-01-01~06, §4.2)
- **RD-6 generation 토큰**: `timerMu` 경계 내에서 재무장 세대 카운터(`rearmGen`)를 증가시키고, 각 등록 타이머는 등록 시점 세대를 캡처한다. 발화 핸들러는 세대 불일치 시 방출을 drop 하여 cancel→re-register 창의 stale in-flight 발화를 폐기한다(no-double-fire, no-orphan).
- **RD-7 빈 스케줄**: 빈 스케줄 live Configure 는 전 타이머 취소 후 유효 IDLE(오류 아님).
- per-schedule/노드 레벨 payload 폴백 보존 확인(REQ-01-07). Init 경로 불변(REQ-01-04).
- **선행**: M2~M5 프런트가 이 경로에 의존하므로 최우선.

### M2 — 대시보드 패널 골격 + 노드 타겟팅 (Priority High · 2차 목표)

- `uiStore.ts` `PanelType` union 에 `'trigger-config'` 추가, `createDefaultPanel`/`panelDefaultSize` 확장.
- `AddPanelDialog.tsx` 에 trigger-node 피커(`useNodeTypeInstances('trigger')`) 추가 → 선택 시 `{flowId,nodeId}` config 저장. (REQ-02-01~03)
- 신규 패널 컴포넌트(계약 `{panelId,title,config,onConfigChange,onTitleChange}`) + `renderDashboardPanel.tsx` 디스패치 케이스. `useFlowNodes` 로 대상 노드 config 로드, **running/stopped 배지** 표시(RD-10). not-running 시 persist-only 표식. (REQ-02-04~06)

### M3 — 스케줄 편집 UI (Priority Medium · 3차 목표)

- 6개 타입 CRUD. `TriggerScheduleEditor.tsx` 재사용/미러. per-schedule 페이로드 셀렉터(카탈로그 연동, M4 후결합). 노드 config 스키마 직렬화. (REQ-03-01~04)

### M4 — 페이로드 카탈로그 + 인라인 주입 (Priority Medium · 4차 목표)

- 패널 config `payloadCatalog`(패널-로컬, RD-9) CRUD 에디터. 직렬화 유틸: 스케줄 이름 참조 → `payloadCatalog[name]` inline **스냅샷** 치환(RD-11 — 배정 시점 복사, 카탈로그 후속 편집 비소급). 노드 config 는 resolved inline payload 만 포함. (REQ-04-01~05, §4.4)

### M5 — dual-write 지속성 (Priority Medium · 5차 목표)

- 저장 핸들러: LIVE `configureNode(flowId,nodeId,fullConfig)` + PERSIST patch-then-PUT(`getFlow`→노드 config 패치→`updateFlow`). 404 persist-only 통지(RD-10). FULL config 전송 규율. (REQ-05-01~05, §4.3)
- **RD-8 충돌 처리**: dual-write persist 는 last-write-wins + 대시보드 통지. 낙관적 잠금 미도입(향후 확장). `updateFlow` 가 대시보드 컨텍스트에서 호출 가능함을 검증(RD-10).

### M6 — 테스트 + no-regression 검증 (Priority High · 최종 목표)

- Go testify: 재등록/취소/no-double-fire/롤백/빈-스케줄/폴백 보존.
- vitest: 타겟 피커, 패널 렌더, 스케줄 CRUD, 카탈로그 inline 주입, dual-write(live+persist), 404 persist-only.
- 기존 패널/flow 에디터 회귀 없음 확인(REQ-06-01).

## 아키텍처 설계 방향

- **최소 침습 노드 변경**: 신규 헬퍼 없이 기존 `cancelAllTimers`/`registerSchedules` 조합. 상태 게이트로 Init 경로 격리.
- **SSOT 규율**: 스케줄/페이로드는 노드 config 가 SSOT. 패널 config 는 타겟(`flowId/nodeId`) + 카탈로그만. 스케줄 복제 금지.
- **가산형 프런트**: 기존 PanelType/렌더/다이얼로그에 케이스 추가만. 기존 패널 경로 불변.

## 리스크 및 대응

| 리스크                                              | 영향 | 대응                                                                                     |
| ------------------------------------------------ | -- | -------------------------------------------------------------------------------------- |
| cancel+re-register 창의 double-fire/orphan(RD-6)    | 중  | generation 토큰 재무장(세대 불일치 발화 drop); 테스트에서 취소 후 미발화·재등록 후 정확 1회 발화 검증                    |
| 빈 스케줄 live Configure 로 노드 무력화(RD-7)               | 중  | 전 타이머 취소 후 유효 IDLE 허용(오류 아님) + 명시 테스트                                                    |
| 부분 config 전송 시 스케줄/페이로드 유실(partial-config hazard) | 높  | REQ-05-04 FULL config 강제; 패널이 노드 현재 config 를 로드 후 병합 전송                                   |
| 동시 flow 편집 충돌(RD-8)                               | 중  | last-write-wins + 대시보드 통지 확정; 낙관적 잠금은 향후 SPEC                                             |
| 대시보드 flow 쓰기 권한(RD-10)                            | 중  | 기존 flow API 재사용(신규 권한 없음); 구현 시 `updateFlow` 호출 가능성 검증; 404 시 persist-only+배지          |
| 카탈로그 편집 후 stale 주입(RD-11)                         | 저  | 스냅샷 주입 확정(배정 시점 복사, 비소급) + 재선택 시 갱신                                                       |

## TRUST 5

- **Tested**: M6 Go testify + vitest, 신규 코드 ≥85%.
- **Readable**: 기존 trigger.go/패널 네이밍·구조 준수, 한국어 주석(code_comments=ko).
- **Unified**: gofmt / prettier.
- **Secured**: 스케줄·페이로드 입력 검증(REQ-06-03).
- **Trackable**: Conventional Commit + SPEC-TRIGGER-PANEL-001 참조.

## 의존성

- M1 선행(백엔드 경로) → M2~M5(프런트) → M6(검증). M3/M4 는 M2 이후 병렬 가능, M5 는 M3/M4 직렬화 규칙에 의존.
- OQ-1~6 은 RD-6~11 로 확정 완료 — 착수 전 미해결 결정 대기 없음. RD-6(M1), RD-7(M1), RD-8(M5), RD-9(M4), RD-10(M2/M5), RD-11(M4) 각 마일스톤에 반영됨.
