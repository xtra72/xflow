# Sync Report — SPEC-XSFM-AGENT-IO-001

- **SPEC**: SPEC-XSFM-AGENT-IO-001 — xsfm 에이전트 수신-전달 옵션 + 상태 방출 모드(event/interval/both)
- **동기화 일자**: 2026-07-30
- **구현 커밋**: `5fc11209`(백엔드 M1~M4) + `6fb084c3`(프런트 M5)
- **브랜치**: `feature/SPEC-XSFM-GROUP-001`
- **버전**: v0.2.0 → v0.3.0
- **SPEC lifecycle**: spec-anchored (Tier M), Level 2 (as-implemented 정정 기록)
- **Phase**: /moai sync (문서 동기화 + as-implemented 정련 + 3-phase close)

---

## SPEC 상태 전이

| 파일 | 항목 | 이전 | 이후 |
|------|------|------|------|
| spec.md | `status` | `draft` | `completed` |
| spec.md | `version` | `0.2.0` | `0.3.0` |
| spec.md | `updated` | `2026-07-30` | `2026-07-30` (유지) |
| plan.md / acceptance.md | 버전 노트(top-of-file) | `0.2.0` | `0.3.0` + 상태 `completed` |

- spec.md 는 12-필드 frontmatter 형식을 유지하며 `status`/`version` 만 갱신(`updated` 는 동일 일자로 유지).
- plan.md / acceptance.md 는 frontmatter 대신 **top-of-file 버전 노트** 형식을 사용하므로 그 형식에 맞춰 버전·상태를 갱신(기존 관례 일치).
- spec.md HISTORY 에 `0.3.0` 행 추가(M1~M5 구현 완료).
- spec-anchored Level 2 규율에 따라 spec.md 에 **§7 Implementation Notes(as-implemented)** 신설(분기 4건, §7.1~§7.4).

---

## 동기화된 문서

| 파일 | 변경 요약 |
|------|-----------|
| `.moai/specs/SPEC-XSFM-AGENT-IO-001/spec.md` | frontmatter `status: draft→completed`, `version: 0.2.0→0.3.0`; HISTORY 0.3.0 행 추가; **§7 Implementation Notes(as-implemented) §7.1~§7.4 신설** |
| `.moai/specs/SPEC-XSFM-AGENT-IO-001/plan.md` | top-of-file 버전 노트 `0.3.0` + 상태 `completed`(구현 M1~M5 완료 + 검증 증거) 추가 |
| `.moai/specs/SPEC-XSFM-AGENT-IO-001/acceptance.md` | top-of-file 버전 노트 `0.3.0` + 상태 `completed`(AC-1.x~AC-7.x 전 시나리오 통과) 추가 (본문 시나리오 무변경) |
| `CHANGELOG.md` | `[Unreleased]` 최상단 `추가` 에 "xsfm 에이전트 수신 forward 옵션 + 상태 방출 모드(event/interval/both)" 항목 추가(기능 + as-implemented 분기 4건 요약) |
| `.moai/project/product.md` | "제공 Agent 목록" 표의 Subway Facilities Manager 행에 수신 forward 옵션 + 상태 방출 모드 요약 추가 |
| `.moai/project/structure.md` | `internal/agent/xsfm/` 항목에 수신 forward + 상태 방출 모드 확장 문단 추가(신규 파일 `emit_mode.go`·config 확장·on-change 게이팅·프런트 3개 컨트롤·분기 4건) |

### 동기화하지 않은(스킵) 문서 — 사유 명시

| 파일 | 사유 |
|------|------|
| `internal/**`, `web/**` (코드) | 본 sync 는 문서 동기화 전용. 코드는 `5fc11209`/`6fb084c3` 에 이미 커밋됨(범위 밖). |
| SPEC-XSFM-GROUP-001 / LINE-001 등 타 SPEC 파일 | 본 SPEC 범위 밖(무변경 제약). |
| `data.json`/`output.json`/`packet.json`/`.moai/memory/*` | 무관한 미커밋 변경(범위 밖, 미변경). |
| `.moai/project/architecture.md`/`tech.md` | 수신 forward·상태 방출 모드는 기존 아키텍처/기술 스택 경계를 변경하지 않는 가산 방출 레이어 → 증분 기재 대상 아님(GROUP-001/LINE-001 도 동일 판단). |
| README / API 문서 | MQTT 토픽/페이로드 규약 불변, 신규 방출은 `msgCh` 노드 계층에만 작용 — 외부 공개 API/README 노출 표면 변경 없음. |

---

## 구현 노트 요약 (as-implemented 분기 — spec.md §7)

| # | 분기 | 사양 | 구현 | 사유 |
|---|------|------|------|------|
| 1 | `state_emit_mode` select **raw enum 표시** | REQ-05-01: mode 선택 UI(표기 방식 미규정) | select 가 원시 enum(`event`/`interval`/`both`) 노출, 한국어 의미는 필드 설명에 기술 | 공용 FormField select 위젯에 옵션 라벨 맵 부재, 기존 xsfm/HVACR select 관례 계승(scope discipline) |
| 2 | interval on-change 억제 **단일 시임 복합 조건** | §4.4: interval 모드 on-change 억제 | `ingestState` 기존 단일 방출 지점의 복합 조건(`len(changed)>0 && (event\|both)`), 별도 플래그/경로 없음 | 단일 게이트 조건이 별도 상태/경로 신설보다 단순(enforce simplicity), on-change 방출 지점 유일 |
| 3 | 메시지 shape (RD-5 확정 그대로) | RD-5: snapshot 단일 배열 / received 단일 디바이스 | `device_state_snapshot` = 단일 `{type,timestamp,devices:[...]}` 배열(N per-device 아님, deviceStateJSON 재사용); `device_state_received` = 단일 디바이스, `changed_fields` 생략 | 스냅샷=배치/roster 개념(request_state 응답 shape 동형), 패스스루 탭은 "변경분" 개념 없음 |
| 4 | `Stop` **변경 없음** | §4.5: 신규 방출기 Stop 정리 | 주기 방출기가 offline 모니터의 `monitorWG`/`stopCh` 재사용, 기존 단일 `monitorWG.Wait()` 로 두 goroutine 커버 | §4.5 사양(monitorWG 재사용·한 번의 Wait)에 정확히 부합, 두 goroutine 모두 채널 송신 중 락 미보유 → Wait 데드락 없음 |

---

## 검증 증거 (오케스트레이터 제공 + 본 sync 확인)

> 아래 커밋·파일은 본 sync 단계에서 직접 관찰(git log / git show)로 확인. 테스트·커버리지 수치는 run-phase(오케스트레이터) 제공 값.

- **구현 커밋 존재 확인** (직접 관찰, `git show --stat`):
  - `5fc11209 feat(xsfm): 수신 forward 옵션 + 상태 방출 모드(event/interval/both) 백엔드 (SPEC-XSFM-AGENT-IO-001 M1~M4)` — `agent.go`(+14/-2), `config.go`(+65), `emit_mode.go`(신규 +101), `emit_mode_test.go`(신규 +478), `errors.go`(+3)
  - `6fb084c3 feat(web): xsfm 에이전트 config 토글 UI (SPEC-XSFM-AGENT-IO-001 M5)` — `agentSchemas.test.ts`(+83), `agentSchemas.ts`(+6)
- **백엔드 테스트** (run-phase 제공): `go test ./...` exit 0 (42 pkgs, 0 FAIL), `go test -race ./internal/agent/xsfm/...` 클린, xsfm 커버리지 **89.3%** (REQ-04-05 목표 85%+ 충족)
- **프런트 테스트** (run-phase 제공): vitest **2287 pass**, `tsc` 클린
- **무회귀** (run-phase 제공): 기본 config(forward off, mode=event) byte-identical — 신규 메시지 미방출, on-change 경로 불변. MQTT 토픽/페이로드 규약 불변.

### 미검증(Gaps)

- 테스트 스위트·커버리지 수치는 run-phase 오케스트레이터 보고 값을 인용(본 sync 단계에서 `go test` 를 재실행하지 않음 — 문서 동기화 전용 phase).
- `-race` 클린·vitest 2287·커버리지 89.3% 는 커밋 시점 측정값이며, 본 sync 는 트리를 코드 변경 없이 유지하므로 재측정 불필요.

### 잔여 위험(Residual Risk)

- 분기 1(mode select raw enum 표시)로 인해 운영자가 `event`/`interval`/`both` 의미를 필드 설명에 의존 — 현재 UI 관례 일치 결정. 필요 시 후속에서 FormField 옵션 라벨 맵 도입 검토.
- interval 모드 주기 방출은 등록 디바이스 수에 비례한 배열 크기 — 대규모 fleet + 짧은 interval 조합 시 `msgCh` drop 가능(non-blocking·가득 참 시 drop 규약 계승). 현재 기본 60s·단일 배열 메시지로 완화.

---

## 3-Phase Close

- **plan → run → sync** 3-phase 완료. run-phase 구현(`5fc11209`, `6fb084c3`) 위에서 sync-phase 문서 동기화 수행.
- SPEC 상태 `draft → completed` 전이(spec.md frontmatter). 커밋은 오케스트레이터가 처리(본 sync 는 커밋하지 않음).
- 후속: 없음(SPEC-XSFM-AGENT-IO-001 단일 SPEC 종결).
