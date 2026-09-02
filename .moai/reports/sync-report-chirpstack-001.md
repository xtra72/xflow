# Sync Report — SPEC-CHIRPSTACK-001

- **SPEC**: SPEC-CHIRPSTACK-001 — ChirpStack LoRaWAN 에이전트 (`chirpstack` 에이전트 + `chirpstack-in` 노드)
- **동기화 일자**: 2026-08-12
- **구현 커밋 (M1~M6)**: `eb0339f9` / `6bb3a965` / `ca024d1d` / `bc08f3ce` / `b26a8850` / `88807142`
- **브랜치**: `feature/SPEC-CHIRPSTACK-001`
- **버전**: v0.1.0 (구현 완료)
- **SPEC lifecycle**: spec-anchored (Tier L), Level 2 (as-implemented 정정 기록)
- **Phase**: /moai sync (문서 동기화 + as-implemented 정련 + 3-phase close)
- **Git 모드**: personal (auto_push=false, auto_pr=false) — 로컬 커밋만, push/PR 미수행

---

## SPEC 상태 전이

| 파일 | 항목 | 이전 | 이후 |
|------|------|------|------|
| spec.md | `status` | `draft` | `completed` |
| plan.md | `status` | `draft` | `completed` |
| acceptance.md | `status` | `draft` | `completed` |
| design.md | `status` | `draft` | `completed` |
| research.md | `status` | `draft` | `completed` |
| (전 파일) | `updated` | `2026-08-12` | `2026-08-12` (유지) |

- spec-anchored Level 2 규율에 따라 spec.md 에 **§ Implementation Notes (as-implemented) IN-1~IN-4** 신설(원 요구사항 텍스트 보존, 분기는 주석).
- 12-field frontmatter 스키마 유효(전 필드 canonical 명칭 유지).
- 이 SPEC 디렉토리는 커밋 이전까지 untracked 였으며, 본 sync 커밋이 추적을 시작한다.

---

## 동기화된 문서

| 파일 | 변경 요약 |
|------|-----------|
| `.moai/specs/SPEC-CHIRPSTACK-001/spec.md` | frontmatter `status: draft→completed`; **§ Implementation Notes (as-implemented) IN-1~IN-4 신설** (산출물·분기·AC/REQ 충족 확인) |
| `.moai/specs/SPEC-CHIRPSTACK-001/plan.md` | frontmatter `status: draft→completed` |
| `.moai/specs/SPEC-CHIRPSTACK-001/acceptance.md` | frontmatter `status: draft→completed` |
| `.moai/specs/SPEC-CHIRPSTACK-001/design.md` | frontmatter `status: draft→completed` |
| `.moai/specs/SPEC-CHIRPSTACK-001/research.md` | frontmatter `status: draft→completed` |
| `.moai/specs/SPEC-CHIRPSTACK-001/progress.md` | §E.2 Run-phase Evidence(M1~M6 커밋) + §E.3 Run-phase Audit-Ready(커버리지/TRUST5) + §E.4 Sync-phase Audit-Ready(상태 전이·문서·sync_commit_sha pending) 추가 |
| `CHANGELOG.md` | `[Unreleased]` 최상단에 "추가 — ChirpStack LoRaWAN 에이전트" 항목 추가(기능 + 분기 4건 + 품질 요약) |
| `README.md` | 구현 현황에 "internal/agent/chirpstack + internal/node - ChirpStack LoRaWAN Agent (SPEC-CHIRPSTACK-001)" 섹션 추가(SPEC-MQTT-001/MODBUS-001 포맷 준수) |
| `.moai/project/structure.md` | 표준 Agent 목록에 `chirpstack/ (SPEC-CHIRPSTACK-001 구현 완료)` 불릿 추가(패키지·노드·wiring) |
| `.moai/project/tech.md` | 표준 Agent 목록에 ChirpStack LoRaWAN(`chirpstack`/`chirpstack-in`) 추가 |
| `.moai/project/product.md` | Agent 유형 표에 "ChirpStack LoRaWAN" 행 추가 |
| `.moai/reports/sync-report-chirpstack-001.md` | 본 보고서(신규) |

### 동기화하지 않은(스킵) 문서 — 사유 명시

| 파일 | 사유 |
|------|------|
| README 예제 표(`examples/agents`/`examples/flows`) | 스킵. 본 SPEC 은 example YAML 을 수반하지 않음 — 예제 파일이 없는 행을 추가하면 존재하지 않는 파일 링크를 만들게 되므로 scope discipline 준수. 대신 "구현 현황" 섹션에 문서화. |
| `.moai/project/architecture.md` | 해당 없음. 신규 에이전트는 기존 Agent/Node 프레임워크 내 최소 침습 확장(기존 MQTT 트랜스포트·device registry·node dedup 재사용)으로 상위 아키텍처 변경 없음. |
| `.moai-backups/` 안전 백업 | **의도적 스킵**. git history 가 롤백 경로를 제공하고, 본 sync 커밋이 SPEC 디렉토리를 신규 추적하므로 백업이 중복. |

---

## as-implemented 분기 (Divergence — spec-anchored Level 2)

계획(plan.md/research.md) 대비 4건의 정련. 모두 **정확성·비침습성 강화** 방향이며 스코프 확장 아님. spec.md § Implementation Notes IN-1~IN-4 에 정식 기록됨.

| # | 항목 | 계획/SPEC 텍스트 | 실제 구현 | 판정 |
|---|------|------------------|-----------|------|
| IN-1 | flow-validation 경로 | REQ-M3-06 `internal/flow/validate.go` (존재하지 않는 파일) | 실제 `pkg/flow/validate.go` 의 `agentRefRequiredTypes` 에 `chirpstack-in` 추가 — 존재성 화이트리스트 아님, agent_ref 검증 활성화 목적(노드 존재 판정은 `node/registry.go` factories) | 경로 정정(REQ-M3-06 충족) |
| IN-2 | tags 지속화 타입 | `Tags []string` 상정 | `DeviceMetadata.Labels map[string]string` 매핑(ChirpStack tags 가 key-value map). 메시지 `metadata.tags` verbatim pass-through(FROZEN-04)는 불변 | 데이터 모델 정정(REQ-M4-04/FROZEN-04 충족) |
| IN-3 | comm-state 배치 | `state` 그룹 위치 미확정 | 메시지 `payload` 에 typed 값(online:bool/rssi:int/snr:float/last_seen_ms:int64), 노드가 `msg.Type()`=`device_state.<trigger>` 계층형 설정, 별도 `device_connection` 타입 미도입 | 구체 구현(FROZEN-03/M5 충족) |
| IN-4 | rxInfo 디코딩 + 전송 | 위치·전송 채널 미확정 | rxInfo 디코딩은 M5(comm-state) 소속, device_state 는 기존 recvCh 에 `record` 판별자로 접힌 단일 전송(folded stream) | 구체 구현(FROZEN-03 fold 충족) |

---

## 품질 게이트 (관측 증거)

> 본 sync 는 문서 전용(코드 무변경)이며, 아래 지표는 run-phase 커밋 시점 보고(오케스트레이터 Phase 0.5 재검증 완료)를 인용한다.

| 항목 | 결과 |
|------|------|
| 신규 코드 커버리지 | 89.4% (목표 85% 초과) |
| TRUST 5 | PASS (Critical 0) |
| go build / vet / gofmt | 클린 |
| 신규 코드 golangci-lint | 0 finding |
| REQ-FROZEN-01~04 | 준수 |
| AC-1~AC-9 | 전량 충족 |
| 회귀 | 무회귀 (무관한 pre-existing flaky E2E `internal/api/service` 는 본 SPEC 과 무관 — 미변경) |

### Gaps (미검증)

- 라이브 ChirpStack 브로커 스모크 테스트는 이연(packet.json 픽스처 기반 테이블 주도 테스트로 대체).
- 본 sync(docs-only)에서 `go test ./...` 를 재실행하지 않고 run-phase 보고를 인용했다(오케스트레이터 Phase 0.5 에서 재검증 완료됨).

### Residual risk (잔여 위험)

- pre-existing flaky E2E(`internal/api/service`)는 무관하나 CI 재실행 시 간헐 실패 가능 — 본 SPEC 범위 외.

---

## 비고

- 소스 코드/테스트 파일(`internal/**`, `pkg/**`, `cmd/**`)은 **수정하지 않았다**(문서/SPEC 전용).
- 스테이징은 SPEC-CHIRPSTACK-001 디렉토리 + 실제 변경한 CHANGELOG/README/.moai/project/* + 본 보고서로 한정했다. `git add -A`/`git add .` 미사용.
- 무관한 미커밋/untracked 파일(다른 SPEC 디렉토리, `.moai/config/sections/llm.yaml`, `.moai/memory/last-session-state.json`, `.moai/state/`, 루트 `data.json`/`output.json`/`packet.json`, `examples/subway/`, `web/data/`, `.pptx` 등)은 **손대지 않았다**.
- 커밋은 로컬 전용 — push/PR 미수행(personal git 모드). 브랜치는 push/PR 준비 완료(사용자 확인 대기).
