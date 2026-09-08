# SPEC-CANVAS-001 — 인수 기준 (acceptance.md)

관련 SPEC: `.moai/specs/SPEC-CANVAS-001/spec.md`
형식: Given-When-Then (Gherkin)

## 시나리오 (Given / When / Then)

### AC-01 — 도형 구성 렌더 + 규칙에 따른 겉모습 변경 (REQ-02, REQ-03, REQ-04)

```gherkin
Given 캔버스 패널 config 에 rect / ellipse / line / text 요소가 각 1개씩 있고
  And rect 요소는 시리즈 "tank.level" 에 바인딩되어 있으며
  And rect 요소의 규칙 표가 [gt 80 → fill 빨강], [gt 50 → fill 노랑] 순서로 설정되어 있을 때
When 패널이 마운트되어 usePanelSeriesData 폴링이 1회 완료되고 최신값이 90 이면
Then 4개 요소가 배열 순서대로 canvas 에 그려지고
  And rect 요소의 채움색은 첫 일치 행인 [gt 80] 의 빨강으로 그려진다
```

### AC-02 — 첫 일치 우선(first-match-wins)과 기본 스타일 병합 (REQ-04)

```gherkin
Given 요소의 기본 스타일이 { fill: 회색, stroke: 검정, strokeWidth: 2 } 이고
  And 규칙 표가 [gt 80 → {fill: 빨강}], [gt 50 → {fill: 노랑, strokeWidth: 4}] 순서일 때
When 바인딩 최신값이 90 으로 평가되면
Then 첫 일치 행 [gt 80] 만 적용되어 fill 은 빨강이 되고
  And 두 번째 행은 평가되지 않아 strokeWidth 는 기본값 2 로 남으며
  And 패치에 없는 stroke 는 기본 스타일의 검정을 그대로 유지한다
```

### AC-03 — 상태 전이 트위닝 (REQ-05)

```gherkin
Given 요소에 tween { duration_ms: 300, easing: 'ease-out' } 이 설정되어 있고
  And 현재 적용 스타일의 fill 이 노랑일 때
When 다음 폴링에서 값이 바뀌어 규칙 일치 결과의 fill 이 빨강이 되면
Then 색은 즉시 바뀌지 않고 300ms 동안 노랑→빨강으로 이징 보간되며
  And 보간 도중 규칙 결과가 다시 바뀌면 현재 보간 중인 색에서 새 목표로 다시 트윈한다(값이 튀지 않는다)
  And text 와 visible 처럼 보간이 성립하지 않는 속성은 트윈 없이 즉시 전환된다
```

### AC-04 — 문구 템플릿 토큰 치환 (REQ-04)

```gherkin
Given text 요소의 기본 문구가 "{name}: {value}{unit}" 이고 decimals 는 1, unit 은 "℃" 이며
  And 바인딩 시리즈의 표시명이 "실외기" 이고 최신값이 23.456 일 때
When 문구가 렌더되면
Then "실외기: 23.5℃" 로 단순 치환되어 그려지고
  And 정의되지 않은 토큰이 섞여 있으면 치환하지 않고 원문 그대로 남긴다
```

## 엣지 케이스 (Edge Cases)

### AC-E1 — 요소 0개 (REQ-05)

```gherkin
Given 캔버스 패널 config 의 elements 가 빈 배열일 때
When 패널이 렌더되면
Then 렌더 예외 없이 빈 상태 안내(예: "구성된 요소가 없습니다")를 표시한다
```

### AC-E2 — 바인딩 시리즈 결측 / 값 없음 (REQ-05)

```gherkin
Given 요소가 시리즈 "sensor.a" 에 바인딩되어 있으나 조회 결과에 해당 시리즈가 없거나 값이 비어 있을 때
When 규칙 평가와 문구 치환이 수행되면
Then 요소는 사라지지 않고 기본 스타일로 그려지며
  And {value} 토큰은 결측 표기(기본 "-")로 치환되고
  And 규칙 표에 nodata 행이 있으면 그 행이 정상적으로 일치한다
```

### AC-E3 — 일치하는 규칙 없음 (REQ-04)

```gherkin
Given 요소의 규칙 표가 [gt 80], [gt 50] 두 행이고
When 바인딩 최신값이 10 이어서 어떤 행도 일치하지 않으면
Then 오류 없이 요소의 기본 스타일과 기본 문구가 그대로 사용된다
```

### AC-E4 — 렌더 도중 폴링 실패 (REQ-05)

```gherkin
Given 이전 폴링으로 정상 렌더된 캔버스 프레임이 존재할 때
When 다음 폴링이 네트워크 오류로 실패하면
Then 패널은 크래시하지 않고 마지막으로 그린 프레임을 그대로 유지하며
  And 오류 배지만 덧붙이고 다음 주기에 재시도한다
```

### AC-E5 — 패널 리사이즈 (REQ-02)

```gherkin
Given 정규화(0..1) 좌표로 배치된 요소들이 렌더된 상태에서
When 사용자가 대시보드 그리드에서 패널 크기를 변경하면
Then canvas 백킹 버퍼가 새 표시 크기와 devicePixelRatio 에 맞게 재계산되고
  And 각 요소는 화면상 같은 상대 위치에 남아 흐림/왜곡/이동 없이 다시 그려진다
```

### AC-E6 — 렌더 루프 유휴 정지와 가시성 게이팅 (REQ-05)

```gherkin
Given 모든 트윈이 완료되어 애니메이션할 것이 없는 상태일 때
When 다음 프레임 시점이 오면
Then requestAnimationFrame 이 더 이상 예약되지 않는다(루프 유휴)
  And 주입된 VisibilitySource 가 "보이지 않음"을 알리면 진행 중이던 루프도 프레임을 예약하지 않으며
  And 다시 보이게 되면 한 프레임을 즉시 그려 최신 상태를 반영한다
```

## 품질 게이트 (Quality Gate — TRUST 5 / hybrid)

- **Tested**: 신규 코드 TDD(RED-GREEN-REFACTOR). 신규 코드 커버리지 **85% 이상**이 최소선이며,
  DOM 무의존 순수 모듈(`canvasRules.ts` · `canvasGeometry.ts` · `canvasTween.ts` · `canvasText.ts` ·
  `canvasConfig.ts`)은 **95% 이상**을 목표로 한다. 이 목표는 임의로 높인 값이 아니라, 같은 순수-모듈 분리
  규율을 쓴 SPEC-HEATMAP-PANEL-001 이 신규 코드 99% 를 달성한 선례에 맞춘 것이다.
  - `canvasRules.ts`: first-match-wins 순서, 미일치 폴백, `between` 경계 포함, `nodata`, 부동소수 `eq/ne` 허용 오차.
  - `canvasTween.ts`: 이징 4종, 색 RGB 보간, 트윈 도중 목표 변경 시 현재값 재출발, `duration_ms: 0` 즉시 전환.
  - `parseCanvasConfig`: 결측·손상·미지 필드 입력에서 예외 없이 기본값 보정.
  - 데이터 바인딩은 `usePanelSeriesData` 주입 지점을 모킹해 검증하며, 해당 경로의 훅 테스트는
    I18n/QueryClient Provider 를 요구하지 않아야 한다(기존 제약).
- **Readable**: 명확한 네이밍, 한국어 코드 주석(프로젝트 규약). 등록 6지점 수정은 기존 스타일 준수.
- **Unified**: 기존 패널 디렉터리 구조·포매터 일치. 신규 파일은 `panels/canvas/` 하위 배치.
- **Secured**: 신규 백엔드/엔드포인트 없음. config 는 불투명 JSON. 식 평가·`eval`·동적 함수 생성 없음
  (규칙 표가 파서를 대신하는 이유이기도 하다). 외부 입력(시리즈 값)은 숫자 파싱 후 NaN/Infinity 방어.
- **Trackable**: 커밋 메시지 한국어 Conventional Commits, `SPEC-CANVAS-001` 참조.
- **LSP 게이트(run 단계)**: errors 0, type errors 0, lint errors 0 (tsc `--noEmit` + eslint 클린).
- **회귀**: 기존 웹 테스트 스위트 전량 green 유지(등록 6지점 수정의 행위 보존 확인).

## Definition of Done

- [ ] REQ-01~05 전부 구현 및 추적성 매핑 충족.
- [ ] AC-01~04, AC-E1~E6 전부 통과.
- [ ] 신규 코드 85% 이상 커버리지(순수 모듈 95% 이상), LSP 0 errors.
- [ ] 패널 등록 6지점 수정으로 기존 패널 동작 회귀 없음(행위 보존), 기존 웹 테스트 전량 green.
- [ ] i18n 키가 `ko.json` 과 `en.json` **양쪽**에 존재.
- [ ] 백엔드 변경 0 확인(패널 config 는 불투명 JSON 유지, 신규 엔드포인트 없음).
- [ ] rAF 루프가 유휴 시 정지하고 비가시 상태에서 프레임을 예약하지 않음을 테스트로 확인.
- [ ] 후속 SPEC(002/003) 확장 지점(`elements[]` 정규화 좌표 스키마, `TweenSpec`, 렌더 루프) 유지.
