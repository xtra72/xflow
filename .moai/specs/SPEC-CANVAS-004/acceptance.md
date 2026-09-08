# SPEC-CANVAS-004 — 인수 기준 (acceptance.md)

관련 SPEC: `.moai/specs/SPEC-CANVAS-004/spec.md`
형식: Given-When-Then (Gherkin)

## 시나리오 (Given / When / Then)

### AC-01 — 심볼 배치는 부품을 가진 그룹 하나를 만든다 (REQ-01, REQ-02)

```gherkin
Given 캔버스 패널 설정에서 요소가 0개인 상태이고
  And 코드 내장 카탈로그에 valve-two-way 심볼이 bodyA · bodyB · stem · handle · label 부품으로 정의되어 있을 때
When 사용자가 심볼 고르기에서 "밸브(2방)" 를 선택하면
Then elements 배열 끝에 kind 가 'group' 인 노드 하나가 추가되고
  And 그 그룹의 parts 에 5개 부품이 카탈로그 정의의 독립된 사본으로 들어가며
  And 각 부품의 기하는 그룹 로컬 정규화(0..1) 좌표이고
  And 그룹에 symbol { catalog_id: 'valve-two-way', version } 출처 기록이 남으며
  And 카탈로그 정의 객체는 스탬핑 결과와 참조를 공유하지 않는다(사본을 고쳐도 카탈로그가 오염되지 않는다)
```

### AC-02 — 중첩 좌표 투영과 001 투영의 항등식 (REQ-03)

```gherkin
Given 그룹 기하가 { x: 0.5, y: 0.25, w: 0.4, h: 0.5 } 이고
  And 부품 기하가 그룹 로컬로 { x: 0, y: 0, w: 1, h: 1 } 일 때
When 스테이지 크기 400x200 으로 렌더되면
Then 부품은 스테이지 px 기준 { x: 200, y: 50, w: 160, h: 100 } 에 그려지고
  And 별도로 projectBoxIn(geo, { x: 0, y: 0, w: stage.width, h: stage.height }) 의 결과가
      projectBox(geo, stage) 의 결과와 정확히 같다(001 의 투영은 중첩 투영의 특수 사례다)
  And 그룹 박스의 폭·높이가 비균등해도 부품은 같은 비율로 함께 늘어난다(종횡비를 따로 지키지 않는다)
```

### AC-03 — 부품 단위 규칙이 심볼의 상태를 바꾼다 (REQ-04)

```gherkin
Given valve-two-way 그룹이 배치되어 있고
  And 그룹 바인딩이 시리즈 "valve.open" 이며
  And stem 부품에는 자기 바인딩이 없고 규칙 표가 [gte 1 → {stroke: 초록}], [nodata → {stroke: 회색}] 이고
  And handle 부품에는 자기 바인딩 "valve.torque" 가 설정되어 있을 때
When 폴링이 완료되어 valve.open 최신값이 1 이면
Then stem 은 그룹 바인딩을 1단 상속해 값 1 을 받고 첫 일치 행의 초록으로 그려지며
  And handle 은 상속하지 않고 자기 바인딩 valve.torque 의 값으로 평가되고
  And 규칙 평가는 001 의 evaluateRules 가 그대로 수행한다(first-match-wins)
  And label 처럼 바인딩도 규칙도 없는 부품은 기본 스타일로 그대로 그려진다
```

### AC-04 — 프레임 키와 트윈 폴백 (REQ-03, REQ-04)

```gherkin
Given 최상위 원시형 요소 "rect-1" 과 그룹 "grp-1"(부품 "stem") 이 함께 있고
  And 패널 기본 트윈이 { duration_ms: 300 }, 그룹 트윈이 { duration_ms: 120 }, stem 트윈은 미설정일 때
When 프레임 상태(목표 스타일 · 문구 · 트윈 장부)가 만들어지면
Then 최상위 요소의 키는 "rect-1" 로 001 과 동일하고
  And 부품의 키는 "grp-1/stem" 복합 키이며
  And stem 의 트윈은 3단 폴백(부품 → 그룹 → 패널)에 따라 그룹의 120ms 를 쓴다
  And 그룹이 삭제되면 "grp-1/stem" 장부 항목도 함께 청소된다
```

### AC-05 — 심볼 미리보기가 실제 렌더 경로를 재사용한다 (REQ-05)

```gherkin
Given 심볼 고르기 UI 가 카탈로그 9종을 격자로 보여줄 때
When 각 칸의 미리보기가 그려지면
Then 미리보기는 drawElements 를 그대로 호출해 그려지고 별도 썸네일 경로나 이미지 자산을 쓰지 않으며
  And 카탈로그의 모든 심볼이 미리보기에서 빈 칸 없이 그려진다
  And DrawContext2D 인터페이스에 drawImage 가 추가되지 않았다
```

### AC-06 — 파생 우선 4단 우선순위 법칙 (REQ-06)

```gherkin
Given 그룹의 기본 스타일이 { fill: 회색, stroke: 검정, strokeWidth: 2 } 이고
  And 그룹 바인딩 "device.online" 에 규칙 [eq 0 → { fill: 흐린회색, textColor: 흐린회색 }] 이 걸려 있으며
  And body 부품의 기본 스타일이 { fill: 파랑 } 이고
  And body 부품의 규칙 표가 [gt 80 → { fill: 빨강 }] 이며 부품 바인딩 최신값이 90 이고
  And label 부품은 기본 스타일도 규칙도 없을 때
When device.online 최신값이 0 이어서 그룹 규칙이 일치하면
Then body 의 fill 은 1층(부품 규칙 패치)의 빨강이다 — 2층 그룹 패치를 이긴다
  And label 의 textColor 는 2층(그룹 규칙 패치)의 흐린회색이다
  And body 의 stroke 는 어느 규칙 패치에도 없어 4층(그룹 기본)의 검정이다
  And 어느 층도 fontSize 를 정의하지 않았으므로 fontSize 는 미지정으로 남는다(렌더측 기본을 만들어 채우지 않는다)
```

### AC-07 — 상태가 저술을 이긴다: 그룹 규칙이 부품 기본 스타일을 덮는다 (REQ-06)

```gherkin
Given 그룹 규칙이 일치해 { fill: 흐린회색 } 패치를 내고 있고
  And body 부품이 자기 기본 스타일로 { fill: 파랑 } 을 저술했으며 자기 규칙 표는 없을 때
When 최종 스타일이 해석되면
Then body 의 fill 은 2층(그룹 규칙 패치)의 흐린회색이다 — 3층(부품 기본)의 파랑을 이긴다
  And 이것이 CSS 유사 구체성 우선(부품 기본이 그룹 규칙을 이김)을 기각한 이유다:
      그 순서였다면 이 부품만 파랑으로 남아 심볼이 얼룩덜룩하게 흐려졌을 것이다
  And 이 부품만 그룹 규칙을 이기게 하려면 사용자는 부품 규칙 표에 행을 두면 된다(1층 > 2층)
```

### AC-08 — opacity 는 곱셈으로 합성된다 (REQ-06, 유일한 예외)

```gherkin
Given 그룹측 해석 opacity 가 0.3 이고(그룹 규칙 패치 또는 그룹 기본)
  And 부품측 해석 opacity 가 0.9 일 때
When 최종 스타일이 해석되면
Then 부품의 opacity 는 승자 독식(0.3 또는 0.9)이 아니라 곱셈 결과 0.27 이다
  And 그룹측만 0.3 이고 부품측이 미지정이면 결과는 0.3 이다(미지정을 1 로 보아 곱한다)
  And 곱셈 결과가 [0,1] 을 벗어나면 clamp 된다
  And opacity 를 제외한 모든 속성은 곱셈이 아니라 4단 법칙을 따른다
```

### AC-09 — 그룹 규칙 평가 순서와 횟수 (REQ-04, REQ-06)

```gherkin
Given 그룹이 바인딩과 규칙 표를 갖고 부품이 8개일 때
When 한 프레임의 목표 스타일이 만들어지면
Then 그룹 규칙이 부품보다 먼저 평가되고(그룹 패치가 부품 해석의 입력이므로)
  And 그룹 규칙 평가는 프레임당 정확히 1회이며 부품 수만큼 반복되지 않는다
  And 바인딩이 없는 부품은 자기 규칙 표가 평가되지 않지만 그룹의 규칙 패치는 그대로 적용받는다
  And 그룹에 바인딩이 없으면 group.rules 가 비어 있지 않더라도 그룹 규칙은 평가되지 않는다
```

### AC-10 — 그룹 규칙 전이 시 트윈은 "실제로 달라진 부품" 만 시작한다 (REQ-06, REQ-05)

```gherkin
Given 그룹에 부품 5개가 있고 그중 1개는 1층 부품 규칙으로 자기 fill 을 이미 이기고 있으며
  And 트윈 사양이 부품 미설정 · 그룹 { duration_ms: 120 } · 패널 { duration_ms: 300 } 일 때
When 그룹 규칙 일치가 뒤집혀 그룹 패치의 fill 이 바뀌면
Then 목표 스타일이 실제로 달라진 4개 부품만 트윈을 시작하고
  And 1층으로 이미 이기고 있던 부품은 목표가 그대로여서 트윈을 시작하지 않으며
  And 시작된 트윈들은 3단 폴백(부품 → 그룹 → 패널)에 따라 그룹의 120ms 를 쓰고
  And 네 트윈이 모두 끝나면 requestAnimationFrame 예약이 멈춘다(001 의 유휴 정지 보장 유지)
```

## 엣지 케이스 (Edge Cases)

### AC-E1 — 빈 그룹 · 알 수 없는 role · 알 수 없는 catalog_id (REQ-05)

```gherkin
Given config 에 parts 가 빈 배열인 그룹, role 이 알 수 없는 문자열인 부품,
      catalog_id 가 카탈로그에 없는 symbol 기록을 가진 그룹이 섞여 있을 때
When 패널이 렌더되면
Then 렌더 예외가 발생하지 않고
  And 빈 그룹은 아무것도 그리지 않은 채 목록에 남으며
  And 알 수 없는 role 과 catalog_id 는 표시 메타일 뿐이라 그림에 영향을 주지 않는다
```

### AC-E2 — parts 안에 다시 group 이 들어온 손상 입력 (REQ-05, A6)

```gherkin
Given 외부에서 손상된 config 의 어느 그룹 parts 배열에 kind 가 'group' 인 항목이 섞여 있고
  And 같은 그룹 안에 id 가 중복된 부품 두 개가 있을 때
When parseCanvasConfig 가 그 config 를 읽으면
Then 예외 없이 파싱되고
  And parts 안의 group 항목은 버려져 재귀 렌더로 들어가지 않으며
  And 중복 id 부품은 먼저 온 것이 이기고 나중 것은 버려진다(001 의 요소 id 중복 규칙과 같다)
```

### AC-E3 — 001 config 상위 호환과 004 config 하향 호환 (REQ-01)

```gherkin
Given SPEC-CANVAS-001 시절에 저장된 config(그룹 노드 없음)가 있을 때
When 004 의 parseCanvasConfig 가 그것을 읽으면
Then 모든 요소가 이전과 동일하게 파싱되어 001 의 파서 테스트가 전량 통과하고
  And 반대로 004 가 쓴 config 를 001 시절 파서가 읽으면 group 노드는 조용히 버려지되
      나머지 요소는 정상 렌더된다(크래시가 아니라 부분 손실이다)
```

### AC-E4 — 그리기 순서는 2단이다 (REQ-03)

```gherkin
Given elements 가 [rect-A, grp-1(parts: [p1, p2]), rect-B] 순서일 때
When 한 프레임이 그려지면
Then 그리기 호출 순서는 rect-A → p1 → p2 → rect-B 이고
  And 그룹의 부품이 그룹보다 뒤에 있는 rect-B 위로 올라오지 않는다
```

### AC-E5 — config 크기 실측 (REQ-05, 가정 A3, 위험 R2)

```gherkin
Given 빈 캔버스 패널에 MVP 카탈로그 심볼을 30개 배치하고 각 그룹에 바인딩과 규칙 2행씩을 설정했을 때
When 패널 config 를 JSON 으로 직렬화하면
Then 직렬화 크기를 실측해 기록하고
  And 그 크기가 대시보드 snapshot 상한 256KB(maxDashboardPayloadBytes)에 비해 충분한 여유를 남기며
      (예산은 대시보드 전체 패널이 공유한다는 사실을 함께 확인한다)
  And 미지정 스타일 필드가 직렬화되지 않는다(001 파서의 "미지정을 만들어 채우지 않는다" 규율 유지)
```

### AC-E6 — 렌더 루프 규율이 부품 증가에도 유지된다 (REQ-05)

```gherkin
Given 그룹 여러 개가 배치되어 도형 수가 100개를 넘고
  And 모든 트윈이 완료되어 애니메이션할 것이 없는 상태일 때
When 다음 프레임 시점이 오면
Then requestAnimationFrame 이 더 이상 예약되지 않는다(001 의 유휴 정지 규율 불변)
  And 주입된 VisibilitySource 가 "보이지 않음" 을 알리면 프레임을 예약하지 않으며
  And 다시 보이게 되면 한 프레임을 즉시 그려 최신 상태를 반영한다
```

### AC-E7 — 캐스케이드 무동작 보장: 쓰지 않으면 001 과 결과가 동일하다 (REQ-06, 가정 A12)

```gherkin
Given 그룹이 style 도 rules 도 갖지 않고(파서가 만들어 채우지도 않았고)
  And 부품이 자기 기본 스타일과 규칙 표를 가질 때
When 부품의 최종 스타일이 해석되면
Then 결과는 evaluateRules(부품값, 부품.rules, 부품.style) 과 정확히 같으며
  And 값뿐 아니라 **키 집합까지** 같다 — 특히 양쪽 모두 opacity 를 지정하지 않았다면
      결과의 opacity 는 1 이 아니라 미지정으로 남는다
      (CanvasSurface.sameStyle 이 STYLE_KEYS 를 === 로 비교하므로 1 을 채우면 트윈 판정이 달라진다)
  And 이 동일성은 "그룹이 비면 001 경로로 분기" 하는 조건문이 아니라
      4단 법칙의 2·4층이 비면서 자연히 성립한다(같은 코드 경로를 지난다)
  And 001 시절 저장된 config 로 렌더한 결과가 001 과 시각적으로 동일하다
```

### AC-E8 — 캐스케이드는 조상을 키로 조회하지 않는다 (REQ-05, 위험 R3)

```gherkin
Given 서로 다른 패치를 내는 그룹 "grp-A"(fill 빨강)와 "grp-B"(fill 파랑)가 한 패널에 있고
  And 두 그룹이 같은 부품 id "body" 를 각각 갖고 있을 때
When 한 프레임의 목표 스타일이 해석되면
Then "grp-A/body" 는 빨강, "grp-B/body" 는 파랑으로 서로 오염 없이 해석되고
  And 캐스케이드 해석 함수는 그룹 기여를 인자로만 받아 조상을 키로 조회하지 않으며
      (부모를 잘못 찾는 상태가 표현 불가능하다)
  And 프레임 키는 결과를 담는 자리를 정할 뿐 캐스케이드 입력을 고르는 데 쓰이지 않는다
```

### AC-E9 — 규칙 함수 합치 항등식 (REQ-06)

```gherkin
Given 임의의 값 v, 임의의 규칙 표 rules, 임의의 기본 스타일 base 에 대하여
When evaluateRules 와 새 형제 함수 matchRulePatch 를 각각 호출하면
Then evaluateRules(v, rules, base) 는 mergePatch(base, matchRulePatch(v, rules) ?? {}) 와 같고
  And evaluateRules 와 matchesRule 의 시그니처·동작은 001 에서 한 글자도 바뀌지 않았다
  And 이 항등식이 두 경로가 시간이 지나며 갈라지는 것을 막는다
```

## 품질 게이트 (Quality Gate — TRUST 5 / hybrid)

- **Tested**: 신규 코드 TDD(RED-GREEN-REFACTOR). 신규 코드 커버리지 **85% 이상**이 최소선이며,
  DOM 무의존 순수 모듈(`symbols/symbolTypes.ts` · `symbols/symbolCatalog.ts` · `symbols/stampSymbol.ts` ·
  `canvasNode.ts` · **`canvasCascade.ts`** · `canvasGeometry.ts` 추가분 · `canvasConfig.ts` 추가분 ·
  `canvasRules.ts` 추가분)은 **95% 이상**을 목표로 한다. 특히 `canvasCascade.ts` 는 이 SPEC 의 하중을
  지므로 4단 법칙 × 속성별 폴백의 조합을 빠짐없이 덮는다.
  001 이 같은 순수-모듈 분리 규율로 그 수준을 달성한 선례에 맞춘다.
  - `projectBoxIn`/`projectLineIn`/`projectPointIn`: 항등식(AC-02), 0 크기·음수 크기 박스 방어(NaN 미유출).
  - `frameKey`: 최상위 원시형 키가 001 과 **동일**, 부품 복합 키 형식, 그룹 삭제 시 장부 청소.
  - `stampSymbol`: 사본 독립성(카탈로그 비오염), 부품 id 유일 발급, `aspect` 반영 기본 박스.
  - **`resolvePartStyle` (캐스케이드)**: 4단 법칙의 각 층이 이기는 경우 4종, 속성별 폴백(한 속성은 1층·
    다른 속성은 4층에서 오는 혼합), `opacity` 곱셈·clamp·**양쪽 미지정 시 미지정 보존**, 무동작 보장
    (그룹 기여가 비면 `evaluateRules` 결과와 **키 집합까지** 동일), 조상 무조회(입력이 전부 인자).
  - **`matchRulePatch`**: `evaluateRules` 와의 합치 항등식, 미일치 시 `undefined`, `nodata` 행 동작.
  - `parseCanvasConfig` 확장분: `group` 갈래, `parts` 안 group 폐기, 부품 id 중복 먼저 승리,
    손상·결측·미지 필드 입력에서 **예외 없음**.
  - `symbolCatalog`: 부품 기하 0..1 범위, id·role 중복 없음, id 에 점 없음,
    모든 `labelKey` 가 `ko.json`·`en.json` **양쪽**에 존재.
  - 렌더 경로는 001 의 `DrawContext2D` 기록 스텁 방식을 그대로 써서 jsdom canvas 없이 검증한다.
- **Readable**: 명확한 네이밍, 한국어 코드 주석(프로젝트 규약). 001 산출물 수정은 기존 스타일 준수.
- **Unified**: 신규 파일은 `panels/canvas/` 및 `panels/canvas/symbols/` 하위 배치. 기존 포매터 일치.
- **Secured**: 신규 백엔드/엔드포인트 없음. config 는 불투명 JSON. 식 평가·`eval`·동적 함수 생성 없음.
  **업로드 자산을 쓰지 않으므로 `svgAsset.ts` 가 다루는 스크립트 격리 문제 자체가 발생하지 않는다.**
  외부 입력(시리즈 값·손상 config)은 001 의 NaN/Infinity 방어와 관용 파서 규율을 그대로 적용한다.
- **Trackable**: 커밋 메시지 한국어 Conventional Commits, `SPEC-CANVAS-004` 참조.
- **LSP 게이트(run 단계)**: errors 0, type errors 0, lint errors 0 (tsc `--noEmit` + eslint 클린).
- **무동작 회귀(캐스케이드 전용 게이트)**: 그룹이 `style`·`rules` 를 갖지 않는 모든 기존 시나리오에서
  해석 결과가 001 과 **키 집합까지 동일**해야 한다(AC-E7). 이 게이트가 통과하지 못하면 캐스케이드가
  "쓰지 않아도 영향을 주는" 상태이므로, 필드 추가가 아니라 법칙 구현이 잘못된 것이다.
- **회귀**: **SPEC-CANVAS-001 의 테스트 스위트를 회귀 게이트로 삼는다** — 001 의 파서·투영·규칙·트윈·
  렌더·편집기 테스트가 한 건도 수정되지 않은 채 전량 green 이어야 한다. 수정이 필요해지면 그것은
  "001 을 고치지 않는다" 는 설계 규율이 깨진 신호이므로 설계를 되돌아본다.

## Definition of Done

- [ ] REQ-01~06 전부 구현 및 추적성 매핑 충족.
- [ ] AC-01~10, AC-E1~E9 전부 통과.
- [ ] 신규 코드 85% 이상 커버리지(순수 모듈 95% 이상), LSP 0 errors.
- [ ] 001 의 `CanvasElement` 유니온 · `evaluateRules` · `matchesRule` · `projectBox`/`projectLine`/`projectPoint` ·
      `DrawContext2D` 시그니처·동작 **불변** 확인(`canvasRules.ts` 변경은 `matchRulePatch` 추가와
      `mergePatch` export 추가 **둘뿐**).
- [ ] 파생 우선 4단 법칙이 구현되고, 구체성 우선이 아님을 AC-07 이 확인.
- [ ] `opacity` 곱셈 예외 구현 및 **양쪽 미지정 시 미지정 보존** 확인(AC-08, AC-E7).
- [ ] 캐스케이드 무동작 보장 확인 — 그룹이 비면 001 과 키 집합까지 동일(AC-E7).
- [ ] 캐스케이드 해석기가 조상을 키로 조회하지 않음을 확인(AC-E8).
- [ ] 001 테스트 스위트 무수정 전량 green(회귀 게이트).
- [ ] MVP 심볼 9종이 카탈로그에 존재하고 미리보기에서 전부 그려짐.
- [ ] i18n 키가 `ko.json` 과 `en.json` **양쪽**에 존재하고, 키 이름 안에 점이 없음.
- [ ] 백엔드 변경 0 확인(신규 엔드포인트 없음, 자산 API 미사용, config 불투명 JSON 유지).
- [ ] 신규 npm 의존성 0 확인.
- [ ] config 크기 실측치 기록(AC-E5) 및 256KB 예산 대비 여유 확인.
- [ ] 후속 SPEC(002/003) 확장 지점(`CanvasNode` · `GroupElement` · `project*In` · `frameKey` ·
      `resolvePartStyle`) 유지.
