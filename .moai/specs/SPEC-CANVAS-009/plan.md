---
id: SPEC-CANVAS-009
type: plan
version: "0.2.0"
created: 2026-09-14
updated: 2026-09-14
---

# SPEC-CANVAS-009 구현 계획

## 구현 전략

### 순서를 이렇게 잡는 이유

세 요구의 크기가 크게 다르다. **작고 독립적인 것을 먼저 끝내고**, 설계를 뒤집는 큰 것을
나중에 한다. 순서를 반대로 잡으면 투명도 칸 하나가 선택 모델 변경에 발이 묶인다.

```
M1  그룹 투명도 편집 칸          — 독립. 선택 모델과 무관
M2  복합 키의 분해와 판별         — 이후 전부의 토대
M3  부품 선택과 핸들             — M2 의 첫 소비자
M4  부품 기하 쓰기               — M3 이 있어야 의미가 있다
M5  부품 속성 편집(목록)          — M2 의 둘째 소비자. M4 와 독립
M6  부품 분리                    — M2 필요. M4·M5 와 독립
M7  뒤집힌 시험 정리와 회귀 확인   — 전부 끝난 뒤
```

M4·M5·M6 은 서로 독립이라 순서를 바꿔도 된다.

### 핵심 설계 결정

1. **선택 자료형을 바꾸지 않는다.** `Set<string>` 그대로이고 담기는 문자열이 복합 키일 수
   있을 뿐이다. 이것이 최상위 선택 경로를 바이트 동일하게 유지하는 방법이며, 001·002·004 의
   선택 시험이 그대로 회귀 게이트로 남는다.

2. **`frameKey` 의 역함수를 그 모듈에 둔다.** 복합 키를 만드는 자리와 푸는 자리가 같은 파일에
   있어야 구분자가 갈라지지 않는다. 소비 측에서 `split('/')` 을 적지 않는다.

3. **부품 기하 쓰기는 `groupOps` 에 둔다.** `canvasEditGeometry` 가 `parts` 를 모르는 상태를
   유지한다 — 그 모듈의 소스 텍스트를 읽는 시험이 이미 그것을 고정하고 있고, 그 시험은
   009 가 뒤집을 대상이 아니다.

4. **역투영은 새로 짓지 않는다.** `withGeometry(part, box, toLocalGeometry)` 의 두 번째
   호출자가 된다. 풀기와 편집이 반올림·퇴화에서 갈라지는 것을 표현 불가능하게 만든다.

5. **분리는 풀기의 부분 적용으로 쓴다.** `ungroupNode` 가 전부에 대해 하는 일을 하나에 대해
   한다. 좌표 환산과 스타일 굽기를 새로 적지 않는다.

---

## 마일스톤

### M1 — 그룹 투명도 편집 칸

**대상**: `CanvasElementsEditor.tsx`(`GroupNodeRow`)

**작업**

1. `GroupNodeRow` 에 투명도 입력 칸을 더한다. 최상위 요소 행이 쓰는 것과 **같은 컨트롤**
   (`setStyleField` · `clamp01` · `parseOptionalNumber`)을 쓴다.
2. i18n 키는 기존 `dashboard.canvas.elements.opacity*` 를 재사용한다. 새 키를 만들지 않는다.
3. `aria-label` 은 그룹 행의 관용구(`fillTokens` + 그룹 id)를 따른다.

4. **(0.2.0 추가) 렌더 캐스케이드를 함께 세운다.** spec.md §③ 의 전제가 틀렸다 —
   `bakeStyle` 은 풀기 경로에만 걸려 있었고 `buildCanvasFrame` 은 그룹을 통째로 건너뛰어
   부품 스타일을 **해석조차 하지 않았다**. 편집 칸만 만들면 값은 저장되지만 화면은 한
   픽셀도 변하지 않는다. 그래서 `buildCanvasFrame` 의 순회를 `walkDrawables` 로 바꾸고
   부품 기본 스타일을 `bakeStyle(그룹, 부품)` 으로 굽는다. **최상위 경로는 갈래를 나눠
   키 집합까지 이전과 같게 유지한다**(불변식 G12). 그룹 규칙 평가와 바인딩 1단 상속은
   004 M8·M9 의 몫으로 남긴다.

**검증**: 입력한 값이 `node.style.opacity` 에 기록되고, 렌더가 부품 투명도와 곱해 적용한다.
시험은 **이음매**를 겨눈다 — 편집 칸 → 저장 → 렌더 곱셈.

> 편집기와 렌더러 양쪽이 100% 여도 그 사이는 덮이지 않는다. 층을 건너는 시험을 쓴다.

---

### M2 — 복합 키의 분해와 판별

**대상**: `group/frameKey.ts`

**작업**

1. `parseFrameKey(key: string): { nodeId: string; partId?: string }` 을 더한다.
   `frameKey` 의 역함수이며 같은 구분자 상수를 쓴다.
2. `isPartKey(key: string): boolean` — 판별의 유일한 자리.
3. 왕복 성질을 시험으로 고정한다: `parseFrameKey(frameKey(a, b))` 가 `{a, b}` 이고,
   `parseFrameKey(frameKey(a))` 가 `{a}` 이다.

**주의**: 요소 id 에 구분자가 들어갈 가능성. `nextElementId` 는 `el-N` 을 내므로 현재는
안전하지만, **분해는 첫 구분자에서 한 번만** 쪼갠다(`indexOf`). 부품 id 에 구분자가 있어도
`nodeId` 는 온전하다.

---

### M3 — 부품 선택과 핸들

**대상**: `canvasEditContext.tsx`, `CanvasSurface.tsx`, `canvasEditGeometry.ts`(읽기만)

**작업**

1. 캔버스 포인터 경로에서 히트 결과의 `partId` 를 버리지 않고 `frameKey` 로 합쳐 선택에
   넣는다. 히트 테스트는 이미 `partId` 를 돌려주므로 **그 값을 쓰기만** 하면 된다.
2. 선택이 부품 키일 때 8핸들을 부품의 **투영 상자**에 세운다. `handlePositions` 는 상자를
   받으므로 무엇을 넘기느냐의 문제다.
3. `canvasAutoExpandedId` 가 부품 키를 받으면 그룹 id 를 내도록 한다 — 목록에서 그 그룹 행이
   펼쳐져야 부품 행이 보인다.

**불변 유지**: 최상위 요소를 누를 때 선택에 들어가는 문자열이 004 와 바이트 동일해야 한다.

---

### M4 — 부품 기하 쓰기

**대상**: `group/groupOps.ts`, 캔버스 드래그 경로

**작업**

1. `patchPartGeometry(nodes, groupId, partId, nextAbsoluteGeometry)` 를 `groupOps` 에 더한다.
   내부에서 `withGeometry(part, box, toLocalGeometry)` 를 부른다.
2. 그룹 상자(`geometry`)는 건드리지 않는다.
3. 형제 노드·형제 부품은 참조 그대로 두고 새 배열을 낸다 (004 `patchNodeGeometry` 의 규율).
4. 정수 반올림과 퇴화 방지는 기존 쓰기 통로의 규율을 그대로 따른다.

**검증**: 부품을 끌면 저장 좌표가 로컬 격자에서 바뀌고, 그룹 상자는 불변이며, 형제는 참조가
유지된다.

---

### M5 — 부품 속성 편집(목록)

**대상**: `CanvasElementsEditor.tsx`(부품 행)

**작업**

1. 부품 행을 펼칠 수 있게 하고, 펼치면 `style` · `binding` · `text` 칸을 낸다. 최상위 요소
   행과 같은 컨트롤을 쓴다.
2. 기하 수치 칸은 **캔버스 단위**로 보인다 — 읽을 때 `toAbsoluteGeometry`, 쓸 때 M4 의
   `patchPartGeometry`. 그룹 로컬 숫자는 화면에 나오지 않는다.
3. 부품 행을 누르면 그 **부품**이 선택된다(004 는 그룹을 선택했다).

---

### M6 — 부품 분리

**대상**: `group/groupOps.ts`, `group/CanvasGroupTools.tsx`

**작업**

1. `detachPart(nodes, groupId, partId): UngroupOutcome` 를 더한다.
   - 부품을 절대 좌표로 환산하고 `bakeStyle` 로 겉모습을 굽는다 — `ungroupNode` 와 **같은
     함수**를 쓴다.
   - 최상위 배열의 **그룹 바로 뒤** 자리에 넣는다.
2. 남은 부품 수에 따라:
   - 0 개 → 그룹 제거
   - 1 개 → 남은 부품도 올리고 그룹 제거 (부품 1개 그룹을 만들지 않는다)
   - 2 개 이상 → 그룹 유지
3. 규칙 손실 안내는 `rulesLostByUngroup` 의 판정을 쓴다. 화면이 다시 짓지 않는다.
4. 도구 막대에 분리 단추를 더한다. 부품이 선택되었을 때만 활성이다.

---

### M7 — 뒤집힌 시험 정리와 회귀 확인

**작업**

1. `canvas004GroupRows.test.tsx` 의 두 단언을 뒤집고, 그 자리에 009 를 가리키는 주석을
   남긴다. 삭제하지 않는다 — 뒤집힌 근거가 시험 옆에 있어야 한다.
2. 004 의 나머지 시험이 전부 통과하는지 확인한다. 특히:
   - 최상위 선택 키가 바이트 동일한가
   - 그룹 크기 조절이 부품 저장 좌표를 바꾸지 않는가 (A17)
   - `canvasEditGeometry` 에 `parts` 대입 자리가 없는가
3. 전체 프론트 스위트 + 타입체크.

---

## 시험 전략

### 층을 건너는 시험을 반드시 둔다

이 계열에서 이미 겪은 일이다 — 편집기와 렌더러가 각각 100% 여도 그 사이는 덮이지 않는다.
009 가 만드는 이음매는 셋이다.

| 이음매 | 겨누는 시험 |
|---|---|
| 편집 칸 → 저장 → 렌더 곱셈 | 투명도를 입력하고 렌더 결과의 실효 투명도를 본다 |
| 히트테스트 `partId` → 선택 → 핸들 자리 | 부품을 누르고 핸들이 부품 상자에 서는지 본다 |
| 캔버스 드래그 → 역투영 → 저장 좌표 | 절대 델타를 주고 로컬 좌표 변화를 본다 |

### 뒤집지 않은 것을 고정하는 시험

뒤집는 작업에서 가장 위험한 것은 **의도하지 않은 것까지 뒤집히는 것**이다. 다음을 명시적으로
고정한다.

- 최상위 요소 선택 키가 004 와 바이트 동일하다
- 그룹만 선택된 동안 핸들은 그룹 상자에 선다
- 그룹 크기 조절이 부품 저장 좌표를 바꾸지 않는다
- 그룹 로컬 격자의 숫자가 화면에 나오지 않는다

### 커버리지

신규·수정 모듈 85% 이상. `frameKey` 의 분해 함수는 순수 함수이므로 100%.

---

## 산출물

**수정**

```
web/src/pages/dashboard/panels/canvas/group/frameKey.ts          (M2)
web/src/pages/dashboard/panels/canvas/group/groupOps.ts          (M4, M6)
web/src/pages/dashboard/panels/canvas/canvasEditContext.tsx      (M3)
web/src/pages/dashboard/panels/canvas/CanvasSurface.tsx          (M3)
web/src/pages/dashboard/panels/canvas/CanvasElementsEditor.tsx   (M1, M5)
web/src/pages/dashboard/panels/canvas/group/CanvasGroupTools.tsx (M6)
web/src/pages/dashboard/panels/canvas/canvas004GroupRows.test.tsx (M7 — 뒤집음)
```

**신규 시험**

```
group/frameKey.parse.test.ts            (M2)
group/groupOps.detach.test.ts           (M6)
canvas009PartSelection.test.tsx         (M3 이음매)
canvas009PartGeometry.test.ts           (M4 이음매)
canvas009GroupOpacity.test.tsx          (M1 이음매)
```

**SPEC**

```
.moai/specs/SPEC-CANVAS-009/{spec,plan,acceptance}.md
```
