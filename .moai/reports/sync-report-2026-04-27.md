# Sync Report — 2026-04-27

- **Sync mode**: auto
- **Date**: 2026-04-27
- **Commits synced**: 11 (range `dbec77e..35a33fc`)
- **SPECs bumped**:
  - SPEC-WEB-005 (v0.4.0 → v0.5.0)
  - SPEC-STORE-003 (v0.1.0 → v0.2.0)
- **Quality status**: PASS (Backend tests OK, Frontend 459 tests pass, TS clean)
- **Files modified (commit range)**: 41 files (+4100 / −309 lines)

---

## Commits synced

| SHA       | Type | Subject |
|-----------|------|---------|
| `b4ad829` | fix  | readOnly 모드에서 설정 값 가시성 복원 (SPEC-STORE-003) |
| `6dfc6cc` | feat | 동적 키를 정적으로 변환하는 UI 추가 (SPEC-STORE-003) |
| `309966e` | fix  | StoreKeysEditor readOnly 가시성 복원 (SPEC-STORE-003) |
| `d20eaf1` | fix  | property 편집기 8종 readOnly 가시성 일괄 복원 |
| `cecc22e` | fix  | StoreKeysEditor 키:태그 컬럼 비율 1:3 (SPEC-STORE-003) |
| `55a41f9` | feat | 저장소 전체/개별 키 초기화 기능 (SPEC-STORE-003) |
| `848dbe7` | fix  | 저장소 행별 액션을 마지막 컬럼으로 분리 + 전체 초기화 색상 중립화 |
| `7950292` | fix  | 버킷 타임스탬프를 인터벌 벽시계 경계에 정렬 (SPEC-STORE-003) |
| `62ef43d` | fix  | 데이터 뷰어 시리즈 multi-select 가시 영역 확장 |
| `da9c697` | feat | TSDB 데이터 뷰어 3종 개선 (SPEC-WEB-005) |
| `35a33fc` | fix  | 정적 키 태그 입력 자동 commit on blur (SPEC-STORE-003) |

---

## Files modified (sync targets)

| Path | Change |
|------|--------|
| `CHANGELOG.md` | `[Unreleased]` 의 `### 추가` 상단에 SPEC-STORE-003 (키 초기화, 동적→정적 변환) + SPEC-WEB-005 (TSDB 뷰어 3종) 항목 추가. `### 변경` 상단에 버킷 정렬, 액션 컬럼 분리, 색상 중립화, 컬럼 비율, multi-select max-h 5건 추가. `### 수정` 상단에 readOnly 가시성 복원 + 태그 blur commit 2건 추가. |
| `.moai/specs/SPEC-WEB-005/spec.md` | `version: 0.4.0 → 0.5.0`, `updated: 2026-04-26 → 2026-04-27`. HISTORY 최상단에 0.5.0 엔트리 (3종 개선 요약). Implementation Notes 에 `keyTagExtractor` 유틸 + `v0.5.0 Notes` 섹션 (react-window 제거, 소수점 옵션, 태그 자동 추출 fallback) 추가. |
| `.moai/specs/SPEC-STORE-003/spec.md` | `version: 0.1.0 → 0.2.0`, `updated: 2026-04-26 → 2026-04-27`. HISTORY 최상단에 0.2.0 엔트리 (5개 항목 요약). HISTORY 표에 0.2.0 행 추가. Implementation Notes 에 `v0.2.0 Notes` 섹션 추가 (`Store.ClearHistory` 인터페이스, DELETE 엔드포인트 2종, 버킷 정렬, 태그 blur commit, 신규 Frontend 컴포넌트, 액션 컬럼 분리). |
| `.moai/project/structure.md` | `web/` 섹션의 재사용 컴포넌트 목록에 `ConfirmDialog`, `PromoteToStaticDialog`, `keyTagExtractor` 추가. HTTP 엔드포인트 목록에 `DELETE /api/v1/store/{name}/keys/{key}` + `DELETE /api/v1/store/{name}/keys` 추가. 신규 백엔드 메서드 (`Store.ClearHistory`, `UserStoreAgent.IsStaticKey/DeleteEntry/ClearHistory`, `agentStore.SetAllowDynamicKeys/SetStaticKeys`) 별도 서브섹션. 버킷 타임스탬프 정렬 정책 변경 노트 추가. |

## Files created

| Path | Description |
|------|-------------|
| `.moai/reports/sync-report-2026-04-27.md` | 이 동기화 리포트 |

## Sections skipped

- `.moai/project/tech.md`: 이번 커밋 범위에서 신규 의존성 추가 없음. `react-window` 는 v0.5.0 에서 활성 사용에서 제거되었으나 `package.json` 에서 패키지 자체는 유지 (단순 코드 경로 제거이므로 tech.md 업데이트 불필요). 사용 변화는 SPEC-WEB-005 spec.md `v0.5.0 Notes` 에 기록됨.
- `.moai/project/product.md`: 사용자 노출 기능 리스트 형식이 아닌 아키텍처 컴포넌트 카탈로그. 이번 변경은 기존 Store / Web Dashboard 보강이므로 새 항목 추가 불필요 (이전 sync 동일 정책 유지).
- `.moai/project/architecture.md`: 변경 없음. 이번 변경은 인터페이스 수준 (Store.ClearHistory 메서드 추가, DELETE 엔드포인트) 으로 아키텍처 다이어그램 영향 없음.
- `README.md`: 이전 sync 에서 SPEC-WEB-005 / SPEC-STORE-003 항목이 이미 추가됨. v0.5.0 / v0.2.0 의 변경은 기존 기능의 점진적 개선이므로 README 수준의 노출 불필요.

---

## Divergence summary

### SPEC-WEB-005 (v0.4.0 → v0.5.0)

- **react-window 가상 스크롤 제거**: v0.3.0 에서 도입한 500행 자동 가상화를 페이지네이션으로 대체. 페이지 크기 [10, 25(기본), 50, 100]. `react-window` 의존성은 `package.json` 에 남아있으나 활성 사용처 0건 (tech.md 의존성 표 업데이트는 보류).
- **소수점 자릿수 옵션**: 평균 집계에 `decimals` 입력 (0-6, 기본 1). 매트릭스 셀과 CSV 양쪽 동일하게 `toFixed(n)` 적용 (분기 없음).
- **`keyTagExtractor` 유틸 추가**: 정적 태그(SPEC-STORE-003) 미존재 시에만 fallback 으로 동작. InfluxDB 라인 프로토콜 + colon/slash 위치 기반 segments 두 모드 지원. 사용자 명시 태그가 항상 우선.
- **데이터 뷰어 multi-select 가시 영역**: `max-h-40 → max-h-[40vh]`. 모달 95vh 활용도 개선 (목록형 시리즈 선택 사용성 향상).

### SPEC-STORE-003 (v0.1.0 → v0.2.0)

- **`Store.ClearHistory(ctx, key) error` 인터페이스 추가**: 3개 구현체 (`VolatileStore`/`NamespacedStore`/`PersistentStore`) 동시 추가. 정적 키는 history 만, 동적 키는 entry 자체 삭제로 분기. 신규 헬퍼 (`UserStoreAgent.IsStaticKey/DeleteEntry/ClearHistory`).
- **버킷 타임스탬프 정렬 시멘틱 변경**: 사용자 시작 시각 기준 → epoch 0 기준 벽시계 정렬 (`floor(tsMs / intervalMs) * intervalMs`). 1m → 초=0, 5m → 분 0/5/10/..., 1h → 분=초=0. 결과 정렬이 더 직관적이며 TSDB 엔진은 이미 동일 로직.
- **DELETE 엔드포인트 2종**: 단일 (`DELETE /api/v1/store/{name}/keys/{key}`) + bulk (`DELETE /api/v1/store/{name}/keys`). bulk 응답에 `cleared_count` / `deleted_count` 포함.
- **태그 blur 자동 commit**: TagChipsEditor 의 keyInput/valInput 에 `onBlur` 추가. pending input 유실 버그 수정 (사용자가 "추가" 버튼 누르지 않고 폼 저장 시).
- **동적 키 → 정적 변환 UI**: 별도 백엔드 변경 없이 Configure API 재사용. `PromoteToStaticDialog` 신규 컴포넌트.
- **공통 컴포넌트 추출**: `ConfirmDialog` (default/danger variant) 재사용 컴포넌트 신설.

### Cross-cutting Issues

- **readOnly 가시성 회귀**: property 편집기 10종 (FormField + StoreKeysEditor + TriggerScheduleEditor + Bridge HTTP/MQTT/Modbus + KeyValueMap + RegisterMap + StringList + TransformPipeline) 에서 `disabled={readOnly}` 패턴이 값까지 흐려지게 만드는 회귀 발견. 일괄 `readOnly={readOnly}` 로 전환하고 `bg-(--color-bg-elevated)` 토큰으로 시각 구분 단순화.
- **버킷 정렬 변경은 시멘틱 차이**: 결과 행의 시작 timestamp 가 변경됨. 사용자 입장에서 "5분 단위 그래프가 항상 0/5/10 분에 정렬" 되어 가독성 향상이지만, 자동화 스크립트가 특정 시작점을 기대하는 경우 영향 가능.
- **react-window 활성 사용 제거**: 페이지네이션이 가상화를 대체. 의존성 자체는 잔존 (package.json 정리 보류 — 다른 컴포넌트 도입 시 재사용 여지).

---

## Next recommended actions

1. **Tag release candidate**: `[Unreleased]` 누적량이 상당함 (SPEC-WEB-005 v0.5.0 / SPEC-STORE-003 v0.2.0 / SPEC-CHART-001 / SPEC-NODE-002 / SPEC-NODE-003 / SPEC-AGENT-005 / SPEC-DEVICE-001). 다음 release tag 분리 검토.
2. **`react-window` 의존성 정리**: 활성 사용처 0건 → `package.json` 에서 제거 또는 사용처 재도입 검토. tech.md 의존성 표 동기화 필요.
3. **버킷 정렬 마이그레이션 노트**: 외부 클라이언트가 Store query 결과를 자동 처리하는 경우 첫 행 timestamp 변경에 영향 가능. 운영 가이드 또는 release note 추가 검토.
4. **DELETE 엔드포인트 권한 모델**: 키 초기화는 destructive operation. 향후 권한/감사 로그 도입 시 우선 대상.
5. **Plan / Acceptance 문서 업데이트**: 두 SPEC 의 plan.md / acceptance.md 는 본 sync 에서 손대지 않음. 회고 시 plan 가정 vs 실제 구현 매핑 검토 권장.
6. **Git commit**: orchestrator 가 별도 phase 에서 처리.

---

## Verification performed

- **CHANGELOG.md**: `## [Unreleased]` 헤더 구조 (`### 추가` / `### 변경` / `### 수정`) 보존 확인. 신규 항목이 각 섹션 상단에 위치하고 기존 항목은 그대로 뒤따름. Markdown 구조 정상.
- **SPEC-WEB-005 frontmatter**: 7개 필드 (`id`, `version`, `status`, `created`, `updated`, `author`, `priority`) 정상. `version: 0.5.0`, `updated: 2026-04-27` 반영. HISTORY 최상단 0.5.0 엔트리, 그 아래 0.4.0 / 0.3.0 / 0.2.0 / 0.1.0 순서 보존.
- **SPEC-STORE-003 frontmatter**: 8개 필드 정상. `version: 0.2.0`, `updated: 2026-04-27` 반영. HISTORY 최상단 0.2.0 엔트리 (불릿 형식) + 표 형식 0.2.0 / 0.1.0 행 동시 유지. 기존 EARS 모듈 (M1~M5), Specifications, TAG Traceability, Implementation Notes 미손상.
- **structure.md**: 기존 `web/` 섹션 컴포넌트 목록 보존 + 3개 신규 컴포넌트 (`ConfirmDialog`, `PromoteToStaticDialog`, `keyTagExtractor`) 추가. HTTP 엔드포인트 표에 DELETE 2건 추가, 신규 백엔드 메서드 서브섹션 추가, 버킷 정렬 정책 변경 노트 추가. 다른 디렉터리 섹션 (api/, configs/, deployments/) 미수정.
- **코드 미수정 확인**: `internal/`, `web/src/`, 테스트 파일, `.mcp.json`, 기존 untracked 파일 (`raw-*.txt`, `references/`, `web/coverage/`, `lgcp-valid.jsonl` 등) 미터치.
