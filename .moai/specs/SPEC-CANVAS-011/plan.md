---
id: SPEC-CANVAS-011
type: plan
version: "0.2.0"
created: 2026-09-15
updated: 2026-09-15
---

# SPEC-CANVAS-011 구현 계획

## 구현 전략

### 순서를 이렇게 잡는 이유

**밖에서 안으로** 간다 — 자료형과 순수 함수를 먼저 굳히고, 그 위에 그리기를 얹고, 마지막에
몸짓을 붙인다. 반대로 잡으면 몸짓을 짜는 동안 자료형이 흔들려 같은 코드를 두 번 쓴다.

```
M1  팔레트 이동                — 완전 독립. 나머지와 한 줄도 닿지 않는다
M2  윤곽 상자를 공유 모듈로     — 값을 바꾸지 않는 이사. 이후 전부의 토대
M3  고정 앵커 아홉             — M2 의 첫 소비자. 순수 함수
M3' 임의 앵커(저장·파서·도구)   — M3 위. 요소에 필드 하나
M4  연결선 자료형과 파서        — 저장 형상 확정
M5  끝점 해석                  — M3 + M4. 그리기·히트·손잡이가 함께 쓸 한 함수
M6  그리기                     — M5 의 첫 소비자. 2차→3차 환산 포함
M7  히트 판정                  — M5 의 둘째 소비자. 평탄화 재사용
M8  긋는 몸짓(직선)             — 앵커에서 앵커로. 연결선이 처음 생기는 자리
M9  고르기와 손잡이             — 끝점·중간점 손잡이, 끝점 갈아 끼우기
M10 꺾임·곡률 편집             — 선 위 더블클릭 · 점 끌기 · 점 지우기
M11 자유선                     — 궤적 받기 · 간소화 · 상한
M12 견고성과 회귀              — 끊긴 연결 · 지우기 연동 · 뒤집힌 시험 정리
```

M6·M7 은 서로 독립이라 순서를 바꿔도 된다. M10·M11 도 마찬가지다.

### 핵심 설계 결정

1. **`CanvasElementKind` 를 넓히지 않는다.** 연결선은 최상위 노드의 셋째 갈래다. 그래서
   `canvasElementKind.test.tsx` 와 `canvas007Regression.test.tsx:269` 의 출시된 가드 둘이
   한 글자도 고쳐지지 않고 초록으로 남는다. 004 가 그룹에서 치른 그 판단 그대로다.

2. **`outlineBox` 를 옮기되 고치지 않는다.** M2 는 **이사**다. 값이 한 글자라도 달라지면
   윤곽선·핸들·마키 판정이 함께 움직이고, 그 움직임은 세 SPEC 의 시험을 건드린다.
   이사가 무동작임을 시험으로 먼저 고정하고 옮긴다.

3. **끝점 해석은 한 함수(`resolveConnector`)다.** 그리는 쪽·잡는 쪽·손잡이가 전부 그것을
   지난다. 셋이 저마다 참조를 풀면 "그린 자리와 잡히는 자리가 다르다" 가 시작된다.

4. **2차 베지어를 새로 짓지 않는다.** `c1 = p0 + 2/3(q−p0)` 한 줄로 3차로 환산하고 기존
   호출을 지난다. `DrawContext2D` 에 멤버가 늘지 않는다(007 가드).

5. **점 목록은 `elbow`·`curve`·`free` 가 공유한다.** `route` 는 **그리기만** 가른다.
   셋을 셋으로 짓지 않는 것이 이 SPEC 의 첫 번째 절약이다.

5'. **임의 앵커는 요소 로컬 격자에 산다.** `ANCHOR_LOCAL_EXTENT = PATH_LOCAL_EXTENT` 로
   **파생**시키고(값을 베끼지 않는다), 변환은 `groupCoords.toAbsolutePoint`/`toLocalPoint`
   를 재사용한다. 그래서 **크기 변경에 쓸 코드가 한 줄도 없다** — 저장값은 그대로고 투영만
   새 상자에서 다시 나온다. 009 의 좌표 변환 가드는 허용 호출자를 **둘로 세어 적는**
   형태로 넓힌다(구멍을 조용히 지나가지 않는다).

6. **곡선 히트는 평탄화를 재사용한다.** `shapes/pathFlatten` 의 두 번째 호출자가 되고,
   곡선 거리 판정을 새로 적지 않는다.

---

## 마일스톤

### M1 — 팔레트 이동

**대상**: `shapes/paletteGroups.ts`, `shapes/shapeCatalog.ts`, `shapes/CanvasShapeCatalog.tsx`

**작업**

1. `PaletteGroupId` 에서 `'primitive'` 를 걷어내고 묶음을 셋(`basic · general · arrow`)으로
   줄인다.
2. 원시 넷(사각형·타원·선·텍스트)을 `basic` 묶음의 **맨 앞**에 세운다. 카탈로그 12종은
   그 뒤에 온다.
3. `DEFAULT_COLLAPSED` 를 `basic: false` 로 둔다 — 008 위험 R10 의 답을 자리만 옮겨 지킨다.
4. `readCollapsed` 는 **이미** 모르는 키를 버린다. 저장된 `primitive` 키는 그대로 무시된다
   (REQ-01-a — 새 코드가 필요 없음을 시험으로 확인한다).
5. i18n: `paletteGroupPrimitive` 키의 소비처가 사라진다. **키는 남기고 소비처만 지운다** —
   지우면 옛 로케일 파일과의 차이가 생기고, 남겨 두는 비용은 한 줄이다.

**검증**: 묶음이 셋이고, 기본 묶음이 펼쳐진 채 태어나며, 그 첫 칸이 사각형이다.

---

### M2 — 윤곽 상자를 공유 모듈로

**대상**: `CanvasEditOverlay.tsx` → `canvasOutline.ts`(신규)

**작업**

1. `outlineBox` 와 그 도우미(`normalizeBox` · `resolveFontSize` · `resolveMeasuredWidth` ·
   `resolveTextOrigin` 호출)를 새 모듈로 옮긴다.
2. 오버레이는 그 모듈을 **부른다**. 두 벌을 두지 않는다.
3. **값이 바뀌지 않았음을 먼저 고정한다** — 옮기기 전에 다섯 종류(rect·ellipse·path·line·
   text)와 그룹에 대한 기대값을 적은 시험을 쓰고, 옮긴 뒤 그대로 통과시킨다.

**주의**: 이 모듈은 DOM 을 모른다. 투영과 글자 폭 장부만 받는다.

---

### M3 — 고정 앵커 아홉

**대상**: `connector/anchors.ts`(신규)

**작업**

1. `FixedAnchorId` = 8핸들의 상자 이름 여덟 + **중심 `c`**. 여덟은 `canvasEditGeometry` 의
   `BOX_HANDLE_IDS` 를 **그대로 재수출**하고, 새로 짓는 이름은 `c` 하나뿐이다.
2. `anchorPoints(node, proj, textWidths)` — 내부는 `outlineBox` **한 번**. 여덟은
   `BOX_HANDLE_FACTORS` 그대로, 중심은 그 상자의 한가운데.
3. 종류별 갈래가 **없다**(A2). 그룹도 제 상자로 아홉을 낸다. 부품은 앵커를 내지 않는다(A3).

**검증**: 다섯 종류와 그룹이 전부 아홉을 내고, 그중 여덟이 `handlePositions` 의 상자 갈래와
**같은 좌표**이며, 중심이 상자 한가운데다.

---

### M3' — 임의 앵커

**대상**: `connector/anchors.ts`, `canvasConfig.ts`, `connector/CanvasAnchorTools.tsx`(신규)

**작업**

1. `ANCHOR_LOCAL_EXTENT = PATH_LOCAL_EXTENT` — **파생**시킨다. 값을 베끼지 않는다.
2. `CustomAnchor { id, x, y }` 를 적고, `CanvasElementBase` 에 `anchors?: CustomAnchor[]` 를
   더한다. **종류 유니온은 한 글자도 넓히지 않는다.**
3. `parseElement` 가 그 필드를 **관용적으로** 읽는다 — 배열이 아니면 부재, 좌표가 손상이면
   그 항목만 버리고, id 중복은 먼저 온 것이 이긴다.
4. `anchorPoints` 가 임의 앵커를 `groupCoords.toAbsolutePoint` 로 함께 낸다. **상자를 두 번
   재지 않는다** — 고정과 임의가 한 상자에서 나온다.
5. `addAnchorAt` · `removeAnchor` — 순수 함수. 놓는 자리는 `toLocalPoint` 로 되돌린다.
   `path` 요소에서는 허용 오차 안의 가장 가까운 **경로 명령 좌표**에 붙인다(REQ-02'-b).
6. 상자형(rect·ellipse·path·group)이 아니면 **아무 일도 하지 않는다**(A11).
7. 전용 **앵커 도구**를 도크에 세운다. 켜진 동안에만 도형 위 더블클릭이 앵커를 더하고,
   앵커 위 더블클릭이 뺀다. 그룹 도구가 세운 형상을 따른다(두 표면이 같은 컴포넌트).

**주의**: 더블클릭 판정은 009 의 `isSecondPress` 를 **그대로 쓴다.** 도구가 켜져 있을 때만
이 뜻이 붙으므로 009 의 그룹 진입과 섞이지 않는다.

**검증**: 도형을 키워도 저장 좌표가 한 자리도 바뀌지 않고, 캔버스 자리는 새 상자에서 다시
나온다. 별의 꼭지점에 놓으면 경로 명령 좌표와 **정확히** 같다.

---

### M4 — 연결선 자료형과 파서

**대상**: `connector/connectorTypes.ts`(신규), `canvasConfig.ts`

**작업**

1. `ConnectorEnd` · `ConnectorRoute` · `ConnectorElement` 를 적는다.
2. `isConnector(node)` — **판별의 유일한 자리**(`isGroup` 과 같은 규율).
3. `CanvasNode` 를 셋째 갈래로 넓히고 `CanvasNodeKind` 에 `'connector'` 를 더한다.
   **`CanvasElementKind` 는 건드리지 않는다.**
4. `parseNode` 에 한 줄을 더한다: `kind === 'connector' → parseConnector`.
5. `parseConnector` 는 관용적이다 — 모르는 `route` 는 `straight` 로, 손상 좌표는 폴백으로,
   끝점이 없으면 **노드 자체를 버린다**(정체성이 없는 항목은 버린다는 001 규율).

**검증**: 저장 왕복에 값이 바뀌지 않는다. 출시된 가드 둘이 무수정 통과한다.

---

### M5 — 끝점 해석

**대상**: `connector/resolveConnector.ts`(신규)

**작업**

1. `resolveConnector(connector, nodes, proj, textWidths)` → 캔버스 단위 점 목록 또는
   `undefined`(끊긴 연결).
2. 참조 끝점은 `anchorPoints` 로, 자유 끝점은 제 좌표 그대로.
3. 참조가 풀리지 않으면 `undefined` — 세 소비자(그리기·히트·손잡이)가 모두 아무것도 하지
   않는다.
4. **부품을 가리키는 참조도 `undefined`** 다. 복합 키가 들어와도 여기서 막힌다(A3).

**검증**: 참조된 요소를 옮기면 끝점이 따라간다. 지워지면 `undefined` 다.

---

### M6 — 그리기

**대상**: `drawElement.ts`, `group/frameKey.ts`(순회)

**작업**

1. `walkDrawables` 가 연결선도 낸다 — **순회는 하나**다(키 집합이 갈라지지 않는다).
2. `drawConnector(ctx, points, route, style, proj)`:
   - `straight`·`elbow`·`free` → `moveTo` + `lineTo` 연쇄.
   - `curve` → 중간점마다 2차를 **3차로 환산**해 기존 호출을 지난다.
3. 중간점이 없으면 `elbow`·`curve`·`free` 가 `straight` 와 **같은 그림**이다(REQ-04-b).

**검증**: `DrawContext2D` 의 멤버가 늘지 않는다(007 가드 무수정 통과). 중간점 0개일 때
네 갈래의 호출 기록이 동일하다.

---

### M7 — 히트 판정

**대상**: `canvasHitTest.ts`

**작업**

1. `hitsConnector` — 해석한 점 목록을 선분으로 훑어 `HIT_TOLERANCE_PX` 안인지 본다.
2. `curve` 는 `flattenPath` 로 폴리라인을 얻은 뒤 **같은 선분 판정**을 지난다.
3. 상자 판정을 두지 않는다(REQ-07-b).

**검증**: 선에서 tolerance 밖이면 잡히지 않는다. 곡선의 불룩한 쪽이 잡히고 오목한 쪽 안쪽은
잡히지 않는다.

---

### M8 — 긋는 몸짓(직선)

**대상**: `CanvasEditOverlay.tsx`, `connector/CanvasConnectorTools.tsx`(신규)

**작업**

1. 도크에 연결선 도구 넷(직선·꺾은 선·곡선·자유선)을 둔다. **그룹 도구가 세운 형상**을
   따른다 — 두 표면이 같은 컴포넌트를 그린다(004 불변식 I24).
2. 도구가 켜진 동안에만 앵커가 보인다(REQ-02-b).
3. 앵커에서 눌러 앵커에서 놓으면 연결선 하나가 생기고 **그것이 선택된다**(놓은 것은 바로
   끌 수 있어야 한다 — 팔레트·카탈로그·서랍이 이미 지킨 규율).
4. 빈 곳에서 놓으면 자유 끝점이다.

**검증**: 두 요소를 이으면 배열에 연결선이 하나 늘고, 요소를 옮기면 선이 따라간다.

---

### M9 — 고르기와 손잡이

**대상**: `CanvasEditOverlay.tsx`

**작업**

1. 연결선이 선택되면 **끝점 둘 + 중간점마다** 손잡이를 세운다. 8핸들은 세우지 않는다.
2. 손잡이 id 가 가변이므로 `CanvasHandleId` 를 넓히지 않고 **별도 렌더 갈래**를 둔다.
3. 끝점 손잡이를 다른 앵커에 놓으면 참조가 갈린다(REQ-07-a).
4. 중간점 손잡이를 끌면 그 점의 좌표가 갱신된다.

**주의**: 009 가 세운 선택 모델(평평한 키 / 복합 키)은 한 글자도 바뀌지 않는다. 연결선은
최상위 노드이므로 **평평한 키**다.

---

### M10 — 꺾임·곡률 편집

**대상**: `connector/connectorEdit.ts`(신규), `CanvasEditOverlay.tsx`

**작업**

1. `insertPointAt(connector, resolved, pointer)` — 눌린 자리에 가장 가까운 **선 위의 점**을
   찾고, **그 구간의 뒤**에 끼워 넣는다(REQ-05-a).
2. `removePointAt(connector, index)` — 중간점 더블클릭은 그 점을 뺀다(REQ-05-b).
3. 오버레이의 더블클릭 판정은 009 가 이미 지은 `isSecondPress` 를 **그대로 쓴다**. 두 번째
   판정을 만들지 않는다.
4. 대상으로 갈린다: 부품 위면 그룹 진입(009), 연결선 위면 점 생성(011). 두 몸짓은 섞이지
   않는다(REQ-05-c).

**검증**: 세 점짜리 곡선이 삼각형의 제어점으로 휜다. 점을 빼면 직선으로 돌아온다.

---

### M11 — 자유선

**대상**: `connector/freehand.ts`(신규)

**작업**

1. 끌기 궤적을 캔버스 단위 점으로 받는다.
2. **간소화** — 허용 오차 안에서 점을 줄인다(Ramer–Douglas–Peucker 계열).
3. **상한** — 008 의 `MAX_PATH_COMMANDS` 와 같은 자리의 상수를 둔다. 상한에 닿으면 더 받지
   않되 몸짓은 계속된다(끊기지 않는다).

**검증**: 포인터 사건 수와 무관하게 저장되는 점 수가 상한 아래다.

---

### M12 — 견고성과 회귀

**작업**

1. 끊긴 연결: 그리지 않고, 목록 행이 그 사실을 말하고, 예외가 없다.
2. `removeNodes` 가 지워지는 요소를 참조하는 연결선도 함께 걷어낸다.
3. 뒤집힌 시험 셋(팔레트 묶음 수 · 기본 접힘 · 009 좌표 변환 가드)을 뒤집고 011 을
   가리키는 주석을 남긴다. 셋째는 **넓히는 것**이다 — 허용 호출자를 이름으로 세어 적으므로
   세 번째 호출자가 생기면 다시 운다.
4. 008·009 의 나머지 시험이 전부 통과하는지 확인한다. 특히:
   - `CanvasElementKind` 유니온이 다섯 그대로인가
   - `DrawContext2D` 멤버가 늘지 않았는가
   - 선택 키 규칙(009)이 바뀌지 않았는가
5. 전체 프론트 스위트 + 타입체크 + 린트.

---

## 시험 전략

### 층을 건너는 시험을 반드시 둔다

011 이 만드는 이음매는 넷이다.

| 이음매 | 겨누는 시험 |
|---|---|
| 앵커 파생 → 끝점 해석 → 그리기 | 요소를 옮기고 그려진 선의 끝 좌표를 본다 |
| 2차 규칙 → 3차 환산 → 캔버스 호출 | 제어점을 주고 `bezierCurveTo` 인자를 본다 |
| 선 위 더블클릭 → 점 삽입 → 다시 그리기 | 구간 중간을 눌러 점이 **그 구간 뒤**에 드는지 본다 |
| 요소 삭제 → 연결선 정리 | 참조된 요소를 지우고 배열에 남은 것을 본다 |

### 뒤집지 않은 것을 고정하는 시험

- `CanvasElementKind` 가 **다섯** 그대로다
- `DrawContext2D` 에 멤버가 늘지 않았다
- 연결선을 쓰지 않은 config 의 결과가 **키 집합까지** 이전과 같다
- 최상위 선택 키·부품 복합 키 규칙(009)이 그대로다
- `outlineBox` 이사가 값을 바꾸지 않았다

### 커버리지

신규·수정 모듈 85% 이상. 순수 모듈(`anchors` · `resolveConnector` · `connectorEdit` ·
`freehand`)은 100% 를 목표로 한다 — DOM 을 모르므로 그럴 수 있다.

### 뒤집지 않은 것 중 특히 지키는 것

- 임의 앵커를 하나도 쓰지 않은 요소는 `anchors` 키가 **생기지 않는다**(부재 보존).

---

## 산출물

**신규**

```
web/src/pages/dashboard/panels/canvas/canvasOutline.ts                    (M2)
web/src/pages/dashboard/panels/canvas/connector/anchors.ts                (M3 · M3')
web/src/pages/dashboard/panels/canvas/connector/CanvasAnchorTools.tsx     (M3')
web/src/pages/dashboard/panels/canvas/connector/connectorTypes.ts         (M4)
web/src/pages/dashboard/panels/canvas/connector/resolveConnector.ts       (M5)
web/src/pages/dashboard/panels/canvas/connector/connectorEdit.ts          (M10)
web/src/pages/dashboard/panels/canvas/connector/freehand.ts               (M11)
web/src/pages/dashboard/panels/canvas/connector/CanvasConnectorTools.tsx  (M8)
```

**수정**

```
shapes/paletteGroups.ts · shapes/shapeCatalog.ts · shapes/CanvasShapeCatalog.tsx  (M1)
canvasConfig.ts            (M3' — anchors 필드 · M4 — parseNode 한 줄 + parseConnector)
group/frameKey.ts          (M6 — 순회가 연결선도 낸다)
drawElement.ts             (M6)
canvasHitTest.ts           (M7)
canvasEditArrange.ts       (M12 — removeNodes 연동)
CanvasEditOverlay.tsx      (M2 · M8 · M9 · M10 · M11)
CanvasElementsEditor.tsx   (M12 — 연결선 행과 끊김 표시)
```

**SPEC**

```
.moai/specs/SPEC-CANVAS-011/{spec,plan,acceptance}.md
```
