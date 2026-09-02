# SPEC-HEATMAP-PANEL-001 — 인수 기준 (acceptance.md)

관련 SPEC: `.moai/specs/SPEC-HEATMAP-PANEL-001/spec.md`
형식: Given-When-Then (Gherkin)

## 시나리오 (Given / When / Then)

### AC-01 — 다중 센서 보간 온도장 렌더 (REQ-02, REQ-03)

```gherkin
Given store 에 좌표가 지정된 온도 센서 3개의 최신 값이 존재하고
  And 히트맵 패널 config 에 3개 센서의 sensor_positions {x,y} 가 설정되어 있을 때
When 패널이 마운트되어 useStoreChartData 폴링이 1회 완료되면
Then <canvas> 에 3개 센서점을 IDW 로 보간한 연속 온도장이 ImageData 로 렌더되고
  And 센서점 위치의 픽셀 색은 해당 센서 원본 값에 대응하는 색과 일치한다
```

### AC-02 — 상하한 설정 시 색상 매핑 clamp (REQ-05)

```gherkin
Given 보간 온도장에 value_bounds {min:18, max:26} 이 설정되어 있고
  And 센서 값 중 일부가 18 미만 또는 26 초과일 때
When 값→색상 매핑(mapValueToColor)이 수행되면
Then 18 이하 값은 min 색으로, 26 이상 값은 max 색으로 clamp 되어 매핑되고
  And 18~26 구간 값은 color_table 정지점에 따라 보간된 색으로 매핑된다
```

### AC-03 — IDW 0-거리 예외 안전성 (REQ-03)

```gherkin
Given 어떤 격자 픽셀이 한 센서점의 좌표와 정확히 일치할 때
When interpolateIDW 가 해당 픽셀 값을 계산하면
Then 0-거리 분모 예외(NaN/Infinity) 없이 그 센서의 원본 값을 그대로 반환한다
```

## 엣지 케이스 (Edge Cases)

### AC-E1 — 센서 0개 (REQ-04)

```gherkin
Given store tag 필터에 매칭되는 센서가 0개일 때
When 패널이 렌더되면
Then 렌더 예외 없이 빈 상태 안내(예: "표시할 센서가 없습니다")를 표시한다
```

### AC-E2 — 좌표 미지정 센서 (REQ-04)

```gherkin
Given store 에 온도 센서 4개가 있으나 그중 1개는 sensor_positions 에 {x,y} 가 없을 때
When 온도장 보간 입력을 구성하면
Then 좌표 미지정 센서 1개는 보간 입력에서 제외되고 나머지 3개로 정상 렌더되며
  And 설정 UI 는 미배치 센서 1개를 사용자에게 안내한다
```

### AC-E3 — store 폴링 실패 (REQ-04)

```gherkin
Given 이전 폴링으로 정상 렌더된 온도장이 존재할 때
When 다음 store 폴링이 네트워크 오류로 실패하면
Then 패널은 크래시하지 않고 오류 상태를 표시하며 마지막 렌더를 파괴하지 않고 다음 주기에 재시도한다
```

### AC-E4 — 패널 리사이즈 (REQ-03)

```gherkin
Given 히트맵 패널이 렌더된 상태에서
When 사용자가 대시보드 그리드에서 패널 크기를 변경하면
Then canvas 픽셀 버퍼가 새 표시 크기(및 devicePixelRatio)에 맞게 재계산되어 흐림/왜곡 없이 다시 렌더된다
```

## 품질 게이트 (Quality Gate — TRUST 5 / hybrid)

- **Tested**: 신규 코드 TDD(RED-GREEN-REFACTOR). 신규 코드 커버리지 목표 85%.
  - `idw.ts`(`interpolateIDW`, `mapValueToColor`)는 순수 함수로 단위 테스트 필수 커버.
  - `parseHeatmapConfig` 하위호환/기본값 분기 테스트.
  - store 바인딩은 `useStoreChartData` 의 `QueryMatrixFn`/`ResolveKeysFn` 주입으로 테스트.
- **Readable**: 명확한 네이밍, 한국어 코드 주석(프로젝트 규약). 패널 등록 4지점은 기존 스타일 준수.
- **Unified**: 기존 차트 패널 디렉터리 구조/포매터 일치. 신규 파일은 `heatmap/` 하위 배치.
- **Secured**: 신규 백엔드/엔드포인트 없음. config 는 불투명 JSON. 외부 입력(store 값)은 숫자 파싱(`storeChartValue`) 후 사용, NaN/Infinity 방어.
- **Trackable**: 커밋 메시지 한국어 Conventional Commits, `SPEC-HEATMAP-PANEL-001` 참조.
- **LSP 게이트(run 단계)**: errors 0, type errors 0, lint errors 0.

## Definition of Done

- [ ] REQ-01~05 전부 구현 및 추적성 매핑 충족.
- [ ] AC-01~03, AC-E1~E4 전부 통과.
- [ ] 신규 코드 85% 커버리지 달성, LSP 0 errors.
- [ ] 패널 등록 4지점 수정으로 기존 패널 동작 회귀 없음(행위 보존).
- [ ] 후속 SPEC(002/003) 확장 지점(`sensor_positions`, IDW 격자 출력) 유지.
