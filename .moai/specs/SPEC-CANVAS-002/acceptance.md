# SPEC-CANVAS-002 — 인수 기준 (acceptance.md)

관련 SPEC: `.moai/specs/SPEC-CANVAS-002/spec.md`
형식: Given-When-Then (Gherkin)

## 시나리오 (Given / When / Then)

### AC-01 — 도형을 눌러 고른다: 히트 테스트와 z-order 우선 (REQ-02)

```gherkin
Given 편집이 켜진 캔버스 패널에 rect 요소 A 와 rect 요소 B 가 이 순서로 배열에 있고
  And 두 요소의 기하가 스테이지 위에서 서로 겹칠 때
When 겹친 영역의 한 점에 pointerdown 이 오면
Then 시스템은 배열 역순으로 훑어 나중에 온 B 를 고른다(배열 뒤 = 위 = 이긴다)
  And 히트 결과는 벌거벗은 문자열이 아니라 { nodeId } 레코드이며 partId 는 채워지지 않는다
  And style.visible 이 false 인 요소는 순회에서 건너뛴다
```

### AC-02 — 도형별 판정과 집기 여유 (REQ-02)

```gherkin
Given 스테이지 위에 rect · ellipse · line · text 요소가 각 1개씩 투영되어 있고
  And line 의 strokeWidth 가 1px 이며
  And text 요소의 실측 글자 폭이 렌더 층에서 넘어와 있을 때
When 각 요소의 경계 안팎으로 포인터 지점을 옮겨 가며 판정하면
Then rect 는 정규화(양수 범위) px 박스를 사방 6px 부풀린 영역에서 맞고
  And ellipse 는 ellipseParams 의 중심·반지름으로 타원 방정식 판정을 하며 반지름에 6px 여유가 더해지고
  And line 은 점–선분 거리가 max(strokeWidth/2, 6px) 이하일 때 맞아 두께 1px 선도 누를 수 있으며
  And text 는 resolveTextOrigin 이 낸 좌측 끝 원점 기준 상자에서 맞되
      textBaseline 이 'middle' 이므로 상자의 세로 중심이 기준점 y 이고
      (기준점 y - fontSize/2) 위쪽과 (기준점 y + fontSize/2) 아래쪽은 여유 밖에서 빗나간다
```

### AC-03 — 도형을 끌어 옮긴다 (REQ-03)

```gherkin
Given 편집이 켜진 캔버스에서 rect 요소가 선택되어 있을 때
When 그 요소 위에서 pointerdown 후 포인터를 움직이면
Then 시스템은 setPointerCapture 로 포인터를 잡고
  And 이동량을 정규화 델타로 바꿔 geometry 에 더해 onConfigChange 로 즉시 반영하며
  And 끄는 동안 그림이 손을 따라온다(트윈 없이 즉시 — 트윈 장부는 ResolvedStyle 만 담는다)
  And 요소가 둘 이상 선택되어 있으면 같은 델타가 선택된 모든 요소에 적용된다
  And pointerup 또는 pointercancel 이 오면 마지막 유효 위치가 그대로 확정된다(되돌리지 않는다)
```

### AC-04 — 종류마다 다른 크기 조절 (REQ-03)

```gherkin
Given 편집이 켜진 캔버스에 rect · line · text 요소가 각각 선택 가능한 상태일 때
When 각 요소를 선택하면
Then rect(및 ellipse)는 모서리 4 + 변 4 = 8개 핸들을 갖고 끌면 geometry 의 x·y·w·h 가 바뀌며
  And line 은 끝점 2개 핸들만 갖고 몸통을 끌면 두 끝점이 함께 움직이며
  And text 는 박스 핸들이 없고 글자 크기 핸들 1개만 가지며 그것을 끌면 style.fontSize 가 바뀐다
  And Shift 를 누른 채 모서리를 끌면 종횡비가 유지되고, Shift 를 누른 채 선 끝점을 끌면 0/45/90도로 죄인다
```

### AC-05 — 도형 팔레트는 목록 편집기와 같은 생성 경로를 부른다 (REQ-01)

```gherkin
Given 편집이 켜진 캔버스에 팔레트가 떠 있고 요소가 이미 2개 있을 때
When 팔레트의 rect 버튼을 누르면
Then 목록 편집기의 addElement('rect') 와 같은 씨앗 기하·같은 계단식 오프셋으로 요소가 추가되어
      연속 배치해도 같은 자리에 겹쳐 쌓이지 않고
  And 목록 편집기에서 그 행이 자동으로 펼쳐지며(001 이 이미 가진 동작)
  And 새 요소가 캔버스에서 선택된 상태가 된다(팔레트가 추가로 하는 유일한 일)
  And 목록 편집기 하단의 기존 추가 버튼 4개(canvas-element-add-*)는 그대로 남아 있다
```

### AC-06 — 클릭하여 속성 설정: 선택이 목록 편집기를 조종한다 (REQ-04)

```gherkin
Given 설정 다이얼로그에 캔버스 미리보기와 요소 목록 편집기가 함께 떠 있고
  And 사용자가 손으로 요소 C 의 행을 펼쳐 둔 상태일 때
When 캔버스에서 요소 A 를 누르면
Then A 의 행이 펼쳐지고 시야로 스크롤되며
When 이어서 캔버스에서 요소 B 를 누르면
Then B 의 행이 펼쳐지고 캔버스가 펼쳤던 A 의 행은 도로 접히며
  And 사용자가 손으로 펼친 C 의 행은 접히지 않는다
When 이어서 A 와 B 를 함께 고르면(보조키 추가 선택)
Then 아무 행도 자동으로 펼쳐지지 않는다(선택 표시만 남는다)
```

### AC-07 — 편집 게이팅 세 겹 (REQ-01)

```gherkin
Given 대시보드 편집모드가 꺼진 상태로 캔버스 패널이 놓여 있을 때
When 캔버스의 도형 위를 누르면
Then 선택되지 않고 핸들도 없으며 포인터가 가로채이지 않는다(표시 전용)
When 대시보드 편집모드를 켜고 패널의 배치 편집 토글을 켜면
Then 같은 누름이 요소를 고르고 핸들이 나타난다
  And 설정 다이얼로그의 미리보기에서는 forced 로 항상 편집이며 토글 버튼이 감춰진다
```

### AC-08 — 드래그가 유일한 수단이 아니다 (REQ-01, REQ-04)

```gherkin
Given 캔버스 편집이 켜져 있을 때
When 마우스를 전혀 쓰지 않고 조작하면
Then 목록 편집기의 기하 수치 입력이 그대로 남아 있어 위치·크기를 타이핑으로 저술할 수 있고
  And 요소 추가는 목록의 추가 버튼으로, z-order 는 목록의 위/아래 이동 버튼으로 도달하며
  And 선택된 요소는 방향키로 미세 이동, Shift+방향키로 한 격자 칸 이동하고
  And 핸들은 초점을 받을 수 있고 aria-label 을 가진다
```

## 엣지 케이스 (Edge Cases)

### AC-E1 — 무동작 보장: 오버레이를 쓰지 않으면 001 과 같다 (REQ-05)

```gherkin
Given CanvasSurface 에 overlay 렌더 prop 과 포인터 핸들러를 모두 넘기지 않았을 때
When 패널이 렌더되고 데이터가 갱신되면
Then 그리기 결과·프레임 예약 횟수·트윈 동작이 001 과 완전히 같다
  And drawElements 의 반환값을 무시하는 기존 호출부는 영향을 받지 않는다
  And 001 이 쓴 config 를 002 를 거쳐 다시 읽으면 parseCanvasConfig 왕복에 값이 바뀌지 않는다
```

### AC-E2 — 좌표 단일 출처: 핸들이 도형에서 미끄러지지 않는다 (REQ-05, 위험 R1)

```gherkin
Given 요소가 배치된 캔버스가 스테이지 크기 W×H 로 그려져 있을 때
When 패널 크기가 바뀌어 스테이지가 W'×H' 가 되면
Then 오버레이는 스테이지 크기를 스스로 재지 않고 표면이 잰 StageSize 를 그대로 받아 쓰며
  And 핸들의 px 좌표가 projectBox/projectLine/projectPoint 결과와 정확히 일치하고
  And 요소와 핸들이 함께 새 자리로 옮겨 어긋나지 않는다
```

### AC-E3 — 빈 지점 누름은 상위 조작으로 흘려보낸다 (REQ-02, REQ-05, 위험 R2)

```gherkin
Given 설정 미리보기(휠 확대와 패널 크기 조절이 이미 걸린 자리)에 캔버스가 떠 있을 때
When 어떤 요소에도 맞지 않는 빈 지점에 pointerdown 이 오면
Then 선택은 비워지되 이벤트는 소비되지 않아(stopPropagation/preventDefault 없음)
      미리보기의 휠 확대·크기 조절이 종전대로 동작한다
When 요소 위 또는 핸들 위에 pointerdown 이 오면
Then 그 이벤트만 소비되어 상위 조작이 함께 반응하지 않는다
```

### AC-E4 — 렌더 루프 유휴 정지가 편집기 때문에 깨지지 않는다 (REQ-05, 위험 R3)

```gherkin
Given 모든 트윈이 완료되어 루프가 유휴에 든 상태에서 편집이 켜져 있을 때
When 요소를 고르고, 다른 요소로 선택을 옮기고, 핸들 위를 지나가면(호버)
Then 주입된 FrameScheduler 에 프레임 요청이 단 한 건도 들어오지 않는다
When 요소를 끌어 옮기면
Then elements 가 바뀌었기 때문에 프레임이 예약되며(종전의 props 변경 경로 그대로)
  And 한 프레임 사이의 여러 pointermove 는 이미 예약된 프레임에 합류해 한 번만 그려지고
  And 드래그가 끝나면 진행 중 트윈이 없으므로 다음 프레임이 예약되지 않아 루프가 유휴로 돌아간다
  And 주입된 VisibilitySource 가 "보이지 않음" 이면 드래그 중에도 프레임이 예약되지 않는다
```

### AC-E5 — 스테이지 밖 배치는 합법이며 상한에 붙잡히지 않는다 (REQ-03, REQ-04, 위험 R6)

```gherkin
Given 격자 붙임이 켜진 채 요소를 스테이지 경계 너머로 끌 때
When 이동량이 스테이지의 40% 를 넘어가면
Then 좌표가 0..1 로 clamp 되지 않고 그대로 저장되며(001 의 기하 규약)
  And 스냅 래퍼가 오프셋 상한을 무한대로 넘기므로 panelEditAlign 의 기본 상한(±40)에 붙잡히지 않고
  And 스냅은 요소의 중심을 격자에 맞춘다(다른 패널의 격자와 같은 기준)
  And 전부 밖으로 나간 요소도 목록 편집기의 수치 입력으로 회수할 수 있다
```

### AC-E6 — 크기 조절 뒤집힘과 퇴화 도형 (REQ-03, REQ-02, 위험 R7)

```gherkin
Given rect 요소의 오른쪽 아래 모서리 핸들을 잡고 있을 때
When 핸들을 왼쪽 위 모서리 너머로 끌면
Then 쓰는 쪽이 박스를 정규화해(x = min, w = 절대값) 잡은 핸들이 커서 아래에 남고
  And 음수 크기 박스가 config 에 저장되지 않는다
Given 폭 또는 높이가 0 인 퇴화 도형이 있을 때
When 그 위를 누르면
Then 여유 크기의 점 판정으로 폴백해 선택할 수 있다(화면에서 되살릴 수 없게 되지 않는다)
```

### AC-E7 — 실측 글자 폭이 아직 없을 때 (REQ-02)

```gherkin
Given text 요소가 방금 추가되어 아직 한 프레임도 그려지지 않았을 때
When 그 요소의 기준점 근처를 누르면
Then 실측 폭이 없으므로 기준점 둘레의 여유 크기 상자로 폴백해 선택할 수 있고
  And 측정은 여전히 프레임당 1회이며 두 번째 측정원이 만들어지지 않는다
  And 폭이 직전 프레임 값이어서 생기는 최악의 오차는 텍스트 상자의 폭 하나뿐이다(도형은 투영만으로 정해진다)
```

### AC-E8 — SPEC-CANVAS-004 전방 호환 (REQ-06)

```gherkin
Given 002 가 구현한 히트 테스트와 편집 계층이 있을 때
When 004 가 group 노드를 얹는다고 가정하면
Then 히트 결과 타입 { nodeId, partId? } 는 넓히지 않아도 되고(002 는 partId 를 채우지 않는다)
  And 선택·오버레이 키는 최상위 배열 원소의 id 이므로 004 의 복합 키와 충돌하지 않으며
  And 기하 쓰기는 patchNodeGeometry 한 함수를 지나므로 그룹 분기를 더할 지점이 한 곳이고
  And "드래그·선택의 단위는 그룹이며 부품이 아니다" 가 002 문서에 이미 못박혀 있다
  And 002 는 kind:'group' 을 도입하지 않았고 영속 그룹 개념도 만들지 않았다
  And 드래그 층은 배열 index 를 비동기 경계 너머로 들고 다니지 않는다(식별은 언제나 nodeId)
```

## 품질 게이트 (Quality Gate — TRUST 5 / hybrid)

- **Tested**: 신규 코드 TDD(RED-GREEN-REFACTOR). 신규 코드 커버리지 **85% 이상**이 최소선이며,
  DOM 무의존 순수 모듈(`canvasHitTest.ts` · `canvasEditGeometry.ts`)은 **95% 이상**을 목표로 한다.
  이 목표는 임의로 높인 값이 아니라, 같은 순수-모듈 분리 규율을 쓴 SPEC-CANVAS-001 과
  SPEC-HEATMAP-PANEL-001 의 선례에 맞춘 것이다.
  - `canvasHitTest.ts`: 도형 4종 판정, 배열 역순 우선, `visible === false` 건너뛰기, 집기 여유 경계값,
    **텍스트 상자의 세로 중심 기준**(위·아래 경계 각각), 퇴화 도형(반지름 0·길이 0·크기 0) 폴백,
    실측 폭 결측 폴백, 라벨에 별도 히트 영역이 없음.
  - `canvasEditGeometry.ts`: 종류별 핸들 집합, 핸들 px 좌표가 투영 결과와 일치, 이동 델타,
    8핸들 크기 조절, **뒤집힘 정규화**, Shift 종횡비·각도 죔, `fontSize` 핸들의 범위,
    스냅 래퍼가 **상한 무한대**를 넘김, 정렬 래퍼의 백분율↔정규화 변환, `patchNodeGeometry` 가
    `nodeId` 로만 식별.
  - 오버레이·패널 통합은 주입된 `FrameScheduler` 와 `VisibilitySource` 로 검증한다(001 의 주입 지점을
    그대로 쓴다 — fake timer 불필요).
  - 포인터 상호작용은 `pointerdown`/`pointermove`/`pointerup`/`pointercancel` 을 직접 발화시켜 검증하고,
    `setPointerCapture` 는 jsdom 에 없을 수 있으므로 존재 확인 후 호출하는 형태로 둔다.
- **Readable**: 명확한 네이밍, 한국어 코드 주석(프로젝트 규약). 001 산출물 수정은 기존 스타일 준수.
- **Unified**: 신규 파일은 `panels/canvas/` 하위 배치. 핸들 어휘·커서 모양은 `FloorPlanTransformOverlay`
  와 일치. 기존 포매터 일치.
- **Secured**: 신규 백엔드/엔드포인트 없음. config 는 불투명 JSON이며 **스키마를 넓히지 않는다**.
  식 평가·`eval`·동적 함수 생성 없음. **업로드 자산을 쓰지 않으므로 `svgAsset.ts` 가 다루는 스크립트
  격리 문제 자체가 발생하지 않는다**(배경 에셋은 005). 포인터 좌표는 NaN/Infinity 방어 후 사용한다.
- **Trackable**: 커밋 메시지 한국어 Conventional Commits, `SPEC-CANVAS-002` 참조.
- **LSP 게이트(run 단계)**: errors 0, type errors 0, lint errors 0 (tsc `--noEmit` + eslint 클린).
- **무동작 회귀(오버레이 전용 게이트)**: `overlay` 와 포인터 핸들러를 넘기지 않은 모든 기존 시나리오에서
  그리기 결과와 **프레임 예약 횟수**가 001 과 동일해야 한다(AC-E1). 이 게이트를 통과하지 못하면
  오버레이 슬롯이 "쓰지 않아도 영향을 주는" 상태이므로, prop 추가가 아니라 루프 배선이 잘못된 것이다.
- **회귀**: **SPEC-CANVAS-001 의 테스트 스위트를 회귀 게이트로 삼는다** — 001 의 파서·투영·규칙·트윈·
  렌더·표면·패널·편집기 테스트가 **한 건도 수정되지 않은 채** 전량 green 이어야 한다. 수정이 필요해지면
  그것은 "001 의 렌더 경로를 바꾸지 않는다" 는 설계 규율이 깨진 신호이므로 설계를 되돌아본다.
  기존 웹 테스트 스위트 전량 green 도 함께 유지한다.

## Definition of Done

- [ ] REQ-01~06 전부 구현 및 추적성 매핑 충족.
- [ ] AC-01~08, AC-E1~E8 전부 통과.
- [ ] 신규 코드 85% 이상 커버리지(순수 모듈 95% 이상), LSP 0 errors.
- [ ] 사용자 요구 세 문장이 화면에 있음: **도형 팔레트** · **드래그 및 리사이즈** · **클릭하여 속성 설정**.
- [ ] 001 의 `CanvasElement` 유니온 · `parseCanvasConfig` 판정 · `projectBox`/`projectLine`/`projectPoint` ·
      `ellipseParams` · `resolveTextOrigin` · `DrawContext2D` 시그니처·동작 **불변** 확인
      (`drawElement.ts` 변경은 `drawElements` 의 **반환 타입 하나뿐**).
- [ ] `CanvasSurface` 의 rAF 유휴 정지·가시성 게이팅이 **그대로** 유지됨을 프레임 요청 계수로 확인(AC-E4).
- [ ] 스테이지 크기 측정원이 하나이고 핸들 px 가 투영 px 와 일치함을 확인(AC-E2).
- [ ] 빈 캔버스 지점 누름이 상위 조작을 막지 않음을 확인(AC-E3).
- [ ] 드래그 좌표가 clamp 되지 않고, 스냅 래퍼가 상한 무한대를 넘김을 확인(AC-E5).
- [ ] 크기 조절 뒤집힘이 박스를 정규화하며 음수 크기가 저장되지 않음을 확인(AC-E6).
- [ ] 수치 입력이 그대로 남아 드래그가 유일한 수단이 아님을 확인(AC-08).
- [ ] 캔버스 선택이 목록 편집기에서 **한 행만** 펼치고 손으로 펼친 행을 접지 않음을 확인(AC-06).
- [ ] 패널 등록 6지점 **무수정** 확인(001 이 `onConfigChange` 를 미리 흘려 두었다).
- [ ] `PanelSettingsDialog.tsx` 에 본문이 아니라 마운트·감싸기만 더해졌음을 확인(001 §위험 R4).
- [ ] i18n 키가 `ko.json` 과 `en.json` **양쪽**에 존재하고, 키 이름 안에 점이 없음.
- [ ] 백엔드 변경 0 확인(config 스키마 무변경, 신규 엔드포인트 없음, 자산 API 미사용).
- [ ] 신규 npm 의존성 0 확인.
- [ ] 001 테스트 스위트 무수정 전량 green(회귀 게이트) + 기존 웹 테스트 전량 green.
- [ ] 후속 SPEC 확장 지점 유지: `CanvasHit.partId`(004) · `patchNodeGeometry`(004) · 노드 id 키잉(004) ·
      오버레이 슬롯(005 의 배경 층 후보) · 선택 모델(003 의 요소 복제).
