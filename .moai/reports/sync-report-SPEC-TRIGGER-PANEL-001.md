# Sync Report — SPEC-TRIGGER-PANEL-001

- **SPEC**: SPEC-TRIGGER-PANEL-001 — Trigger 노드 대시보드 패널 (스케줄 설정 + 페이로드 카탈로그 + 노드 맵핑)
- **동기화 일자**: 2026-07-31
- **구현 커밋**: `31a0e08`(백엔드 M1 — live 타이머 재등록) + `cf00c74`(프런트 M2~M5 — 대시보드 패널)
- **브랜치**: `develop`
- **버전**: v0.2.0 → v0.3.0
- **SPEC lifecycle**: spec-anchored (Tier L), Level 2 (as-implemented 정정 기록)
- **Phase**: /moai sync (문서 동기화 + as-implemented 정련 + 3-phase close)

---

## SPEC 상태 전이

| 파일 | 항목 | 이전 | 이후 |
|------|------|------|------|
| spec.md | `status` | `draft` | `completed` |
| spec.md | `version` | `0.2.0` | `0.3.0` |
| spec.md | `updated` | `2026-07-31` | `2026-07-31` (유지) |
| plan.md / acceptance.md / design.md / research.md | 버전 노트 | v0.2.0 | v0.3.0 추가 |

- spec.md HISTORY 에 `0.3.0` 행 추가(M1~M6 구현 완료 + 3-phase close).
- spec-anchored Level 2 규율에 따라 spec.md 에 **§8 구현 노트(as-implemented) IN-1~IN-6** 신설.
- spec.md 12-field frontmatter 스키마 유효(전 필드 canonical 명칭 유지, snake_case 별칭 없음).

---

## 동기화된 문서

| 파일 | 변경 요약 |
|------|-----------|
| `.moai/specs/SPEC-TRIGGER-PANEL-001/spec.md` | frontmatter `status: draft→completed`, `version: 0.2.0→0.3.0`; HISTORY 0.3.0 행 추가; **§8 구현 노트(as-implemented) IN-1~IN-6 신설** |
| `.moai/specs/SPEC-TRIGGER-PANEL-001/plan.md` | v0.3.0 버전 노트 추가(M1~M6 구현 완료 + close, as-implemented → spec.md §8 포인터) |
| `.moai/specs/SPEC-TRIGGER-PANEL-001/acceptance.md` | v0.3.0 버전 노트 추가(§A~§F 커버 테스트 파일 매핑 + 검증 지표) |
| `.moai/specs/SPEC-TRIGGER-PANEL-001/design.md` | v0.3.0 버전 노트 추가(설계→구현 실현 매핑, `schedules` 키 정정 포인터) |
| `.moai/specs/SPEC-TRIGGER-PANEL-001/research.md` | v0.3.0 버전 노트 추가(조사 기준선 구현 검증 확인) |
| `CHANGELOG.md` | `[Unreleased]` 최상단에 "추가 — Trigger 노드 대시보드 패널 + 백엔드 live 타이머 재등록" 항목 추가(기능 + 분기 6건 + 품질 요약) |
| `.moai/project/structure.md` | 대시보드 패널 섹션에 "Trigger 노드 대시보드 패널 + 백엔드 live 타이머 재등록" 문단 추가(파일·와이어링) |
| `.moai/project/product.md` | Dashboard 기능 목록에 "Trigger 노드 대시보드 패널" 불릿 추가 |

### 동기화하지 않은(스킵) 문서 — 사유 명시

| 파일 | 사유 |
|------|------|
| `README.md` | 저가치 스킵. trigger 노드/대시보드 패널 전용 섹션이 README 에 없고, 본 기능은 example YAML 추가를 수반하지 않음. 스킵해도 README 실질 완전성 불변 → scope discipline 준수. |
| `.moai/project/tech.md` | 중복. 신규 기술 스택·외부 의존성 없음(기존 Go/React 스택 + 기존 flow/panel API 재사용). |
| `.moai/project/architecture.md` | 해당 없음. 백엔드는 기존 노드 lifecycle 내 최소 침습 확장, 프런트는 기존 패널 시스템 가산으로 상위 아키텍처 변경 없음. |

---

## as-implemented 분기 (Divergence — spec-anchored Level 2)

계획(§4/§5) 대비 6건의 정련이 있으며, 모두 **비침습성·정확성을 강화**하는 방향이다(스코프 확장 아님). spec.md §8 IN-1~IN-6 에 정식 기록됨.

| # | 항목 | 계획 | 실제 구현 | 판정 |
|---|------|------|-----------|------|
| IN-1 | 재무장 게이트 | `state()==Running && timer!=nil`(§4.2) | **started-gate + `StateRunning`** 로 세분 — 재무장을 running 노드 live 재설정(ReconfigureNode) 케이스에만 격리. 초기 `Configure→Init` 는 이중 등록 안 함 | 강화형 정련(REQ-01-01/03/04) |
| IN-2 | `payloadMu` | 계획에 없음 | live 재무장이 `buildMessage` payload 읽기 vs 동시 `Configure` payload 쓰기 race 를 유발(-race 로 표면화) → 신규 `payloadMu` 로 보호 | in-scope 필수 정련(REQ-01-02/07 정확성) |
| IN-3 | 트리거 config 키 | SPEC 산문 `trigger_schedules`(느슨) | 구현 SSOT 는 **`schedules`** — LIVE(configureNode)/PERSIST(node.data) 양쪽 사용(trigger.go 대조) | 문서 표기 정정 |
| IN-4 | dual-write patch shape | patch-then-PUT(§4.3) | `getFlow` 는 config 를 reactflow flat `node.data` 반환 → `patchNodeConfigInDefinition` 이 fullConfig 병합(nodeType/label/다른 노드/와이어 보존) | §4.3 patch 형상 구체화 |
| IN-5 | 스냅샷 주입 | 배정 시점 인라인 복사(RD-11/§3.3) | **선택 시점 JSON deep-clone**, `payloadRef` 미직렬화 → 노드는 resolved inline payload 만 관측, 비소급 | RD-11 구체 구현 |
| IN-6 | 충돌 감지 | last-write-wins + 통지(RD-8) | **지속 config vs hydration 기준선 비교** → 불일치 시에만 덮어쓰기 통지, version/etag 잠금 미도입 | RD-8 구체 감지 메커니즘 |

---

## 확정 설계 결정(RD-1~11) 반영 확인

| 결정 | 구현 반영 |
|------|-----------|
| RD-1 Trigger 맵핑 | `config.flowId/nodeId` 타겟 + `useNodeTypeInstances('trigger')` 피커 + `trigger-config` PanelType |
| RD-2 live 재등록 | `TriggerNode.Configure` cancel+re-register(started+StateRunning 게이트), FULL config 전송, Init 불변 |
| RD-3 dual-write | LIVE `configureNode` + PERSIST `updateFlow`(patch-then-PUT), 404 persist-only |
| RD-4 카탈로그 inline | 패널 config `payloadCatalog` inline 주입, 노드 페이로드 모델 불변 |
| RD-5 스케줄 편집 UI | 6타입 CRUD + 카탈로그 에디터 + per-schedule 셀렉터(`TriggerScheduleEditor` 재사용) |
| RD-6 generation 토큰 | `rearmGen` 세대 캡처/비교 → stale in-flight 발화 drop(no-double-fire/no-orphan) |
| RD-7 빈 스케줄 IDLE | 빈 스케줄 live Configure → 전 타이머 취소 후 유효 IDLE(오류 아님) |
| RD-8 last-write-wins | 지속 config vs hydration 비교 통지, 낙관적 잠금 미도입 |
| RD-9 카탈로그 패널-로컬 | `config.payloadCatalog`, 공유 저장소 out of scope |
| RD-10 flow API 재사용 | useFlows/useFlowNodes read + updateFlow write, running/stopped 배지, 404 persist-only |
| RD-11 스냅샷 비소급 | 선택 시점 JSON deep-clone, 카탈로그 후속 편집 비소급, 재선택 시 갱신 |

---

## 품질 게이트 (관측 증거)

### sync-phase 직접 재검증 (docs-only, 코드 무변경)

| 항목 | 명령 | 결과 |
|------|------|------|
| M1 재무장 테스트(-race) | `go test -race -run 'Rearm\|Reconfigure\|Configure' ./internal/node/` | exit=0, `ok github.com/xtra/xflow/internal/node` (`.moai/state/verify/trigger-panel/m1-rearm.log`) |
| 구현 커밋 존재 | `git show --stat 31a0e08 cf00c74` | `31a0e08`: trigger.go(+100) + trigger_rearm_test.go(+310); `cf00c74`: TriggerConfigPanel.tsx(+372)·triggerPanelUtils.ts(+121)·AddPanelDialog.tsx(+125) 등 10파일 +1169 |

> 참고: `-race` 재검증은 동일 `internal/node` 패키지에 무관한 미커밋 변경(mqtt_test.go/registry.go/xsfm.go)이 존재하는 상태에서 수행되었으며 exit 0 로 통과했다. macOS `ld` LC_DYSYMTAB warning 은 벤ign 링커 경고로 테스트 실패가 아니다.

### run-phase 보고 지표 (커밋 시점 — 본 sync 에서 재실행하지 않음)

- 백엔드 M1(`31a0e08`): 재무장/취소/no-double-fire/롤백/빈-스케줄/폴백 신규 함수 커버리지 100%, `-race` 클린, 노드 테스트 green.
- 프런트 M2~M5(`cf00c74`): vitest 2319 pass(+32), `tsc`/eslint 클린.
- 전체: `go test ./...` exit 0(42 pkgs), 무회귀.

> 미검증(Gaps): 전체 `go test ./...`(42 pkgs), 프런트 vitest/tsc/eslint, 백엔드 커버리지 100% 는 본 sync(docs-only)에서 재실행하지 않고 run-phase 커밋 보고를 인용했다. 본 sync 에서 직접 관측한 것은 M1 재무장 테스트(-race, exit 0)와 두 구현 커밋의 stat 뿐이다.
>
> 잔여 위험(Residual risk): (a) `-race` 재검증이 미커밋 동일-패키지 변경과 함께 실행되어 커밋-격리 결과와 미세 차이 가능(관측 결과는 통과). (b) 프런트 지표는 재실행 없이 인용 → 커밋 이후 미커밋 web 변경(nodeSchemas 등)이 있으면 현 트리 vitest 수치는 보고값과 다를 수 있음.

---

## 비고

- 소스 코드/테스트 파일(`internal/**`, `web/**`)은 **수정하지 않았다**(문서/SPEC 전용).
- 무관한 pre-existing 미커밋 변경(`internal/node/mqtt_test.go`·`registry.go`·`xsfm.go`·`registry_test.go`·`xsfm_test.go`, `web/src/config/nodeSchemas*`, `web/src/lib/flow/nodeType.ts`, 루트 `data.json`/`output.json`/`packet.json`, `web/data/` 등)은 **손대지 않았다**.
- 커밋/푸시/PR 생성은 하지 않았다(오케스트레이터가 sync 커밋 + PR 을 별도 처리).
