# SPEC-CANVAS-002 — 구현 계획 (plan.md)

관련 SPEC: `.moai/specs/SPEC-CANVAS-002/spec.md`
개발 모드: hybrid (신규 순수 모듈·오버레이는 TDD, 001 산출물 수정은 DDD 특성 — 행위 보존)

## 기술 스택 (Tech Stack)

- React 19 (패널·오버레이는 client 컴포넌트), TypeScript 5.9(엄격 타입, 판별 합집합 좁히기)
- Vite 6 + Vitest, React Testing Library
- Tailwind v4 (오버레이 핸들·팔레트·격자 레이아웃)
- **Pointer Events API** (`pointerdown`/`pointermove`/`pointerup`/`pointercancel` + `setPointerCapture`)
- HTML Canvas 2D API — 001 의 렌더 경로를 **그대로** 쓴다. 002 는 캔버스에 아무것도 새로 칠하지 않는다.
- 신규 의존성 **0**. 드래그 라이브러리·도형 편집 라이브러리를 넣지 않는다.
- 백엔드 변경 **0** (config 스키마를 넓히지 않는다 — 배경 에셋을 005 로 뺀 것이 이 성질을 지킨다)

## 재사용 자산 (Reconnaissance 검증 완료 — 인용)

| 자산 | 경로 | 재사용 / 참조 구분 |
|------|------|--------------------|
| 좌표 투영 | `panels/canvas/canvasGeometry.ts` (`projectBox`/`projectLine`/`projectPoint`/`ellipseParams`/`resolveTextOrigin`/`labelAnchor`) | **그대로 재사용**. 히트 테스트는 이 투영의 **순방향 결과를 px 공간에서 판정**하는 것이므로 역산 함수를 새로 만들지 않는다. 오버레이의 핸들 좌표도 같은 함수에서 나온다 — 렌더·히트·핸들이 한 투영을 공유하면 셋이 갈라질 수 없다 |
| 편집 모드 게이팅 | `panels/PanelEditToggle.tsx` (`usePanelEditMode`) | **그대로 재사용**. 세 겹 게이팅(`canEdit` / `dashboardEditMode` / `forced`)은 히트맵·게이지·통계·바·파이가 이미 공유하는 규칙이다. 캔버스만 다른 규칙을 두면 사용자가 패널마다 다르게 배운다 |
| 격자 오버레이 | `PanelEditGrid.tsx` | **그대로 재사용**. `repeating-linear-gradient` + `absolute inset-0` + `pointer-events-none` 이라 좌표 변환이 아예 필요 없다. 신규 격자 컴포넌트를 만들지 않는다 |
| 스냅·정렬 순수 계산 | `panels/charts/panelEditAlign.ts` (`GRID_STEP_PERCENT` · `Box` · `PanelElementBox<K>` · `AlignAxis` · `AlignMode` · `snapOffsetToGrid` · `computeAlignPatches<K>`) | **재사용(변환 비용 있음)**. DOM 무의존이고 `K extends string` 제네릭이라 요소 id 로 그대로 들어간다. 정규화↔백분율은 ×100. **대가 셋**(상한 무한대 · 중심 기준 스냅 · `base.offset = 0`)은 spec.md §스냅·정렬 재사용의 대가에 적었다 |
| 선택 모델 | `panels/charts/panelEditSelection.ts` (`PanelSelection<K>` · `EMPTY_SELECTION` · `nextSelection<K>`) | **그대로 재사용**. "선택은 런타임 상태이며 config 에 저장하지 않는다" 는 규율도 함께 물려받는다 |
| 무리 이동 죄기 | 같은 파일의 `clampGroupDelta` · `GroupMember` | **의도적으로 쓰지 않는다.** 캔버스에는 오프셋 상한이 없으므로(clamp 하지 않는다) 이 함수는 무동작이 된다. 아무 일도 하지 않는 호출은 읽는 사람에게 상한이 있다고 거짓말한다 |
| 백분율 도우미 | `panels/charts/panelGeometry.ts` (`pixelsToPercent` · `clampPercentOffset` · `PANEL_OFFSET_LIMIT`) | **간접 재사용**(위 두 모듈이 내부에서 쓴다). `clampPercentOffset(v, Infinity) === v` 임을 확인했으므로 상한 무력화가 성립한다 |
| Canvas 위 DOM 오버레이 선례 | `panels/heatmap/SensorPlacementOverlay.tsx` · `panels/heatmap/FloorPlanTransformOverlay.tsx` · `panels/heatmap/placement.ts` | **규율 참조**. Canvas 2D 표면 위에 편집 손잡이를 DOM 으로 얹는 갈래를 이 코드베이스가 이미 택했다. 핸들 어휘(네 모서리, 커서 모양)도 여기서 가져온다 |
| 요소 목록 편집기 | `panels/canvas/CanvasElementsEditor.tsx` (접이식 행 · `expandedIds` · `addElement` · `newElement`/`seedOffset` · `moveAt`) | **그대로 재사용**. 팔레트는 `addElement` 를 부르고, 캔버스 선택은 `expandedIds` 를 조종하며, z-order 는 `moveAt` 을 부른다. **새 생성·정렬 경로를 만들지 않는다** |
| DOM 드래그 층 | `PanelDragLayer.tsx` · `usePanelElementEdit.tsx` | **재사용 불가 — 확인됨.** 전자는 `data-panel-drag`/`data-panel-resize` 를 단 **DOM 노드**를 잡고(캔버스 요소는 칠해진 픽셀이라 잡을 노드가 없다), 저장 축이 **패널 상자 대비 백분율 + 절대 px 크기**로 캔버스의 정규화 0..1 과 다르다. 후자는 정렬 대상을 `querySelector('[data-panel-drag=...]')` 로 찾는다. **다만 두 파일의 결정은 참조한다** — "아무 데나 잡아도 끌리면 미리보기의 다른 조작과 부딪힌다", "편집이 꺼져 있으면 표시만 남는다", "글자 요소의 크기 핸들은 `fontSize` 를 바꾼다" |
| 대시보드 에셋 API | `POST/GET /api/v1/dashboard-assets` · `panels/heatmap/svgAsset.ts` · `useFloorPlanSources.ts` | **본 SPEC 에서 쓰지 않는다.** 배경 에셋은 SPEC-CANVAS-005 로 분리했다(spec.md §개요의 근거 4가지) |
| 선행 SPEC 상태 | `.moai/specs/SPEC-CHART-004/spec.md` · `SPEC-CHART-005/spec.md` | **`status: completed` 확인.** 001 §위험 R5("002 착수는 004/005 API 안정화 후")가 해소되었다 |

## 작업 분해 (Task Decomposition)

### Primary Goal (1차 목표) — 히트 테스트 + 선택 + 오버레이 골격 + 드래그 이동

우선순위 High. 이 단계가 끝나면 **캔버스 위의 도형을 눌러 고르고 끌어 옮길 수 있다** — 사용자 요구
세 문장 중 둘의 절반이 여기서 선다.

- T1. `canvasHitTest.ts`(신규, **TDD 핵심**): `HIT_TOLERANCE_PX` · `CanvasHit` · `hitTest`.
  도형 4종 판정 + 배열 역순(위가 이긴다) + `visible === false` 건너뛰기 + 퇴화 도형 폴백 +
  **`textBaseline='middle'` 을 반영한 텍스트 상자**. DOM 무의존이므로 jsdom 없이 전량 단위 테스트한다.
- T2. `drawElement.ts` 수정(DDD, 행위 보존): `drawElements` 의 **반환 타입만** `void → Record<string, number>`.
  인자·`DrawContext2D`·그리기 순서·그리기 결과 불변. 기존 테스트가 한 건도 바뀌지 않아야 한다.
- T3. `CanvasSurface.tsx` 수정(DDD, 행위 보존): `overlay` 렌더 prop(`{ stage, textWidths }`) **하나**만
  더한다. **미지정이면 001 과 동작이 완전히 같다** — 이 무동작 보장을 테스트로 먼저 못박고 시작한다.
  (착수 시점에는 포인터 통과 슬롯 넷도 함께 달았으나, T5 가 포인터를 오버레이 루트에서 받기로
  확정하면서 호출부가 없는 API 로 남아 걷어냈다 — spec.md HISTORY 0.2.0.)
- T4. `canvasEditContext.tsx`(신규): 선택 상태 + 캔버스 자동 펼침 id. provider 없이도 동작하는 기본값
  (대시보드에 놓인 패널에는 목록 편집기가 없다).
- T5. `CanvasEditOverlay.tsx`(신규) 1차: 선택 외곽선 + `pointerdown` 히트 분기(**히트 없으면 이벤트를
  소비하지 않는다**) + `setPointerCapture` 드래그 이동 + 무리 이동.
- T6. `CanvasPanel.tsx` 수정: `usePanelEditMode` 배선, `onConfigChange` 를 **실제로 쓴다**,
  오버레이 마운트. 등록 6지점은 건드리지 않는다.

### Secondary Goal (2차 목표) — 크기 조절 + 팔레트 + 클릭 → 속성 편집

우선순위 High. 이 단계가 끝나면 사용자 요구 세 문장이 **전부** 화면에 있다.

- T7. `canvasEditGeometry.ts`(신규, **TDD 핵심**): 종류별 핸들 집합과 핸들 px 좌표, `moveGeometry`,
  `resizeBox`(8핸들 + 뒤집힘 정규화 + Shift 종횡비), `resizeLine`(끝점 2 + Shift 각도 죔),
  `patchNodeGeometry`(기하 쓰기 단일 통로 — REQ-06).
- T8. 오버레이 2차: 핸들 렌더(초점 가능·`aria-label`)와 드래그. `text` 는 **글자 크기 핸들 하나**이며
  `style.fontSize` 를 바꾼다(기하가 아니다).
- T9. 도형 팔레트: 오버레이 안 떠 있는 띠. 각 버튼은 목록 편집기의 `addElement(kind)` 를 **그대로**
  부르고, 추가로 새 요소를 **선택**한다. 목록의 기존 버튼은 그대로 남는다.
- T10. 선택 → 목록 편집기 연동: 단일 선택 시 해당 행 펼침 + 스크롤, **캔버스가 펼친 행은 하나뿐**,
  **손으로 펼친 행은 접지 않음**, **다중 선택 시 자동 펼침 없음**.
- T11. `PanelSettingsDialog.tsx` 수정: 두 마운트 지점을 provider 로 감싸고 미리보기에 `forced` 를 넘긴다.
  **본문은 더하지 않는다**(001 §위험 R4).

### Final Goal (최종 목표) — 스냅 · 정렬 · z-order · 접근성 · 견고성

우선순위 Medium. 001 로드맵이 002 에 배정한 나머지와 품질 게이트가 여기서 닫힌다.

- T12. 스냅: `snapOffsetToGrid` 래퍼(**상한 무한대**, 중심 기준, `base.offset = 0`) + `PanelEditGrid`
  재사용 + 격자 토글.
- T13. 정렬: `computeAlignPatches` 래퍼(투영 px 상자 → 백분율 오프셋 → ÷100 정규화 델타). 선택 2개 이상일 때만.
- T14. z-order: 캔버스에서 앞/뒤로 보내기. 목록 편집기의 **배열 순서 이동과 같은 연산**을 부른다.
- T15. 접근성(REQ-01): 방향키 미세 이동 · Shift+방향키 한 칸 이동 · 핸들 초점·`aria-label` ·
  **수치 입력 상시 유지** 확인. i18n `ko.json`/`en.json` 양쪽, 키 이름에 점 없음.
- T16. 견고성(REQ-05) + 품질 게이트: 유휴 정지 회귀 테스트(선택·호버가 프레임을 요청하지 않음),
  빈 지점 비가로채기, clamp 금지, `pointercancel` 확정, 신규 코드 85%+ 커버리지(순수 모듈 95%+),
  LSP 0, 001 테스트 스위트 무수정 전량 green.

## 아키텍처 설계 방향

- **순수 모듈 분리가 001 에서 이어지는 중심 규율이다.** 히트 판정(`canvasHitTest.ts`)과 편집 수학
  (`canvasEditGeometry.ts`)을 DOM 무의존으로 두어 jsdom 없이 단위 테스트한다. 001 이 규칙·기하·트윈·문구를
  그렇게 갈라 두어 커버리지를 얻은 것과 같은 판단이며, 편집기는 **입력이 포인터라 상태가 더 많으므로**
  그 필요가 더 크다.
- **오버레이는 DOM, 캔버스는 그대로.** 002 는 캔버스에 한 픽셀도 새로 칠하지 않는다. 이 결정이 001 의
  rAF 유휴 정지·가시성 게이팅 규율을 **손대지 않고** 지나가게 하는 유일한 방법이다(spec.md §선택 오버레이).
- **측정원과 투영은 각각 하나다.** 스테이지 크기는 `CanvasSurface` 의 `ResizeObserver` 하나, 투영은
  `canvasGeometry` 의 함수 하나. 오버레이는 둘 다 넘겨받는다. 두 벌이 되는 순간 핸들이 도형에서 미끄러지고,
  그 버그는 "가끔 어긋난다" 로만 보고되어 원인을 찾기 어렵다.
- **생성·정렬·순서 경로는 목록 편집기가 이미 가진 것을 부른다.** 팔레트는 `addElement`, z-order 는
  `moveAt`. 캔버스에 두 번째 규칙을 만들면 "어디서 했는가" 에 따라 결과가 달라진다.
- **config 스키마를 넓히지 않는다.** 기존 `geometry` 와 `style.fontSize` 를 다른 수단으로 쓸 뿐이다.
  그래서 백엔드 변경이 없고, 001 이 쓴 config 가 002 를 거쳐도 왕복에 값이 바뀌지 않는다.
- **004 가 얹힐 자리를 미리 비운다.** 히트 결과 레코드 · 노드 id 키잉 · 기하 쓰기 단일 통로 · "그룹이
  드래그 단위" 규칙 명문화. 넷 다 지금 값이 싸고 나중에 비싸다.

## 위험 분석 (Risk Analysis)

| # | 위험 | 영향 | 완화 |
|---|------|------|------|
| R1 | 오버레이와 캔버스가 **서로 다른 좌표**를 믿어 핸들이 도형에서 미끄러진다 | 조작 불가에 가까운 어긋남. "가끔 어긋난다" 로만 보고되어 원인 추적이 어렵다 | 측정원 하나(`CanvasSurface` 의 `ResizeObserver`) + 투영 하나(`canvasGeometry`). 오버레이는 스테이지 크기를 **스스로 재지 않는다**. AC-E2 가 핸들 px 와 투영 px 의 일치를 검증한다 |
| R2 | 캔버스 드래그가 미리보기의 **휠 확대·패널 크기 조절**과 부딪힌다 | 미리보기에서 도형을 끌면 패널이 함께 움직이는 종류의 결함 | `PanelDragLayer` 가 같은 이유로 택한 규칙을 그대로 쓴다 — **히트가 있을 때만 이벤트를 소비**하고, 빈 지점 누름은 소비하지 않는다. 핸들·팔레트는 자기 `pointer-events-auto` 로 캔버스에 닿지 않는다. AC-E3 가 검증한다 |
| R3 | 편집기 상태가 **rAF 루프를 깨우는 새 경로**를 만든다 | 001 이 금지 조항으로 못박은 유휴 정지가 무너져 대시보드 전체가 발열·배터리를 태운다 | 오버레이를 DOM 에 두어 선택·호버·핸들이 캔버스 props 를 건드리지 않게 한다. 깨우기 경로는 종전의 props 변경 하나뿐이다. AC-E4 가 "선택·호버 시 프레임 요청 0" 을 직접 센다 |
| R4 | **004 와 같은 파일을 고친다**(`drawElement.ts` · `CanvasSurface.tsx` · `CanvasPanel.tsx` · `CanvasElementsEditor.tsx`) | 순서가 뒤집혀 004 가 002 위에 오므로 충돌·재작업 | 002 의 변경을 **덧붙임(additive)** 으로 한정하고 명세에 이름으로 적는다: `drawElements` 는 **반환 타입만**, `CanvasSurface` 는 **선택 prop `overlay` 하나만**(HISTORY 0.2.0 — 포인터 통과 슬롯 넷은 호출부가 없어 걷어냈다), `drawElement` 의 인자·`DrawContext2D` 는 불변. 여기에 REQ-06 의 네 가지 전방 호환(히트 레코드 · 노드 id 키잉 · 기하 쓰기 단일 통로 · 그룹이 드래그 단위)을 더한다 |
| R5 | `PanelSettingsDialog.tsx`(8,000행+)가 또 자란다 | 001 §위험 R4 의 재발 | 오버레이·팔레트·provider 본문을 전부 `panels/canvas/` 아래에 두고, 다이얼로그에는 **감싸는 줄과 `forced` prop** 만 더한다 |
| R6 | 스냅 재사용 시 **상한 무한대를 잊는다** | 캔버스 드래그가 스테이지의 40% 지점에 **조용히** 붙잡힌다. 오동작이 아니라 "왜 더 안 가지" 로 보인다 | 래퍼 함수 하나(`snapDelta`)만 상한을 넘기게 하고 호출부가 원 함수를 직접 부르지 못하게 한다. AC-E5 가 스테이지 밖 드래그로 검증한다 |
| R7 | 크기 조절로 **음수 크기 박스**가 저장된다 | 잡은 핸들이 커서에서 떨어져 나가고, 이후 편집이 예측 불가가 된다 | 쓰기 직전에 박스를 **정규화**한다(`x = min`, `w = abs`). 001 이 음수 크기를 그릴 수 있게 해 둔 것은 **읽기 경로의 견고성**이지 편집기가 만들 값이 아니다 |
| R8 | 텍스트 히트 상자의 **수직 기준**을 상단으로 착각한다 | 글자를 눌러도 잡히지 않고, 빈 곳을 누르면 잡힌다 | `drawElement.TEXT_BASELINE === 'middle'` 이므로 기준점이 상자의 **세로 중심**이다. 순수 모듈에 두고 AC-02 가 위·아래 경계를 각각 확인한다 |
| R9 | 요소가 많을 때 히트 테스트 비용 | 포인터 반응 지연 | 히트 테스트는 **프레임당이 아니라 포인터 이벤트당 1회**이고 선형 순회다(가정 A10). 100 요소 × pointerdown 1회는 무시 가능하다. 공간 색인은 도입하지 않는다 — 지금 필요 없는 자료구조는 004 의 중첩이 오면 다시 짜야 한다 |
| R10 | 드래그가 **유일한 저술 수단**이 되어 포인터를 쓰지 못하는 사용자가 배제된다 | 접근성 후퇴 | 수치 입력 유지를 REQ-01(항상)과 REQ-05(금지)에 **양쪽으로** 못박고, 핸들을 초점 가능한 요소로 만들며, 방향키 등가 조작을 둔다. AC-08 이 검증한다 |
| R11 | 배경 에셋 분리로 004 문서의 "그림이 필요한 자리는 002 가 이미 맡고 있다" 가 어긋난다 | 문서 간 모순 | 본 SPEC 이 근거와 함께 명시하고(§개요), 004 의 §순서·해당 문장은 **후속 편집 대상**으로 남긴다. **004 문서는 본 SPEC 에서 수정하지 않는다.** 004 의 (B) 기각 근거(규칙이 부품을 지목해야 한다)는 배달 시점과 무관하므로 약화되지 않는다 |

## 후속 SPEC 로드맵

- **SPEC-CANVAS-005 — 배경 에셋 레이어**(신설): 도면·사진을 캔버스 배경으로 깐다. 001 로드맵이 002 에
  배정했으나 축이 달라 분리했다(spec.md §개요의 근거 4가지 — 비동기 이미지 로드가 rAF 유휴 정지 규율과
  충돌, 인증 자산 왕복, `svgAsset.ts` 의 `preserveAspectRatio`·`naturalWidth` 함정, 256KB snapshot 예산).
  기존 `POST/GET /api/v1/dashboard-assets` 와 `svgAsset.ts` 의 data-URL 규율(업로드 SVG 인라인 금지)을
  따르며, `DrawContext2D` 에 `drawImage` 를 들일지 아니면 캔버스 **뒤에 `<img>` 층**을 깔지가 그 SPEC 의
  중심 설계 결정이 된다(후자면 001·004 의 금지 조항을 건드리지 않고 끝난다).
- **SPEC-CANVAS-004 — 설비 심볼 라이브러리**: 002 다음이다. 002 가 REQ-06 으로 비워 둔 자리
  (`CanvasHit.partId` · 노드 id 키잉 · `patchNodeGeometry` 단일 통로 · "그룹이 드래그 단위" 규칙)에
  `group` 노드를 얹는다. 004 문서의 §순서 절은 002 를 뒤에 두고 쓰였으므로 후속 편집이 필요하다.
- **SPEC-CANVAS-003 — 애니메이션 전량 + 고급 표현**: 반복 효과와 값 구동 애니메이션, 요소 복제.
  002 의 선택 모델이 서면 "고른 것을 복제" 가 자연스러운 조작이 되므로 003 의 복제는 002 위에 얹힌다.

세 후속 SPEC 은 본 SPEC 이 정의한 히트 테스트·편집 수학·오버레이 슬롯을 확장 지점으로 사용하며, 본
SPEC 에서는 파일을 생성하지 않는다(로드맵 기록만).
