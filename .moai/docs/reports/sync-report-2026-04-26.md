# Sync Report — 2026-04-26

- **Sync mode**: auto
- **Date**: 2026-04-26
- **SPECs synced**:
  - SPEC-WEB-005 (v0.4.0, status: completed)
  - SPEC-STORE-003 (v0.1.0, status: completed)
- **Quality status**: PASS (369 frontend tests, all backend packages pass)

---

## Files modified

| Path | Change |
|------|--------|
| `CHANGELOG.md` | `[Unreleased] / ### 추가` 상단에 SPEC-WEB-005 v0.4.0 + SPEC-STORE-003 v0.1.0 항목 추가, `### 변경` 에 Configure runtime / lazy resolver 항목 추가, `### 수정` 에 FormField boolean 기본값 항목 추가 |
| `.moai/specs/SPEC-WEB-005/spec.md` | `status: draft → completed`, `updated: 2026-04-26`, `## Implementation Notes` 섹션 추가 (Divergence + Architectural additions + Status) |
| `.moai/specs/SPEC-STORE-003/spec.md` | `status: draft → completed`, `updated: 2026-04-26`, `## Implementation Notes` 섹션 추가 (Divergence + Post-implementation fixes + Status) |
| `.moai/project/tech.md` | 프론트엔드 의존성 표에 `react-window 2.2.7` 추가 |
| `.moai/project/structure.md` | `web/` 섹션에 react-window, SeriesDataSource/CSV/StoreKeysEditor/TagFilterChips 재사용 컴포넌트, 신규 store HTTP 엔드포인트 3건 기재 |
| `README.md` | `주요 기능` 섹션에 TSDB/Store 데이터 뷰어 + Store 정적 키/태그 메타데이터 2줄 추가 |

## Files created

| Path | Description |
|------|-------------|
| `.moai/reports/sync-report-2026-04-26.md` | 이 동기화 리포트 |

## Sections skipped

- `.moai/project/product.md`: 검토 결과 "핵심 기능" 섹션은 사용자 노출 기능 리스트가 아닌 아키텍처 구성 요소(Flow Engine, Agent, Script Engine 등) 카탈로그 형식. SPEC-WEB-005 / SPEC-STORE-003 변경은 기존 Web Dashboard / Store System Agent 의 보강이므로 새 섹션을 추가하지 않고 README.md 의 "주요 기능" 섹션을 통해 사용자 노출 처리.

---

## Divergence summary

### SPEC-WEB-005 (v0.1.0 → v0.4.0)

- 파일 경로 컨벤션: `web/src/components/agents/` → `web/src/pages/agents/`, `web/src/api/` → `web/src/services/api/`
- 백엔드 query API 인터페이스 차이 흡수: 단일 series_key + RFC3339Nano + bucket → 다중 키 + epoch ms + N개 병렬 호출 + 클라이언트 merge 어댑터
- 시리즈 탭 통합 (v0.4.0): store 에이전트는 별도 시리즈 탭 대신 기존 "저장소" 탭에 데이터 보기 + 페이지네이션 통합. tsdb 타입은 별도 시리즈 탭 유지
- react-window v2.2.7 API: `FixedSizeList` → `List` + rowComponent 패턴
- 서버측 집계 graceful degradation: store 서버측 집계 4xx 시 클라이언트 집계 fallback
- 추가 추상화: `SeriesDataSource` (v0.2.0), `tsdbCsvExport.ts` (v0.3.0), `TagFilterChips` / `StoreKeysEditor` (v0.4.0)

### SPEC-STORE-003 (v0.1.0)

- `allow_dynamic_keys` 게이트를 `agentStore.Set/SetWithTTL` 직접 검사 대신 `NamespacedStore` 경계의 `keyGatekeeper` 인터페이스로 배치 (정적 키 이름이 namespace prefix 부착 전 user-facing 형식이어서 매칭 정확도 확보)
- `parseStoreConfig` 시그니처 변경: `[]StoreOption` → `([]StoreOption, error)` 검증 에러 명시 전파
- `api.Context.QueryValues(name) []string` 추가: 다중 `?tag=` 쿼리 파라미터 지원
- Post-implementation fix `e114781`: `UserStoreAgent.Configure` runtime 정책 반영 누락 수정 (`SetAllowDynamicKeys`, `SetStaticKeys` runtime setter 추가)
- Post-implementation fix `329a3d9`: `NodeStoreAdapter` 재시작 후 고립 수정. 생성 시점 store 스냅샷 → resolver 함수 패턴

---

## Next recommended actions

1. **Tag release candidate**: `[Unreleased]` 섹션을 새 release tag (예: `v0.X.0`) 로 분리. SPEC-WEB-005 / SPEC-STORE-003 / SPEC-CHART-001 / SPEC-NODE-002 / SPEC-NODE-003 / SPEC-AGENT-005 가 unreleased 에 누적되어 있음.
2. **Plan 및 acceptance 문서 업데이트 검토**: `plan.md` / `acceptance.md` 에는 본 sync 에서 손대지 않음. 향후 SPEC 회고/감사 시 plan 의 가정과 실제 구현 간 차이를 acceptance evidence 와 매핑하면 traceability 강화.
3. **TSDB 타입 에이전트 도입 시 SPEC-WEB-005 v0.5.0**: 현재 store 에이전트 위주로 통합되어 있고 별도 시리즈 탭 분기 로직이 잠재 미사용 상태. tsdb 에이전트 정식 도입 시 분기 검증 + 테스트 추가 필요.
4. **Store 정적 키 마이그레이션 가이드**: `allow_dynamic_keys=false` 운영 모드 전환 시 기존 동적 생성 키를 정적 정의로 흡수하는 스크립트/가이드 검토. 현재는 Configure runtime 적용 가능하나 운영 절차 문서 부재.
5. **Git commit**: orchestrator 가 별도 phase 에서 처리.

---

## Verification performed

- CHANGELOG.md: `[Unreleased]` 섹션 헤더 구조 (`## [Unreleased]` / `### 추가` / `### 변경` / `### 수정`) 보존 확인. 기존 SPEC-CHART-001 항목이 새 두 SPEC 항목 뒤에 그대로 위치. Markdown 구조 정상.
- SPEC-WEB-005 frontmatter: `id`, `version`, `status`, `created`, `updated`, `author`, `priority` 7개 필드 정상. `status: completed` 반영, `updated: 2026-04-26` 반영.
- SPEC-STORE-003 frontmatter: `id`, `title`, `version`, `status`, `created`, `updated`, `author`, `priority` 8개 필드 정상. `status: completed` 반영, `updated: 2026-04-26` 반영.
- HISTORY 블록 / 기존 EARS 요구사항 / 기존 Resolved Defaults / Library Constraints 미손상 확인.
- 백엔드 `internal/`, 프론트엔드 `web/src/`, 테스트 파일, `.mcp.json`, 사전 존재 untracked 파일 미수정 확인.
