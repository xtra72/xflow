# Sync Report — 히트맵 좌표계·배치 편집 결함 수정 + 범례 직접 조작

- 실행: `/moai sync` (auto 모드)
- 브랜치: `feature/SPEC-TSDB-002`
- 범위: 커밋 `33671218`..`8b3fd5cb` (9개, 사용자 보고 기반 연속 수정 — SPEC 없음)

## 1. 품질 검증 (Phase 0.5)

프로젝트 언어 2종 감지(Go: `go.mod`, TypeScript: `web/tsconfig.json`). 이번 변경에 Go 파일은 **0개**이지만 회귀 확인을 위해 함께 돌렸다.

| 검사 | 명령 | 결과 |
|---|---|---|
| 웹 테스트 | `npx vitest run` | **378 files / 5923 tests 통과** (exit 0) |
| 웹 타입 | `npx tsc --noEmit` | exit 0 |
| 웹 린트 | `npx eslint src` | exit 0 — 0 error / 10 warning |
| Go 빌드 | `go build ./...` | exit 0 |
| Go 정적검사 | `go vet ./...` | exit 0 |

린트 경고 10건은 모두 이번 변경과 **무관한 기존 파일**이다: `panels/gauge/gaugeShapes.tsx`, `panels/sysmetrics/SysMetricsItemView.tsx`, `panels/sysmetrics/SysMetricsNetworkPanel.tsx`.

**종합: PASS**

## 2. 변경 요약

18개 파일(소스 8 · 테스트 8 · i18n 2). 신규 모듈 1개(`svgAsset.ts`), 신규 테스트 파일 3개.

| 커밋 | 내용 |
|---|---|
| `33671218` | 마커 화이트리스트 ↔ 좌표 쓰기 키 공간 불일치로 배치한 포인트가 사라지던 문제 |
| `b9206583` | 판독값 없으면 팔레트가 비어 배치 불가 + 마커 대신 도면이 끌려가던 포인터 우선순위 |
| `bd11441e` | 범례 자리·크기를 드래그로, 글자 크기·색 설정 추가 (드롭박스 2종 제거) |
| `6de931b0` | 패널 리사이즈 시 레거시 좌표 재환산으로 센서가 미끄러지던 문제 |
| `8e19ca39` | 종횡비 미상 시 스테이지가 패널 전체로 퇴화 (`onLoad` 확보) |
| `67b64b11` | 캐시된 이미지는 `load` 이벤트가 오지 않아 종횡비를 놓치던 문제 |
| `cb06de38` | 읽은 원본 크기를 `floor_plans[0].natural_*` 로 영속 |
| `4b625ac6` | 대시보드·미리보기 두 경로 회귀 테스트 |
| `8b3fd5cb` | SVG 도면의 늘려서 채우기·종횡비 (근본 원인) |

## 3. SPEC 정합성 (Phase 1.5)

이번 작업은 **SPEC 실행이 아니라 사용자 보고 기반 결함 수정**이다. 대응 SPEC 문서(`SPEC-HEATMAP-PANEL-001/002/003`)는 이미 완료 상태이며, 이번 변경은 그 산출물의 결함을 고친 것이라 SPEC 본문을 되돌려 고치지 않았다(Level 1 spec-first — 완료된 SPEC 은 유지보수 대상이 아니다).

배치 편집·범례·도면 배경은 002 의 산출물이므로, 실제 구현 상태는 `structure.md` 의 신규 항목이 정본으로 기술한다.

## 4. 문서 동기화 (Phase 2)

| 문서 | 조치 | 근거 |
|---|---|---|
| `CHANGELOG.md` | **이미 최신** | 커밋마다 `[Unreleased]` 에 항목 추가 완료(9건) |
| `.moai/project/structure.md` | **갱신** | 신규 순수 모듈(`svgAsset.ts`) + 좌표계/포인터/범례 변경을 기술. 기존 heatmap 절이 모듈 파일을 열거하는 문서라, 빼면 실제와 어긋난다 |
| `.moai/project/product.md` | 변경 없음 | 신규 제품 기능이 아니라 기존 기능의 결함 수정 + 조작 방식 개선. 범례 조작은 structure 층위에서 기술 |
| `.moai/project/tech.md` | 변경 없음 | 신규 의존성·기술 스택 0 |
| `docs/guides/chart-panel-flow.md` | 변경 없음 | 히트맵 범례를 다루지 않는다(파이 차트 `show_legend` 만 언급) — 제거한 드롭박스에 대한 문서 부채 없음 |
| `README.md` | 변경 없음 | 패널 내부 동작 변경으로 README 범위 밖 |

## 5. Git 상태 (Phase 3)

- `git_strategy.mode: personal`, `automation.auto_push: false`, `automation.auto_pr: false` → **자동 push/PR 하지 않음**(설정 준수).
- 원격 브랜치 `origin/feature/SPEC-TSDB-002` **없음**(미푸시).
- `origin/main...HEAD` = `1  409` — 로컬 409 앞섬, origin/main 1 앞섬(**다이버전스**). PR 전 rebase 여부 결정 필요.
- `gh` CLI **미인증**(`gh auth login` 필요) → PR 생성 불가.

## 6. 남은 사항

- 워킹 트리 미커밋: `.moai/memory/last-session-state.json`(수정), `references/modbus-device/`(미추적) — 이번 작업과 무관해 손대지 않았다.
- 브라우저 실기 확인은 사용자가 완료("성공").
