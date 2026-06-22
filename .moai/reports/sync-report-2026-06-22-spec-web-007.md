# Sync Report — SPEC-WEB-007 (로컬 인스턴스 시스템 정보 표출)

- 날짜: 2026-06-22
- SPEC: SPEC-WEB-007 v0.2.0 — Self / Local Instance System Info Display
- 라이프사이클: Level 1 (spec-first)
- 모드: `/moai sync` auto (문서 동기화 전용, 코드 미수정)
- 브랜치: develop
- 동기화 언어: ko (conversation_language)

---

## 1. 요약

SPEC-WEB-007 구현(이미 완료, 품질 게이트 PASS)에 대한 문서 동기화를 수행했다.
코드 파일은 일절 수정하지 않았고, SPEC 문서 3종 / CHANGELOG / sync 리포트만 갱신·생성했다.

- spec.md 에 Implementation Notes 섹션 append (구현 요약 + 계획 대비 분기 5건 기록)
- spec.md / plan.md / acceptance.md frontmatter `status: draft → completed`, `updated: 2026-06-22`
- spec.md 마지막 Status 줄 `draft → completed (Level 1 spec-first, 구현 완료 2026-06-22)`
- CHANGELOG `[Unreleased]` 에 사용자 가시 기능 항목 추가
- README / .moai/project 갱신은 불필요로 판단 (사유 후술)

---

## 2. Phase 1.5 분기 분석 (계획 대비 실제 변경)

`git status --short` + 작업 대상 파일 diff 로 plan.md 의 예정 경로(TAG Traceability,
컴포넌트 구조)와 실제 변경 파일을 대조했다.

### 실제 변경 파일

| 파일 | 상태 | 계획 명시 |
| --- | --- | --- |
| `internal/api/dto/update.go` | 수정 (5필드 append) | O (plan/spec) |
| `internal/api/handler/system_update.go` | 수정 (Mode/StartedAt, Version()) | O |
| `internal/api/handler/system_update_test.go` | 수정 (table-driven) | O |
| `cmd/xflowd/main.go` | 수정 (buildSystemHandler 시그니처) | X (분기 #2) |
| `web/src/services/api/systemUpdate.ts` | 수정 (VersionInfo 확장) | O |
| `web/src/services/api/monitorService.ts` | 수정 (SystemMetrics + 훅) | O |
| `web/src/services/api/monitorService.test.tsx` | 신규 | O (검증 기준) |
| `web/src/components/system/SystemInfoCard.tsx` (+test) | 신규 | O |
| `web/src/components/system/SystemRuntimeCard.tsx` (+test) | 신규 | O |
| `web/src/lib/utils/formatUptime.ts` (+test) | 신규 | X (분기 #1) |
| `web/src/pages/system/SystemStatusPage.tsx` | 수정 | O |
| 기존 픽스처 보강 6개 (SystemVersionCard/UpdateDialog/useUpdateAvailableNotification/SystemStatusPage(.test/.integration.test)/systemUpdate.test) | 수정 | 회귀 방지 (DDD characterization) |

### 분기 사항 (5건) — 모두 검증 완료

1. **신규 유틸 파일 추가** (`web/src/lib/utils/formatUptime.ts`): plan.md 컴포넌트
   구조에 없던 파일. uptime 가독 형식 변환 + 단위 테스트 분리를 위한 의도된 추가.
   **정정**: `web/src/lib/utils/` 는 기존 디렉토리(cn.ts/format.ts/deviceLabels.ts 등 존재)임을
   `ls` 로 확인. 따라서 작업 지침 #5의 "신규 디렉토리" 우려는 해당 없음 — 파일 1개 추가뿐.
2. **main.go 시그니처 변경**: `buildSystemHandler(cfg, obs)` →
   `buildSystemHandler(cfg, obs, time.UnixMilli(daemonStartedAtMs))`. diff 로
   `Mode: cfg.RemoteManagement().Mode`, `StartedAt: startedAt` 와이어링 + `@SPEC:SPEC-WEB-007`
   주석 확인. uptime 정확도(프로세스 부팅 시각 재사용, epoch ms 규약 준수)를 위한 최소 변경.
3. **queryKey 결정**: `useSystemMetrics` 가 `queryKey: ['monitor', 'metrics']` 사용(코드 100줄
   확인). 기존 대시보드 메트릭 소비자와 캐시 공유 → 중복 폴링 회피. 기존 키 컨벤션 우선.
4. **SystemMetrics 인덱스 시그니처**: `[key: string]: unknown;` 추가 확인(코드 31줄).
   기존 `Record<string, unknown>` 소비자 호환 + `any` 미사용. 회귀 방지 트레이드오프.
5. **신규 외부 의존성 0**: acceptance.md 품질 게이트 충족 확인.

분기 분석 최종 결과: **계획 대비 분기 2건(main.go 시그니처, formatUptime 유틸 신설)은
모두 와이어링/테스트 분리상 필요한 의도된 최소 변경이며, 설계 의도(version 확장 / 카드 공존 /
5초 폴링 / 페이지 통합 / 시크릿 비노출 / 신규 의존성 0)를 위배하지 않는다.** spec.md
Implementation Notes 에 5건 모두 기록.

---

## 3. 상태 전환 결과

| 문서 | status | updated | 마지막 Status 줄 |
| --- | --- | --- | --- |
| spec.md | draft → **completed** | 2026-06-21 → **2026-06-22** | draft → **completed (Level 1 spec-first, 구현 완료 2026-06-22)** |
| plan.md | draft → **completed** | 2026-06-21 → **2026-06-22** | (해당 줄 없음) |
| acceptance.md | draft → **completed** | 2026-06-21 → **2026-06-22** | (해당 줄 없음) |

---

## 4. 문서 처리 내역

### CHANGELOG.md — 처리함

`[Unreleased]` 섹션 상단에 `### 추가 — 로컬 인스턴스 시스템 정보 표출 (self/local Identity +
Runtime)` 항목 추가. Keep a Changelog 형식 + 기존 항목의 "bold 한 줄 요약 + 들여쓴 하위 불릿"
스타일 준수. Identity 백엔드 확장 / 시크릿 비노출 / Runtime 5초 폴링 / UI 영역 추가 / 신규
의존성 0 / 관련 SPEC 을 요약.

### README.md — 생략

`README.md` 에 시스템 정보/`/admin/system` 관련 기존 섹션이 없음(grep 결과 0건). SPEC-WEB-007 은
기존 admin 페이지에 영역을 추가하는 비-breaking UI 확장으로, README 의 큰 구조 변경을 요하지 않음.
CHANGELOG 항목으로 사용자 가시성이 충분히 확보되어 README 갱신 생략.

### .moai/project/structure.md — 생략

신규 외부 의존성 0, 신규 디렉토리 0(formatUptime 은 기존 `web/src/lib/utils/` 에 추가),
주요 아키텍처 변경 없음. structure.md 는 `components/system` 개별 컴포넌트 파일을 나열하지 않는
추상 수준(grep 결과 system 컴포넌트/formatUptime/lib/utils 미기재)으로, 파일 수 개 추가는
구조 문서 갱신 임계를 넘지 않음. 따라서 생략.

---

## 5. TAG 정합성

- `@SPEC:SPEC-WEB-007` → spec.md
- `@PLAN:SPEC-WEB-007` → plan.md
- `@ACCEPTANCE:SPEC-WEB-007` → acceptance.md
- 구현 코드 `@SPEC:SPEC-WEB-007` 주석: cmd/xflowd/main.go (buildSystemHandler 와이어링) 등에서 확인

3개 SPEC 문서의 TAG 와 구현 코드 TAG 정합 유지. spec.md Implementation Notes 가 예정 경로와
실제 경로의 매핑을 보강하여 추적성 강화.

---

## 6. 품질 게이트 (구현 단계 결과 — 참고)

| 항목 | 기준 | 결과 |
| --- | --- | --- |
| TRUST 5 | 전 항목 PASS | PASS |
| LSP errors / type / lint | 0 / 0 / 0 | 0 / 0 / 0 |
| 신규 코드 커버리지 | >= 85% | 백엔드 Version() 94.7%, 프론트 신규 96~100% |
| 기존 코드 회귀 | 0 | 0 (characterization 통과) |
| 시크릿 노출 | 0 | 0 (테스트 강제) |
| 신규 외부 의존성 | 0 | 0 |
| AC 커버리지 | AC-1~AC-13 (AC-14 Optional) | AC-1~AC-14 전부 |

---

## 7. 제약 준수 확인

- 코드 파일(.go/.ts/.tsx) **미수정** — 문서/SPEC/리포트만 변경.
- 무관 파일 미변경: `.moai/memory/last-session-state.json`, `data.json` 손대지 않음.
- @SPEC/@PLAN/@ACCEPTANCE TAG 정합 유지.

---

## 8. 생성/갱신 산출물

- 갱신: `.moai/specs/SPEC-WEB-007/spec.md` (Implementation Notes append + status/Status)
- 갱신: `.moai/specs/SPEC-WEB-007/plan.md` (status/updated)
- 갱신: `.moai/specs/SPEC-WEB-007/acceptance.md` (status/updated)
- 갱신: `CHANGELOG.md` ([Unreleased] 항목 추가)
- 생성: `.moai/reports/sync-report-2026-06-22-spec-web-007.md` (본 리포트)
