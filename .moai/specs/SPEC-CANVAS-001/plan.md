# SPEC-CANVAS-001 — 구현 계획 (plan.md)

관련 SPEC: `.moai/specs/SPEC-CANVAS-001/spec.md`
개발 모드: hybrid (신규 순수 모듈·컴포넌트는 TDD, 기존 등록 6지점 수정은 DDD 특성 — 행위 보존)

## 기술 스택 (Tech Stack)

- React 19 (패널은 client 컴포넌트), TypeScript 5.9(엄격 타입, `satisfies`)
- Vite 6 + Vitest, React Testing Library
- Tailwind v4 (빈 상태·오류 배지·설정 UI 레이아웃)
- HTML Canvas 2D API (`CanvasRenderingContext2D`) — 도형 경로 + `fillText` + `measureText`
- `requestAnimationFrame` 렌더 루프 (본 SPEC 이 도입)
- 신규 의존성 **0**. 식 평가 라이브러리는 도입하지 않는다(규칙 표가 대신한다). 차트 라이브러리도 쓰지 않는다.
- 백엔드 변경 **0** (패널 config 는 Go 측 불투명 JSON)

## 재사용 자산 (Reconnaissance 검증 완료 — 인용)

| 자산 | 경로 | 재사용 / 참조 구분 |
|------|------|--------------------|
| 통합 시리즈 훅 | `web/src/pages/dashboard/panels/charts/usePanelSeriesData.ts` | **그대로 재사용**. store/TSDB/sysmetrics 3종이 이 훅 뒤에 있으므로 신규 데이터 경로를 만들지 않는다 |
| Canvas 렌더 선례 | `web/src/pages/dashboard/panels/heatmap/HeatmapCanvas.tsx` | **규율 재사용**(DPR 백킹 버퍼 스케일, `ResizeObserver` 재계산). 코드 복제가 아니라 같은 규율을 따른다 |
| 정규화 좌표 변환 | `web/src/pages/dashboard/panels/heatmap/stage.ts` | **좌표 규약 재사용**. 요소 좌표를 0..1 로 두어 패널 리사이즈 시 요소가 미끄러지지 않게 한다 |
| 관용 config 파서 패턴 | `web/src/pages/dashboard/panels/heatmap/heatmapConfig.ts` (`parseHeatmapConfig`) | **패턴 재사용**. 결측/손상/구버전 입력을 예외 없이 기본값 보정 |
| 가시성 게이팅 | `web/src/pages/dashboard/panels/charts/visiblePolling.ts` (`VisibilitySource`) | **그대로 재사용**. document 가시성 구독을 주입 가능한 인터페이스로 이미 분리해 두었다(훅 통합 검증 선례: `pollingVisibility.test.tsx`) |
| 임계값 어휘 선례 | `web/src/pages/dashboard/panels/charts/thresholdFill.ts`, GaugePanel thresholds, `web/src/pages/dashboard/panels/acControlColors.ts` | **어휘 참조**. 규칙 표가 새 문법이 아니라 기존 임계값 UI 와 같은 것으로 읽히게 한다 |
| 자족 도형 렌더 선례 | `web/src/pages/dashboard/panels/AgentStatusDiagram.tsx` | **참조**. 외부 차트 라이브러리 없이 직접 그리는 코드베이스 규약 |
| 패널 편집 툴킷 (부분) | `web/src/pages/dashboard/panels/charts/panelEditAlign.ts`, `panelEditSelection.ts` | **부분 재사용, 002 대상**. 두 모듈은 `K extends string` 제네릭 순수 계산(스냅·정렬 수학·선택 집합 전이)이라 재사용 가능하다. 그러나 **저장 축이 패널 상자 대비 백분율 오프셋이고 드래그 층이 DOM 기준**이므로 `PanelDragLayer.tsx` / `usePanelElementEdit.tsx` 자체는 canvas 에 그대로 쓸 수 없다 — 001 은 어느 쪽도 쓰지 않는다(수치 입력 배치) |
| 대시보드 에셋 API | `POST/GET /api/v1/dashboard-assets` (`internal/api/handler/dashboard_asset.go`), `web/src/pages/dashboard/panels/heatmap/svgAsset.ts` | **002 로드맵 참조만**. 배경 에셋은 002 범위. `svgAsset.ts` 는 업로드 SVG 를 `<img>` data-URL 로만 렌더해야 하는 이유(스크립트 격리)를 이미 문서화해 두었다 |

## 작업 분해 (Task Decomposition)

### Primary Goal (1차 목표) — 패널 등록 + config 스키마 + 정적 도형 렌더

우선순위 High. 이 단계가 끝나면 "데이터 없는 그림판"이 대시보드에 뜬다.

- T1. `canvasConfig.ts`(신규, TDD): `CanvasPanelConfig` 타입 + `parseCanvasConfig` + `buildDefaultCanvasConfig`.
  관용 파서를 **첫날에** 만든다 — 스키마가 002/003 을 거치며 자라기 때문이다(§위험 R3).
- T2. `canvasGeometry.ts`(신규, TDD): 정규화 ↔ px 투영, 도형별 경로 수치, 텍스트 정렬 기준점. DOM 무의존.
- T3. 패널 등록 6지점 수정(DDD, 행위 보존): `PanelType` 유니온 → `PANEL_DEFAULT_SIZES`(컴파일러가 강제) →
  `createDefaultPanel` → `AddPanelDialog` 카탈로그 행 → `renderDashboardPanel` switch → `PanelSettingsDialog`
  섹션 골격. i18n `ko.json`/`en.json` **양쪽**.
- T4. `drawElement.ts` + `CanvasSurface.tsx`: 정적 요소를 그리는 최소 렌더러. DPR·`ResizeObserver` 규율 적용.
  이 단계의 루프는 아직 없다(데이터 변경 시 1회 그리기).

### Secondary Goal (2차 목표) — 데이터 바인딩 + 규칙 표 + 문구

우선순위 High. 이 단계가 끝나면 패널이 "동적"이 된다.

- T5. `canvasRules.ts`(신규, **TDD 핵심**): `evaluateRules(value, rules, baseStyle) → ResolvedStyle`.
  first-match-wins, 미일치 시 기본 스타일 폴백, `nodata` 연산자, `between` 경계 포함, 부동소수 `eq/ne` 허용 오차.
- T6. `canvasText.ts`(신규, TDD): `{value}`/`{name}`/`{unit}` 단순 치환 + 결측 표기 + 미지 토큰 보존.
- T7. `CanvasPanel.tsx`: `usePanelSeriesData` 로 시리즈 획득 → 요소별 바인딩 최신값 추출 → T5/T6 결과를 렌더에 전달.
  시리즈 참조는 **동일성 키**로 하고 시리즈 이름 생성은 로케일에 의존하지 않는다(훅 경로 제약).
- T8. 설정 UI: 요소 목록 편집(추가·삭제·순서) + 요소별 기하 수치 입력 + 기본 스타일 + 바인딩 선택 +
  **규칙 표 편집기**(행 추가·삭제·순서 이동, 연산자 선택, 임계값, 패치). 라이브 미리보기 연결.

### Final Goal (최종 목표) — 트위닝 + 렌더 루프 + 견고성

우선순위 Medium. 애니메이션 (a) 와 그 루프 규율이 여기서 선다.

- T9. `canvasTween.ts`(신규, TDD): 이징 4종 + 색 RGB 보간 + 수치 보간 + 트윈 상태 진행.
  진행 중 목표가 바뀌면 **현재 보간값에서** 재출발(값 튐 방지).
- T10. `CanvasSurface.tsx` 에 rAF 루프 도입: 유휴 정지(트윈 없으면 프레임 미예약) + 가시성 게이팅
  (`VisibilitySource` 주입) + 다시 보일 때 1프레임 즉시 갱신.
- T11. 견고성(REQ-05): 요소 0개 빈 상태 / 시리즈 결측 폴백 / **폴링 실패 시 마지막 프레임 보존 + 오류 배지** /
  패널 리사이즈 시 요소 위치 유지. i18n 문구 포함.
- T12. 품질 게이트: 신규 코드 85%+ 커버리지(순수 모듈은 95% 이상 목표), LSP 0(tsc `--noEmit` + eslint),
  기존 웹 테스트 전량 green.

## 아키텍처 설계 방향

- **순수 모듈 분리가 이 SPEC 의 중심 규율이다.** 규칙 평가기(`canvasRules.ts`), 기하 수학(`canvasGeometry.ts`),
  트윈(`canvasTween.ts`), 문구 치환(`canvasText.ts`) 을 DOM 의존 없는 모듈로 두어 jsdom 없이 단위 테스트한다.
  이는 히트맵에서 `heatmapJoin.ts` / `idw.ts` 를 렌더 컴포넌트에서 떼어낸 것과 같은 판단이며, 그때 신규 코드
  커버리지 99% 를 가능하게 한 이유이기도 하다. 캔버스는 요소·규칙·트윈이 곱해지므로 그 필요가 더 크다.
- 렌더 층(`drawElement.ts`, `CanvasSurface.tsx`)은 **얇게** 유지한다. 계산은 위 순수 모듈이 끝내고, 렌더 층은
  받은 수치를 context 에 옮기기만 한다. 그래야 "무엇을 그릴지"의 버그와 "어떻게 그릴지"의 버그가 섞이지 않는다.
- 데이터는 신규 훅 없이 `usePanelSeriesData` 를 그대로 쓴다. 폴링·중단·소스 3종 분기는 이미 검증되어 있고,
  캔버스가 그것을 다시 만들 이유가 없다.
- config 는 불투명 JSON 이므로 백엔드 무변경. 요소·규칙·트윈은 전부 프론트 전용 필드다.
- 좌표는 정규화(0..1) 로 저장한다. 패널 그리드는 사용자가 수시로 크기를 바꾸는 곳이라, px 로 저장하면 요소가
  리사이즈마다 미끄러진다.

## 위험 분석 (Risk Analysis)

| # | 위험 | 영향 | 완화 |
|---|------|------|------|
| R1 | rAF 루프가 항상 돌면 패널 N개 × 60fps 로 유휴 CPU 를 태운다 | 대시보드 전체 발열·배터리 | **유휴 정지**(트윈 없으면 프레임 미예약)와 **가시성 게이팅**(`visiblePolling.ts` 의 `VisibilitySource` 재사용)을 기능이 아니라 REQ-05 의 금지 조항으로 못박았다. 두 규율 모두 인수 기준으로 검증한다(AC-E6) |
| R2 | 요소·규칙이 많을 때 프레임당 전면 재그리기 비용 | 트윈 중 끊김 | A2(요소 수백 이하) 전제에서 전면 재그리기가 dirty-rect 보다 단순하고 충분하다. 규칙 평가는 요소당 O(행 수)에 first-match 조기 종료. 상한 초과 시의 최적화는 003 이후로 이연 |
| R3 | config 스키마가 001→002→003 을 거치며 바뀌어 구버전 패널이 깨진다 | 사용자 대시보드 파손 | **첫날부터 관용 파서**(`parseCanvasConfig`). 알 수 없는 필드는 보존하지 말고 무시하되 결측은 기본값으로 채운다. `parseHeatmapConfig` 가 MVP config 를 002 필드 추가 후에도 읽어낸 선례가 있다 |
| R4 | `PanelSettingsDialog.tsx` 가 이미 8,250행인데 요소 목록 + 규칙 표 편집기가 더 붙는다 | 파일 비대·리뷰 난이도 | 캔버스 설정 UI 의 **본문은 `panels/canvas/` 하위 컴포넌트로 두고**, 다이얼로그에는 마운트 지점만 추가한다. 규칙 표 편집기는 요소 편집기와 분리된 컴포넌트로 둔다 |
| R5 | SPEC-CHART-004/005 가 미커밋 상태이며 패널 편집 툴킷(`panelEditAlign.ts`/`panelEditSelection.ts`/`PanelDragLayer.tsx`/`usePanelElementEdit.tsx`)을 일반화하는 중이다 | **002 의 API 전제가 흔들린다** | **001 은 이 툴킷에 의존하지 않는다**(캔버스 내 편집기가 없고 배치는 수치 입력이다). 따라서 순서 위험은 002 에만 걸린다 — 002 착수는 004/005 의 API 가 안정된 뒤로 둔다 |
| R6 | 규칙 표가 사용자에게 "또 다른 문법"으로 읽힌다 | 학습 비용·오설정 | 연산자·임계값·색 패치의 어휘와 UI 배치를 기존 임계값 UI(`thresholdFill.ts`, GaugePanel, `acControlColors.ts`)에 맞춘다. 새로 배우는 개념은 "위에서부터 첫 일치" 하나로 한정한다 |
| R7 | 텍스트가 도형 밖으로 넘치거나 잘린다(`measureText` 기반, 자동 줄바꿈 없음) | 판독 불가 | 정렬 기준점 계산을 순수 모듈로 두고 단위 테스트한다. 줄바꿈·말줄임은 001 범위 밖임을 명시하고 사용자가 `fontSize` 로 조절한다 |

## 후속 SPEC 로드맵

- **SPEC-CANVAS-002 — 캔버스 내 시각 편집기**: 드래그 배치, 크기 조절(resize handle), 정렬·스냅, z-order 조작,
  그룹, 배경 에셋. 001 의 `elements[]` 정규화 좌표 스키마를 그대로 소비하고 저술 수단만 바꾼다.
  Canvas 에서 요소 선택은 좌표 역산 **히트 테스트**이므로 001 이 이연한 그 문제가 여기서 풀린다.
  선행 조건: SPEC-CHART-004/005 의 편집 툴킷 API 안정화(§위험 R5). 배경 에셋은 기존
  `POST/GET /api/v1/dashboard-assets` 와 `svgAsset.ts` 의 data-URL 규율(업로드 SVG 인라인 금지)을 따른다.
- **SPEC-CANVAS-003 — 애니메이션 전량 + 고급 표현**: 반복 효과(점멸·맥동·회전·파선 흐름)와 값 구동 연속
  애니메이션(탱크 수위, RPM 비례 회전). 001 이 세운 rAF 루프·유휴 정지·가시성 게이팅 위에 프레임 계산을 얹고,
  001 의 트윈 엔진이 이미 기하 수치를 보간할 수 있으므로 값 구동은 "규칙 결과" 대신 "값 → 속성 사상"을
  목표값으로 넣는 형태가 된다. 고급 표현식 형태와 요소 복제도 여기서 다룬다.

두 후속 SPEC 은 본 SPEC 이 정의한 `CanvasPanelConfig`(특히 `elements[]` 와 `TweenSpec`)와 렌더 루프를 확장
지점으로 사용하며, 본 SPEC 에서는 파일을 생성하지 않는다(로드맵 기록만).
