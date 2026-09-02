# SPEC-HEATMAP-PANEL-002 — 인수 기준 (acceptance.md)

관련 SPEC: `.moai/specs/SPEC-HEATMAP-PANEL-002/spec.md`
형식: Given-When-Then (Gherkin)

## 시나리오 (Given / When / Then)

### AC-01 — 도면 배경 위 히트맵 합성 렌더 (REQ-01, REQ-05)

```gherkin
Given 히트맵 패널에 좌표가 지정된 온도 센서와 도면 이미지(floor_plan.image data-URL)가 설정되어 있고
  And heatmap_opacity 가 0.6 으로 설정되어 있을 때
When 패널이 렌더되면
Then 도면 이미지가 히트맵 레이어 뒤(배경)에 종횡비 보존(contain)으로 렌더되고
  And 보간 히트맵이 도면 위에 불투명도 0.6 으로 합성되어 도면이 비쳐 보이며
  And 센서 마커/온도장이 도면과 동일한 정규화 좌표 공간에 정렬된다
```

### AC-02 — 이미지 첨부 시 data-URL 저장·반영 (REQ-02)

```gherkin
Given 도면 이미지가 첨부되지 않은 히트맵 패널에서
When 사용자가 설정에서 크기 상한 이내의 이미지 파일을 첨부하면
Then 시스템은 이미지를 data-URL 로 인코딩하여 config.floor_plan.image 에 저장하고
  And 백엔드 스키마 변경 없이 기존 config 저장 경로(불투명 JSON)로 영속하며
  And 배경 도면이 즉시 반영된다
```

### AC-03 — 마커 드래그로 센서 좌표 갱신 (REQ-03)

```gherkin
Given 배치 편집 모드가 활성이고 도면 위에 센서 마커가 현재 좌표에 표시되어 있을 때
When 사용자가 한 마커를 도면 내 다른 위치로 드래그하여 놓으면
Then 포인터의 컨테이너 상대 위치가 정규화 좌표(0..1)로 변환되어
  And 해당 센서의 sensor_positions[key] 가 실시간 갱신되고 배치가 미리보기되며
  And store 폴링과 기존 히트맵 렌더는 중단되지 않는다
```

### AC-04 — 미배치 센서 배치-인 / 마커 제거 (REQ-03)

```gherkin
Given 좌표가 없는 미배치 센서가 배치 목록에 표시되어 있을 때
When 사용자가 미배치 센서를 도면 위에 드래그하여 놓으면
Then 해당 센서에 정규화 좌표가 부여되어 sensor_positions 에 추가되고
When 사용자가 이후 그 마커를 선택하여 제거하면
Then sensor_positions[key] 항목만 삭제되고 센서 자체는 store 바인딩에서 유지된다
```

## 엣지 케이스 (Edge Cases)

### AC-E1 — 도면 미첨부 graceful (REQ-04)

```gherkin
Given 도면 이미지가 첨부되지 않은 히트맵 패널에서
When 패널 렌더 및 배치 편집 모드 진입이 발생하면
Then 렌더 예외 없이 배경 없는(빈) 정규화 좌표 공간 위에 히트맵/마커가 표시된다
```

### AC-E2 — 좌표 범위 이탈 clamp (REQ-04)

```gherkin
Given 배치 편집 모드에서 마커를 도면 경계 밖으로 드래그할 때
When 드래그가 종료되면
Then 저장되는 정규화 좌표는 [0,1] 범위로 clamp 되어 도면 밖 좌표가 저장되지 않는다
```

### AC-E3 — 대용량 이미지 상한 (REQ-04)

```gherkin
Given 사용자가 설정된 크기 상한(예: 2MB)을 초과하는 이미지를 첨부하려 할 때
When assertImageSizeUnderLimit 검사가 수행되면
Then 이미지는 data-URL 로 무제한 임베드되지 않고 경고/차단되며 축소 안내가 표시된다
```

### AC-E4 — 리사이즈 후 배치 보존 (REQ-05)

```gherkin
Given 도면 위에 센서 마커가 배치된 상태에서
When 사용자가 대시보드 그리드에서 패널 크기를 변경하면
Then 정규화 좌표(0..1) 불변성에 의해 마커/온도장/도면 정렬이 새 크기에서도 보존되어 표시된다
```

### AC-E5 — MVP 하위호환 (REQ-04)

```gherkin
Given floor_plan/heatmap_opacity/editor 필드가 없는 MVP 시절 저장된 히트맵 config 가 로드될 때
When parseHeatmapConfig 가 실행되면
Then 신규 필드는 기본값(배경 없음, opacity=0.6, fit=contain)으로 채워지고
  And 기존 sensor_positions/MVP 필드 의미는 변경 없이 정상 렌더된다
```

## 품질 게이트 (Quality Gate — TRUST 5 / hybrid)

- **Tested**: 신규 코드 TDD(RED-GREEN-REFACTOR). 신규 코드 커버리지 목표 85%.
  - `placement.ts`(`toNormalized`/`fromNormalized`/`clamp01`/`applySnap`)는 순수 함수 단위 테스트 필수 커버(경계값/도면밖/스냅).
  - `imageAsset.ts`(`readImageAsDataUrl`/`assertImageSizeUnderLimit`)는 FileReader 주입/모킹 테스트.
  - `parseHeatmapConfig` 신규 필드 하위호환/기본값 분기 테스트.
  - `SensorPlacementOverlay` 드래그는 `@testing-library/user-event` 포인터 시뮬레이션.
- **Readable**: 명확한 네이밍, 한국어 코드 주석(프로젝트 규약). MVP 컴포넌트 확장은 기존 스타일 준수.
- **Unified**: MVP `heatmap/` 디렉터리 구조/포매터 일치. 신규 파일은 `heatmap/` 하위 배치.
- **Secured**: 신규 백엔드/엔드포인트 없음(1차 data-URL). config 는 불투명 JSON. 첨부 이미지 크기 상한/타입 검증, 좌표 clamp.
- **Trackable**: 커밋 메시지 한국어 Conventional Commits, `SPEC-HEATMAP-PANEL-002` 참조.
- **LSP 게이트(run 단계)**: errors 0, type errors 0, lint errors 0.

## Definition of Done

- [ ] REQ-01~05 전부 구현 및 추적성 매핑 충족.
- [ ] AC-01~04, AC-E1~E5 전부 통과.
- [ ] 신규 코드 85% 커버리지 달성, LSP 0 errors.
- [ ] MVP config 필드/의미 불변(additive-only), MVP AC 회귀 없음.
- [ ] `sensor_positions` 정규화 좌표 스키마를 드래그로 쓰되 후속 SPEC(003 등고선)과 충돌 없음.
