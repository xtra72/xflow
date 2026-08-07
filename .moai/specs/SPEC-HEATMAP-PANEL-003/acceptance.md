# SPEC-HEATMAP-PANEL-003 — 인수 기준 (acceptance.md)

관련 SPEC: `.moai/specs/SPEC-HEATMAP-PANEL-003/spec.md`
형식: Given-When-Then (Gherkin)

## 시나리오 (Given / When / Then)

### AC-01 — 기존 격자 재사용 등고선 생성 (REQ-01, REQ-04)

```gherkin
Given MVP idw.ts 가 계산한 IDW 보간 격자(Float32Array, gridW×gridH)와 value bounds 가 존재하고
  And 등고선이 활성이며 등치 레벨이 3개로 설정되어 있을 때
When computeContours 가 각 등치값에 대해 실행되면
Then 시스템은 별도 재보간 없이 동일 MVP 격자를 입력으로 marching squares 를 수행하여
  And 3개 등치값 각각의 등치선 세그먼트 집합을 생성한다
```

### AC-02 — 폴링 갱신 시 등고선 재렌더 (REQ-02)

```gherkin
Given 등고선이 활성인 히트맵 패널이 렌더된 상태에서
When store 폴링으로 IDW 격자 값이 변경되면
Then 시스템은 변경된 격자로 등치선을 재계산하여 히트맵 위 오버레이로 재렌더하고
  And 히트맵/센서 마커와 정렬된 상태를 유지한다
```

### AC-03 — 레벨 개수 vs 명시 값 목록 (REQ-05)

```gherkin
Given 등고선 설정에서 level_count 와 explicit levels 의 우선순위를 검증할 때
When explicit levels 가 [20, 24] 로 설정되면
Then resolveLevels 는 level_count 를 무시하고 [20, 24] 를 등치값으로 사용하고
When explicit levels 가 비어 있고 level_count 가 5 이면
Then resolveLevels 는 격자 값 범위(min..max)를 균등 분할한 5개 등치값을 산출한다
```

### AC-04 — 선 스타일·라벨 옵션 (REQ-05)

```gherkin
Given 등고선 설정에서 선 색/두께/점선(dash)과 라벨 표시가 설정되어 있을 때
When 등치선이 렌더되면
Then 각 등치선이 설정된 색/두께/dash 스타일로 그려지고
  And 라벨 표시가 켜진 경우 각 등치선에 등치값 라벨이 배치된다
```

## 엣지 케이스 (Edge Cases)

### AC-E1 — saddle(모호) 케이스 결정성 (REQ-01, REQ-04)

```gherkin
Given 한 셀의 4코너가 marching squares saddle 케이스(5 또는 10)를 형성할 때
When computeContours 가 해당 셀을 처리하면
Then 결정적 규칙(예: 셀 중앙값 기준 분기)으로 모호성을 해소하여
  And 끊기거나 교차하는 잘못된 선 없이 일관된 세그먼트를 생성한다
```

### AC-E2 — 토글 off / 빈 격자 graceful (REQ-04)

```gherkin
Given 등고선 토글이 off 이거나 IDW 격자가 비어 있을 때
When 패널이 렌더되면
Then 렌더 예외 없이 등고선이 표시되지 않고 기존 히트맵 렌더는 그대로 유지된다
```

### AC-E3 — 범위 밖 등치값 무시 (REQ-04)

```gherkin
Given 격자 값 범위가 18..26 인데 등치값 30 이 설정되어 있을 때
When resolveLevels 가 레벨을 산출하면
Then 범위 밖 등치값 30 은 필터링되어 잘못된(빈/전체) 선을 그리지 않고 안내된다
```

### AC-E4 — 격자 불변 시 재계산 생략 (REQ-04, 성능)

```gherkin
Given 등고선이 활성이고 이전 폴링과 동일한(불변) IDW 격자가 반환될 때
When 다음 폴링이 완료되면
Then 시스템은 격자 참조/해시 기준 memoize 로 등고선을 재계산하지 않고 기존 렌더를 재사용한다
```

### AC-E5 — 재보간 금지 (REQ-01, REQ-04)

```gherkin
Given 등고선 생성이 요구될 때
When 등치선 입력 격자를 구성하면
Then 등고선은 MVP idw.ts 격자를 그대로 입력으로 사용하며 별도 보간을 수행하지 않는다
```

## 품질 게이트 (Quality Gate — TRUST 5 / hybrid)

- **Tested**: 신규 코드 TDD(RED-GREEN-REFACTOR). 신규 코드 커버리지 목표 85%.
  - `marchingSquares.ts`(`computeContours`/`resolveLevels`/`segmentsToPath`)는 순수 함수 골든 케이스 단위 테스트
    필수 커버(셀 case, saddle 5/10, 경계 셀, 범위 밖 레벨, 빈 격자).
  - `parseHeatmapConfig` 의 `contour` 하위호환/기본값 분기 테스트.
  - `ContourLayer` memoize(격자 불변 시 재계산 생략) 동작 테스트.
- **Readable**: 명확한 네이밍, 한국어 코드 주석(프로젝트 규약). MVP 컴포넌트 확장은 기존 스타일 준수.
- **Unified**: MVP `heatmap/` 디렉터리 구조/포매터 일치. 신규 파일은 `heatmap/` 하위 배치.
- **Secured**: 신규 백엔드/엔드포인트 없음. config 는 불투명 JSON. 격자 값은 숫자 방어(NaN/Infinity), 범위 밖 레벨 필터.
- **Trackable**: 커밋 메시지 한국어 Conventional Commits, `SPEC-HEATMAP-PANEL-003` 참조.
- **LSP 게이트(run 단계)**: errors 0, type errors 0, lint errors 0.

## Definition of Done

- [ ] REQ-01~05 전부 구현 및 추적성 매핑 충족.
- [ ] AC-01~04, AC-E1~E5 전부 통과.
- [ ] 신규 코드 85% 커버리지 달성, LSP 0 errors.
- [ ] 등고선 입력이 MVP `idw.ts` 격자 재사용임을 검증(재보간 없음).
- [ ] `contour` config additive-only, 토글 off 시 MVP/002 렌더 회귀 없음.
