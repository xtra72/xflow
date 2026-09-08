# SPEC-CANVAS-004 — 구현 계획 (plan.md)

관련 SPEC: `.moai/specs/SPEC-CANVAS-004/spec.md`
선행 SPEC: `.moai/specs/SPEC-CANVAS-001/spec.md` (완료 — 본 SPEC 이 확장하는 토대)
개발 모드: hybrid (신규 순수 모듈·카탈로그는 TDD, 001 산출물 수정은 DDD 특성 — 행위 보존)

## 기술 스택 (Tech Stack)

- React 19 (패널·설정 UI 는 client 컴포넌트), TypeScript 5.9(엄격 타입, 판별 유니온)
- Vite 6 + Vitest, React Testing Library
- Tailwind v4 (심볼 고르기 격자·그룹 접힘 행 레이아웃)
- HTML Canvas 2D API — **001 이 쓰는 멤버만** 쓴다. `drawImage` 는 쓰지 않는다(SPEC §개요 (B) 기각).
- 신규 의존성 **0**. 심볼이 원시형 합성이라 아이콘 라이브러리·SVG 파서·식 평가기 어느 것도 필요 없다.
- 백엔드 변경 **0** (카탈로그는 코드 내장, 배치 결과는 기존 불투명 JSON config)

## 재사용 자산 (Reconnaissance 검증 완료 — 인용)

| 자산 | 경로 | 재사용 / 참조 구분 |
|------|------|--------------------|
| 규칙 평가기 | `canvas/canvasRules.ts` (`evaluateRules` · `matchesRule` · `mergePatch`) | **그대로 재사용 + 형제 함수 1개 추가**. `evaluateRules` 는 시그니처·동작 불변이며 캐스케이드 전체가 그 `baseStyle` 인자 안에서 표현된다. 그룹 규칙 패치를 기본 스타일과 **분리해** 얻어야 해서 `matchRulePatch` 를 더하고(이미 export 된 `matchesRule` 의 1줄 조합), `mergePatch` 는 **export 만** 추가한다. 두 함수의 합치 항등식을 테스트로 못박는다 |
| 관용 파서 | `canvas/canvasConfig.ts` (`parseCanvasConfig` · `parseElements` · `parseBoxGeometry`) | **확장 + 내부 재사용**. `parts` 파싱은 기존 `parseElements` 를 그대로 부르고 `group` 항목만 버린다 |
| 좌표 투영 | `canvas/canvasGeometry.ts` (`projectBox`/`projectLine`/`projectPoint`) | **불변 + 일반화**. `project*In` 3종을 더하고, 001 의 3종이 그 특수 사례임을 항등식 테스트로 못박는다 |
| 트윈 엔진 | `canvas/canvasTween.ts` (`beginTween`/`sampleTween`/`retargetTween`) | **그대로 재사용**. 장부 키만 복합 키가 된다. 엔진 자체는 손대지 않는다. 그룹 규칙이 뒤집혀도 트윈은 부품별로 서며, `sameStyle` 판정이 실제로 달라진 부품만 걸러 낸다 |
| 렌더 층 | `canvas/drawElement.ts` (`drawElements`, `DrawContext2D`) | **분기 추가, 인터페이스 불변**. 심볼 미리보기도 이 함수를 그대로 부른다 |
| rAF 루프 | `canvas/CanvasSurface.tsx` | **규율 유지**. 유휴 정지·가시성 게이팅은 004 에서도 그대로다. 부품이 늘어도 트윈이 없으면 루프는 선다 |
| 요소 편집기 | `canvas/CanvasElementsEditor.tsx` (911행) · `CanvasRuleTableEditor.tsx` (558행) | **부품 행에 재사용**. 부품은 일반 요소와 같은 컨트롤로 편집된다(REQ-02) — 두 번째 편집 UI 를 만들지 않는다 |
| 도메인 근거 | `internal/agent/{lg,samsung,modbus}`, `internal/agent/hvac/codes.go`, `panels/{OutdoorControlPanel,AcControlPanel,PropertiesGridPanel}.tsx`, `acControlTypes.ts`, i18n `device.type.*`·`propValve`·`bulkPlaceholder` | **카탈로그 출처**. MVP 9종은 전부 여기서 나왔다(SPEC §MVP 심볼 카탈로그가 심볼마다 근거를 댄다) |
| 저장 상한(제약) | `internal/api/handler/dashboard.go` `maxDashboardPayloadBytes = 256 * 1024` | **제약 참조**. 대시보드 전체가 공유하는 예산이다. 스탬핑의 config 팽창을 이 축에서 실측한다(§위험 R2, AC-E5) |
| 에셋 API(불채택) | `POST/GET /api/v1/dashboard-assets`, `heatmap/svgAsset.ts`, `heatmap/useFloorPlanSources.ts` | **쓰지 않는다**. (B) 기각의 근거 자산이며, 그림 배경은 002 의 몫이다. `svgAsset.ts` 가 문서화한 인라인 금지·`preserveAspectRatio` 제약은 002 가 물려받는다 |

## 작업 분해 (Task Decomposition)

### Primary Goal (1차 목표) — 스키마 확장 + 중첩 투영 + 그룹 렌더

우선순위 High. 이 단계가 끝나면 **손으로 쓴 그룹 config 가 화면에 그려진다**(UI 는 아직 없다).

- T1. `symbols/symbolTypes.ts`(신규, TDD): `SymbolDef` · `SymbolPart` · `SymbolStamp`. DOM 무의존.
- T2. `canvasConfig.ts` 확장(DDD, 행위 보존): `GroupElement` + `CanvasNode` 추가, `CanvasElement` 는 **불변**.
  `parseCanvasConfig` 에 `group` 갈래를 더한다 — `parts` 는 기존 `parseElements` 재사용 + `group` 항목 폐기
  (재귀 차단), 부품 id 중복은 먼저 온 것이 이긴다. **001 파서 테스트 전량이 그대로 통과해야 한다.**
- T3. `canvasGeometry.ts` 확장(TDD): `projectBoxIn`/`projectLineIn`/`projectPointIn`.
  **항등식 테스트를 먼저 쓴다** — `project*In(geo, 스테이지 전체 박스) === project*(geo, stage)`.
  이 테스트가 두 투영 경로의 갈라짐을 영구히 막는 죔쇠다.
- T4. `canvasNode.ts`(신규, TDD): `isGroup` 판별, `frameKey(nodeId[, partId])`, 2단 순회 도우미.
  최상위 원시형 키가 001 과 **바이트 동일**해야 한다(기존 동작 보존의 근거).
- T5. `drawElement.ts` 그룹 분기(DDD): 그룹 박스 투영 → 부품 2단 순회. `DrawContext2D` 는 **손대지 않는다**.
  기록 스텁 기반 기존 테스트 방식을 그대로 쓴다(jsdom canvas 불필요).

### Secondary Goal (2차 목표) — 카탈로그 + 스탬핑 + 바인딩 상속

우선순위 High. 이 단계가 끝나면 **심볼이 데이터에 반응한다**.

- T6. `symbols/symbolCatalog.ts`(신규, TDD): MVP 9종 정의. 순수 데이터.
  테스트는 **불변식**을 검증한다 — 모든 부품 기하가 0..1 안, id 중복 없음, `role` 중복 없음,
  id 에 점 없음, 모든 `labelKey` 가 `ko.json`·`en.json` **양쪽**에 존재.
- T7. `symbols/stampSymbol.ts`(신규, **TDD 핵심**): 정의 → `GroupElement`.
  사본 생성(정의 객체를 공유하지 않는다 — 한 번 놓은 심볼을 고치면 카탈로그가 오염되는 사고를 막는다),
  부품 id 발급, `aspect` 반영 기본 배치 박스, `symbol` 출처 기록.
- T8. `canvasRules.ts` 확장(TDD, **최소 침습**): `matchRulePatch(value, rules)` 추가(`matchesRule` 조합),
  `mergePatch` export 추가. `evaluateRules`·`matchesRule` 은 **한 글자도 바꾸지 않는다**.
  **합치 항등식 테스트를 먼저 쓴다**: `evaluateRules(v, r, b) === mergePatch(b, matchRulePatch(v, r) ?? {})`.
- T9. `canvasCascade.ts`(신규, **TDD 핵심 — 이 SPEC 의 하중이 여기 있다**):
  `resolvePartStyle({ groupStyle, groupPatch, partStyle, partRules, partValue })`.
  파생 우선 4단 법칙(속성별 폴백) + `opacity` 곱셈 예외 + **양쪽 미지정 시 미지정 보존**.
  **조상을 조회하지 않는 순수 함수** — 그룹 기여는 전부 인자로 들어온다(위험 R3 의 구조적 완화).
  무동작 테스트를 같은 파일에서 쓴다: 그룹 기여가 비면 결과가 `evaluateRules(v, partRules, partStyle)` 와
  **완전히 동일**(키 집합까지)해야 한다.
- T10. `CanvasPanel.tsx` 수정(DDD): `buildCanvasFrame` 이 복합 키를 내고, **그룹 바인딩 1단 상속**
  (`부품.binding ?? 그룹.binding`) + **그룹 규칙 프레임당 1회 평가** 후 2단 순회로 그룹 기여를
  `resolvePartStyle` 에 **인자 전달**한다. 그룹에 바인딩이 없으면 그룹 규칙을 평가하지 않는다(001 규율).
- T11. `CanvasSurface.tsx` 수정(DDD): 트윈 장부 복합 키 + **3단 트윈 폴백**(`부품 → 그룹 → 패널`).
  그룹 패치가 뒤집혀도 `sameStyle` 이 같다고 판정한 부품은 트윈을 시작하지 않는다.
  사라진 부품 키 청소가 그룹 삭제 시에도 동작해야 한다. 유휴 정지·가시성 게이팅 규율 불변.

### Final Goal (최종 목표) — 심볼 고르기 UI + 편집기 중첩 + 견고성

우선순위 Medium. 이 단계가 끝나면 **사용자가 실제로 심볼을 놓는다**.

- T12. `symbols/CanvasSymbolPicker.tsx`(신규): 카탈로그 격자 + 작은 `<canvas>` 미리보기.
  미리보기는 `drawElements` 를 **그대로** 호출한다(REQ-05) — 그리지 못하는 카탈로그 항목이 즉시 드러난다.
- T13. `CanvasElementsEditor.tsx` 수정(DDD): "심볼 추가" 진입점, 그룹 행 접힘/펼침, 부품 행이
  **기존 편집 컨트롤을 재사용**, `bindable` 부품의 규칙 표 우선 펼침, 그룹 삭제 = 부품 동반 삭제,
  **그룹 기본 스타일 + 그룹 규칙 표 편집**(같은 컨트롤 재사용). 캐스케이드로 내려온 값은
  "그룹에서 상속됨" 으로 표시해 사용자가 층을 눈으로 볼 수 있게 한다 — 예측 가능성이 이 법칙의 목적이다.
- T14. i18n `ko.json`/`en.json` **양쪽**: 심볼 이름 9종 + 부품 이름 + 고르기 UI 문구.
  **키 이름 안에 점을 넣지 않는다**(프로젝트 가드).
- T15. 견고성(REQ-05/REQ-06): 빈 그룹 / 알 수 없는 `role`·`catalog_id` / `parts` 안 그룹 / 부품 id 중복 /
  다운그레이드 시 그룹 폐기. 전부 예외 없이 처리.
- T16. 품질 게이트: 신규 코드 85%+ 커버리지(순수 모듈 95% 이상 목표), LSP 0(tsc `--noEmit` + eslint),
  **001 을 포함한 기존 웹 테스트 전량 green**, config 크기 실측(AC-E5).

## 아키텍처 설계 방향

- **001 을 넓히되 고치지 않는다.** `CanvasElement` 유니온, `evaluateRules`, `project*` 3종, `DrawContext2D`
  는 전부 시그니처가 그대로다. 004 가 더하는 것은 최상위 원소 타입(`CanvasNode`), 투영 3종, 키 함수,
  그리고 카탈로그다. 이 규율이 001 의 테스트 스위트를 **회귀 방어망으로** 계속 쓰게 해 준다.
- **중첩 상한을 타입으로 강제한다.** `GroupElement.parts: CanvasElement[]` 이므로 그룹 안 그룹은
  컴파일되지 않는다. 런타임 깊이 검사를 쓰지 않는 이유이며, 파서의 재귀 차단은 **외부에서 들어온 손상
  입력**만을 위한 것이다(내부 코드 경로에는 그 가능성이 없다).
- **키 전략이 이 SPEC 의 조용한 하중이다.** 001 은 상태를 평평한 `el.id` 로 들고 있고, 중첩은 그 가정을
  깬다. 복합 키(`groupId/partId`)를 순수 함수 하나(`frameKey`)로 격리해, 바뀌는 지점을 세 곳
  (`buildCanvasFrame` · `advance` · `drawElements`)으로 묶어 둔다. 세 곳 모두 최상위 원시형에 대해서는
  **이전과 같은 키**를 낸다 — 그래서 001 의 기존 테스트가 그대로 유효하다.
- **캐스케이드는 새 병합 의미론을 만들지 않는다.** 4단 법칙은 001 이 이미 가진 `mergePatch` 를 두 번
  겹치고 `evaluateRules` 를 한 번 부르는 것으로 전부 표현된다. 새로 만드는 것은 `matchRulePatch` 하나뿐이며
  그것도 이미 export 된 `matchesRule` 의 1줄 조합이다. **적게 만들수록 001 과 어긋날 여지가 적다.**
- **무동작 보장은 분기가 아니라 대수(代數)로 얻는다.** "그룹이 비었으면 001 경로로 간다" 는 if 문을
  쓰지 않는다. 그런 분기는 두 경로를 만들고 두 경로는 갈라진다. 대신 빈 그룹 기여가 항등원이 되도록
  식을 세워, 캐스케이드를 쓰지 않는 패널이 **같은 코드를 지나면서** 001 과 같은 값을 낸다.
- **캐스케이드 해석기는 조상을 조회하지 않는다.** `resolvePartStyle` 은 그룹의 기여를 인자로만 받는
  순수 함수라, "부모를 잘못 찾는" 상태가 표현 불가능하다. 캐스케이드가 도입한 조상 의존성을
  키 조회가 아니라 **인자 전달**로 처리하는 것이 위험 R3 의 구조적 완화다.
- **카탈로그는 데이터, 스탬핑은 순수 함수, 렌더는 얇은 층.** 001 이 세운 "계산은 순수 모듈이 끝내고
  렌더 층은 옮기기만 한다" 는 분리를 그대로 잇는다. 심볼은 데이터일 뿐이라 카탈로그를 늘리는 일이
  렌더 코드를 건드리지 않는다.
- **미리보기가 렌더 경로를 재사용하는 것은 편의가 아니라 검증 장치다.** 별도 썸네일 경로를 두면
  "카탈로그에는 있는데 캔버스에서는 안 그려지는 심볼" 이 조용히 생길 수 있다. 같은 함수를 쓰면 그 상태가
  존재할 수 없다.
- config 는 계속 불투명 JSON 이므로 백엔드 무변경. 카탈로그는 앱과 함께 배포되며 config 에 복제되지 않는다.

## 위험 분석 (Risk Analysis)

| # | 위험 | 영향 | 완화 |
|---|------|------|------|
| R1 | 부품이 늘어 프레임당 그릴 도형 수가 001 의 전제(A2: 요소 수백 이하)를 넘긴다 | 트윈 중 끊김 | 심볼당 부품 10개 이하(A2), 그룹 30개면 도형 약 180개로 001 전제 안이다. 전면 재그리기 규율은 유지하고, dirty-rect 최적화는 003 이후로 계속 이연한다 |
| R2 | 스탬핑이 config 를 부풀려 **대시보드 전체가 공유하는 256KB snapshot 예산**을 잠식한다 | 대시보드 저장 실패(다른 패널 변경까지 유실) | 신규 확인 제약이며 정면으로 다룬다. 인수 기준 AC-E5 가 "심볼 30개 배치 후 직렬화 크기"를 **실측**한다. 부품 스타일은 미지정을 만들어 채우지 않는 001 파서 규율 덕에 기본값이 직렬화되지 않는다. 초과가 관측되면 카탈로그 부품 수를 줄이거나 사용자에게 심볼 수 상한을 안내한다 |
| R3 | **복합 키 전환 + 캐스케이드가 겹쳐, 부품의 해석 결과가 조상에 의존하게 된다.** 키가 어긋나면 다른 심볼의 그룹 패치가 새어 들어와 **화면이 조용히 거짓말을 한다**(트윈이 튀는 정도가 아니다) | 잘못된 색·투명도가 정상처럼 표시됨. 사용자가 오판할 수 있는 유일한 위험 | **명명된 완화 셋.** (1) **조상 무조회 원칙** — `resolvePartStyle` 은 그룹 기여를 인자로만 받는 순수 함수이며, 프레임 키는 결과를 담는 자리만 정하고 캐스케이드 입력을 고르는 데 쓰이지 않는다. 이 형태에서 "부모를 잘못 찾음" 은 **표현 불가능**하다. (2) `frameKey` 순수 함수 격리 + **최상위 원시형 키가 001 과 동일**함을 단위 테스트로 못박음. (3) **AC-E8** 이 두 그룹이 서로 다른 패치를 가질 때 교차 오염이 없음을 검증. 001 기존 스위트를 회귀 게이트로 유지(T16) |
| R4 | 두 투영 경로(`project*` / `project*In`)가 시간이 지나며 갈라진다 | 그룹 안팎의 좌표가 미묘하게 어긋남 | **항등식 테스트**를 T3 에서 먼저 쓴다 — 스테이지 전체를 박스로 넣으면 두 경로의 결과가 같아야 한다. 갈라지는 순간 빨간불이 켜진다 |
| R5 | 카탈로그가 커지며 한국어 문자열이 코드에 스며든다 | i18n 우회·번역 누락 | 카탈로그는 `labelKey` 만 들고 표시 이름을 갖지 않는다(REQ-01). T6 의 불변식 테스트가 모든 `labelKey` 의 `ko.json`·`en.json` **양쪽** 존재를 검증한다 |
| R6 | 심볼 배치가 여전히 수치 입력이라 "가능해졌지만 쾌적하지 않다" | 사용자 기대와 어긋남 | SPEC §순서에서 이 한계를 **명시적으로 인정**했다. 004 는 가능하게 만들고 002 가 즐겁게 만든다. 004 의 `group` 이 002 의 드래그 단위를 미리 마련해 두므로 이 순서는 낭비가 아니다 |
| R7 | `CanvasElementsEditor.tsx` 가 이미 911행인데 그룹/부품 중첩 행이 더 붙는다 | 파일 비대·리뷰 난이도 | 심볼 고르기는 별도 컴포넌트(`symbols/CanvasSymbolPicker.tsx`)로 분리하고, 부품 행은 **기존 요소 행 컨트롤을 재사용**한다(새 편집 UI 를 만들지 않는다). 001 §위험 R4 와 같은 규율 |
| R8 | MVP 9종이 사용자의 실제 설비와 어긋난다 | 여전히 "쓸 심볼이 없다" | 9종을 전부 이 코드베이스가 실제로 모델링하는 대상에서 뽑고 심볼마다 출처를 SPEC 표에 댔다(A8). 그럼에도 부족하면 **놓은 뒤 부품 단위로 고칠 수 있다**(REQ-02) — 카탈로그는 출발점이지 감옥이 아니다 |
| R9 | 002 가 `elements[]` 를 `CanvasElement[]` 로 가정한 코드를 이미 설계해 두었다면 충돌한다 | 002 재작업 | 002 는 아직 착수 전이고(SPEC-CHART-004/005 편집 툴킷 API 안정화 대기, 001 §R5), 004 가 먼저 들어가면 002 는 처음부터 `CanvasNode` 위에 선다. 이것이 §순서가 004 를 앞에 두는 두 번째 근거다 |
| R10 | **그룹 규칙 패치는 광역이라 무디다** — 그룹 규칙이 `fill` 을 담으면 채움을 그리는 모든 부품이 함께 바뀐다. 사용자가 의도보다 넓게 덮을 수 있다 | 심볼이 통째로 한 색이 되어 판독성 상실 | 법칙 자체가 탈출구를 갖는다: (a) 그룹 규칙 패치는 **속성별**이므로 덮고 싶은 속성만 넣는다, (b) 특정 부품만 이기게 하려면 그 부품 규칙 표에 행을 둔다(1층 > 2층). 여기에 설정 UI 가 **상속된 값을 "그룹에서 상속됨" 으로 표시**(T13)해 광역 효과가 눈에 보이게 하고, 001 의 라이브 미리보기가 즉시 결과를 보여 준다 |
| R11 | `opacity` 곱셈 예외를 구현자가 잊고 4단 법칙에 넣거나, 양쪽 미지정일 때 `1` 을 채운다 | 전자는 "전체 흐리게" 실패, 후자는 **무동작 보장 파손**(`sameStyle` 이 `===` 비교라 `1 !== undefined`) | 예외를 SPEC §명세에 독립 소제목으로 못박고(§`opacity` 예외), REQ-05 에 **두 방향 모두 금지 조항**으로 넣었으며, AC-07(곱셈)과 AC-E7(무동작·키 집합 동일)이 각각 검증한다. `canvasCascade.ts` 단위 테스트가 두 경우를 모두 덮는다(T9) |

## 후속 SPEC 로드맵

- **SPEC-CANVAS-002 — 캔버스 내 시각 편집기**: 드래그 배치, 크기 조절, 정렬·스냅, z-order 조작,
  **그룹 조작(이동·크기 조절·해제)**, 배경 에셋. 004 의 `group` 노드를 드래그·히트 테스트의 단위로
  쓴다. 배경 에셋은 기존 `POST/GET /api/v1/dashboard-assets` 와 `svgAsset.ts` 의 data-URL 규율
  (업로드 SVG 인라인 금지, `preserveAspectRatio` 가 CSS 를 이김, `naturalWidth` 불신)을 따른다 —
  004 가 (B) 를 기각하며 남겨 둔 그림 표현력이 여기서 배달된다.
  선행 조건: SPEC-CHART-004/005 편집 툴킷 API 안정화(001 §위험 R5) + 본 SPEC.
- **SPEC-CANVAS-003 — 애니메이션 전량 + 고급 표현**: 반복 효과(점멸·맥동·회전·파선 흐름)와 값 구동
  연속 애니메이션. 004 의 부품 구조와 특히 잘 맞는다 — `hvac-fan.blade` 회전, `pump.casing` 맥동,
  `pipe-segment.line` 파선 흐름이 전부 **부품 하나에 걸리는 효과**다. 001 의 트윈 엔진이 이미 기하
  수치를 보간하므로 값 구동은 목표값 사상만 얹으면 된다.
- **사용자 정의 심볼(미할당)**: 캔버스 위 선택 영역을 심볼로 저장. 저장 위치 결정(config 내장 /
  자산 API / 신규 엔드포인트)이 필요하고 그 질문이 "백엔드 변경 0" 을 흔들기 때문에 004 에서 의도적으로
  제외했다(A9). 004 의 `GroupElement` 가 그대로 직렬화 단위가 되므로 확장 지점은 이미 서 있다.
- **그룹 단위 `visible`(미할당)**: 그룹 `style`·`rules` 는 본 개정에서 004 로 들어왔으나, "심볼 통째
  숨김" 전용 필드는 아직 없다(`visible` 은 캐스케이드 4단 법칙을 통해 부품으로 내려가므로 그룹
  기본 스타일에 `visible: false` 를 두면 이미 동작한다 — 별도 필드가 필요한지는 002 의 그룹 조작에서
  실사용을 보고 판단한다).

후속 SPEC 들은 본 SPEC 이 정의한 `CanvasNode`·`GroupElement`·`project*In`·`frameKey`·`resolvePartStyle`
(파생 우선 4단 법칙)을 확장 지점으로
사용하며, 본 SPEC 에서는 파일을 생성하지 않는다(로드맵 기록만).
