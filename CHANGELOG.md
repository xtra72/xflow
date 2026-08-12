# Changelog

이 프로젝트의 주요 변경 사항을 기록한다.
형식은 [Keep a Changelog](https://keepachangelog.com/ko/1.1.0/)를 따르며,
[Semantic Versioning](https://semver.org/lang/ko/)을 적용한다.

## [Unreleased]

### 추가 — ChirpStack LoRaWAN 에이전트 (`chirpstack` 에이전트 + `chirpstack-in` 노드)

- **ChirpStack LoRaWAN Network Server 의 MQTT 업링크를 수신하여 측정값별로 팬아웃하는 신규 에이전트/노드 추가 (SPEC-CHIRPSTACK-001, Tier L)**

  ChirpStack 가 발행하는 MQTT 업링크(토픽 `application/#`)를 구독해 `deviceInfo`/`object`/`rxInfo`/`time` 을 디코드하고, 업링크 `object` 필드를 **측정값 1개당 메시지 1개**(`type="event"`, top-level `timestamp`=업링크 `time`, `payload={value}`, `metadata.measurement`/`device`/`tags`)로 팬아웃한다. 이는 기존 Lua `script`+`split` 파이프라인을 대체하며, 다운스트림 read path(`$.payload.value`, `$.metadata.measurement`, `$.metadata.device.*`, `$.metadata.tags.*`, `$.timestamp`)를 그대로 보존한다(REQ-FROZEN-02). `devEui` 기준 디바이스 자동 생성/조회(UUID v4)와 `deviceName`/`tags` 메타데이터 지속화를 수행하고, 선택적으로 comm-state(`device_state.<trigger>`, online/rssi/snr/last_seen 을 device_state 스트림에 fold)를 방출한다.

  - **신규 패키지**: `internal/agent/chirpstack/` (`agent.go`, `config.go`, `decode.go`, `message.go`, `provider.go`, `watchdog.go`, `registration.go` + 테스트 + `testdata/packet.json`). 기존 `system/mqtt_agent.go` 트랜스포트·`device_id_repo`·`device/registry` 재사용.
  - **신규 노드**: `internal/node/chirpstack.go` (`chirpstack-in` SourceNode) — 수신 전용, store/influx 소비자에 직결. 노드 device 그룹 승격(dedup) 활용으로 에이전트는 `unit_id`(=devEui)만 방출.
  - **신규 타입**: 에이전트 `chirpstack`, 노드 `chirpstack-in`. Wiring 3곳: `cmd/xflowd/main.go`(RegisterChirpStackTypes + import), `internal/node/registry.go`(chirpstack-in 등록), `pkg/flow/validate.go`(`agentRefRequiredTypes`).
  - **분기(Divergence, as-implemented — spec.md § Implementation Notes IN-1~4)**: (1) flow-validation 경로는 존재하지 않는 `internal/flow/validate.go` 가 아니라 실제 `pkg/flow/validate.go` 의 `agentRefRequiredTypes`(존재성 화이트리스트 아님·agent_ref 검증 목적). (2) tags 지속화는 `DeviceMetadata.Labels map[string]string`(ChirpStack tags 가 map 이므로 `Tags []string` 아님); 메시지 `metadata.tags` verbatim pass-through 불변. (3) comm-state `state` 그룹은 `payload` 에 typed 값(online:bool/rssi:int/snr:float/last_seen_ms:int64), 노드가 `msg.Type()`=`device_state.<trigger>` 계층형 설정. (4) rxInfo 디코딩은 M5(comm-state) 소속, device_state 는 기존 recvCh 에 `record` 판별자로 접힌 단일 전송(folded stream).
  - **품질**: M1~M6 구현(커밋 `eb0339f9`/`6bb3a965`/`ca024d1d`/`bc08f3ce`/`b26a8850`/`88807142`). 신규 코드 커버리지 89.4%(목표 85% 초과), TRUST 5 PASS(Critical 0), go build/vet/gofmt 클린·신규 코드 lint 0, REQ-FROZEN-01~04 준수, 무회귀. Gaps: 라이브 ChirpStack 브로커 스모크 테스트는 이연(packet.json 픽스처 기반 테이블 주도 테스트로 대체).
  - **관련**: SPEC-CHIRPSTACK-001 v0.1.0(구현 완료, Tier L). 관련 SPEC: SPEC-DEVICE-001, SPEC-DEVICE-IDENTITY-001(Phase D). 적용에는 `bin/xflowd` 재빌드·재시작 + flow config 의 기존 Lua `script`+`split` 단계를 `chirpstack-in` 노드로 교체 필요.

### 추가 — store-write `data_type: "auto"` 값 기반 타입 추론 (SPEC-STORE-003 v0.3.1)

- **store-write 노드 config 의 `data_type` 에 `"auto"` sentinel 을 추가 (additive, non-breaking, 기존 빈 data_type 의 string 동작 보존)**

  단일 store-write 노드가 `key_template` 로 측정별 키를 만들며 boolean/int/float 등 **가변 타입 측정값**(예: 온도=float, 모드=int, 전원=boolean)을 저장할 때, 고정 리터럴 `data_type`(예: `float`)은 한 타입만 담을 수 있어 boolean 값에서 `ErrTypeMismatch`("store: value type does not match registered data_type")가 발생했다. `"auto"` 는 쓰기 값의 Go 타입을 M7 추론 매핑으로 판별해 **키별로** 구체 타입을 고정한다. 키 단위 타입은 일정하므로 첫 쓰기 추론이 올바른 타입을 보장하며, 스위치 노드 없이 단일 노드로 이종 측정값을 정확한 타입으로 저장한다.

  - **system**: `DataTypeAuto` sentinel + `resolveWriteDataType` 헬퍼(추론/폴백), `SetWithMeta` 의 "auto" 추론 경로. 추론 불가(`nil`/channel/func) 시 타입 고정을 생략하고 동적 string 폴백에 맡긴다(새 실패 모드 없음).
  - **node**: store-write `Configure` / `parseMetricSpec` 가 `"auto"` 를 유효값으로 수용(노드 레벨 + `metrics[].data_type`).
  - **품질**: 특성 테스트(boolean/int/float/mixed/nil) + 노드 config 수용 테스트 추가. build/test/vet/gofmt/golangci-lint 클린.
  - **관련**: SPEC-STORE-003 v0.3.1(M7 확장). 적용에는 flow config `metrics[].data_type: "auto"` 설정 + 데몬 재시작 필요(스토어의 stale 타입 등록은 메모리에만 존재).

### 변경 — egress 메타데이터 슬림화 제거 (full 그룹 전달)

- **egress(WS tap / egress 노드 / influxdb-write)에서 agent·device 그룹을 id-only 로 축소하던 `SlimGroupsToID` 의 슬림 로직을 제거 (full 그룹 전달)**

  클라이언트·스토리지·대시보드가 `device.name` / `agent.type` 등을 직접 필요로 하는 경우가 많아, egress 는 이제 `type/id/name` 전체(full)를 그대로 내보낸다. 내부 노드 간 흐름은 종전과 동일(항상 full)하며, 함수는 슬림화만 중단하고 내부 제어 마커 `_slimKeep` 제거 책임은 유지한다. 호출처(tap.go/egress_metadata.go/influxdb_write.go)는 변경하지 않도록 함수 이름을 보존했다(최소 변경 범위). enrich 의 `_slimKeep` 설정과 ws expand opt-in 기계는 무해하게 잔존한다.

  - **pkg/message**: `slim.go` 의 `slimGroupKeys`/`parseSlimKeep` 삭제, full pass-through(+마커 제거)로 변경.
  - **품질**: 슬림 단언 테스트 6종(slim/expand/enrich_slim/debug_group/influxdb_write_group/tap)을 full 보존 단언으로 전환. build/test/vet/gofmt/golangci-lint 클린.
  - **주의(트레이드오프)**: wire 페이로드 크기 증가, influxdb-write 태그에 `agent.type`/`device.name` 등 추가 → 태그 카디널리티 증가 가능. 적용에는 `bin/xflowd` 재빌드·재시작 필요(Go 코드 변경).
  - **관련**: 정식 SPEC 없는 `message-slim-metadata` 기능의 동작 역전. SPEC 문서 대상 없음(CHANGELOG 기록).

### 개선 — 패널 설정 데이터 소스 시리즈 선택 UI (표시 필터 · 인라인 세부 정보 · 동적 바인딩 분리)

- **패널 설정(SPEC-PANEL-SETTINGS-001)의 데이터 소스 시리즈 선택 UX 를 개선 (Non-breaking, 프론트엔드 전용, 신규 백엔드 0, 신규 의존성 0, config JSON shape 무변경)**

  3분할 셸 재설계(M1~M5) 위에 데이터 소스 시리즈 선택 경험을 다듬었다(M6, REQ-15~22). 컬럼 필터를 **표시(display) 필터**로 통일하고, 시리즈 편집 지점을 선택 행의 **인라인 펼침 세부 정보** 단일 지점으로 통합했으며, "동적 바인딩"을 표시 필터와 **명시적으로 분리된 토글**로 노출했다. 모든 변경은 additive 이며 기존 `selection_mode`/`tag_filters`/`color` config 의미는 불변이다(하위호환: 기존 `selection_mode:'tag'` 저장 패널은 동적 바인딩 토글 ON + `tag_filters` 보존으로 로드).

  - **통일된 표시 필터(REQ-15)**: 선택 테이블의 모든 컬럼 필터(key/name/metric/tag)를 표시 필터로 일반화 — **같은 컬럼 내 다중 값 OR, 서로 다른 컬럼 간 AND**. 태그는 단일 컬럼이 아니라 **태그 키(종류)별** 로 처리 — 같은 태그 키 내 다중 값 OR, 서로 다른 태그 키 간 AND(예: `device_id ∈ {A,B}` AND `type ∈ {report}`). 신규 순수 매처 `storeColumnValueFilter.ts`.
  - **이름 편집 · 컬럼 순서(REQ-16/17)**: 사용자 표기 "별칭"→"이름"(저장 필드 `alias` 유지). 선택 테이블 컬럼 순서를 **key · 이름 · metric · tag** 로 재배치.
  - **행 펼침 세부 정보(REQ-18/20/21)**: "시리즈별 세부 정보"(이름 편집 · 색상 · 선 스타일 · (heatmap)좌표)를 별도 `SelectedSeriesList` 그룹이 아니라 **선택된(체크된) 각 시리즈 행의 인라인 펼침/접힘 상세**로 제공(단일 편집 지점). heatmap 센서 좌표(x/y)를 이름 뒤 2열 레이아웃으로 세부 정보에 통합. 미선택 행은 상세 없음.
  - **키 설명 라인 제거(REQ-19 폐지/superseded)**: 펼침 상세의 키·종류·태그 설명 서브라인이 테이블 컬럼 + alias 컬럼 키 에코와 중복되어 제거 — 키는 키 컬럼 + alias 컬럼 mono 에코로만 노출.
  - **동적 바인딩 토글(REQ-22)**: 명시적 "동적 바인딩" 토글을 표시 필터와 분리하여 신설(신규 패널 기본 OFF=명시 선택, ON=poll 시 태그 기준 동적 해석 `selection_mode:'tag'` + `tag_filters`). 세부 정보 편집은 토글/`selection_mode` 와 완전 독립.
  - **필터 팝오버 잘림 수정 · 폰트 확대**: 컬럼 필터 팝오버가 컨테이너 경계에서 잘리던 문제를 viewport-clamped fixed positioning 으로 수정. 세부 정보 영역 폰트를 본문(sm) 수준으로 확대.
  - **품질**: REQ-15~22 구현(REQ-19 폐지, 재번호 없음). 회귀 프론트 236 files / 2956 tests 전량 통과, `tsc -b` exit 0, build exit 0. 신규 순수 매처 `storeColumnValueFilter.ts` 는 design.md 파일 계획에 포함(비계획 스코프 0). 신규 npm 의존성 0, 백엔드/불투명 config JSON shape 무변경.
  - **관련**: SPEC-PANEL-SETTINGS-001 v0.5.0(구현 진행 중, M1~M6, Tier L). M6 = REQ-15~22. 잔여: `tag_filters` 는 `Record<string,string>` 유지(다중값-per-키는 표시 필터 상태 관심사, 동적 바인딩 파생은 기존 best-effort last-value).

### 추가 — 히트맵 패널 등고선(marching squares) 오버레이

- **히트맵 패널(SPEC-HEATMAP-PANEL-001 MVP)에 등고선(contour lines / iso-lines, marching squares) 오버레이를 가산 (Non-breaking, 프론트엔드 전용, 신규 백엔드 0, 신규 의존성 0)**

  MVP 가 생성하는 **IDW 보간 스칼라 격자(temperature field grid)** 위에 사용자가 지정한 등치값(iso-value)마다 **marching squares** 를 적용해 등치선 경로를 산출하고, 히트맵 위에 **선택(optional) 토글** 오버레이로 렌더한다. 핵심 제약은 **재보간 금지** — 등고선은 MVP 가 이미 계산한 **동일 격자**를 재사용하므로 히트맵 색과 등치선이 같은 스칼라장을 반영해 시각적으로 일관된다. 이를 위해 `interpolateIDW` 호출을 `HeatmapPanel` 로 상승시켜 패널당 1회만 실행하고, 동일 `Float32Array` 를 히트맵 canvas 와 등고선 레이어가 공유한다. 모든 신규 config 필드는 **추가만(additive)** 하며 MVP/002 필드 의미는 불변이다. SPEC-002(도면 배경/드래그 배치)와 독립이며 그 위에 층으로 쌓인다. run 커밋 `07a4efd4`.

  - **순수 로직(TDD)**: `marchingSquares.ts`(`computeContours` — 셀 case 0..15 분기 + 모서리 선형 보간 + saddle(5·10) 셀 중앙값 결정적 처리, `resolveLevels` — explicit 우선·균등 분할·범위 밖 필터·빈 격자 방어, `segmentsToPath` — SVG path `d` 문자열 변환). DOM 없이 골든 케이스 27 테스트, 커버리지 95.5%.
  - **렌더/컴포넌트**: `ContourLayer.tsx`(격자 + contour config → `computeContours` → SVG `<path>` 오버레이, viewBox 0..1 + `preserveAspectRatio=none` 로 리사이즈 정합, field 참조 `useMemo`, 선 스타일/등치값 `<text>` 라벨, 커버리지 100%), 레이어 순서 히트맵(z10)→등고선(z15)→마커(z20).
  - **확장(행위 보존)**: `heatmapConfig.ts`(`contour` 하위호환 additive 파싱, 미설정 `undefined`/기본 enabled=false), `HeatmapCanvas.tsx`(격자 계산을 상위로 상승 — `field`/`gridW`/`gridH`/`hasData` props consume, 기존 테스트 통과), `HeatmapPanel.tsx`(`interpolateIDW` `useMemo` 상승 + `ContourLayer` z-15 마운트), `PanelSettingsDialog.tsx`(등고선 섹션). i18n `lib/i18n/{ko,en}.json`(+8키).
  - **분기(Divergence, as-implemented — spec.md §구현 노트 IN-1~5)**: (1) 렌더 방식 = SVG `<path>` 오버레이(오케스트레이터 확정, canvas stroke 미채택). (2) 격자 공유 = `HeatmapPanel` 로 상승(interpolateIDW 패널당 1회, 동일 Float32Array 공유 → 재보간 금지 R3 엄격 충족, `HeatmapCanvas` field-consume 리팩터 행위 보존). (3) 등치값 라벨 = SVG `<text>` user-unit fontSize(비균등 스케일 왜곡 가능, 별도 HTML 오버레이 미추가 YAGNI). (4) `parseContour` 미설정 반환 = `undefined`(floor_plan/editor 선례 통일). (5) path/label testid = 값 대신 인덱스(`contour-path-0`).
  - **품질**: REQ-01~05 전량 구현. 회귀 프론트 2836 tests 통과, LSP 0(tsc `--noEmit` + eslint 클린), 신규 순수 코드 커버리지 95%+(`marchingSquares` 95.5%, `ContourLayer` 100%). 신규 npm 의존성 0, 백엔드 무변경(불투명 JSON config 유지). Gaps: `PanelSettingsDialog` 등고선 UI 다이얼로그 테스트·SVG 라벨 시각 왜곡 jsdom 검증·E2E 실렌더 미수행(MVP 동일 선례).
  - **관련**: SPEC-HEATMAP-PANEL-003 v1.0.0(구현 완료, `07a4efd4`, Tier M). MVP SPEC-HEATMAP-PANEL-001 을 가산 확장, SPEC-002 와 독립. 후속: 라벨 충돌 회피 등은 향후 SPEC.

### 추가 — 히트맵 패널 도면 배경 + 드래그 앤 드롭 센서 배치 에디터

- **히트맵 패널(SPEC-HEATMAP-PANEL-001 MVP)에 floor-plan 이미지 배경 + 히트맵 합성 불투명도 + 시각적 드래그 앤 드롭 센서 배치 에디터를 가산 (Non-breaking, 프론트엔드 전용, 신규 백엔드 0, 신규 의존성 0)**

  MVP 히트맵 패널에 **공간 맥락(spatial context)** 을 부여했다. 사용자가 도면 이미지(floor-plan)를 첨부하면 보간 히트맵 레이어 **뒤(배경)** 에 도면이 렌더되고, 히트맵은 **설정 가능한 불투명도(opacity)** 로 도면 위에 합성되어 온도장이 실제 공간 위에 겹쳐 보인다. 또한 MVP 에서 숫자 입력으로만 지정하던 센서 좌표를 도면 위에 마커로 겹쳐 올린 **시각적 드래그 앤 드롭 배치 에디터**로 보완했다. 좌표계는 **정규화(0..1)** 를 유지하여 패널 리사이즈 후에도 배치가 보존된다. 모든 신규 config 필드는 **추가만(additive)** 하며 MVP 필드 의미는 불변이다. run 커밋 `bb32958a`.

  - **순수 로직(TDD)**: `placement.ts`(`toNormalized`/`fromNormalized`/`clamp01`/`applySnap` — DOM 없이 좌표 변환·스냅·[0,1] clamp, 커버리지 100%), `imageAsset.ts`(`readImageAsDataUrl` — FileReader data-URL 인코딩 + `assertImageSizeUnderLimit` — 2MB 상한 경고/차단, FileReader 모킹 테스트).
  - **렌더/컴포넌트**: `FloorPlanBackground.tsx`(data-URL 도면 배경 레이어, contain/cover fit, 미첨부 시 graceful), `SensorPlacementOverlay.tsx`(정규화 좌표 absolute 마커 오버레이 — `FacilityLinePanel` 배치 패턴 참고, 드래그 배치 + 미배치 배치-인 + 마커 제거), `HeatmapPanel.tsx`(배경→히트맵(opacity)→마커 오버레이 z-스택 + 편집 모드 — store 폴링 비파괴).
  - **확장 4지점**: `heatmapConfig.ts`(`floor_plan`/`heatmap_opacity`/`editor` 하위호환 additive 파싱), `HeatmapPanel.tsx`(배경 레이어 + 편집 모드), `renderDashboardPanel.tsx`(배선), `PanelSettingsDialog.tsx`(이미지 첨부/제거/미리보기 + opacity 슬라이더 + fit select + 에디터 옵션). i18n `lib/i18n/{ko,en}.json`.
  - **분기(Divergence, as-implemented — spec.md §구현 노트 IN-1~IN-4)**: (1) 이미지 저장 = data-URL-in-config + 2MB 상한(오케스트레이터 확정 1차 결정, 백엔드 asset 엔드포인트는 후속 이연). (2) 배경 합성 = 별도 배경 DOM 레이어 + CSS opacity(방식 (a)) — `HeatmapCanvas.tsx` 불변(방식 (b) canvas drawImage 미채택). (3) 드래그 테스트 = `fireEvent` + MouseEvent 기반 PointerEvent 폴리필(@testing-library/user-event 미설치, 신규 의존성 0). (4) 편집 모드 토글 = 패널 내부 런타임 비영속 상태(설정 다이얼로그는 에디터 옵션 snap/marker_size + 미배치 힌트만 제공).
  - **품질**: REQ-01~05 전량 구현. 신규 코드 커버리지 96~100%(`placement.ts` 100%), LSP 0(tsc `--noEmit` + eslint 클린), 회귀 프론트 826 tests 통과. 신규 npm 의존성 0, 백엔드 무변경(data-URL, 불투명 JSON config 유지).
  - **관련**: SPEC-HEATMAP-PANEL-002 v1.0.0(구현 완료, `bb32958a`, Tier M). MVP SPEC-HEATMAP-PANEL-001 을 가산 확장. 후속: SPEC-HEATMAP-PANEL-003(등고선, marching squares — 002 와 독립, 그 위에 층으로 쌓임).

### 추가 — 히트맵 대시보드 패널 (MVP) — Canvas 2D IDW 온도 히트맵

- **신규 대시보드 PanelType `heatmap` 도입 — 공간 온도 센서를 IDW 보간하여 Canvas 2D 로 렌더하는 온도장 시각화 (Non-breaking, 프론트엔드 전용, 신규 백엔드 0)**

  대시보드에 신규 패널 타입 `heatmap` 을 가산했다. 공간에 배치된 다수의 온도 센서를 store 태그 필터로 동적 바인딩하고, 각 센서의 수동 배치 좌표 `{x,y}` 를 기준으로 **IDW(Inverse Distance Weighting)** 보간을 수행해 연속적인 온도장을 **HTML Canvas 2D** 에 픽셀 단위로 렌더한다. 보간값은 사용자 지정 상·하한으로 clamp 된 뒤 색상표로 색에 매핑된다. 코드베이스 최초의 실제 2D drawing canvas 도입(허용된 결정)이며, 백엔드/엔드포인트/전송 계층 변경은 없다(패널 config 는 기존과 동일 불투명 JSON, store REST 폴링 `POST /api/v1/store/{agent}/query` 재사용). 커밋 `69294ced`.

  - **순수 로직(TDD)**: `idw.ts`(`interpolateIDW` — 0-거리 안전 분기 + `mapValueToColor` — clamp·색상표 정지점 보간), `heatmapConfig.ts`(`HeatmapPanelConfig` 타입 + `parseHeatmapConfig` 결측/손상 입력 기본값 보정 하위호환 파서), `heatmapJoin.ts`(센서 최신값 `aggregation:'last'` + 배치 좌표 결합, 좌표 미배치 센서는 보간 입력 제외 + 설정 UI 노출용 별도 목록).
  - **렌더/패널**: `HeatmapCanvas.tsx`(저해상 IDW 격자 → `ImageData` → devicePixelRatio 업스케일 blit, `ResizeObserver` 리사이즈 재계산), `HeatmapPanel.tsx`(`LineChartPanel` isStore 분기 미러링으로 `useStoreChartData` 태그 바인딩 + 센서값·좌표 결합, 센서 0개·미배치·폴링 실패 graceful).
  - **와이어링 4지점**: `stores/uiStore.ts`(`PanelType` 유니온 + `panelDefaultSize` + `createDefaultPanel`), `pages/dashboard/renderDashboardPanel.tsx`(렌더 switch), `pages/dashboard/AddPanelDialog.tsx`(chart 카테고리 옵션), `pages/dashboard/PanelSettingsDialog.tsx`(heatmap 전용 설정 섹션 — 태그필터/센서 x·y/value_bounds/color_table/IDW power·resolution). 신규 디렉토리 `web/src/pages/dashboard/panels/heatmap/`. i18n `lib/i18n/{ko,en}.json`.
  - **분기(Divergence, as-implemented — spec.md §구현 노트 IN-1)**: 좌표-센서값 결합 로직을 `HeatmapPanel.tsx` 내부(plan.md 설계)가 아니라 순수 모듈 `heatmapJoin.ts` 로 분리 — 단위 테스트 격리 목적이며 신규 추상화 계층은 아님. 그 외 계획 파일·설계 일치.
  - **품질**: REQ-01~05 전량 구현. 신규 코드 커버리지 99%, LSP 0(tsc `--noEmit` + eslint 클린), 회귀 프론트 786 tests 통과. 신규 의존성 0, 신규 백엔드 엔드포인트/저장소 0.
  - **관련**: SPEC-HEATMAP-PANEL-001 v1.0.0(구현 완료, `69294ced`, Tier M). 범위 분할 후속: SPEC-HEATMAP-PANEL-002(floor-plan 배경 + 드래그 배치 에디터), SPEC-HEATMAP-PANEL-003(등고선, marching squares).

### 추가 — MODBUS Client RTU 지원 + 트랜스포트 선택 + 그룹별 폴링 + 데이터타입 확장 + 노드 런타임 set_config

- **기존 `modbus-tcp` 에이전트에 Modbus RTU(시리얼) 지원 + 트랜스포트 선택 + 레지스터 그룹별 독립 폴링 + 데이터타입 4순열/raw + 노드 런타임 재구성(`set_config`) 을 가산 (Non-breaking, 가산형, `modbus-tcp` type id 보존)**

  기존 MODBUS/TCP 클라이언트 에이전트(`internal/agent/modbus/`, type id `modbus-tcp`)를 **신규 패키지 복제 없이 동일 패키지 내에서 확장**했다. TCP 전용이던 트랜스포트를 `transport: tcp | rtu` 디스크리미네이터로 선택 가능하게 하고, in-house RTU 마스터(CRC-16 poly 0xA001)를 추가했으며, 레지스터 그룹별 독립 폴링 주기·`raw` 패스스루 + 완전한 4순열 바이트순서·플로우 노드를 통한 런타임 재구성(`set_config`)을 가산했다. `transport` 미지정 기존 설정은 자동으로 `tcp` 로 해석되어 **기존 동작 바이트 동일**(무회귀). 9개 마일스톤 M1~M9 + 후속(`unit_id` 런타임 변이)로 완성.

  - **트랜스포트 선택 + RTU 마스터(M1~M4)**: `ModbusTransport` 인터페이스를 `SendAndReceive(ctx, unitID byte, pdu []byte) → respPDU` 로 리팩터링하여 MBAP 프레이밍 + txID 관리를 `ModbusTCPTransport` 내부로 이동(TCP 와이어 동작 byte-identical, golden test). 신규 `ModbusRTUTransport` — RTU ADU `[unitID][PDU][CRC-lo][CRC-hi]` 프레이밍, CRC-16(poly 0xA001, init 0xFFFF, 리틀엔디언 부착), T3.5 프레임 간 정적, 반이중 turnaround mutex. CRC/ADU 를 `rtu_crc.go`/`rtu_adu.go` 로 분리(본체 `transport_rtu.go`). RTU 트랜스포트는 **시리얼 버스당 단일 공유 인스턴스**(multi-drop, 단일 turnaround mutex — 버스 정확성; 한 디바이스 Close 가 공유 포트 닫음 한계 기록). go.bug.st/serial 재사용(신규 의존성 0).
  - **레지스터 그룹별 독립 폴링(M5~M6)**: 각 `register_group` 에 선택적 `poll_interval` 추가 — 미지정 그룹은 에이전트/디바이스 기본 주기로 폴백(하위 호환, 기존 단일 `time.Ticker` 는 "전 그룹 기본 주기" 특수 케이스). 그룹별 스케줄러로 한 그룹의 지연이 타 그룹 정시성에 영향을 주지 않게 함. 런타임 주기 변경은 기존 `PollingConfigurable`(`SetPollInterval`→`pollResetCh`→`pollTicker.Reset`) 경로 재사용.
  - **데이터 타입 변환 확장(M5, `internal/modbus/types.go`)**: `raw` 패스스루(읽은 uint16 워드 무변환 전달) + 완전한 **4순열 바이트순서**(ABCD/BADC/CDAB/DCBA = word-swap × byte-swap). 기존 word-swap 의미는 별칭으로 보존(`big_endian`≡ABCD, `little_endian`≡CDAB, 바이트 동일). 알 수 없는 타입/바이트순서는 파싱/설정 오류로 처리. `internal/modbusserver` 와 공유 유지(중복 구현 회피).
  - **상태·통계(M7)**: 디바이스별·그룹별 성공/오류/지연 카운터를 **에이전트 레이어**(`devStats`/`groupStats` 맵)에 배치(`device.go` 의 offline/reconnect 로직 불변). 한계: 런타임 `set_config` 로 추가된 신규 그룹의 per-group 통계는 init-불변 맵이라 생성되지 않음(문서화 한계).
  - **노드 런타임 재구성 `set_config`(M8~M9 + 후속)**: `ModbusAgent.Process` 스위치에 신규 명령 `set_config` 추가(`set_config.go`). 플로우 노드가 기존 제네릭 `callAgentProcess`(`internal/node/modbus.go`) 경로로 `agent.Process([]byte)` 경계에서 재구성 명령 발행 — **에이전트 재시작 없이** 레지스터 그룹·그룹별 `poll_interval`·디바이스 파라미터·타입 오버레이를 `a.mu`(RWMutex) 보호 하에 갱신. init 경로와 동일 파싱/검증 규칙 재사용. **런타임 가변**: 레지스터 그룹, 그룹별 주기, `unit_id`/timeout/reconnect, 타입/바이트순서 오버레이. **init 전용(런타임 거부)**: 트랜스포트 `tcp↔rtu` 전환 + RTU 시리얼 하드웨어 파라미터(port/baud/data_bits/stop_bits/parity) — 오류 반환, 부분 적용 없음. `unit_id` 는 후속(`0fe9170d`)에서 `ModbusDevice` atomic `unitID` SSOT 로 완성(`config.UnitID`=생성 시드, 런타임 판독 `dev.UnitID()`; `parseUnitIDParam` 이 init `toByte` 보다 엄격 — 범위 초과/비정수 거부).
  - **프론트엔드(`web/src/config/agentSchemas.ts`)**: `transport` select + RTU 시리얼 필드(port/baud/data_bits/stop_bits/parity) + 데이터타입/바이트순서 필드를 스키마 주도로 가산(신규 React 컴포넌트 불필요). `tsc --noEmit` 클린.
  - **분기(Divergence, as-implemented — spec.md §8 IN-1~IN-9)**: (1) `SendAndReceive(ctx, unitID, pdu)→respPDU` 시그니처 변경 + MBAP/txID 를 TCP 트랜스포트로 이동(TCP byte-identical). (2) RTU CRC/ADU 를 `rtu_crc.go`/`rtu_adu.go` 로 분리. (3) RTU 트랜스포트 = 시리얼 버스당 단일 공유 인스턴스 + 단일 turnaround mutex(공유 포트 Close 한계). (4) 바이트순서 별칭 `big_endian`≡ABCD/`little_endian`≡CDAB. (5) M7 통계 에이전트 레이어 배치(device.go 불변, 신규-그룹 per-group 통계 미생성 한계). (6) `unit_id` 런타임 변이 **분기 해소** — atomic `unitID` SSOT, §5.5.1 완전 부합. (7) init 전용 필드 런타임 거부(부분 적용 없음). (8) 노드측 `set_config` 전용 operation 미추가 — 기존 `callAgentProcess` 경로로 AC-08 충족(최소 스코프). (9) Optional float64/uint64(4워드) 미구현(SPEC Optional 그대로).
  - **품질**: AC-01~08 전량 pass. build/vet/golangci-lint 클린(0 issues), 커버리지 `internal/agent/modbus` **85.6%** / `internal/modbus` **99.4%**, `go test -race` 클린, 기존 modbus/modbusserver/node 테스트 무회귀 green, `tsc --noEmit` 클린. `register.go` + `cmd/xflowd/main.go` 불변(type id 보존). 신규 Go/npm 의존성 **0**(go.bug.st/serial v1.6.4 재사용). 잔여 위험: 실제 하드웨어 RTU 타이밍(T3.5/turnaround)은 mock 수준 검증(plan.md §5), `defaultRTUSerialOpener` 는 하드웨어 전용 경로로 설계상 커버리지 제외.
  - **관련**: SPEC-MODBUS-006 v1.0.0(구현 완료, `833af42e`·`d7b01886`·`35fb409d`·`46f8e0ad`·`4e478657`·`0fe9170d`). depends_on: SPEC-MODBUS-001. SPEC-MODBUS-001/003 의 `modbus-tcp` 표면을 가산 확장.

### 추가 — 설비 제어 예약 패널 (규칙 테이블 + 모달) + Trigger 스케줄 규칙 메타·발화 게이팅

- **설비 제어 예약 전용 대시보드 패널 도입 + Trigger 스케줄 규칙 메타/fire-gating 백엔드 (Non-breaking, 가산형)**

  선행 SPEC-TRIGGER-PANEL-001 의 범용 `trigger-config` 패널 위에, xsfm(지하철 설비) 제어 예약에 특화된 **신규 `facility-schedule` 패널**을 가산했다(범용 패널과 공존, 대체 아님). 운영자는 예약 제어 규칙을 표(테이블)로 조회하고 모달로 생성/편집하며, 각 규칙에 이름·유효기간·대상 설비(TARGET)·실행 계획(PLAN)·제어 명령(ACTION)·우선순위·활성 여부를 지정한다. 백엔드는 규칙 메타를 발화 시점에 존중한다. 백엔드 M1~M2(커밋 `03e8827b`) + 프런트 M3~M6(커밋 `fb40c4b2`)로 완성.

  - **백엔드 규칙 메타 + 발화 게이팅(M1~M2, `internal/node/trigger.go`)**: `TriggerSchedule` 에 `name`/`valid_from`/`valid_to`/`priority`/`enabled` 5필드 확장. `enabled` 는 `*bool` **트라이스테이트**(config 부재→`true` 무회귀, 명시 `false` 구별). `makeHandler` 가 generation-token 게이트 뒤에서 **`enabled` ∧ `withinValidity`** 로 발화를 게이팅 — 비활성이거나 유효기간(서버 로컬·날짜 단위 **양끝 inclusive**, 빈 경계=무제한, 잘못된 날짜=무제한) 밖이면 emit 을 스킵하되 **타이머는 유지**(재활성/기간 진입 시 재개). 날짜 포맷은 `YYYY-MM-DD` + RFC3339 수용. `buildMessage` 는 `rule_name`/`priority` 메타를 **조건부 pass-through**(기존 스케줄 방출 byte-identical). `-race` 클린, 신규 함수 커버리지 100%.
  - **프런트 특화 패널(M3~M6, `web/`)**: 신규 `facility-schedule` PanelType + `FacilitySchedulePanel`(6컬럼 테이블 — SCHEDULE 이름+유효기간(무기한)/TARGET 전체(line)·그룹·개별 배지/PLAN/ACTION 2축 라벨/PRIO 안정 정렬/STATE 토글) + `FacilityRuleModal`(생성·행별 편집). `TargetPicker` 는 패널 config `agentId` 로 대상 열거(설정 시 `useStations`/`useGroups`/`useXsfmDevices` id 셀렉터, 미설정 시 free-form + "이름으로 지정" 폴백 → `group_name`/`device_name`). `ActionEditor` 는 v1 전원/풍량 **2축**만(모드 축 없음, payload 에 `mode` 키 무방출). PLAN 은 `TriggerScheduleEditor` 를 단일 원소 배열로 재사용(타이밍 키 전용). dual-write 는 선행 `triggerPanelUtils`(`buildFullTriggerConfig`/`patchNodeConfigInDefinition`/`detectConflict`) 재사용 — configureNode(live)+updateFlow(persist), 404 persist-only, last-write-wins. 순수 로직은 `facilityScheduleUtils`(셀렉터/액션 build·parse·label·정렬·검증).
  - **분기(Divergence, as-implemented — spec.md §8 IN-1~IN-7)**: (1) `Enabled` = `*bool` 트라이스테이트. (2) 날짜 포맷 `YYYY-MM-DD` + RFC3339 수용, 잘못된 날짜=경계 무제한. (3) `rule_name`/`priority` 조건부 메타 pass-through(기존 스케줄 byte-identical). (4) TARGET 열거 = 패널 config `agentId` 기반(설정 시 id 셀렉터, 미설정 시 free-form+이름 폴백). (5) ACTION 인코딩 = `{power:bool|null, fanSpeed:number|null}`, 무-축/범위밖 저장 차단, `mode` 키 무방출. (6) PLAN = `TriggerScheduleEditor` 단일 원소 배열 재사용(타이밍 키 전용). (7) dual-write + STATE 토글 단일 `persist()` 경로(범용 패널과 동일 규약 계승).
  - **품질**: `go test ./...` exit 0(42 pkgs)·`-race` 클린·백엔드 M1 신규 함수 커버리지 100%, 프런트 vitest 2370 pass(+50)·`tsc`/eslint 클린. 기존 범용 `trigger-config` 패널 + trigger 노드 6종 스케줄·per-schedule payload 해결 순서 무회귀. 신규 외부 의존성 0, 신규 백엔드 엔드포인트/저장소 0(노드 config dual-write 재사용).
  - **관련**: SPEC-TRIGGER-SCHED-001 v0.3.0(구현 완료, `03e8827b` + `fb40c4b2`, Tier L). RD-1~9 반영, OQ 잔여 없음. priority 기반 런타임 충돌 해소·xsfm 모드 축(Auto/Sleep) 도입은 향후 SPEC.

### 추가 — Trigger 노드 대시보드 패널 + 백엔드 live 타이머 재등록

- **대시보드에서 Trigger 노드를 직접 운영하는 전용 패널 도입 + `TriggerNode.Configure` live 타이머 재무장 (Non-breaking, 가산형)**

  기존 Trigger 노드 스케줄/페이로드 편집은 flow 에디터 속성 위젯에서만 가능했고 재배포를 거쳐야 반영됐다. 여기에 (1) 대시보드에서 특정 trigger 노드를 타겟팅해 스케줄을 편집하면 **즉시(live)** 노드에 반영되고 동시에 flow 정의로 **지속화(persist)**되는 dual-write 패널, (2) 이를 뒷받침하는 백엔드 live 재등록을 가산했다. 백엔드 M1(커밋 `31a0e08`) + 프런트 M2~M5(커밋 `cf00c74`)로 완성.

  - **백엔드 live 재등록(M1, `internal/node/trigger.go`)**: `TriggerNode.Configure` 가 running 노드 live 재설정 시 기존 타이머를 취소하고 새 스케줄로 재무장한다. **started-gate + `StateRunning` 체크**로 재무장을 running 노드의 live 재설정 케이스에만 격리(초기 `Configure→Init` 은 이중 등록 안 함). **`rearmGen` generation 토큰**으로 cancel→re-register 창의 stale in-flight 발화를 drop(double-fire·orphan 없음). 빈 스케줄은 전 타이머 취소 후 유효 IDLE(오류 아님). live 재무장이 유발한 `buildMessage` payload 읽기 vs 동시 `Configure` payload 쓰기 race 를 신규 `payloadMu` 로 보호. `-race` 클린, 노드 테스트 green.
  - **대시보드 패널(M2~M5, `web/`)**: 신규 `trigger-config` PanelType + `TriggerConfigPanel`. `useNodeTypeInstances('trigger')` 기반 타겟 피커(running/stopped 배지). `TriggerScheduleEditor` 재사용 스케줄 CRUD. 패널-로컬 `payloadCatalog` + per-schedule 인라인 스냅샷 주입(선택 시점 JSON deep-clone, 카탈로그 후속 편집 비소급 — 재선택 시에만 갱신). dual-write: LIVE `configureNode` + PERSIST `getFlow`→`node.data` 패치→`updateFlow`(patch-then-PUT, 다른 노드/와이어 보존). 404(미실행)는 persist-only + 통지, 동시 편집은 last-write-wins + 대시보드 통지(낙관적 잠금 미도입). 순수 유틸 `triggerPanelUtils`.
  - **분기(Divergence, as-implemented — spec.md §8 IN-1~IN-6)**: (1) 재무장 게이트를 started + StateRunning 으로 세분(Paused/Stopped 미재등록). (2) `payloadMu` 신규 추가(-race 로 표면화된 Configure-vs-fire race). (3) 트리거 config 키는 구현 SSOT 인 `schedules`(SPEC 산문 `trigger_schedules` 정정). (4) dual-write patch 는 reactflow flat `node.data` 병합. (5) 스냅샷 주입 = 선택 시점 JSON deep-clone(`payloadRef` 미직렬화). (6) 충돌 감지 = 지속 config vs hydration 기준선 비교.
  - **품질**: 노드 재무장/취소/no-double-fire/롤백/빈-스케줄/폴백 신규 함수 커버리지 100%, `go test ./...` exit 0(42 pkgs), `-race` 클린, 프런트 vitest 2319 pass(+32)·`tsc`/eslint 클린. 기존 대시보드 패널·flow 에디터 무회귀(가산형). 신규 외부 의존성 0.
  - **관련**: SPEC-TRIGGER-PANEL-001 v0.3.0(구현 완료, `31a0e08` + `cf00c74`, Tier L). RD-1~11 반영, OQ-1~6 확정. 공유 명명 페이로드 저장소·낙관적 잠금은 향후 SPEC.

### 추가 — xsfm 토픽 `{line_code}` placeholder (SPEC-XSFM-LINE-001 amendment v0.4.0)

- **sub/pub 토픽 템플릿에 `{line_code}` placeholder 도입 (Non-breaking, 가산)** — 예: `cmd/{line_code}/{station_code}/{place_code}/bse9000/{device_index}`.

  - **Outbound(명령/pub) 파생 렌더**: `{line_code}` 를 디바이스의 파생 라인(`ResolveLine(device.Station)`)으로 채운다. 라인 해석은 로스터 락 **밖에서** 수행(lineHint 패턴, `composeName` 미러 — station 레지스트리 락을 로스터 락 안에 중첩하지 않아 RWMutex 재진입 deadlock 회피). 라인 미해석 시 **빈 세그먼트**로 렌더(`cmd//st99/...`).
  - **Inbound(상태/sub) 무시**: `{line_code}` 는 구독 시 와일드카드, 파싱 시 추출되나 **디바이스 식별에는 무시**(식별=station_code+place_code+index, 라인은 station→line 파생 SSOT). `Device.Address`/composite key 에 저장하지 않아 역류·중복 방지.
  - **범위**: direct 모드 토픽 한정(port 모드+노드 무관), 기존 placeholder 무회귀. `commandHasLineCode` 게이팅.
  - **품질**: 신규/변경 함수 커버리지 100%, `go test ./...` green(42 pkgs), `-race` 클린, 무회귀. SPEC-XSFM-LINE-001 정식 amendment(v0.3.1→v0.4.0, Module 8/RD-8, §7 `## Amendments`).

### 추가 — xsfm 이름 기반 제어 셀렉터(device_name / group_name)

- **`xsfm` 에이전트·노드에 사람이 읽는 이름(`device_name`/`group_name`)으로 제어 대상을 지정하는 이름 기반 셀렉터 도입 (Non-breaking, 비침습 가산 셀렉터)**

  기존 제어 대상 지정은 `device_id`(개별) 또는 `station`/`line`/`group_id`(셀렉터)로만 가능했다. 여기에 기존 셀렉터의 자료구조·우선순위·디스패치 경로를 **무회귀**로 두고, 이름을 **정확히 하나의 대상으로 해소한 뒤 기존 개별/fan-out 경로를 재사용**하는 얇은 해소 레이어를 **가산**했다. 백엔드+노드 M1~M3(`2acb980d`)로 핵심 완성. 프런트엔드 이름 제어 UI(M4)는 선택·저우선으로 이연 — 에이전트+노드 레벨에서 기능 완전 사용 가능.

  - **이름 리졸버(신규 `name_resolver.go`)**: `DeviceByName`/`GroupByName` — 공백 trim 후 정확 일치, **대소문자 구분**(case-sensitive, RD-6). ≥2 매치 시 신규 센티널 `ErrAmbiguousName` 으로 **거부(무방출, fail-closed, RD-2)**, 0 매치 시 `ErrDeviceNotFound`/`ErrGroupNotFound`, 정확히 1개일 때만 진행. 스냅샷-안전 락 규율(로스터/그룹 레지스트리 락을 스냅샷 후 해제, 락 미중첩 — RWMutex 재진입 deadlock 회피). `GroupByName` 은 **전 타입 그룹**(custom + 파생 station/line) 표시명 매칭(RD-5), 타입 간 충돌도 `ErrAmbiguousName` 로 안전 거부.
  - **셀렉터 우선순위 체인 확장**: `dispatchControl`/`handleSelectorControl` 이 확정 순서 **`device_id > device_name > station > line > group_id > group_name`**(RD-4, "개별 먼저")로 확장. `device_name` → 개별 제어(controlDevice), `group_name` → 그룹 fan-out(GroupMembers → fanOutControl) 로 해소 후 **기존 경로 재사용**(제어 의미론 재구현 없음). 신규 `processRequest` 필드 `DeviceName`/`GroupName`(json `device_name`/`group_name`, CRUD `name` 과 별개) + `fillFromParams` 승격.
  - **노드 pass-through**(`internal/node/xsfm.go`): `buildXsfmControlCommand` 가 `device_name`/`group_name` 을 top-level 셀렉터로 방출, `hasXsfmControlCommand` 가 이름 셀렉터 존재 시 제어로 라우팅, `xsfmExtractDeviceName`/`xsfmExtractGroupName` 미러. 다중 셀렉터 공존 시 노드는 드롭 없이 모두 top-level 로 실어 우선순위 판정을 에이전트에 위임.
  - **분기(Divergence, as-implemented — spec.md §7)**: (1) 우선순위 체인이 `dispatchControl`(device_id) + `handleSelectorControl`(device_name if-guard + group_name 최종 case) 로 분산 배치, 개별 제어는 `handleIndividualControl` 추출(동작 보존). (2) `GroupByName` 전 타입 매칭을 위해 `handleListGroups` 에서 `allGroups()`(custom+파생) 추출·공유(DRY). (3) `group_name` fan-out 집계 셀렉터 라벨 `selectorRef{Type:"group_name"}`(원 셀렉터 종류 보존). (4) 리졸버는 신규 `name_resolver.go` 배치(사양 권장 대안), `control.go` 불변.
  - **품질**: 신규 함수 커버리지 **100%**, `go test ./...` exit 0(42 pkgs, 0 FAIL), `-race` 클린, 기존 셀렉터/CRUD `name`/그룹 fan-out 무회귀(NF-01). MQTT 토픽/페이로드 규약 불변(순수 논리 해소 레이어, NF-03). 신규 외부 의존성 0.
  - **관련**: SPEC-XSFM-NAMESEL-001 v0.3.0(핵심 M1~M3 구현 완료, `2acb980d`, Tier M). M4(프런트 이름 제어 UI) 이연(optional follow-up). SPEC-XSFM-001 의 셀렉터 표면을 가산 확장.

### 추가 — xsfm 에이전트 수신 forward 옵션 + 상태 방출 모드(event/interval/both)

- **`xsfm` 에이전트에 (1) 수신 파싱-상태 전달 옵션과 (2) 상태 방출 모드를 도입 (Non-breaking, 비침습 가산 방출 레이어)**

  기존 xsfm 방출은 상태 **변경 시에만** `device_state_changed` 를 내보내는 이벤트 기반(on-change) 단일 모델이었다. 여기에 기존 방출 경로를 **재작성하지 않고** 두 가산 기능을 추가했다. 기본 설정(forward off, mode=event)은 현행 동작과 **바이트 동일**(무회귀)이다. 백엔드 M1~M4(`5fc11209`) + 프런트 M5(`6fb084c3`).

  - **수신 파싱-상태 전달 옵션(RD-1)**: `forward_received_to_node`(기본 `false`) 옵션 — ON 이면 수신된 **모든** 디바이스 상태를 파싱/정규화된 형태(원시 브로커 바이트 아님)로 노드에 전달하는 패스스루 탭(`device_state_received`, 단일 디바이스 메시지, `changed_fields` 생략). 상태 **변경 여부와 무관**하게 매 유입마다 방출하며(변경분만 방출하는 `device_state_changed` 와 구분), `state_emit_mode` 와도 독립. `ingestState`(direct·port 공유 시임) 단일 지점 배치로 **mode-agnostic**(양 모드 동일 적용, 특별 케이스 없음, RD-4).
  - **상태 방출 모드(RD-2)**: `state_emit_mode` enum `{event, interval, both}`(기본 `event`). `event`=현행 on-change. `interval`=`state_emit_interval` 주기(기본 60s, RD-3)로 **전체 등록 디바이스 풀 스냅샷**(`device_state_snapshot`, 단일 `{timestamp, devices:[...]}` 배열 메시지, offline 포함)을 방출하고 on-change 는 **억제**. `both`=on-change + 주기 스냅샷 heartbeat 병행. 주기 방출기는 `startOfflineMonitor` 를 미러(min-interval 가드·`monitorWG`/`stopCh` 공유·스냅샷-후-락해제, 채널 송신 중 락 미보유).
  - **설정 파싱·검증**: `state_emit_mode` enum 위반 시 `ErrInvalidStateEmitMode` 반환(`transport_mode`/`liveness_source` 검증 패턴 동형, 조용한 폴백 없음). `state_emit_interval` 기본 60s(offline_timeout 기본 90s 와 정합)·음수 거부·유효 tick < `minStateEmitInterval` 시 하한 클램프. 신규 파일 `emit_mode.go`(`emitStateReceived`/`startStateEmitter`/`emitStateSnapshot`).
  - **전이 이벤트 불변**: `device_online`/`device_offline` 전이 이벤트는 방출 모드와 무관하게 항상 방출(게이팅 제외). MQTT 토픽/페이로드 규약 불변(두 기능은 `msgCh` 노드 방출 계층에만 작용).
  - **프런트엔드(M5)**: `agentSchemas.ts` XSFM_FIELDS 에 3개 컨트롤 추가 — `forward_received_to_node` 토글 + `state_emit_mode` select(event/interval/both) + `state_emit_interval` duration(mode=interval|both 시 표시). `Transport.Options` 키 1:1 매핑, 기본값 무회귀(off/event/60s).
  - **분기(Divergence, as-implemented — spec.md §7)**: (1) `state_emit_mode` select 는 raw enum 값 노출(공용 FormField 위젯 옵션 라벨 맵 부재, 기존 xsfm/HVACR select 관례 계승). (2) interval 모드 on-change 억제는 `ingestState` 기존 단일 방출 지점의 복합 조건(별도 플래그/경로 없음). (3) `device_state_snapshot` 단일 배열 메시지(N per-device 아님), `device_state_received` 단일 디바이스·`changed_fields` 생략(RD-5 확정 그대로). (4) `Stop` 변경 없음 — 주기 방출기가 offline 모니터의 `monitorWG`/`stopCh` 재사용(한 번의 Wait 로 두 goroutine 커버).
  - **품질**: xsfm 커버리지 89.3%, `go test -race` 클린, `go test ./...` exit 0(42 pkgs, 0 FAIL), 프런트 vitest 2287 pass·`tsc` 클린. 기본 config byte-identical 무회귀. 신규 외부 의존성 0.
  - **관련**: SPEC-XSFM-AGENT-IO-001 v0.3.0(구현 완료, `5fc11209` + `6fb084c3`, Tier M). SPEC-XSFM-001 의 이벤트 기반 단일 방출 모델을 가산 확장.

### 추가 — xsfm 라인 1급화 + 코드 기반 주소 체계 + 디바이스 네이밍

- **`xsfm` 에이전트에 라인(line)을 1급 엔티티로 승격 + station/line/custom 을 관통하는 코드 기반 통일 주소 체계 + 디바이스 자동 이름 규칙 확장 (Non-breaking, 비침습 레이어 추가)**

  기존에 역사의 속성(`StationRegistryEntry.Line`, 역사당 단일 문자열)으로만 파생 존재하던 라인을, 코드·이름·정렬을 가진 1급 엔티티로 승격했다. SPEC-XSFM-GROUP-001 의 "**별도 레지스트리를 비침습 가산 레이어로 신설**" 패턴을 그대로 적용해 기존 station→line 파생·fan-out·그룹 동작의 **구조를 변경하지 않고** 구현했다. 백엔드 M1~M5(`ef4f28a7`) + 프런트 M6(`e8678033`)로 완성.

  - **라인 레지스트리(신설 레이어)**: `Line{Code, Name, Order}` + 자체 RWMutex + write-through atomic 영속(`StationRegistry`/`GroupRegistry` 락·영속 패턴 미러). 신규 파일 `line_registry.go`. 명령 `add_line`/`remove_line`/`list_lines`(Order 오름차순). **멤버 미저장** — `line:<code>` 멤버는 항상 `DevicesByLine` 로 파생(station→line SSOT `ResolveLine` 보존). 참조 중인 라인 `remove_line` 은 `ErrLineInUse` 로 거부(dangling 방지, RD-5). 빈 라인(역사 0개) 유효(RD-1).
  - **코드 기반 통일 주소**: 커스텀 그룹에 사용자 코드 도입 — 그룹 id `custom:<name>` → `custom:<code>`(name 표시 전용). 제어/셀렉터를 `station:<code>`/`line:<code>`/`custom:<code>` 로 통일. 통일 코드 포맷 `^[a-z0-9][a-z0-9_-]*$` + §4.6 slugify(신규 `code.go`: NFC 정규화, 한글 Revised Romanization, 비허용문자→`-`, 충돌 시 접미 번호). 노드 레이어(GROUP-001 Module 7)는 문자열 pass-through 로 **코드 변경 없음**.
  - **디바이스 네이밍**: 자동 생성 이름을 `{line}:{station}:{place}:{index:03d}`(4-세그먼트)로 합성. 라인 미해석 시 라인 세그먼트를 생략해 `{station}:{place}:{index:03d}`(3-세그먼트, 하위호환·무회귀, RD-4). `nameOverridden`(sticky) 이름 보존, 라인 후지정 시 4-세그먼트 재계산. 보조 인덱스 키(정규화 int)는 불변(표시 vs 매칭 분리).
  - **로드 마이그레이션(1회성·비파괴·멱등)**: `station.Line` 문자열 → Line 엔티티 ensure-create(레거시 라인 코드 원문 보존), 레거시 `custom:<name>` → `custom:<slug>` 승격(name 원문 표시 보존, 예: `"2층 창고"` → `2cheung-changgo`).
  - **프런트엔드**: `useLine` 훅 + `XsfmLinesTab`(라인 관리 — add_line/list_lines, Order 정렬) + `XsfmGroupsTab` 그룹 코드 입력 필드 + `AgentDetailPanel` 라인 탭 가산. 디바이스 이름은 백엔드 산출값을 그대로 렌더(프런트 표시 코드 변경 불필요).
  - **분기(Divergence, as-implemented — spec.md §7)**: (1) `add_group{code}` 를 **선택 파라미터**로 구현(GROUP-001 `add_group{name}` 테스트 무회귀). (2) 코드 포맷 검증을 station 코드에는 **미강제**(기존 `ST-101`/`S1` 대문자 코드 회귀 방지). (3) 마이그레이션 시 라인 코드는 **slugify 미적용**(원문 보존, station.Line 참조 정합). (4) `golang.org/x/text` 를 slugify NFC 정규화용 **직접 의존성**으로 승격(`go mod tidy`). (5) 디바이스 이름 표시는 **프런트 변경 없음**(기존 `device.name` 렌더가 새 포맷 자동 반영).
  - **품질**: xsfm 커버리지 89.1%, `go test -race` 클린, `go test ./...` exit 0(0 FAIL), 프런트 vitest 2275 pass, `tsc` 클린. SPEC-XSFM-GROUP-001 / station / node / api-service 무회귀. 신규 외부 신규 패키지 0(x/text 는 표준 확장 모듈 직접화).
  - **후속 — 역사 역번호(station_number)**: 역사에 선택 필드 `station_number`(역번호, 실세계 역번호, 내부 코드와 구분) 추가 — `add_station`/`list_stations`/`station_registered` 관통, 중복 역번호는 경고만. 디바이스 자동 이름 station 세그먼트가 역번호 우선·코드 폴백(`{line}:{역번호|코드}:{place}:{index:03d}`, 내부 주소/셀렉터는 코드 유지). 프런트 `XsfmStationsTab` 표시·편집. config-seed 경로는 미배선(런타임 `add_station` 만). 원 EARS 범위 밖 직접 후속(`a35c468b`), spec.md §7.6.
  - **관련**: SPEC-XSFM-LINE-001 v0.3.1(구현 완료 + 역번호 후속, `ef4f28a7` + `e8678033` + `a35c468b`, Tier M). SPEC-XSFM-001 의 라인=역사 파생 속성 가정을 의도적으로 갱신.

### 추가 — xsfm 그룹 1급(first-class) 개념 도입 (그룹 엔티티·다대다 멤버십·일괄 제어)

- **`xsfm` 에이전트에 그룹(group)을 1급 개념으로 승격 — 그룹 엔티티·다대다 멤버십·커스텀 CRUD·기본 그룹 자동 동기화·그룹 셀렉터 일괄 제어 + 프런트 그룹 탭/패널 (Non-breaking, 비침습 레이어 추가)**

  기존에 디바이스 속성(`Device.GroupID`, 디바이스당 단일 태그)으로만 존재하던 그룹을, 이름·타입·멤버를 가진 1급 엔티티로 승격했다. 기존 station→line 레지스트리·디바이스 로스터·fan-out 의 **구조를 변경하지 않고** 그룹 레지스트리를 별도 레이어로 신설하는 비침습 방식으로 구현했다. 백엔드 M1~M5(`ae53b344`) + 프런트 M6~M7(`97c2bafd`)로 완성.

  - **그룹 레지스트리(신설 레이어)**: `Group{ID,Name,Type,Ref,Members}` + 자체 RWMutex + write-through 영속(`StationRegistry` 락/영속 패턴 미러). 그룹 id 는 **타입 접두사 인코딩**(`station:<code>`/`line:<code>`/`custom:<name|uuid>`)으로 타입을 id 만으로 판별하고 충돌을 원천 차단(RD-2). 신규 파일 `group_registry.go` + `group_membership.go`, 센티널 에러 3종 추가(`errors.go`).
  - **다대다 멤버십**: 한 디바이스가 라인 그룹 + 역사 그룹 + 다수 커스텀 그룹에 동시 소속. `Device.GroupID` 는 폐기하지 않고 **대표(primary) 그룹**으로 유지하여 텔레메트리/이벤트/InfluxDB 단일 `group_id` 태그 방출(status.go/monitor.go/provider.go)을 **무회귀** 보존(RD-1). 조회/fan-out 시 **로스터 대조 필터**로 유령 멤버(삭제된 device_id)를 자동 무시하여 `remove_device` 에 연쇄 제거 로직 불필요(RD-3, 비침습).
  - **커스텀 그룹 CRUD**: `Process()` switch 에 `add_group`/`remove_group`/`set_group`/`list_groups` 배선(예약된 미구현 `set_group` 스텁 대체). 부분 갱신(present 패턴), `group_registered`/`group_unregistered` 이벤트, 기본 그룹(type≠custom) 편집 차단(`ErrGroupNotCustom`).
  - **기본 그룹 자동 동기화**: 역사(station)/호선(line) 그룹은 저장하지 않고 `StationRegistry` 에서 조회 시점 순수 파생 → 디바이스 위치 변경이 즉시 반영되고 동기화 코드가 불필요. 셀렉터 병존(RD-4): 기존 `line`/`station` 셀렉터와 `group_id=line:<code>`/`station:<code>` 그룹 셀렉터를 둘 다 허용, 동일 `DevicesByLine`/`DevicesByStation` 로 수렴하여 결과 동일.
  - **그룹 셀렉터 일괄 제어**: `handleSelectorControl`(본 코드베이스상 `group.go`)의 group_id 분기가 접두사별 멤버 도출 후 기존 `fanOutControl`/`buildControlPlan`/`aggregateStatus` 를 **무변경 재사용**(재구현 없음). 우선순위 `device_id > station > line > group_id` 불변, 빈 그룹 `ErrEmptyGroup`, 미등록 그룹 `ErrGroupNotFound`.
  - **프런트엔드**: `useGroups`/`useAddGroup`/`useSetGroup`/`useRemoveGroup` 훅 + xsfm 에이전트 상세에 **그룹 탭(`XsfmGroupsTab`)** 신설(그룹 CRUD + 멤버 편집(커스텀만) + 그룹 일괄 제어) + 대시보드 **설비 그룹 패널(`FacilityGroupPanel`, `facility-group` 타입) 가산**. `facilityShared.tsx`/`renderDashboardPanel`/`AddPanelDialog`/`uiStore`/`AgentDetailPanel` 정합 반영, i18n ko/en 추가.
  - **분기(Divergence, as-implemented)**: (1) **M4 순수 파생** — station/line 그룹은 `handleAddStation`/`handleRemoveStation` 훅 없이 조회 시 파생하여 `station_registry.go` 완전 불변(계획한 명시적 훅보다 강한 비침습성). (2) **M7 가산형 패널** — 기존 `FacilityStationPanel` 을 파괴적으로 개편하지 않고 신규 `FacilityGroupPanel` 을 가산하여 15개 역사-패널 테스트 무회귀. (3) **신규 `group_membership.go`** — 에이전트-레벨 그룹 로직을 신규 파일에 배치해 `agent.go` diff 최소화, `control.go` 불변. (4) **미접두사 group_id fallback** — 접두사 없는 group_id 는 `Device.GroupID`(primary) 를 1차 대조하여 기존 미접두사 셀렉터 테스트 무회귀.
  - **품질**: xsfm 커버리지 88.8%, `go test -race` 클린, 프런트 vitest 2254 pass, `tsc` 클린. 신규 외부 의존성 0. station/line 레지스트리·로스터·기존 fan-out 무회귀(NF-02).
  - **관련**: SPEC-XSFM-GROUP-001 v0.3.0(구현 완료, `ae53b344` + `97c2bafd`, Tier M). SPEC-XSFM-001 A-4 가정(그룹=단일 태그)을 의도적으로 갱신.

### 추가 — 지하철 시설물 관리 대시보드 패널 (라인·역사·기기)

- **지하철 시설물 관리 대시보드 3종 패널 신설 — 라인(호선)/역사(station)/기기(device) 계층 조망·제어 (프론트엔드 전용, Non-breaking)**

  지하철 역사 시설물 관리 비전에서 첫 디바이스인 설비(facility)를 대상으로, 운영자가 라인 → 역사 → 기기 계층으로 시설물 상태를 조망·통계·일괄 제어할 수 있는 신규 대시보드 패널 타입 3종을 추가했다. SPEC-XSFM-001이 소유하는 디바이스 위치 계층(station/place/index) + 역사 레지스트리(station→line) + line/station fan-out exec 표면을 **소비**하며, **신규 백엔드 코드는 0**이다(기존 xsfm exec 표면 재사용). 신규 17개 + 수정 8개 프론트엔드 파일로 구현되었다.

  - **3종 패널**: (1) **라인 패널** — 한 호선 전체 기기의 라인도(line diagram) + 역사별 상태 요약 + 라인 통계 타일 + 라인 일괄 제어. (2) **역사 패널** — 한 역사의 통계 + 기기별 상태 목록 + 역사 일괄 제어. (3) **기기 패널** — 단일 기기 상태 + 제어(응답 대기 피드백 포함).
  - **순수 집계 로직**(`web/src/lib/facilityAggregation.ts`): 역사→호선 해석, 상태 통계 카운트, 라인도 순서 레이아웃, 미분류 기기 분리. **클라이언트 사이드 집계** 방식으로, 신규 집계 엔드포인트 없이 xsfm `list_devices`/`list_stations` exec 결과를 웹에서 조합한다.
  - **셀렉터 제어 훅**(`web/src/hooks/useXsfmControl.ts`): `set_power`/`set_fan_speed`/`set_multiple`을 device_id/station/line 셀렉터로 execAgent 호출(`fanOutResponse` 타입) + `useFacilityRoster`(refresh 폴링).
  - **패널 컴포넌트**: `panels/{FacilityLine,FacilityStation,FacilityDevice}Panel.tsx` + 공용 `facilityShared.tsx`(StatTiles/ControlResultView/BulkControl).
  - **와이어링 4지점**: `uiStore.ts`(PanelType/기본값), `renderDashboardPanel.tsx`(패널 타입 분기), `AddPanelDialog.tsx`(패널 옵션 + facility 대상 선택 단계), `PanelSettingsDialog.tsx`(FacilitySection 설정). i18n는 `lib/i18n/{ko,en}.json`(`dashboard.facility.*`).
  - **분기(Divergence)**: (1) **클라이언트 사이드 집계 채택**(A-2, REQ-05-01) — 백엔드 집계 엔드포인트 대신 xsfm exec 재사용. 대규모 fleet 성능을 위한 백엔드 엔드포인트는 OI-1로 이연. (2) **REQ-04-04 테스트 갭** — PanelSettingsDialog의 facility 설정 하위 폼(FacilitySection)은 **구현 완료**되었으나 단위 테스트가 없다. FacilitySection이 비-export 내부 컴포넌트라 프로덕션 변경 없이는 테스트 커버가 불가 → 이연(후속: export + scoped RTL 테스트). (3) **REQ-05-04(폴링)/REQ-06-04(로딩 스피너)** — 타이머/브라우저 거동으로 manual/단위-범위-외 분류. (4) **기기 제어 경로** — 기기 패널은 `deviceService.executeCommand`가 아닌 `useXsfmControl`(device_id 셀렉터 execAgent)을 사용 — 기능적으로 동등(둘 다 에이전트 `Process`에 도달). (5) **런타임 의존** — 대시보드는 xsfm exec 표면(develop 머지 완료)을 소비 → xsfm 백엔드 실행 필요.
  - **품질**: 전체 vitest 스위트 2141개 green, `tsc` 0 errors, `eslint` 클린. 백엔드(Go) 코드/테스트 무변경. 신규 외부 의존성 0.
  - **관련**: SPEC-FACILITY-DASHBOARD-001 v0.1.0(구현 완료, `74ada88`, Tier M). depends_on: SPEC-XSFM-001.

### 추가 — xsfm 에이전트 (지하철 역사 설비 MQTT 관리)

- **`xsfm` 시스템 에이전트 신설 — 지하철 역사 설비 MQTT 제어·모니터링 관리 에이전트 (Non-breaking)**

  지하철 역사 시설물 관리 비전의 첫 디바이스로, MQTT 기반 설비 다수를 개별·그룹·역사(station)·호선(line) 단위로 제어·모니터링하는 신규 에이전트를 추가했다. 기존 Eclipse Paho MQTT 스택을 재사용하며 신규 외부 의존성은 없다. 13개 마일스톤에 걸쳐 신규 패키지 + 노드/디바이스 어댑터/스토리지/와이어링/프론트엔드로 구현되었다.

  - **듀얼 트랜스포트(direct/port)**: `transport_mode`에 따라 `direct`는 에이전트가 브로커 sub/pub을 직접 소유하고, `port`는 외부 mqtt-in/out 노드가 브로커 I/O를 담당하고 에이전트는 순수 프로토콜/로직 레이어로 동작한다(상태 입력 포트 · 제어 출력 포트 분리). I/O 경계(CommandSink + 상태 ingress)를 공유 페이로드 매핑/로직과 추상화로 분리했다.
  - **개별 제어(2축)**: 디바이스별 `set_power`/`set_fan_speed`(1/2/3)/`set_multiple` 제어 + 전원 OFF 게이트. 제어는 **응답 대기(state echo 대기)** — 디바이스별 pending-command 레지스트리 + 에코 상관(correlation by device_id), 타임아웃 시 `ErrControlTimeout`, 동시성 안전.
  - **일괄 제어(fan-out)**: 그룹 + 역사(station) + 호선(line) 셀렉터 팬아웃(우선순위 `device_id > station > line > group_id`), best-effort ok/error/timeout 집계.
  - **역사 레지스트리(station registry)**: station→line 매핑을 SSOT로 정의(디바이스는 station만 보유, line은 station→line로 해석) + 디바이스 위치 계층(station/place/index, 선택/하위호환, `device_metadata` 패턴).
  - **상태 모니터링**: observed 기반 방출, 오프라인 감지(LWT + 타임아웃), `request_state`.
  - **감사·영속화**: 제어/그룹 감사(`remote_audit_repository`) + 로스터 영속화(device_id 키잉, v0.2.0 하위호환).
  - **플로우/디바이스/프론트엔드 통합**: 플로우 노드(`xsfm-status`/`xsfm-control`, 상태-입력/제어-출력 포트 분리) + 디바이스 어댑터 `CommandSpec` + `cmd/xflowd/main.go` 와이어링 + API 어댑터 + 웹 설정 스키마.
  - **분기(Divergence)**: (1) **i18n** — acceptance 9.1은 en/ko i18n 문자열을 요청했으나, 코드베이스 관례상 에이전트 설정 필드 라벨은 `agentSchemas.ts` + `agentTypeMeta.ts`(하드코딩 한국어)에 둔다(에이전트별 i18n JSON 아님). xsfm는 이 관례를 따르며 죽은 i18n 키를 추가하지 않는다. (2) **LWT 토픽 스킴** — 디바이스 매뉴얼 미확보(SPEC 가정 A-1)로 오프라인 전이 로직은 구조적/테스트 가능하나, 라이브 LWT 토픽 구독 와이어링은 config 기반 확정으로 이연했다.
  - **품질**: 빌드/vet/`-race` 클린, xsfm 관련 패키지 전부 green, 커버리지 85.1%, `golangci-lint` xsfm 0 issues. 신규 외부 의존성 0.
  - **관련**: SPEC-XSFM-001 v0.3.0(구현 완료, `bbd6365`). 대시보드 패널은 본 SPEC 범위 외(SPEC-FACILITY-DASHBOARD-001).

### 추가 — thingplus-gateway 에이전트 (ThingsBoard Gateway MQTT 양방향 IoT 연동)

- **`thingplus-gateway` 시스템 에이전트 신설 — 단일 MQTT 연결로 다수 하위 디바이스를 프록시하는 양방향 게이트웨이 (Non-breaking)**

  ThingsBoard Gateway MQTT API(`v1/gateway/*`)를 지원하는 신규 시스템 에이전트를 추가했다. 하나의 게이트웨이 MQTT 연결이 다수의 논리 디바이스를 다중화(multiplexing)하며, 업링크(텔레메트리/속성)와 다운링크(RPC/공유 속성)를 양방향으로 중계하고 xflow `device_id`↔ThingsBoard 디바이스 NAME 매핑을 관리한다. 기존 `mqtt-client` 에이전트 및 Eclipse Paho 스택을 재사용하며 신규 외부 의존성은 없다.

  - **코어/연결(M1)**: `"thingplus-gateway"` 타입 등록, 설정 파싱, access token 기반 MQTT username 인증, TLS(8883/CA), `State()`, 재연결. access token은 로그/`State()`에서 마스킹된다.
  - **매핑/connect(M2)**: JSONPath 기반 디바이스 NAME 추출(기본 `$.device`), NAME↔device_id 양방향 매핑, 디바이스 상태 머신(`disconnected→connecting→connected`), 미등록 디바이스 자동 connect/auto-provision, 재연결 시 알려진 디바이스 재connect, repo-nil fallback(NAME을 device_id로 사용).
  - **업링크(M3)**: 텔레메트리(`ts=epoch ms`, 부재 시 생략) 및 클라이언트 속성 발행, 배치 조립, 경계가 있는 무손실 버퍼(연결 끊김 시 버퍼링, 재연결 시 flush, 초과 시 관찰 가능).
  - **다운링크(M4)**: `v1/gateway/rpc`·`v1/gateway/attributes` 구독 → `Type()`이 `thingplus.rpc.request` / `thingplus.attr.update`인 플로우 메시지 방출, RPC 응답 발행, 디바이스별 `pendingRPC` 상관.
  - **스키마/관찰성(M5)**: 웹 설정 스키마(`agentSchemas.ts`), `ConnectionStats`/`BufferInfo` 관찰성, 예제 agent/flow YAML(`examples/agents/thingplus-gateway.yaml`, `examples/flows/thingplus-gateway.yaml`).
  - **Bridge 어댑터(stateless)**: 플로우 경계를 넘어 메시지 `Type()`을 보존하기 위한 얇은 무상태 어댑터(`internal/node/adapter/thingplus.go`)를 추가했다. Bridge 코어는 변경하지 않는다.
  - **이연(라이브 브로커)**: A7(MQTT v5 PUBACK 타이밍), A8(`attributes/response` 다중 키 인코딩)은 라이브 브로커 스모크 테스트로 이연했다. 빌더/파서는 구현되었으나 tolerant/deferred 상태.
  - **품질**: 전체 회귀 11개 패키지 0 FAIL, `go test -race` 통과, `golangci-lint` 0 issues. 커버리지 codec 92.6% / mapping 96.6% / adapter 95.0% / 에이전트 코어 80.4%(브로커 전용 경로 제외 시 >90%). 신규 외부 의존성 0.
  - **관련**: SPEC-THINGPLUS-001 v1.1.0(구현 완료, `eda584a`).

### 변경 — 저장소 탭 필터 UI 정리

- **저장소 탭에서 "메트릭 타입" 필터 드롭다운과 태그 필터 칩을 제거 (기능·데이터 무영향, Non-breaking)**

  데이터 테이블 상단에 있던 두 필터 블록(메트릭 타입 `<select>` + 태그 필터 칩 `TagFilterChips`)이 중복 UI 로 판단되어 제거되었다. 동일한 필터링은 기존 키워드 검색 상자(키 / metric_type / 태그를 매칭)와 컬럼별 Excel 스타일 헤더 필터로 그대로 수행할 수 있다. 테이블 상단의 검색 입력창은 유지된다.

  - **제거 대상**: 메트릭 타입 선택 드롭다운, 태그 필터 칩 섹션, 그리고 이와 연동된 상태/핸들러/파생값 및 해당 단위 테스트 블록("메트릭 타입 필터").
  - **대체 수단**: 키워드 검색 + 컬럼별 Excel 필터가 동일한 필터링 요구를 커버한다. 데이터·기능 동작에는 영향이 없으며 UI 단순화에 해당한다.
  - **관련**: SPEC-STORE-003(v0.4.0에서 최초 도입된 필터 UI), SPEC-WEB-005.

### 추가 — 태그 컬럼 필터 그룹핑 (팝오버 확장 + 태그 타입 일괄 선택)

- **태그 컬럼의 Excel 스타일 헤더 필터 팝오버를 넓히고 태그 타입별 그룹핑·그룹 단위 선택을 추가 (Non-breaking)**

  긴 `key=value` 태그(예: 긴 UUID 형태의 `device_id=...`)가 잘려 보이지 않도록 팝오버 폭을 확장(`w-56` → `w-80`)하고 긴 태그 값을 줄바꿈(`break-all`) 처리해 전체 값을 노출한다. 필터 매칭 의미(전체 `key=value` 문자열 매칭)는 변경되지 않는다.

  - **태그 타입별 그룹핑**: 필터 값들을 태그 키(`=` 앞부분) 기준으로 그룹화한다. 각 그룹 헤더는 태그 키와 `선택/전체` 개수를 표시하며, 그룹 단위 체크박스(indeterminate 상태 포함)로 해당 태그 타입의 모든 값을 한 번에 선택/해제할 수 있다.
  - **행 표시 최적화**: 개별 행은 값 부분만 표시하고, 전체 `key=value` 는 title 툴팁으로 확인할 수 있다. 이 그룹핑은 태그 컬럼에만 적용된다.
  - **관련**: SPEC-WEB-005.

### 추가 — 라인 차트 스타일 옵션 (Y축 데이터 타입 · 축 폰트 · 자동 색상)

- **대시보드 라인 차트에 Y축 데이터 타입(숫자형/열거형)·축 폰트·시리즈 자동 색상 설정을 추가 (Non-breaking)**

  기존 boolean 시리즈의 `true`/`false` 축 표시 로직을 사용자 정의 값→라벨 매핑으로 일반화하고, 축 텍스트 폰트와 시리즈 색상 배정을 설정 가능하게 했다. 모든 신규 config 필드는 선택값이며 미지정 시 기존 동작을 유지한다.

  - **Y축 데이터 타입(패널 단위)**: `y_axis_type`(`numeric`/`enum`) + `y_enum_labels`(값→라벨 매핑) 추가. 열거형이면 Y축 눈금·툴팁·범례 현재값을 라벨로 표시한다(예: `0→정지`, `1→운전`). 열거형 미설정 시 boolean 시리즈는 기존 `true`/`false` 자동 표시를 유지한다. 숫자형 min/max 고정·자동은 기존 `y_axis_mode`/`y_min`/`y_max` 를 그대로 사용한다.
  - **축 폰트(축별 독립)**: X/Y 축의 레이블(제목)·값(눈금) 4종에 대해 폰트 크기·색상·굵기를 개별 설정하는 `x_label_font`/`x_tick_font`/`y_label_font`/`y_tick_font` 추가. 미지정 필드는 기본값(size 10, `#9ca3af`, normal)으로 폴백한다.
  - **시리즈 자동 색상**: 데이터 소스(Store 키·채널) 선택 시 공용 팔레트에서 시리즈 인덱스별로 서로 다른 색을 자동 배정한다. 이전에는 색상 미지정으로 편집기 스와치가 모두 동일 색으로 보였다. 사용자는 색상 스와치로 개별 변경할 수 있다.
  - **관련**: SPEC-WEB-005, SPEC-CHART-001, SPEC-STORE-004.

### 수정 — 라인 차트 Store 소스 · 축 렌더링 결함

- **Store boolean 시리즈 집계 유실 수정**: 백엔드 서버 집계 경로(`toFloat64`)가 Go `bool` 타입을 처리하지 못해 boolean 데이터가 집계 전에 누락되어 라인 차트에 표시되지 않던 문제를 수정했다(`true→1`/`false→0` 변환 추가). 프론트엔드는 이미 1/0 을 처리하고 있었으나 서버가 값을 버려 표시되지 않았다.
- **X축 타이틀 잘림 수정**: X축 제목이 `insideBottomRight` 로 오른쪽 끝에 앵커되어 컨테이너 경계에서 잘리던 문제를 하단 중앙 정렬 + 양수 offset 으로 수정했다.
- **범례 현재값 포맷 수정**: 범례의 마지막값이 원시 숫자(`0.0`)로 표시되던 것을 축·툴팁과 동일하게 열거형 라벨/boolean(`true`·`false`) 로 표시하도록 수정했다.

### 추가 — xflow CLI–Web UI 기능 패리티 (remote 관리 + dashboard/chart/influxdb)

- **`xflow` CLI 가 백엔드 `/api/v1/*` API 도메인을 동등하게 커버하도록 명령 그룹을 확충 (Non-breaking)**

  Web UI 가 소비하는 모든 기능은 `/api/v1/*` REST 라우트로 노출되므로, CLI 가 동일 라우트를 호출하면 동일 기능을 수행한다는 전제 아래 도메인 갭을 메웠다. 본 변경은 SPEC-CLI-004 의 P3(원격 관리)·P4(저우선 도메인)를 다룬다. P0(죽은 명령 교정)·P1(auth/device/store/tsdb/monitor/system/settings 신규 그룹)·P2(flow/agent 보완)는 선행 완료되었다.

  - **원격 관리(P3)**: `xflow remote node`(list/get/approve/reject/revoke/pre-register), `xflow remote group`(list/set/clear/rename/delete/update/command) + `xflow remote token`(create/list/revoke), `xflow remote release`(list/create/delete/delete-asset) + `xflow remote version`(target get·set / source get·set / history / update), `xflow remote command` / `xflow remote audit` / `xflow remote inventory`(flows|agents|devices, mirror+live) 명령군을 신설했다.
  - **저우선 도메인(P4)**: `xflow dashboard`(shared/mine get·set), `xflow chart channels`, `xflow influxdb query` 를 신설했다.
  - **기존 스택 재사용 + 신규 의존성 0**: 모든 신규 명령은 기존 client/output/errors/resolve 스택(HTTP 클라이언트, `PrintResult` 포맷터, `MapAPIError`, 이름→ID 해석, 글로벌 플래그)을 재사용한다. 파괴적 작업은 확인 프롬프트 + `--yes` 로 게이트하고, enrollment 토큰은 생성 직후 1회만 표시한다.
  - **품질**: `internal/cli` 커버리지 85.3%, 전 게이트(build/vet/test-race/gofmt/golangci-lint) 통과, 기존 명령 회귀 0.
  - **후속 과제(별도 하위 SPEC 후보)**: remote 심화 per-resource 조회(flow status/logs/nodes, agent stats/config/devices/topics/store/sessions/series, device state/commands/metadata), remote 편집 CRUD, SSE/WS 실시간 스트림(`--follow`/chart WS/로그 스트림), dashboard delete 라우트는 범위에서 제외했다.
  - **관련**: SPEC-CLI-004 v0.3.0(P0~P4 완료), SPEC-REMOTE-001, SPEC-AUTH-001/003, SPEC-DEVICE-001, SPEC-STORE-001, SPEC-TSDB-001, SPEC-CHART-001, SPEC-UPDATE-001, SPEC-WEB-006/007.

### 추가 — 로컬 인스턴스 시스템 정보 표출 (self/local Identity + Runtime)

- **접속한 xflowd 인스턴스 자신의 시스템 정보를 `/admin/system` 및 설정 "시스템" 탭에 표출 (Non-breaking)**

  관리 서버·client 노드·standalone 어디에 로그인하든 **그 인스턴스 자신(self)** 의 hostname / OS·Arch / 버전 / remote 모드 / uptime / 실시간 리소스를 한눈에 확인한다. 관리 서버가 원격 노드 보고를 표시하는 NodeDashboard(REMOTE-001)와는 별개의 SELF/로컬 케이스로, 원격 프로토콜은 변경하지 않는다.

  - **Identity 백엔드 확장**: 기존 `GET /api/v1/system/version` 응답에 `os`(GOOS)·`arch`(GOARCH)·`hostname`·`mode`(remote_management.mode)·`uptime_seconds` 5필드를 추가(기존 7필드 불변, SystemVersionCard 회귀 없음). 신규 `/system/info` 엔드포인트는 만들지 않음. uptime 기준 시각은 프로세스 부팅 시각(epoch ms)을 재사용해 정확도 확보.
  - **시크릿 비노출**: 확장 응답은 jwt_secret·bootstrap_secret·enrollment_token·키 등 어떤 비밀 값도 포함하지 않음(단위 테스트로 강제).
  - **Runtime 표시**: 기존 `GET /api/v1/monitor/metrics` 재사용(신규 백엔드 0), TanStack Query 5초 폴링(`refetchIntervalInBackground: false`, 기존 대시보드 메트릭과 캐시 공유). uptime 가독 형식("3d 4h 12m"), CPU 0%는 "측정 미지원" 안내(v1 샘플링 미지원).
  - **노출 위치 추가**: 시스템 정보 카드를 `/admin/system` 외에 **설정(Settings) → "시스템" 탭에도 추가**(동일 컴포넌트 재사용, 양쪽 공존). `/admin/system` 으로 가는 명시적 메뉴 진입점이 없어 사용자가 도달하기 어려웠던 문제를 해소한다.
  - **표시 필드 축소**: 인스턴스 정보 카드(`SystemInfoCard`)에서 **Commit / Build Date / Go Runtime 필드를 UI 에서 제거**. Makefile/Dockerfile 이 `main.Version` 만 ldflags 주입하여 commit/build_date 가 항상 "unknown" 이고 Go Runtime 은 운영자에게 불필요하기 때문. 남은 식별 필드: hostname, OS/Arch, 버전, 원격 모드. 백엔드 `GET /system/version` 응답 스키마는 불변이며, 별개 `SystemVersionCard` 가 commit/build_date/go_version 을 계속 소비한다.
  - **룩앤필 통일**: 시스템 정보/런타임 카드를 원격 노드 카드(NodeDashboard, REMOTE-001)와 시각 통일 — 기존 "monospaced 칩" 표기를 제거하고 plain 텍스트 + 라벨 스타일로, 카드 컨테이너/헤더 아이콘/제목 크기를 원격 카드와 일치시켰다. "이 인스턴스 (self)" 배지는 유지. mode 한글 라벨 매핑(관리 서버/클라이언트 노드/독립 실행), 영역별 독립 로딩/에러 처리, mode 무관 동작, 기존 admin 게이팅 재사용.
  - **신규 외부 의존성 0**.
  - **관련**: SPEC-WEB-007 v0.3.0, SPEC-WEB-006(공존), SPEC-UPDATE-001·SPEC-OBS-001(데이터 출처).

### 변경 — 설정 "시스템" 탭 부수 개선 (API 서버 주소 / 로그 레벨 UI)

- **설정 시스템 탭의 API 서버 주소 표시 정정 (Non-breaking)**: "API 서버" 카드가 보여주던 서버 URL 을 `import.meta.env.VITE_API_URL` 하드코딩(`localhost:8080`)에서 **실제 접속 주소(`window.location.origin`)** 로 정정했다. API client 가 상대경로 `/api/v1` 를 사용하므로 origin 이 곧 백엔드 주소이며, HTTPS 보안 접속 시에도 자동 반영된다.
- **컴포넌트별 로그 레벨 UI 개선 (Non-breaking)**: 컴포넌트명 첫 세그먼트를 기준으로 종류 분류 표시(에이전트/플로우/노드/원격/엔진/API/DB 등), 레벨 직접 변경(select), 종류·컴포넌트·레벨 정렬, 종류 필터 + 이름 검색, 선택 기반 일괄 레벨 변경·일괄 리셋을 추가했다.

### 변경 — 대시보드 자동 갱신 주기 컨트롤 위치 이동

- **자동 갱신 주기 컨트롤을 설정 페이지에서 대시보드 페이지 헤더로 이동 (Non-breaking)**: 설정의 "대시보드" 탭을 제거하고, 대시보드 자동 갱신 주기 컨트롤을 대시보드 페이지 우측 상단 헤더(일반/읽기 모드의 새로고침 옆)에 컴팩트 드롭다운(5/10/15/30/60초)으로 배치했다. 값은 기존 전역 상태(`dashboardRefreshInterval`)를 그대로 사용하여 동작은 동일하며(폴링 `refetchInterval` 에 즉시 반영), 위치만 이동했다. 편집 권한과 무관하게 항상 사용 가능하다.

### 추가 — 원격 프로그램 버전 관리 (릴리스 호스팅 + 업데이트 소스 + 아키텍처-aware 그룹 일괄 업데이트)

- **관리 서버가 노드용 프로그램 이미지를 호스팅하고 원격 자가 업데이트를 오케스트레이션 (Non-breaking)**

  관리 서버(`xflowd` 서버 모드)가 아키텍처별 `xflowd` 바이너리 + 사전 서명된 Ed25519 `.sig` 를 저장·배포하고, 노드의 기존 자가 업데이트(SPEC-UPDATE-001)를 **무변경**으로 구동한다. 공개키는 노드 로컬에만 존재하고(서버 비전송), 서버는 서명을 생성하지 않으며 무결성은 노드 Ed25519 검증으로 보장된다.

  - **릴리스 저장소**: SQLite 메타(`releases`/`release_assets`) + 디스크 `{data}/releases/{version}/`. 업로드 시 SHA256 자동 계산·`checksum.txt` 자동 생성.
  - **노드용 익명 GitHub-Releases 호환 피드** (인증 없음, 노드 updater 무변경 소비): `GET /api/v1/updates/releases/latest`·`/releases`·`/releases/download/{version}/{filename}`. asset `browser_download_url` 은 https 강제(`remote_management.public_base_url` 설정 또는 요청 Host 유도).
  - **admin 릴리스 관리 API**: `GET/POST /remote/releases`, `DELETE /remote/releases/{version}`, `DELETE /remote/releases/{version}/assets/{os}/{arch}`, multipart 업로드 `POST /remote/releases/{version}/assets`(os/arch/binary/signature).
  - **업데이트 소스 서버 저장**: `update_url`+채널을 SettingsRepository(`remote.update_source`)에 1회 저장하고 필요시에만 변경. `GET/PUT /remote/update-source`(admin). 원격 업데이트 명령(UpdateNode/UpdateGroup)에 서버가 자동 주입(요청 명시 시 우선). `update_url` 은 빈 값 또는 `https://` 만 허용. 공개키는 절대 전송하지 않음.
  - **아키텍처/OS-aware 그룹 일괄 업데이트**: 서버가 각 노드의 보고된 OS/Arch 로 per-node 타깃 버전을 계산해 per-node `system/update` 디스패치. 전략 3종 — `latest`(채널 내 각 os/arch 최고 semver)·`pin`(단일 버전, 기존 호환)·`per_arch`((os/arch)→버전 맵). 자산 없는 노드는 사유와 함께 건너뜀(부분 성공).
  - **client WS insecure_skip_verify**: `remote_management.insecure_skip_verify`(기본 false) — 관리 WS(wss) TLS 인증서 검증 스킵(자체 서명/사설망 전용 옵트인). 전송 무결성 보장과 무관(명령=토큰·자가 업데이트=Ed25519).
  - **신규 설정**: `remote_management.public_base_url`·`remote_management.releases_dir`·`remote_management.insecure_skip_verify`.
  - **관련**: SPEC-REMOTE-001 v1.7(그룹 O).

### 추가 — 릴리스 이미지 생성·서명 도구 및 원격 업데이트 운영 요구사항

- **xflowd 릴리스 이미지 빌드·서명 도구와 노드 운영 요구사항 명세 (Non-breaking)**

  - **서명 도구**: `xflowd update keygen`(Ed25519 키쌍, 비공개 `.key` `0600` + 공개 `.pub`), `xflowd update sign --key K BIN`(바이너리 본문 서명 → raw 64-byte `.sig`).
  - **빌드/CI**: `make keygen`, `make release-images VERSION=<v> SIGN_KEY=<key>`(6 타깃 교차컴파일+서명+`checksum.txt`, 인자 미지정 시 가드 실패). CI `release.yml` `release-images` 잡 — 태그 푸시 시 서명 이미지 산출·GitHub Release 첨부, 시크릿 `XFLOW_RELEASE_PRIVATE_KEY`(미설정 시 스킵). 공개키는 노드 로컬, 비공개키는 릴리스 담당자/CI 만.
  - **노드 설정 요구사항**: `update.public_key_path` 가 원격 업데이트 필수(미설정 시 거부 — 내장 핀닝 키 없음), `update.insecure_skip_verify` 를 원격 업데이트 다운로드(Checker/Downloader)에 연결(Ed25519 검증 유지). 채널 불일치/자산 누락 시 구체적 진단 메시지.
  - **배포(systemd)**: 자가 업데이트가 설치 디렉토리에 새 바이너리를 원자 교체하므로 `ReadWritePaths=/opt/xflow` 필요(`/opt/xflow/data` 만으로는 `read-only file system` 실패).
  - **관련**: SPEC-UPDATE-001 v0.2.0(M15~M18).

### 수정 — 원격 자가 업데이트 빌드/업로드 정합성

- **버전 ldflag 대소문자 정정 (Non-breaking)**: 빌드 LDFLAGS 의 버전 주입 심볼을 코드 빌드 변수와 일치하는 `-X main.Version=$(VERSION)`(대문자 `Version`)로 정정. 불일치 시 노드 보고 버전이 기본값(`dev`)으로 떨어지던 문제를 해결 — 버전 표시·다운그레이드 방지·버전 이력 갱신의 전제(SPEC-UPDATE-001 M18).
- **릴리스 자산 multipart 업로드 FormData 정정 (Non-breaking)**: 웹 admin 의 릴리스 자산 업로드를 axios FormData(multipart, os/arch/binary/signature 필드)로 전송하도록 정정해 `POST /remote/releases/{version}/assets` 와 정합(SPEC-REMOTE-001 REQ-O04).

### 추가 — Switch 노드 완성 (문자열 조건 라우팅 / first·all / default_port·드롭 / 동적 포트)

- **Switch 노드를 에디터에서 사용 가능하도록 완성 (Non-breaking)**

  기존 `switch` 노드는 `routes` 가 Go 함수 클로저(`func(msg) bool`)만 받아들여 에디터/yaml 에서 사실상 사용할 수 없었다. 이제 문자열 조건 표현식을 받아 `compileCondition` 표현식 엔진으로 컴파일하므로 에디터에서 정의·저장·실행이 가능하다. filter 노드와 동일한 표현식 엔진을 공유한다.

  - **문자열 조건 라우트**: `routes: [{name, condition}]` 형식 (배열 순서 보존). 각 `condition` 은 `compileCondition` 으로 컴파일된다. `name` 이 곧 출력 포트 이름이자 와이어 `_target_port` 규약이 된다. 기존 `[]SwitchRoute` (Go 클로저) 형식도 그대로 허용되어 하위 호환.
  - **`match_mode`** (string, 기본 `first`): `first` 는 순차 평가하여 첫 매칭 라우트로만 라우팅, `all` 은 매칭되는 모든 라우트로 메시지를 Clone 팬아웃. 미설정·미인식 값은 `first` 로 폴백.
  - **`default_port`** (string): 매칭이 하나도 없을 때 라우팅할 포트 이름. 비어 있으면 메시지를 **드롭**(빈 슬라이스 반환). 출력 포트 이름은 고정 `"default"` 가 아니라 `default_port` 값 그 자체이다.
  - **동적 포트**: `Ports()` 가 라우트 `name` 목록(중복 제거, 순서 보존) + `default_port` 값으로부터 출력 포트를 파생한다. `routes` 가 비면 `[in, out, _error]` 정적 포트로 폴백하여 기존 플로우 동작을 보존한다.
  - **원자적 설정 적용**: `routes` 파싱 중 조건 컴파일 에러가 발생하면 실패한 라우트(인덱스·name)를 식별하는 에러를 반환하고 기존 설정을 변경하지 않는다(부분 적용 금지).
  - **프론트엔드**: 순서가 있는 조건+포트 행을 편집하는 `RoutesEditor`(신규 `routes_editor` ConfigField 타입) 추가, `switch` 스키마 갱신. `computePortsForNode` 가 라우트 name + `default_port` 값으로 출력 포트를 파생.
  - **조건 표현식**: `$.payload.x`, `$.metadata.k` 경로, 연산자 `== != > < >= <=` / `&& || !`, `exists()`, 리터럴 지원.
  - **회귀 위험 없음**: `routes` 가 빈 기존 플로우는 `[in, out]` 포트를 그대로 유지하며 동작 변화 없음.
  - **사용 예**: `routes: [{name: 'hot', condition: '$.payload.temp >= 30'}, {name: 'cold', condition: '$.payload.temp < 10'}]`, `match_mode: 'first'`, `default_port: 'normal'` → temp ≥ 30 이면 `hot`, temp < 10 이면 `cold`, 둘 다 아니면 `normal` 포트로 라우팅.
  - **관련**: SPEC-SWITCH-001.

### 추가 — Century `log_state_changes_only` 진단 분석 모드 옵션

- **Century HVACR-01 에이전트에 byte-equal dedup 기반 진단 로그 모드 추가 (Non-breaking)**

  프로토콜 RE / fan 인코딩 탐색 등 byte 단위 변화 탐지가 목적인 작업에서, 매 polling cycle (~512ms) 마다 동일한 reg02/reg03/reg04 frame 이 로그를 폭주시키는 문제를 해결한다. `log_state_updates=true` 와 함께 활성화하면 변화가 있을 때만 로그가 출력되어 분석이 용이.

  - **신규 옵션**: `log_state_changes_only` (bool, default false). `log_state_updates=true` 와 조합하여 사용.
  - **동작**: `logDecodedState` 가 (sub_dev_id, register, role) 별로 직전 raw payload 와 **byte-equal 비교**. 동일하면 로그 출력 생략, 다르면 출력.
  - **diff 정보**: 변경된 byte 위치 리스트를 `changed_bytes` 추가 field 로 노출 (예: `data[2],data[7]`). payload offset 0..2 (prefix) 는 `p[0]/p[1]/p[2]`, 그 이후는 `data[N]` 로 표기. 길이 차이는 `len(prev→cur)` 형식.
  - **첫 관측**: `changed_bytes="(initial)"` 로 표기하여 cold start 와 변화 케이스를 구분.
  - **Reg04 read/write 분리**: 같은 register 0x04 라도 read response (0x04) 와 write request (0x84) 가 별도 cache key 로 dedup 되어 양쪽 모두 진단 가능.
  - **동시성**: 캐시는 captureLoop 단일 goroutine 에서만 접근되므로 mutex 불필요. `lastRegisterEmit` (v0.3.6) 와 동일 패턴.
  - **회귀 위험 없음**: 기본값 false, 옵션을 켜지 않는 한 v0.21.x 와 완전 동일 동작.
  - **사용 예**: 운영 yaml 의 transport.options 에 `log_state_updates: true` + `log_state_changes_only: true` 임시 추가 → 분석 종료 후 둘 다 false 로 복원.
  - **관련**: SPEC-CENTURY-HVACR-001 v0.22.0.

### 수정 (BREAKING) — Century current_temp 소스 정정 (Reg04 → Reg02 data[7..8], 실측 검증)

- **Century ICP-01 프로토콜 spec 의 current_temp 소스 정정 (Breaking — emit 값 / 필드명 변경)**

  사용자 OFF↔ON 캡처 (동일 ambient 26°C 환경) 결과 `data[7..8]` LE u16 ÷ 10 이 OFF/ON 양쪽 모두 `0x0104` (=260 → 26.0°C) 로 실측 ambient 와 일치함이 확정됨. v0.20.0 의 "cooling capacity ceiling / max compressor speed" 가설 (`reg02_word_7`) 은 폐기 — 6-point setpoint 실험 (18~28°C) 당시 관측된 240~250 값들은 cooling ceiling 이 아니라 그 시점의 ambient 온도였음.

  - **byte source 변경**: DeviceStateEvent 의 `current_temp` source 가 register 0x04 read response data[10..11] (`temp_A_c` inferred) → **register 0x02 data[7..8]** (`current_temp_c` confirmed) 로 변경. 디코더에서 `Reg02Word7` (inferred) → `CurrentTempC` (confirmed) 로 리네이밍. Reg04 `TempAC` (inferred 25.2°C 추정) 는 의미 미확정으로 격하되어 `Reg04Word10` (inferred) 로 리네이밍 — 운전 중에만 채워지는 값이지만 indoor temp 가설은 폐기.
  - **strict gate 완화 (v0.4.2 → v0.5)**: `maybeEmitDeviceState` 가 이전엔 `Reg02 != nil && Reg04Read != nil` 둘 다 요구했으나, 이제 5 핵심 필드 모두 Reg02 단일 register 에서 공급되므로 **`Reg02 != nil` 단독** 으로 축소. emit latency 가 더 짧아짐 (master polling cycle 의 첫 Reg02 도착 시점).
  - **다운스트림 영향**: emit 메시지의 `current_temperature` 값이 이제 실제 ambient 와 일치 (이전엔 운전 중인 경우에만 25.2°C 안정값, 꺼짐 시 0). Reg04 register-decoded 메시지의 `temp_A_c` 필드명이 `reg04_word_10` 으로 rename — `$.payload.fields.temp_A_c` 참조 코드 갱신 필요. Reg02 register-decoded 메시지에 `current_temp_c` (confirmed) 신규 노출 (이전엔 `reg02_word_7` inferred).
  - **회귀 위험**: CAP-1/3/4 fixture 의 두 byte 위치가 모두 25°C 였기에 swap 전후 fixture 값은 영향 없음 (Confirmed status 로 격상되었을 뿐 값은 동일). 사용자 환경의 `current_temperature` 값은 변경됨 — 운전 중이라면 큰 차이 없으나 (둘 다 ~25°C), 꺼짐 상태에서 이제 실내 ambient 가 노출됨 (이전엔 Reg04 미수신으로 emit 보류 또는 0).
  - **관련**: `references/protocols/century_icp01_protocol_spec.md` v0.5, SPEC-CENTURY-HVACR-001 v0.21.0.

### 변경 (BREAKING) — `lgcp` 식별자 rename 으로 LG ICP-02 프로토콜 / LG HVACR-02 에이전트 분리

- **`lgcp` 식별자를 protocol / agent / node 3 가지 역할별로 분리 (Breaking)**

  기존 `lgcp` 단일 식별자가 protocol code, agent type, node type 3 가지 의미로 동시에 쓰이던 모호함을 해소. 동일 패턴의 lgcnp → lg_icp01/lg_hvacr01 (v1.x 이전 적용) 의 후속 작업.

  - **프로토콜 코드**: `lgcp` → `lg_icp02` (LG ICP-02 wire protocol)
  - **에이전트 타입**: `lgcp` → `lg_hvacr02` (LG HVACR-02 agent)
  - **노드 타입**: `lgcp` / `lgcp-status` / `lgcp-control` → `lg_hvacr02` / `lg_hvacr02_status` / `lg_hvacr02_control`
  - **복합 디바이스 ID**: `lgcp:<addr>` → `lg_icp02:<addr>`
  - **SPEC 디렉토리**: `SPEC-LGCP-001/002/003` → `SPEC-LG-HVACR-002-001/002/003`
  - **프로토콜 분석 문서**: `references/protocols/LGCP_Protocol_Analysis.md` → `LG-ICP-02_Protocol_Analysis.md`
  - **Go identifiers (B-3 convention)**: Agent side `LGCP*` → `Hvacr02*`, Protocol side `LGCP*` → `Icp02*`, Node side (vendor prefix) `LGCP*Node` → `LGHvacr02*Node`, Adapter `LGCP*` → `LGIcp02*`
  - 기존 yaml / flow 가 deprecated alias 를 사용했다면 부팅 실패 (parse error). 운영자 마이그레이션: examples 폴더의 새 형식 파일 참조.
  - 관련 commits: backend (bc700c9), frontend (b803ffe).

### 수정 (BREAKING) — Century reg 0x02 setpoint byte 위치 정정 (실측 검증)

- **Century ICP-01 프로토콜 spec 의 setpoint byte 위치 정정 (Breaking — emit 값 변경)**

  사용자 AC remote 6-point 실험 (18 / 20 / 22 / 24 / 26 / 28°C 순차 설정) 결과 setpoint 의 부호화 위치가 `data[7..8]` 이 아닌 **`data[11..12]`** 임이 실측으로 확정. 관측값 `data[11..12] ÷ 10` = 180/200/220/240/260/280 → 18~28°C 와 **완벽 linear 일치**.

  - **byte 위치 swap**: `setpoint_c` 의 source 가 `data[7..8] LE u16 ÷ 10` 에서 `data[11..12] LE u16 ÷ 10` 으로 변경. `data[7..8]` 은 별개 운전 파라미터로 재분류되어 새 필드 `reg02_word_7` (inferred) 로 노출 — ≤25°C 설정 시 250 고정, 26°C → 245, 28°C → 240 으로 5 씩 감소 (cooling capacity ceiling / max compressor speed 등 추정).
  - **이전 가정 미검증의 원인**: CAP-3/4 fixture 가 우연히 두 byte 쌍 모두 `0x00FA` = 250 (25.0°C) 이라 디코더 byte position 이 잘못되어도 fixture 테스트가 통과해왔음. 단일 setpoint 만 가진 fixture 의 검증 한계.
  - **CAP-1 (꺼짐) 재해석**: 이전 spec 의 "꺼짐 상태에서도 setpoint 25°C 유지" 관찰은 실제로 `data[7..8]` (현 `reg02_word_7`) 이 유지된 것이며 setpoint 그 자체가 아님. 꺼짐 상태에서 `data[11..12]=0` (active cooling target 없음) 이 자연스러운 해석.
  - **다운스트림 영향**: Go 필드 `Reg02Word11` + JSON key `reg02_word_11` → `Reg02Word7` / `reg02_word_7` 로 rename. `setpoint_c` 필드 이름은 유지. emit 메시지의 `target_temperature` 값이 이제 사용자 실제 설정과 일치 (이전엔 잘못된 byte 로 인해 일치하지 않을 수 있었음).
  - **회귀 위험**: CAP-3/4 fixture 테스트는 두 byte 쌍 모두 250 이라 swap 후에도 통과. CAP-1 (꺼짐) 테스트 어서션은 갱신 필요 (이미 적용). 25°C 단일 설정으로만 운영해왔다면 사용자 영향 없음, 다양한 setpoint 사용 시 이제 정확한 값 표출.
  - **관련**: `references/protocols/century_icp01_protocol_spec.md` v0.4, SPEC-CENTURY-HVACR-001 v0.20.0.

### 변경 (BREAKING) — Samsung HVACR-01 transport_type tcp-client/tcp-server 분리 지원

- **Samsung HVACR-01 의 `transport_type` 옵션이 LG / Century 와 동일한 3-모드 (`serial` / `tcp-client` / `tcp-server`) 로 통일 (Breaking)**

  이전엔 Samsung 만 `serial` / `tcp` 2-모드만 지원하여 TCP 서버 모드 (시리얼-Ethernet 컨버터의 push 연결) 가 불가능했다. LG / Century 와 동일한 패턴으로 `NasaTCPServerTransport` 를 추가하고 `transport_type` validation 을 확장한다. 기존 `tcp` 값은 더 이상 인식되지 않고 parse error 로 거부되며, 사용자는 `tcp-client` 로 명시적으로 마이그레이션해야 한다.

  - **신규**: `NasaTCPServerTransport` — LG `lgapTCPServerTransport` 패턴을 따르며, bind 주소 (`tcp_host`, 기본 `0.0.0.0`) 에서 단일 활성 연결 정책으로 동작한다. 새 연결이 들어오면 기존 활성 연결을 close 하고 교체한다.
  - **검증 변경**:
    - `transport_type`: `serial` / `tcp-client` / `tcp-server` 만 허용. `tcp` 입력 시 parse error.
    - `tcp_host`: `tcp-server` 는 기본값 `0.0.0.0` (모든 인터페이스), `tcp-client` 는 필수 (서버 IP 명시 필요).
    - `tcp_port`: 두 TCP 모드 모두 필수.
  - **마이그레이션**:
    - 기존 `transport_type: "tcp"` 설정은 `transport_type: "tcp-client"` 로 변경.
    - 예제 파일 rename: `examples/agents/samsung_hvacr01-tcp.yaml` → `samsung_hvacr01-tcp-client.yaml`.
    - 프론트엔드 schema (agentSchemas.ts, agentTypeMeta.ts) 도 3-모드 옵션 노출.

### 변경 (BREAKING) — 3종 HVACR-01 에이전트 (LG / Samsung / Century) config 필드·기본값·로그 옵션 통일 (LG 명세 기준)

- **3종 HVACR-01 에이전트 (LG / Samsung / Century) 의 에이전트 config 필드, 기본값, 로그 옵션을 LG 명세 기준으로 통일 (Breaking)**

  세 에이전트가 서로 다른 필드명·alias·기본값·로그 옵션을 사용하던 비대칭을 제거하고, 운영자가 어느 벤더 에이전트를 사용하더라도 동일한 멘탈 모델로 동작을 예측할 수 있도록 정렬한다. 본 변경은 backend 가 alias 를 silent accept 하지 않고 **명시적 parse error 로 거부**하므로, 부팅 즉시 실패 (fail loud) 한다.

  - **기본값 통일**:
    - Samsung `report_interval`: `0` → `"60s"` (기본 keepalive 활성화. 이전엔 변경 감지만 동작)
    - Samsung `auto_discovery`: `false` → `true` (LG / Century 와 동일하게 자동 탐색을 기본 활성)
    - Century `offline_timeout`: `"5s"` → `"30s"` (LG / Samsung 과 동일하게 30s 로 통일. 폴링 cycle 의 약 60배)

  - **필드 rename (alias 미수용, breaking)**:
    - Century `reconnect_initial` → `reconnect_interval` (Samsung 의 동명 필드와 정렬)
    - Samsung `include_raw_message_sets` → `include_raw_hex` (LG / Century 의 동명 필드와 정렬. 의미는 동일 — register-decoded / state response 에 원시 바이트 hex 포함 여부)
    - 3개 에이전트 모두: `notify_interval` deprecation alias **완전 제거** (이전엔 v1.6.0 / v0.6.0 부터 `report_interval` 로 통일하면서 alias 만 유지). 이제 `notify_interval` 키는 parse error.
    - Century: `keepalive_interval` / `keepalive_mode` **완전 제거** (이전 v0.3.x 의 device_state fallback emit 옵션). `report_interval` / `report_mode` 만 인식되며, 의미·동작 (relative / absolute crontab 패턴) 은 보존된다.

  - **로그 옵션 상향 통일 — 3개 에이전트 모두 동일 keys 노출**:
    - LG 신규 추가: `log_decode_errors`, `log_drops`, `log_state_updates`
    - Samsung 신규 추가: `log_drops`, `log_state_updates` (`log_decode_errors` 는 v1.9.0 부터 보유)
    - Century: 변경 없음 (이미 3개 모두 보유 — `log_decode_errors`, `log_drops`, `log_state_updates`)

  - **backend 거부 동작 (breaking — fail loud)**:
    - 다음 키가 config 에 존재하면 에이전트 init 시점에 parse error 로 즉시 부팅 실패: `notify_interval`, `include_raw_message_sets`, `reconnect_initial`, `keepalive_interval`, `keepalive_mode`.
    - 이전 v1.6.0 / v0.6.0 의 silent accept 방식이 운영자가 deprecation 사실을 인지하지 못한 채 alias 를 누적하던 문제 (구버전 yaml 이 작동하는 것처럼 보이지만 default 값이 적용됨) 를 해소한다.

  **운영자 마이그레이션**:
  - greenfield 환경: 별도 조치 불필요.
  - brownfield 환경: yaml 의 deprecated 필드를 신규 필드로 일괄 치환 후 부팅. 자세한 절차는 `docs/migration/hvacr-config-unification.md` 참조.
    - `notify_interval` → `report_interval`
    - `keepalive_interval` → `report_interval`
    - `keepalive_mode` → `report_mode`
    - `reconnect_initial` → `reconnect_interval`
    - `include_raw_message_sets` → `include_raw_hex`
    - Samsung `auto_discovery: true` 를 명시했던 기존 yaml: 생략 가능 (default 가 true)
    - Samsung `report_interval` 미설정 환경: 60s keepalive emit 이 시작됨. 변경 감지만 원하는 경우 `report_interval: "0s"` 명시.

### Removed

- **3종 HVACR-01 에이전트 (LG / Samsung / Century) deprecated config alias 5종 완전 제거 (breaking)** — `notify_interval`, `keepalive_interval`, `keepalive_mode`, `reconnect_initial`, `include_raw_message_sets`. config 에 존재 시 silent accept 되지 않고 parse error 로 거부된다. 이전엔 v1.6.0 / v0.6.0 부터 deprecation alias 로 일부만 수용되었으나, 본 변경에서 backend 가 명시적으로 거부하도록 통일했다.
- **LGAP / LGCP 에이전트의 `notify_interval` alias 완전 제거 (breaking)** — HVACR-01 3종에 이은 후속 정리. 두 에이전트가 마지막까지 `notify_interval` deprecation alias 를 silent accept 하던 비대칭을 해소. config 에 `notify_interval` 키가 존재하면 parse error 로 거부되며, `report_interval` 만 허용된다. 프론트엔드 schema 의 alias 안내 텍스트 (`이전 notify_interval, deprecation alias 유지`, `v0.6.0 통합 옵션`) 도 함께 제거.

### 변경 (BREAKING) — status 노드 3종 통일 (LG inactivity 모델) + 어드레싱 + 메타데이터 정리

- **`*_hvacr01_status` 노드 3종 (LG / Samsung / Century) config 구조를 LG inactivity 모델로 통일 (Breaking)**

  세 가지 status 노드가 서로 다른 모델 (LG = inactivity-fallback, Samsung/Century = ticker 기반 polling) 을 사용하던 비대칭을 제거하고, LG ICP-01 의 inactivity-fallback 모델을 표준으로 채택해 통일한다. 신규 어드레싱 필드 (`group_id`, `unit_id`) 를 도입하고, 의미가 모호하던 출력 metadata (`unit_id`, `slot_num`) 는 제거한다.

  - **동작 통일 — inactivity-fallback 모델**:
    - 노드는 에이전트의 `FrameNotifyCh` 신호를 수신하면서 frame 도착 시 즉시 처리한다.
    - `inactivity_timeout` (기본 `"90s"`) 동안 frame 신호가 수신되지 않으면 에이전트에 `request_state` 명령을 전송한다 (회선 silent 상태에서도 주기적 상태 확보).
    - 어드레싱 필드가 설정된 경우 매칭 frame 만 emit + `request_state` 의 target 으로 사용.

  - **신규 어드레싱 필드 (3종 status + 3종 combined 노드, advanced)**:
    - `unit_id` (string, hex): 프로토콜 디바이스 식별자.
      - LG: STX byte (`"58"` ODU, `"81"`–`"BF"` IDU, 64 units)
      - Samsung: NASA addr byte 2 (`"00"`–`"3F"` indoor; outdoor 는 group_id 와 동일)
      - Century: `sub_dev_id` (`"3B"` 등)
    - `group_id` (string, hex, Samsung 전용): NASA addr byte 1 / 외기 인덱스 (`"00"`–`"0F"`). LG / Century 는 schema parity 위해 필드 유지하나 미사용.
    - 두 필드 모두 빈 값일 때 모든 디바이스 처리 / broadcast `request_state`.

  - **Samsung status / combined 노드 변경**:
    - 제거: `device_id` (input config), `poll_command`, `poll_interval`, `device_address`
    - 추가: `inactivity_timeout`, `group_id`, `unit_id`
    - 단일 디바이스 조회는 노드 input 메시지 payload 의 `device_id` / `unit_id` override 로 가능 (제어 노드 / 통합 노드의 payload-level 지정은 유지).

  - **Century status / combined 노드 변경**:
    - 제거: `poll_command`, `poll_interval`, `recent_count`
    - 추가: `inactivity_timeout`, `group_id` (미사용), `unit_id`
    - 유지: `emit_raw_frames` (직전 raw-frame 통합 옵션)

  - **LG status / combined 노드 변경 (additive)**:
    - 추가: `group_id` (미사용, schema parity), `unit_id` (선택적 STX 필터)
    - 기존 `poll_interval` / `poll_command` / `recent_count` 는 deprecation alias 로 계속 수용 (no-op). LG 는 이미 inactivity 모델 — 동작 변경 없음.

- **출력 메시지 metadata 정리 — `unit_id` / `slot_num` 제거 (Breaking, 전 노드)**

  출력 metadata 의 `unit_id` 와 `slot_num` 은 프로토콜 해석 단계에서만 의미가 있는 내부 표현 (LGCNP `"0"`/`"1"`–`"5"`, NASA addr 분해 등) 으로, downstream consumer 가 알 필요가 없는 artifact 였다. 동일 정보가 필요한 경우 `metadata.device_id` (UUID) → DeviceRegistry 조회 또는 노드의 어드레싱 필드 (`unit_id`, `group_id`) 로 일대일 대응 가능하다.

  - `MetadataEmitOptions.UnitID` / `MetadataEmitOptions.SlotNum` 필드 제거
  - 노드 config 의 `emit_unit_id` / `emit_slot_num` 옵션 제거 (전 노드 — LG / Samsung / Century status·control·combined)
  - `dedup_helper` 의 `promoteDevIDToMetadata` / `promoteDevIDWithUUID` 에서 `unit_id` metadata emit 경로 삭제. payload 의 `unit_id` 는 항상 삭제되며 metadata 에는 노출되지 않는다.
  - 다운스트림 마이그레이션: `$.metadata.unit_id` / `$.metadata.slot_num` 참조 제거. 대신 `$.metadata.device_id` (UUID) 사용.

  **운영자 가이드**:
  - greenfield 환경: 자동 동작 — 별도 조치 불필요.
  - brownfield 환경:
    - Samsung flow yaml 의 status 노드 config 에서 `device_id` / `poll_interval` / `poll_command` / `device_address` 필드 제거 (필요 시 payload-level override 로 대체).
    - Century flow yaml 의 status 노드 config 에서 `poll_interval` / `poll_command` / `recent_count` 필드 제거. `emit_raw_frames` 는 유지.
    - 어드레싱이 필요한 경우 (단일 디바이스 만 처리) `unit_id` (Samsung 은 `group_id` 도) 를 advanced 필드로 설정.
    - downstream 의 `metadata.unit_id` / `metadata.slot_num` 필터 / 조인 키를 `metadata.device_id` 로 마이그레이션.

### 변경 (BREAKING) — `century-hvac` 식별자 rename 으로 Century ICP-01 프로토콜 / Century HVACR-01 에이전트 분리

- **Century `century-hvac` 식별자 rename — 프로토콜·에이전트·노드 명명 일관화 (Breaking)**

  세 가지 별개 도메인을 단일 식별자 `century-hvac` 가 표현하던 혼동을 제거하기 위해 코드베이스 전반의 식별자를 분리·rename 한다 (`lgcnp` / `samsung-nasa` rename 과 동일 패턴).

  - **프로토콜 코드**: `century-hvac` → `century_icp01` (Century ICP-01 와이어 프로토콜)
  - **에이전트 타입**: `century-hvac` → `century_hvacr01` (Century HVACR-01 에이전트)
  - **노드 타입**: `century` / `century-status` / `century-control` → `century_hvacr01` / `century_hvacr01_status` / `century_hvacr01_control`
  - Composite device ID 예: `century:3b` → `century_icp01:3b` (legacy ID 는 `internal/migrate/tsdbtags` / `internal/migrate/deviceids` 기존 마이그레이션 경로로 자동 이전)
  - SPEC 디렉터리: `SPEC-CENTURY-001` → `SPEC-CENTURY-HVACR-001`
  - 프로토콜 분석 문서: `references/protocols/century_hvac_protocol_spec.md` → `references/protocols/century_icp01_protocol_spec.md`
  - 예제 에이전트: `examples/agents/century-hvac*.yaml` → `examples/agents/century_hvacr01*.yaml`, 예제 플로우: `examples/flows/century-status-flow.yaml` → `examples/flows/century_hvacr01-status-flow.yaml`
  - Backend (`internal/agent/century/`, `internal/node/century_hvacr01.go`, 노드 레지스트리) 및 frontend (`web/src/config/agentSchemas.ts` / `nodeSchemas.ts` 의 타입 ID) 일괄 rename 완료. 본 CHANGELOG 항목은 문서 정합화를 마무리한다.

### 제거 (BREAKING) — `century-raw-frame` 노드 통합

- `century-raw-frame` 노드 타입이 제거되었다. 회선상 관측된 모든 raw frame (CRC 불일치 / payload prefix 위반 프레임 포함) 의 비파괴 emit 은 `century_hvacr01_status` 노드의 `emit_raw_frames: true` 옵션으로 흡수되었다 (ring buffer drain + raw frame 메시지 emit, dedupe_writes 와 무관). 동일한 raw frame payload schema 가 status 노드의 `out` 포트로 emit 되며, decoded 메시지 (`type=="century_reg02_response"` 등) 와 raw frame 메시지 (`type=="century_raw_frame"`) 는 `type` 필드로 구분한다. SPEC-CENTURY-HVACR-001 의 REQ-CENTURY-019 는 추적성 보존을 위해 REMOVED / CONSOLIDATED 노트로 유지된다.

  **운영자 가이드**:
  - greenfield 환경: 자동 동작 — 별도 조치 불필요.
  - brownfield 환경: 기존 device_metadata / TSDB tag / yaml `pinned` 의 `century:XX` 또는 `century/...` 참조는 `internal/migrate/tsdbtags` / `internal/migrate/deviceids` 의 기존 마이그레이션 경로로 자동 이전된다. flow yaml 에서 `century`, `century-status`, `century-control` 노드 타입 또는 `century-hvac` 에이전트 타입을 직접 참조하는 경우 `century_hvacr01`, `century_hvacr01_status`, `century_hvacr01_control` 로 갱신 필요. `century-raw-frame` 노드를 사용하던 flow 는 `century_hvacr01_status` + `emit_raw_frames: true` 옵션 조합으로 마이그레이션 필요.

### 변경 (BREAKING) — `nasa` / `samsung-nasa` 식별자 rename 으로 Samsung NASA 프로토콜 / Samsung HVACR-01 에이전트 분리

- **Samsung `nasa` / `samsung-nasa` 식별자 rename — 프로토콜·에이전트·노드 명명 일관화 (Breaking)**

  세 가지 별개 도메인을 단일 식별자 `nasa` / `samsung-nasa` 가 표현하던 혼동을 제거하기 위해 코드베이스 전반의 식별자를 분리·rename 한다.

  - **프로토콜 코드**: `nasa` → `samsung_nasa` (Samsung NASA 와이어 프로토콜)
  - **에이전트 타입**: `samsung-nasa` → `samsung_hvacr01` (Samsung HVACR-01 에이전트)
  - **노드 타입**: `nasa` / `nasa-status` / `nasa-control` → `samsung_hvacr01` / `samsung_hvacr01_status` / `samsung_hvacr01_control`
  - Composite device ID 예: `nasa:0x12` → `samsung_nasa:0x12` (legacy ID 는 `internal/migrate/tsdbtags` 기존 마이그레이션 경로로 자동 이전)
  - SPEC 디렉터리: `SPEC-NASA-001` → `SPEC-SAMSUNG-HVACR-001`
  - 예제 플로우: `examples/flows/nasa-*.yaml` → `examples/flows/samsung_hvacr01-*.yaml`, 예제 에이전트: `examples/agents/samsung-nasa-*.yaml` → `examples/agents/samsung_hvacr01-*.yaml`, 예제 스크립트: `examples/scripts/nasa-*.xflow` → `examples/scripts/samsung_hvacr01-*.xflow`
  - LG 노드 Go 타입은 본 rename 의 선행 작업으로 `internal/node/lg_hvacr01.go` 에서 `LG` prefix 적용 완료 (Samsung 노드 타입과 충돌 회피).
  - Backend (`internal/agent/samsung/`, `internal/node/adapter/samsung_nasa.go`, 노드 레지스트리) 및 frontend (`web/src/config/agentSchemas.ts` / `nodeSchemas.ts` 의 타입 ID) 일괄 rename 완료. 본 CHANGELOG 항목은 문서 정합화를 마무리한다.

  **운영자 가이드**:
  - greenfield 환경: 자동 동작 — 별도 조치 불필요.
  - brownfield 환경: 기존 device_metadata / TSDB tag / yaml `pinned` 의 `nasa:XX` 또는 `nasa/...` 참조는 `internal/migrate/tsdbtags` / `internal/migrate/deviceids` 의 기존 마이그레이션 경로로 자동 이전된다. flow yaml 에서 `nasa`, `nasa-status`, `nasa-control` 노드 타입 또는 `samsung-nasa` 에이전트 타입을 직접 참조하는 경우 `samsung_hvacr01`, `samsung_hvacr01_status`, `samsung_hvacr01_control` 로 갱신 필요.

### 변경 (BREAKING) — `lgcnp` 식별자 rename 으로 LG ICP-01 프로토콜 / LG HVACR-01 에이전트 분리

- **LG `lgcnp` 식별자 rename — 프로토콜·에이전트·노드 명명 일관화 (Breaking)**

  세 가지 별개 도메인을 단일 식별자 `lgcnp` 가 표현하던 혼동을 제거하기 위해 코드베이스 전반의 식별자를 분리·rename 한다.

  - **프로토콜 코드**: `lgcnp` → `lg_icp01` (LG ICP-01 와이어 프로토콜)
  - **에이전트 타입**: `lgcnp` → `lg_hvacr01` (LG HVACR-01 에이전트)
  - **노드 타입**: `lgcnp` / `lgcnp-status` / `lgcnp-control` → `lg_hvacr01` / `lg_hvacr01_status` / `lg_hvacr01_control`
  - Composite device ID 예: `lgcnp:81` → `lg_icp01:81` (legacy ID 는 `internal/migrate/tsdbtags` 기존 마이그레이션 경로로 자동 이전)
  - SPEC 디렉터리: `SPEC-LGCNP-001` → `SPEC-LG-HVACR-001`
  - 프로토콜 분석 문서: `references/protocols/LGCNP-01_Protocol_Analysis.md` → `references/protocols/LG-ICP-01_Protocol_Analysis.md`
  - Backend (`internal/agent/lg/lgcnp_*.go` → `lg_hvacr01_*.go` / `lg_icp01_*.go`, `internal/node/lgcnp.go` → `lg_hvacr01.go`) 및 frontend (`web/src/config/agentSchemas.ts` / `nodeSchemas.ts` 의 타입 ID) 일괄 rename 완료. 본 CHANGELOG 항목은 문서 정합화를 마무리한다.

  **운영자 가이드**:
  - greenfield 환경: 자동 동작 — 별도 조치 불필요.
  - brownfield 환경: 기존 device_metadata / TSDB tag / yaml `pinned` 의 `lgcnp:NN` 또는 `lgcnp/...` 참조는 `internal/migrate/tsdbtags` / `internal/migrate/deviceids` 의 기존 마이그레이션 경로로 자동 이전된다. flow yaml 에서 `lgcnp`, `lgcnp-status`, `lgcnp-control` 노드 타입을 직접 참조하는 경우 `lg_hvacr01`, `lg_hvacr01_status`, `lg_hvacr01_control` 로 갱신 필요.

### 변경 (BREAKING) — xflowd v1.0 진입 준비

- **SPEC-DEVICE-IDENTITY-001 Phase D — xflowd v1.0 메이저 (Breaking)**

  v0.x 의 composite 디바이스 식별자 (`agent_name:local_id` — 예: `lgcnp:81`) 가 완전히 제거되고, UUID v4 가 유일한 글로벌 식별자가 된다. 본 변경은 greenfield 환경 가정 (외부 클라이언트 부재) 하에 호환 alias 비용 없이 메이저 릴리스로 진행한다. brownfield 환경은 사전 마이그레이션 (`xflowd migrate device-ids`) 이 선결 조건이다.

  **핵심 Breaking 변경**:

  - **`Device.ID()` 시맨틱 변경**: composite key 대신 UUID v4 (`UID()` 와 동일 값) 반환. 5 어댑터 (NASA/LGCNP/LGCP/Century/Modbus) 모두 일관 적용. 사람이 읽는 식별이 필요한 호출자는 `AgentName()` + `Name()` 또는 `agent/name` REST 라우트 사용.
  - **REST URL composite alias 거부**: `GET /api/v1/devices/{composite}` (예: `lgcnp:81`) 는 HTTP 404 + 마이그레이션 안내 메시지. `ClassifyDeviceRef` 가 composite 패턴을 `DeviceRefUnknown` 으로 분류하여 `DeviceRegistry.ResolveDevice` 가 자동 거부.
  - **WebSocket emit payload 의 `device_id` 필드 완전 제거**: `device.status` 메시지가 `uid` (UUID) 만 노출. Frontend 는 PR2 (D-T20) 에서 마이그레이션 완료.
  - **HVAC 에이전트 emit payload**: 5 에이전트 모두 `{type, unit_id, device_id (UUID), trigger, state, metadata}` schema. composite `id` 필드 부재 확인.
  - **yaml composite 거부**: yaml 의 `pinned: ["lgcnp:81"]` 같은 composite 참조는 부팅 즉시 실패 (`ErrInvalidDeviceReference`). PR1 D-T13 의 yaml_resolver 변경에 D-T2 의 ClassifyDeviceRef 변경이 함께 적용되어 default 분기로 일관 거부.
  - **부팅 시 자동 sanity check**: runServer 시작 시 `device_metadata.json` 의 composite key 잔존을 자동 검증. 발견 시 부팅 거부 + `xflowd migrate device-ids` 명령 안내.

  **신규 명령**:

  - **`xflowd preflight`** — 부팅 사전 점검 명령 (D-T5). config yaml 형식 + composite 참조 부재, `device_ids.json` 로드 가능, `device_metadata.json` UUID-key 검증을 read-only 로 수행. 성공 시 exit 0 + PASSED, 실패 시 exit 1 + 항목별 actionable 메시지.

  **운영자 마이그레이션 가이드** (`docs/migration/device-identity.md`):

  - **greenfield 환경**: 자동 동작 — 아무 조치 불요.
  - **brownfield 환경 (v0.x → v1.0)**:
    1. `xflowd migrate device-ids --metadata-dir <data>/device_metadata` 실행 (PR1 D-T19 의 deprecated noop 이 아닌 실제 변환은 v0.x 빌드에서 수행).
    2. 모든 디바이스 메타데이터 파일이 UUID-key 명명인지 확인.
    3. 외부 클라이언트 / 대시보드 마이그레이션 확인 (REST URL / yaml / MQTT 구독자).
    4. `xflowd preflight` 실행 → PASSED 확인.
    5. v1.0 으로 업그레이드.

  **세부 인수 기준 (D-AC1 ~ D-AC18) 충족 매트릭스**: `.moai/specs/SPEC-DEVICE-IDENTITY-001/acceptance.md` 참조. PR1~PR4 누적으로 모두 GREEN.

### 추가 (Added)

- **SPEC-DEVICE-IDENTITY-001 Phase A** — 디바이스 ID 체계 통일의 첫 단계 (비파괴 추가). `Device` 인터페이스에 `UID() string` 메서드를 1급으로 격상하여 글로벌 유일·불변 UUID 를 노출한다 (Kubernetes 의 `metadata.uid` 패턴 차용). 5개 디바이스 어댑터(NASA/LGCNP/LGCP/Century/Modbus) 가 생성 시점에 `agent.ResolveDeviceID` 또는 동등 경로로 UUID 를 발급받아 보유하며, REST `GET /api/v1/devices` / `GET /api/v1/devices/{id}` 응답에 `uid` 필드가 1급으로 노출된다 (omitempty graceful degradation — `DeviceIDRepository` 미설정 환경에서는 필드가 생략된다). 기존 composite `id` 필드 (`"agent:local_id"`) 는 그대로 유지되어 외부 클라이언트는 영향을 받지 않는다. `DeviceIDRepository` 가 nil 인 경우 부팅 시 1회 경고 로그가 출력되며 (Phase D 에서 부팅 실패로 전환 예정), Prometheus 메트릭 `xflowd_device_uid_missing_total` 로 UUID 미발급 디바이스 수를 관측 가능하다. 인수 기준 A-AC1/A-AC2/A-AC4/A-AC5 충족 (A-AC3 emit map literal `uid` 키 추가는 Phase B 통합). 4 커밋 (`991e793`, `be86904`, `bc67561`, `510fc4b`), production 8 파일 / 테스트 5 파일, 회귀 0건. 후속 Phase B/C/D 는 별도 SPEC 진화로 진행 예정.

### 변경 (Changed)

- **SPEC-DEVICE-IDENTITY-001 Phase A** — `internal/node/inventory.go` 의 `resolveDeviceUUID` 가 디바이스 UUID 발급 시 `Device.UID()` 를 직접 사용하도록 정렬 (`510fc4b`, A-AC5). 이전에는 어댑터별 local ID 형식 차이로 인해 Century 어댑터의 `localID` 형식 불일치(`agent:device:N` 표준 대비 자체 ID 만 반환)가 inventory 노드 UUID 발급 경로에서 잠재적 매핑 실패를 일으킬 수 있었으나, 본 변경으로 동시에 해소되었다. 외부 동작 변경 없음 (비파괴).

### 수정 (Fixed)

- **SPEC-AUTH-004** — REST `POST /api/v1/auth/login` 응답 스키마 정합화 (`{user, tokens}` 중첩 구조) 및 클라이언트 `authStore` 의 토큰 보존 자가 회복 로직 도입. SPEC-AUTH-002 시점(`8635e1f`, 2026-03-31)부터 잠재했던 결함이 SPEC-DASHBOARD-001 v0.2.0 의 `basic_auth: true` 기본값 전환과 함께 표면화된 것을 해소. 서버 DTO 재구조화(`internal/api/dto/auth.go`, `internal/api/handler/auth.go`) + 클라이언트 매핑 변환(`web/src/services/api/authService.ts`) + UB1 `saveTokens` falsy 가드 + UB2 `loadTokens` broken state 자가 회복(`web/src/stores/authStore.ts`) 으로 구성. 기존 활성 사용자 세션은 invalidation 되며 자동 클린업 후 재로그인이 필요하다 (UB2 가 literal `"undefined"` 가 저장된 broken localStorage state 를 자가 회복). 자동화 acceptance AC-1/AC-2/AC-5/AC-6/AC-7 GREEN (906 tests pass), AC-3 (페이지 새로고침 후 인증 복원) / AC-4 (SPEC-AUTH-003 통합 WS 회귀) 는 main 머지 이전 수동 검증 게이트. 신규 의존성/디렉터리/아키텍처 패턴 0건. (commit `8bf49e0`)
- **SPEC-DASHBOARD-001 v0.2.1 hotfix** — dashboard handler 응답 envelope 표준화. `internal/api/handler/dashboard.go` 의 3개 `ctx.JSON` (GET, PUT 200, PUT 409) 이 `dto.NewSuccessResponse(...)` envelope 을 누락하여 클라이언트 axios interceptor (`client.ts:19-44`) 의 strict `body.success` 검증에서 정상 200/409 응답이 `APIError('UNKNOWN', 200)` 으로 변환되고 `DashboardServerError('unexpected status 200')` 를 throw 하여 사용자에게 "대시보드 저장에 실패했습니다…" 토스트가 표시되던 결함을 해소. 서버 측 DB 저장은 성공이었으나 클라이언트가 실패로 인지하던 비대칭 결함. 본 결함은 SPEC-AUTH-004 AC-3/AC-4 수동 검증 (`basic_auth.enabled: true`) 중 dashboard PUT 흐름 관찰로 식별됨. 22 dashboard handler tests + 222 전체 handler tests GREEN, SPEC-AUTH-004 auth 테스트 회귀 0건. 신규 의존성 0건. (commit `9d239c0`)

### 변경 (BREAKING)

- **대시보드 구성 서버 영속화 v0.2.0** (SPEC-DASHBOARD-001 v0.2.0, BREAKING)

  대시보드 페이지/그리드/레이아웃 구성이 브라우저 `localStorage` 에서 SQLite 서버 영속 저장소로 전환된다. v0.1.0 의 결정 일부가 무효화되어 SQLite 채택 + 공유/개인 병행 모델 + 자격증명 SQLite 이관 + localStorage 블랭크 슬레이트 정책이 1급 채택되었다.

  **사용자 안내 (UI 토스트 / CHANGELOG / README 공통 문구)**:

  > 이번 업데이트(v0.2.0)로 대시보드 구성이 서버 저장으로 전환되었습니다. 기존 로컬 구성은 초기화됩니다. 공유 대시보드는 관리자가 다시 구성해 주세요.

  **basic_auth 필수화 (ASM-003)**:
  - `serverCfg.BasicAuth.Enabled=true` 가 v0.2.0 기본값이며 `/api/dashboards/*` 모든 엔드포인트가 유효한 JWT 를 요구한다 (UR-004).
  - `basic_auth.enabled=false` 로 부팅 시 거부되거나, 개발/데모 환경 한정으로 `XFLOW_ALLOW_NO_AUTH=1` 환경변수를 설정하면 경고 로그와 함께 강제 활성화된다.

  **자격증명 저장소 이관 (UR-006, UB-007)**:
  - `~/.xflow/users.yaml` → SQLite `users` 테이블로 부팅 시 1회성 자동 이관 (`INSERT OR IGNORE`) 후 yaml 파일이 `users.yaml.migrated` 로 rename 되어 이후 어떤 인증 흐름에서도 참조되지 않는다.
  - `internal/auth/credentials.go` 의 외부 API (`Load`/`Save`/`Authenticate`/`ChangePassword`/`EnsureDefaultAdmin`/`GetUser`) 시그니처는 유지되어 호출부 변경 없음 (백워드 호환).
  - 로그: `auth: migrated N users from yaml to sqlite`.

  **localStorage 블랭크 슬레이트 (ASM-006 폐기)**:
  - v0.1.0 의 "localStorage → 서버 1회성 마이그레이션" 결정 폐기. 첫 부팅 시 클라이언트가 `dashboardPages`, `activeDashboardId`, `dashboardGridCols`, `dashboardShowGridLines`, `dashboardRefreshInterval`, `deviceGridLayout` 6개 키를 명시적으로 제거하고 토스트를 1회 노출한다.
  - `xflow-ui:dashboard-migrated-v0.2` 플래그로 1회 보장 (새로고침 시 재실행 방지).
  - `theme`, `sidebarCollapsed`, `customThemeTokens` 는 기기별 환경설정으로 보존된다.

  **Zustand `partialize` 축소**:
  - 위 6개 대시보드 키가 직렬화 대상에서 완전 제외된다 (UB-002).
  - 메모리 상태에 `sharedSnapshot`, `mineSnapshot` 두 슬롯과 `activeDashboardScope: 'shared' | 'mine'` 추가.

  **품질 게이트 (TRUST 5 PASS)**:
  - 백엔드 85%+ 커버리지, `go test -race ./...` 통과.
  - 프론트엔드 `useDashboardSync` / `uiStore` 단위 테스트 통과.
  - golangci-lint / biome 0 issues.

### 추가

- **공유 + 개인 대시보드 REST API 6 엔드포인트** (SPEC-DASHBOARD-001 v0.2.0)

  운영자가 어떤 브라우저/기기/시크릿창에서 접속하더라도 (공유) + (본인 개인) 두 snapshot 이 일관되게 보이도록 한다. 자세한 API 명세는 `docs/api/dashboards.md` 참조.

  | Method | Path | 권한 |
  |--------|------|------|
  | GET | `/api/dashboards/shared` | 인증된 사용자 (전부) |
  | PUT | `/api/dashboards/shared` | admin only |
  | DELETE | `/api/dashboards/shared` | admin only |
  | GET | `/api/dashboards/mine` | 인증된 사용자 |
  | PUT | `/api/dashboards/mine` | 인증된 사용자 |
  | DELETE | `/api/dashboards/mine` | 인증된 사용자 |

  **신규 백엔드 파일**:
  - `internal/storage/dashboard_sqlite.go` — SQLite `dashboards` 테이블 기반 `DashboardRepository` 유일 구현 (`Get` / `Put` / `Delete`, 트랜잭션 내 If-Match + 서버측 version 부여).
  - `internal/storage/users_sqlite.go` — `users` 테이블 CRUD + yaml → SQLite 1회 이관 헬퍼.
  - `internal/api/handler/dashboard.go` — 6 엔드포인트 + JWT 미들웨어 + `requireAdmin` (shared PUT/DELETE) + owner spoofing 차단 + URL/body scope 불일치 거부.
  - `internal/api/dto/dashboard.go` — `DashboardSnapshot` DTO (`scope`, `owner`, `version`, `updatedAt`, `payload`).

  **SQLite 스키마 (자동 생성, `CREATE TABLE IF NOT EXISTS`)**:
  - `dashboards` 테이블 (`scope` ∈ `{'global','user'}`, `owner` nullable, `version` 단조 증가, `updated_at` epoch ms, `payload` JSON TEXT).
  - `dashboards_scope_owner_uidx` partial unique index 로 `COALESCE(owner, '')` 기반 cross-scope 공존 보장 (`scope=global,owner=NULL` 과 `scope=user,owner=<username>` 이 동일 인덱스에서 충돌 없이 공존).
  - `users` 테이블 (`username` UNIQUE, `password_hash`, `role` ∈ `{'admin','editor','viewer'}`, `created_at`/`updated_at` epoch ms).

  **신규 프론트엔드 파일**:
  - `web/src/types/dashboard.ts` — `DashboardSnapshot`, `DashboardScope`, `DashboardPayload` 타입.
  - `web/src/services/api/dashboardService.ts` — 6 메서드 REST 클라이언트 (`If-Match` 헤더 지원, 401/403/409 응답 분기).
  - `web/src/hooks/useDashboardSync.ts` — 부팅 시 `Promise.all([getShared, getMine])` 병렬 GET, 500ms debounce PUT, 409 last-write-wins 재PUT (1회 한정).
  - `web/src/pages/dashboard/DashboardPage.tsx` "공유" / "내 대시보드" 탭 토글 (admin 외에는 공유 탭 편집 컨트롤 비활성).

  **응답 코드 매트릭스**:
  - `200 OK` — 성공
  - `204 No Content` — DELETE 성공
  - `400 Bad Request` — payload schema 오류, URL vs body scope 불일치
  - `401 Unauthorized` — JWT 없음/만료
  - `403 Forbidden` — 권한 부족 (editor 가 shared PUT/DELETE 시)
  - `404 Not Found` — GET 시 snapshot 미존재 (초기 상태)
  - `409 Conflict` — `If-Match` version 불일치 (body 에 서버측 최신 snapshot)
  - `413 Payload Too Large` — payload 크기 256 KB 초과
  - `500 Internal Server Error` — 저장소 I/O 실패

### Deprecated

- **`~/.xflow/users.yaml`** (SPEC-DASHBOARD-001 v0.2.0): v0.2.0 부팅 시 SQLite `users` 테이블로 1회성 자동 이관된 후 `users.yaml.migrated` 로 rename 된다. 이후 어떤 인증 흐름에서도 yaml 은 참조되지 않으며, SQLite 만 source-of-truth 다 (UB-007). 신규 사용자는 SQLite 에 직접 INSERT 되며, 별도 등록/삭제/목록 관리 REST API 는 `SPEC-USER-MGMT-001` (OI-004, 추후) 로 분리된다.

### Removed

- **대시보드 페이지/그리드/레이아웃 localStorage 영속화** (SPEC-DASHBOARD-001 v0.2.0): `web/src/stores/uiStore.ts` 의 `partialize` 에서 `dashboardPages`, `activeDashboardId`, `dashboardGridCols`, `dashboardShowGridLines`, `dashboardRefreshInterval`, `deviceGridLayout` 6개 키가 완전 제외되었다 (UB-002). 첫 부팅 시 1회 명시적 제거 후 더 이상 직렬화되지 않는다.
- **v0.1.0 의 `internal/storage/dashboard_file.go` 결정 폐기** (SPEC-DASHBOARD-001 v0.2.0): JSON 파일 1차 채택 결정이 SQLite 채택으로 무효화되었다 (OI-003 CLOSED). 해당 파일은 실제로 작성된 적이 없으며, v0.2.0 에서도 작성하지 않는다.
- **v0.1.0 의 "localStorage → 서버 1회성 마이그레이션" 흐름 삭제** (SPEC-DASHBOARD-001 v0.2.0, ASM-006 폐기): 사용자별 스코프와 권한 모델 도입으로 클라이언트 측 단일 페이로드를 "공유" 와 "개인" 중 어디로 보낼지 자의적으로 결정할 수 없기 때문이다. 대신 모든 사용자는 빌트인 기본 대시보드에서 새로 시작하며, admin 이 공유 대시보드를 새로 구성한다.

### Security

- **Cross-user 대시보드 접근 차단** (SPEC-DASHBOARD-001 v0.2.0, UB-005): `/api/dashboards/mine` 은 항상 JWT `Claims.Username` 으로만 owner 가 결정되며 별도의 username 파라미터를 받지 않는다. 사용자 A 는 사용자 B 의 개인 대시보드를 GET/PUT/DELETE 할 수 없다.
- **Owner spoofing 차단** (SPEC-DASHBOARD-001 v0.2.0, UB-003): 서버는 PUT 페이로드의 `scope`/`owner`/`version`/`updatedAt` 을 모두 무시하고, URL (shared/mine) + JWT `Claims.Username` + 저장소 상태로 결정한다. 클라이언트가 body 에 `owner: 'bob'` 을 보내도 alice 의 JWT 로 요청하면 alice 의 snapshot 이 갱신된다.
- **URL vs body scope 불일치 거부** (SPEC-DASHBOARD-001 v0.2.0, UB-006): silent normalize 금지. URL 의 scope (shared/mine) 와 body 의 `scope` 가 불일치하면 `400 Bad Request` 로 명시적으로 거부한다.
- **Admin role guard** (SPEC-DASHBOARD-001 v0.2.0, UB-004): editor/viewer 는 공유 대시보드를 GET 만 가능하며, `PUT /api/dashboards/shared` 또는 `DELETE /api/dashboards/shared` 시도는 `403 Forbidden` 으로 거부된다.
- **Payload 크기 캡 256 KB** (SPEC-DASHBOARD-001 v0.2.0, UR-003): 초과 시 `413 Payload Too Large`. JSON schema 오류는 `400 Bad Request` 로 거부.
- **basic_auth 강제** (SPEC-DASHBOARD-001 v0.2.0, UR-004): 모든 `/api/dashboards/*` 엔드포인트는 유효한 JWT 를 요구한다. 익명 접근 불허. `XFLOW_ALLOW_NO_AUTH=1` 환경변수는 개발/데모용 opt-out 으로만 사용되며 경고 로그를 남긴다.

- **Frontend Store 키 모델 v0.7.0 적응** (SPEC-WEB-005 v0.7.0, BREAKING for frontend internal API)
  
  SPEC-STORE-003 v0.3.0 백엔드 BREAKING (registration_type, data_type, metric_type, 객체 배열 응답)에 대응하는 frontend 단독 진화. 운영자에게 노출되지 않는 내부 API contract 변경이므로 end-user 마이그레이션 가이드는 불필요하며 개발자 대상 변경만 다룬다.
  
  **타입 진화 (M11)**:
  - `StoreKeysRawResponse.keys: string[]` → `keys: StoreKeyObject[]` (`{key, registration, data_type, metric_type, tags}`)
  - 신규 타입: `DataType`, `RegistrationSource`, `StoreKeyObject` (`@/services/api/store`)
  - 신규 함수: `fetchStoreKeyObjects(agentName)` (Phase E 에서 직접 활용)
  - 백워드 호환: `useStoreKeysWithTags` 가 `{keys: string[], tags: StoreKeyTagsMap, keyObjects: StoreKeyObject[]}` 반환 (기존 소비자 무수정)
  
  **Config UI 진화 (M12, M13)**:
  - `agentSchemas.ts`: `allow_dynamic_keys: bool` 토글 → `registration_type: select` (manual|auto, default auto)
  - `StoreKeysEditor`: 신규 `data_type` 셀렉트 컬럼 (6종 enum) + `metric_type` 입력 컬럼 (정규식 검증)
  - 신규 helper `storeKeysValidation.ts`: `validateDataType`, `validateMetricType`, `DATA_TYPE_OPTIONS`
  
  **PromoteToStaticDialog 진화 (M14)**:
  - 동적→정적 변환 시 `data_type` 필수 + `metric_type` 옵션 입력
  - `defaultDataType` prop 으로 백엔드 추론 값 사전 채움
  - `onConfirm` 시그니처 변경: `(tags) => void` → `(payload: PromoteToStaticPayload) => void`
  
  **에러 매핑 (M15)**:
  - 신규 모듈 `storeErrorMapper.ts`: 4종 백엔드 에러 (`ErrTypeMismatch`, `ErrUnsupportedValueType`, `ErrInvalidDataType`, `ErrInvalidMetricType`) + 마이그레이션 에러를 한국어 사용자 친화 메시지로 매핑
  - `mapStoreError(err): StoreErrorMapped` 통합 진입점
  
  **메타데이터 표시 + 필터 UI (M16, Task 13, Task 14)**:
  - 신규 컴포넌트 `MetadataChips`: data_type (6종 색상) / metric_type / registration auto/manual 배지
  - `TsdbDataViewerModal` 시리즈 행에 메타데이터 칩 표시
  - 신규 필터 UI 3축: `?data_type=`, `?metric_type=` (datalist 자동완성), `?registration=` (segmented), 모두 AND 결합
  - `StoreKeysEditor` 행에 manual 배지 (yaml 정의 = manual 시각 reminder)
  - `metric_type === "unknown"` 키는 muted 표시
  
  **품질 게이트**:
  - 625/625 tests pass (Vitest, +103 신규)
  - TypeScript strict pass (any 사용 0)
  - storeErrorMapper.ts / MetadataChips.tsx / storeKeysValidation.ts 100% 커버리지
  - StoreKeysEditor 99.35%, PromoteToStaticDialog 97.87%, TsdbDataViewerModal 90.62%
  - Vite production build success
  - 신규 외부 라이브러리 추가 없음
  
  **알려진 차이**: SPEC-STORE-003 v0.3.0 의 M9 known divergence (?metric_type= 빈 값) 는 v0.7.0 frontend 측 필터에서도 동일하게 no-op passthrough 로 처리됨.

- **Store 에이전트 키 메타데이터 모델 v0.3.0 진화** (SPEC-STORE-003 v0.3.0)

  v0.2.0의 `allow_dynamic_keys` (bool)을 `registration_type` (enum: `manual` | `auto`)로 **clean rename** 한다 (하위호환 shim 없음). 또한 `data_type` (6종 enum), `metric_type` (semantic free string) 1급 필드를 신설하고, `GET /keys` API 응답을 string 배열에서 객체 배열로 진화시킨다.

  **YAML 스키마 변경 (BREAKING)**:
  - `allow_dynamic_keys: false` → `registration_type: "manual"`
  - `allow_dynamic_keys: true` → `registration_type: "auto"` (또는 생략, default `auto`)
  - manual 모드의 `keys[]` 각 엔트리는 `data_type` 명시 필수 (6종: `int`/`float`/`string`/`boolean`/`bytes`/`json`)
  - 신규 optional `metric_type` 필드 (free string `^[a-zA-Z0-9_-]+$`, default `"unknown"`)
  - **부팅 가드**: `allow_dynamic_keys` 잔존 시 명시적 에러로 부팅 실패 ("removed in v0.3.0; use 'registration_type: manual|auto' instead")

  **API 응답 변경 (BREAKING)**:
  - `GET /api/v1/store/{name}/keys` 응답: string 배열 + 별도 `tags` 맵 → 객체 배열 `[{key, registration, data_type, metric_type, tags}]`
  - 응답 객체는 항상 5개 필드 모두 포함 (빈 tags도 `{}`로 명시)
  - 응답 배열은 `key` 알파벳 오름차순 정렬 (안정성 보장)

  **API 신규 필터 (NEW)**:
  - `?data_type=<int|float|string|boolean|bytes|json>` (단일 값)
  - `?metric_type=<value>` (단일 값)
  - `?registration=<manual|auto>` (단일 값)
  - 기존 `?tag=key:value`와 모두 **AND 조건** 결합 (`?registration=manual&metric_type=temperature&tag=room:1`)

  **신규 에러 4종**:
  - `ErrTypeMismatch`: 등록된 `data_type`과 쓰기 값 Go 타입 불일치 (auto 모드 첫 쓰기 후 영구 고정)
  - `ErrUnsupportedValueType`: nil/chan/func 등 추론 불가 타입 (auto 모드)
  - `ErrInvalidDataType`: yaml의 `data_type` 값이 6종 enum 외이거나 manual 모드에서 누락
  - `ErrInvalidMetricType`: `metric_type`이 정규식 위반

  **운영 마이그레이션** (필수):
  - 기존 yaml의 `allow_dynamic_keys` 모두 `registration_type`으로 변환 필요
  - manual 모드의 모든 정적 키에 `data_type` 추가 필요
  - 자세한 절차: `docs/migration/v0.3.0-store-keys.md` 참조

  **알려진 차이 (M9 known divergence)**: `?metric_type=` 빈 값은 SPEC 명시("빈 결과 반환")와 달리 no-op passthrough로 처리된다. metric_type normalize 정책으로 사용자 영향 없음. 다음 SPEC 갱신에서 SPEC을 구현에 맞춰 정렬할 예정.

  **연관 SPEC**:
  - SPEC-WEB-005 v0.5.0 (예정): UI는 객체 배열 응답에 적응 + `data_type`/`metric_type` 편집 UI 제공

  **품질**: TRUST 5 PASS, 1296 race-clean 테스트, `store_data_type.go` 100% 커버리지, golangci-lint 0 issues.

### 추가

- **xflowd 자동 업데이트 v0.2.0 진화** (SPEC-UPDATE-002 v0.1.0)

  SPEC-UPDATE-001 v0.1.0 (xflowd 자동 업데이트 기반) + SPEC-WEB-006 v0.1.0 (Admin UI) 의 후속 진화. v0.1.0 에서 운영자 부담으로 남겨두었던 3가지 한계 (수동 재시작, 채널 변경 부재, 단일 바이너리만 지원) 를 모두 해소한다. 신규 의존성 추가 없이 기존 라이브러리 재사용으로 backward compatibility 를 100% 유지한다.

  **In-Process Restart (M-1)**:
  - graceful drain (active connections 보호 + in-flight 요청 완료 대기)
  - TOCTOU 재검증 (`syscall.Exec` 직전 SHA256 + Ed25519 재검증으로 다운로드 후 디스크 변조 차단)
  - `syscall.Exec` 자기 교체 (PID 보존, OS 가 자동으로 새 바이너리로 프로세스 이미지 교체)
  - health check + auto rollback (재시작 후 health endpoint 폴링, 실패 시 `.previous` 자동 복원 후 재기동)
  - 신규 sentinel error: `ErrUpdateRestartFailed`, `ErrUpdateHealthCheckFailed`

  **Channel REST API (M-2, M-7, M-8)**:
  - `GET /api/v1/system/update/channel` — 현재 채널 (stable/beta/nightly) + manifest URL 조회
  - `POST /api/v1/system/update/channel` — 채널 변경 (admin role guard, 비-admin 시 HTTP 403)
  - `ContextKeyUserRole` export 로 role 기반 가드 일관화
  - `ChannelChangeDialog` UI (admin 전용 채널 선택/변경 다이얼로그)
  - `SystemVersionCard` 가 `isAdmin` 일 때만 채널 변경 버튼 노출

  **Multi-Binary Auto-Update (M-3, M-9, M-11, M-12)**:
  - 3개 바이너리 지원: `xflowd` (daemon) / `xflow-agent` (edge agent) / `xflow` (CLI)
  - `DependencyManifest` 에 semver constraint 표현 + `ManifestFetcher` (HTTPS 강제 + 64KB DoS cap + Ed25519 서명 검증)
  - `CompatibilityChecker` 로 다운그레이드/non-compatible upgrade 사전 차단
  - ReDoS-resistant semver regex (`^...$` 앵커 적용으로 백트래킹 폭발 차단)
  - target whitelist (`xflowd|xflow-agent|xflow` 3종만 허용 → path traversal / arbitrary binary swap 방어)
  - `UpdateDialog` 의 admin target dropdown (target 선택 + auto_restart checkbox)
  - 신규 sentinel error: `ErrUpdateIncompatibleVersion`

  **11-state OperationStatus Machine (M-1)**:
  - 기존 9-state 머신 → 11-state 확장
  - 신규 상태: `restarting` (in-process restart 중), `health_checking` (재시작 후 health 검증 중)
  - `UpdateProgressStepper` 시각화 + `StatusLabel` 한국어 라벨 + admin role 별 표시
  - 신규 상태는 `auto_restart=true` 시에만 진입 (v0.1.0 동작 보존)

  **Backward Compatibility (M-14)**:
  - `ApplyRequest` 확장: `target`, `auto_restart` 모두 옵셔널 (v0.1.0 동작 100% 보존)
  - `target` 미지정 → `xflowd` default
  - `auto_restart` 미지정 → `false` (v0.1.0 과 동일하게 운영자 수동 재시작 경로 유지)
  - 11-state machine 의 신규 상태 (`restarting`, `health_checking`) 는 `auto_restart=true` 시에만 진입
  - Scenario 11: `target=xflow` (CLI) 선택 시 `auto_restart` 자동 해제 + disabled (CLI 는 daemon 이 아니므로 self-restart 불필요)

  **품질 지표 (TRUST 5 PASS)**:
  - `internal/updater` 91.9% / `internal/api/system_update.go` 92.0% 커버리지
  - `go test -race ./...`: 모든 패키지 통과 (race-clean)
  - web test suite: 858/858 통과 (신규 ~50 tests 추가)
  - 41 `UpdateDialog` tests + 24 `RestartOrchestrator` tests + 20+ manifest tests
  - `ChannelChangeDialog` 99.46% 커버리지
  - TypeScript strict / ESLint / `gofmt` / `go vet` 모두 클린

  **보안 검증 (PASS)**:
  - TOCTOU 재검증 (`syscall.Exec` 직전 Ed25519 + SHA256 재검증으로 디스크 변조 공격 차단)
  - Admin role guard (HTTP 403 + `ContextKeyUserRole` 일관 적용)
  - target whitelist (`xflowd|xflow-agent|xflow` 만 허용, path traversal / arbitrary binary swap 방어)
  - HTTPS 강제 + 64KB DoS cap + ReDoS-resistant semver
  - OWASP A01 (Broken Access Control) / A02 (Cryptographic Failures) / A03 (Injection) / A06 (Vulnerable Components) / A08 (Software and Data Integrity Failures) 점검 통과

  **신규 외부 의존성**: 0개 (기존 lib 재사용 — `crypto/ed25519`, `crypto/sha256`, `syscall`, Go stdlib + 기존 frontend stack)

  **Out of Scope (후속)**:
  - SPEC-UPDATE-003 (예정): Windows 지원 (`syscall.Exec` 대안 — Windows 는 exec semantic 차이로 별도 SPEC 필요)
  - 향후: `cmd/xflowd` CLI 의 `--auto-restart`, `--target` 플래그를 daemon-side API 호출 모드로 활용 (현재는 daemon-side ApplyRequest 만 지원)

- **Web Admin: 시스템 자동 업데이트 UI** (SPEC-WEB-006 v0.1.0)

  관리자가 Web UI 에서 xflowd 자동 업데이트를 안전하게 관리할 수 있다.
  SPEC-UPDATE-001 v0.1.0 의 5 REST API 를 소비하는 frontend 컴포넌트 모음.

  **System Status Panel** (`/admin/system`):
  - 현재 버전 + 빌드 메타 (commit, build_date, go runtime) 표시
  - 채널 정보 (stable/beta/nightly) 배지
  - 업데이트 가능 인디케이터 + 최신 버전 표시
  - 60s 자동 폴링 (TanStack Query refetchInterval)
  - "업데이트 확인" 버튼 (즉시 채널 폴)

  **Update Dialog** (5단계 UX):
  - info → confirm → apply → progress → result
  - 9-state machine 시각화 (UpdateProgressStepper)
  - 1s 진행률 폴링 (백엔드 OperationStatus 동기화)
  - 다운그레이드 force checkbox (필요 시)
  - 실패 시 명시적 rollback 버튼 + 다시 시도

  **Restart Guide** (M8, v0.1.0 한계 보완):
  - in-process restart 미지원 → 운영자 수동 재시작 안내
  - systemd 명령 + 수동 명령 양쪽 표시
  - 클립보드 복사 버튼 (per-command)

  **Header Badge + Toast Notification**:
  - 헤더 우측 RefreshCw 아이콘 + 노란색 dot (update_available 시)
  - 한 번만 발생: false→true 전환 감지 (useRef 패턴)
  - 클릭 시 /admin/system 페이지로 이동
  - admin role 미보유 시 disabled

  **권한 모델** (M11, Decision Point 5):
  - 기존 AuthGuard 에 requireRole="admin" 옵션 확장
  - 비-admin 접근 시 ForbiddenPage 노출 (한국어 403)
  - authEnabled=false (dev mode) 우회 보존

  **에러 메시지 매핑** (M12):
  - 12종 백엔드 에러 분류 → 한글 사용자 친화 메시지
  - HTTP 401/409 + body keyword 우선순위 매칭
  - reuse 패턴 (storeErrorMapper, SPEC-WEB-005 v0.7.0)

  **품질 검증 (TRUST 5 PASS)**:
  - 808/808 vitest 통과 (신규 ~182 tests)
  - 신규 파일 함수 커버리지 100%
  - storeErrorMapper.ts 100%, UpdateProgressStepper.tsx 100%, etc.
  - TypeScript strict 통과 (any 0건)
  - Vite production build 성공 (SystemStatusPage 27.58 kB)

  **신규 외부 의존성**: 0개 (React 19 + TanStack Query + Tailwind + lucide-react 모두 기존)

  **알려진 차이/제약 (v0.1.0 백엔드 한계 보완)**:
  - in-process restart 미지원 → CLI 안내 + 클립보드 복사
  - 자동 health check + rollback wiring 미연결 → 명시적 rollback 버튼
  - 채널 변경 REST API 부재 → 읽기 전용 표시 + CLI 안내

  **후속 SPEC**:
  - SPEC-UPDATE-002 (예정): 멀티 바이너리 + 채널 변경 API + in-process restart
  - SPEC-WEB-007 (예정): RBAC 정식화

- **xflowd 자동 업데이트 메커니즘** (SPEC-UPDATE-001 v0.1.0)

  운영자는 GitHub Releases 채널 (stable/beta/nightly) 에서 새 xflowd 바이너리를
  안전하게 다운로드/검증/적용할 수 있다. 자가 교체 + 자동 롤백으로 BREAKING
  배포 후 운영자 부담 경감.

  **보안 (M4, M13)**:
  - Ed25519 디지털 서명 + SHA256 체크섬 검증 (timing-safe 비교, crypto/subtle.ConstantTimeCompare)
  - HTTPS 강제 (Checker + Downloader 다중 경계 검증)
  - 공개키 핀닝 (PEM/hex/file 로더, RSA 자동 거부)
  - TOCTOU 방어 (다운로드 직후 + 원자적 교체 직전 2회 검증)
  - DoS 방어 (io.LimitReader: checksum 1MB / signature 64KB)

  **흐름 (M3-M7)**: check → download (HTTPS) → verify → apply (atomic rename
  via go-update) → restart (syscall.Exec). 실패 시 백업 자동 복원 (.previous 접미사).

  **다운그레이드 차단 (M8)**: --force 플래그 없이 거부. 3단 방어 (Checker +
  CLI + REST API).

  **CLI (M9)**:
  - `xflowd update check` — 새 버전 확인
  - `xflowd update apply [--version vX.Y.Z] [--force] [--yes]` — 적용
  - `xflowd update status [--json]` — 작업 상태
  - `xflowd update rollback [--yes]` — 이전 버전 복원
  - `xflowd update channel <stable|beta|nightly>` — 채널 변경

  **REST API (M10)**:
  - `GET /api/v1/system/version` — 현재 버전 + 메타데이터
  - `POST /api/v1/system/update/check` — 채널 폴
  - `POST /api/v1/system/update/apply` — 비동기 작업 시작 (operation_id 반환)
  - `POST /api/v1/system/update/rollback` — 백업 복원
  - `GET /api/v1/system/update/status` — 마지막 작업 스냅샷

  **설정 (M11, `.moai/config/update.yaml`)**:
  - `enabled: false` (기본값, 명시적 opt-in)
  - `channel: stable|beta|nightly`
  - `update_url: https://api.github.com/...`
  - `public_key_path` 또는 `public_key_hex`
  - `auto_apply: false` (수동 승인 권장)
  - `drain_timeout`, `health_check_timeout`, `health_check_endpoint`

  **품질 게이트**:
  - 280+ 신규 테스트 (15 GWT + 16 보안 + 10 E2E + 핸들러 + CLI + 단위)
  - 16개 위협 벡터 보안 검증 (MITM/replay/서명 위조/바이너리 변조/timing/DoS)
  - internal/updater 92.6% 커버리지, verifier.go 100%
  - golangci-lint 0 issues, go vet clean, race-detector pass
  - 모든 39 패키지 회귀 0건

  **신규 의존성**: `github.com/inconshreveable/go-update` (atomic rename, 검증된 lib)

  **Out of Scope (후속 SPEC)**:
  - SPEC-UPDATE-002: xflow-agent / xflow CLI 멀티 바이너리 조정
  - SPEC-UPDATE-003: Windows 지원
  - SPEC-WEB-006: Web UI System Status Panel + 업데이트 다이얼로그

  **Known Limitations**:
  - In-process restart는 v0.1.0에서 ready_to_restart 상태로 종료 (외부 supervisor 의존)
  - 자동 헬스체크 + 자동 롤백 wiring 은 후속 SPEC iteration에서 구현

- **저장소 전체/개별 키 초기화 기능** (SPEC-STORE-003)
  - `DELETE /api/v1/store/{name}/keys/{key}` — 정적 키는 history만 삭제, 동적 키는 entry 완전 삭제
  - `DELETE /api/v1/store/{name}/keys` — bulk 적용, 카운트 응답
  - 신규 백엔드 메서드: `Store.ClearHistory(ctx, key)` (VolatileStore/NamespacedStore/PersistentStore 구현)
  - Frontend: `ConfirmDialog` 재사용 컴포넌트, "전체 초기화" 헤더 버튼, 행별 휴지통 아이콘

- **동적 키를 정적으로 변환하는 UI** (SPEC-STORE-003)
  - 저장소 리스트의 동적 키 행에 변환 버튼 추가
  - `PromoteToStaticDialog` 신규 컴포넌트: 태그 입력 후 config.keys 추가
  - Configure API 재사용 (별도 백엔드 변경 없음)

- **TSDB 데이터 뷰어 3종 개선** (SPEC-WEB-005)
  - 키 세그먼트에서 태그 자동 추출 (`keyTagExtractor` 유틸): InfluxDB 스타일 + colon/slash segments
  - 평균 집계 소수점 자릿수 입력 (기본 1, 0-6 범위) — 매트릭스 셀 + CSV 모두 적용
  - 매트릭스 페이지네이션 (페이지 크기 [10, 25(기본), 50, 100])
  - react-window 가상화 제거 (페이지네이션으로 대체)

- **TSDB/Store 에이전트 시리즈 탐색 및 데이터 뷰어** (SPEC-WEB-005 v0.4.0)
  - 페이지네이션 (10/25/50/100), 다중 시리즈 매트릭스 쿼리(키=컬럼, 시간=행)
  - 데이터 뷰어 모달 (95vw×95vh): 절대/상대 시간 모드, 인터벌 프리셋, 집계(min/max/avg)
  - 5,000행 경고 + react-window 가상 스크롤(500행 이상 자동)
  - CSV 내보내기 (UTF-8 BOM, 로컬 ISO-8601 timezone offset)
  - SeriesDataSource 통합 어댑터로 tsdb/store 모두 지원
  - 저장소 탭에 통합된 데이터 보기 버튼 + 페이지네이션
  - 설정 탭 운영/데이터 섹션 분리 + 태그 chip 필터(저장소 + 데이터 뷰어)
  - 363+ 테스트 신규/추가 (frontend), 35+ 테스트 (backend)

- **Store 에이전트 정적 키 정의 및 태그 메타데이터** (SPEC-STORE-003 v0.1.0)
  - `allow_dynamic_keys` (default true): false 시 정적 목록 외 키 쓰기 거부 (`ErrKeyNotAllowed`)
  - `keys: [{key, tags: map[string]string}]` 정적 키 정의 (태그 key regex `^[a-zA-Z0-9_-]+$`)
  - 신규 API: `GET /api/v1/store/{name}/keys?tag=k:v` (다중 AND 필터), `GET /api/v1/store/{name}/tags` (유니크 태그 페어 목록)
  - 기존 `GET /keys` 응답에 optional `tags` 맵 포함 (omitempty 하위호환)
  - api.Context 에 `QueryValues(name) []string` 추가 (다중 쿼리 파라미터)

- **chart-emitter 배치 입력 모드** (SPEC-CHART-001 v1.2.0)
  - `entries_field` config 추가. 설정 시 `payload[entries_field]` 배열을 개별 ChartEntry 로 분해하여 publish.
  - 배열은 **timestamp 오름차순으로 정렬**된 뒤 순차 publish → FIFO 링버퍼가 `buffer_size` 를 초과해도 최신 타임스탬프가 남음.
  - `store-read(read_mode=last_n/duration/time_range)` 의 배열 출력을 라인/바 차트 backfill 에 직접 공급 가능 (기존 v1.1.0 에서는 배열 전체가 하나의 `value` 로 감싸져 차트가 그려지지 않던 문제 해결).
  - 필드가 없거나 배열이 아니면 단일 엔트리 모드로 fallback → 동일 emitter 에 이력 배치 + 실시간 append 혼합 공급 허용.
  - primitive 배열 (`[21, 22, 23]`) 도 지원: 각 값이 `value` 로 저장되고 `timestamp` 는 현재 epoch ms 로 자동 주입.
  - Go 테스트 7개 추가 (`TestChartEmitterNode_Process_BatchMode_*`), race clean.
  - 문서 (`docs/guides/chart-panel-flow.md`) 의 Example 2 를 `entries_field` 사용 패턴으로 개편.
  - 노드 상세 패널의 입력 예제가 배치/단일 모드 양쪽을 명시적으로 보여주도록 갱신.

- **차트 패널 플로우 연동 시스템 구현** (SPEC-CHART-001)
  - **`chart-emitter` 종단 노드** (`internal/node/chart_emitter.go`): 입력 메시지를 WebSocket 차트 채널로 발행하고 링버퍼(FIFO + retention 스윕)에 보관. config: `channel_name` (정규식 검증), `buffer_size` (1-10000), `retention_sec` (0-86400). 채널 이름 중복 시 fail-fast Init 에러.
  - **`ChartChannelRegistry` 싱글톤** (`internal/agent/system/chart_channel_registry.go`): 프로세스 전역 채널 레지스트리. ChartSubscriber 인터페이스, EncodeChart{Backfill,Append,Closed,Error} 프레임 헬퍼. race-clean (sync.RWMutex).
  - **`GET /ws/chart/{channel}` WebSocket 엔드포인트** (`internal/api/ws/chart_channel.go`): channel_name 정규식 검증 (HTTP 400), 미존재 채널 `chart.error`, backfill + append fan-out, slow-consumer 보호 (256 항목 버퍼 + 초과 시 close), 연결 종료 시 자동 unsubscribe.
  - **HTTP 쿼리 API** (외부 도구 / 디버깅용):
    - `GET /api/v1/charts/channels` — 활성 chart-emitter 채널 목록
    - `POST /api/v1/store/{agent}/query` — Store 5-모드 HistoryQuery (latest/last_n/duration/time_range/since_n)
    - `POST /api/v1/influxdb/{agent}/query` — Flux / InfluxQL 쿼리 (`InfluxDBAgent.ExecuteFluxQuery`/`ExecuteInfluxQLQuery` 신규 메서드)
    - 표준 응답 스키마: `{entries: [{timestamp, value, labels}], count, truncated}`
  - **5종 차트 패널** (`web/src/pages/dashboard/panels/charts/`): Stat (delta + 임계값 색상), Line Chart (multi-series 지원), Bar Chart (category / time_bin 모드), Pie Chart (집계), Table (정렬 + 페이지네이션). Recharts 3.7 기반 + HTML table.
  - **`useChartChannel` React 훅 + `ChartChannelClient`** (`web/src/services/ws/chartChannel.ts`): exponential backoff 재연결 (1→16s), chart.closed 수신 시 영구 종료, maxPoints 슬라이딩 윈도, factory 주입으로 테스트 가능.
  - **대시보드 UI 확장**:
    - AddPanelDialog: 차트 타입 선택 시 채널 드롭다운 (`GET /api/v1/charts/channels` 연동) + 수동 입력 + 정규식 인라인 검증
    - PanelSettingsDialog: 5종 차트 타입별 config 편집 섹션
    - `panelDefaultSize` SPEC 값 적용 (stat 2×1, line-chart 6×3, bar-chart 4×3, pie-chart 3×3, table 6×4)
    - uiStore v3→v4 persist 마이그레이션 (기존 dataSource/period config 보존)
  - **플로우 캔버스 통합**: `web/src/config/nodeSchemas.ts` + `web/src/pages/nodes/nodeTypeMeta.ts` 에 chart-emitter 등록. NodePalette 이 `category=output` 그룹에 자동 배치, PropertyPanel 이 DynamicForm 으로 설정 편집 UI 자동 생성.
  - **활용 가이드 문서** (`docs/guides/chart-panel-flow.md`): 아키텍처 다이어그램, payload 정규화 규칙, 3종 예시 플로우 YAML, 기존 노드 조합 패턴, 운영 주의사항, HTTP 쿼리 API curl 예시, 문제 해결 표.
  - **핵심 설계 원칙**:
    - 차트 패널은 데이터 소스를 몰라야 한다 — 오직 `channel_name` 만 안다
    - 필터링/집계/정렬은 플로우 노드(`filter`, `aggregate`, `mapping`)가 담당
    - 모든 타임스탬프는 epoch ms (int64) 로 통일
    - 채널 = 하나의 chart-emitter 인스턴스 (중복 이름 fail-fast)
  - **테스트**: Go 신규 파일 평균 93% 커버리지 + `go test -race` 통과 / Vitest 132 테스트 평균 88% 커버리지. TRUST 5 게이트 전부 통과.

- **TCP 소스 노드에 `connection_id` 메타데이터 주입** (SPEC-NODE-003)
  - `TCPInNode.receiveLoop`에서 매 메시지에 `connection_id` 메타데이터를 설정하여, framer 노드와 결합 시 TCP 서버의 다중 클라이언트 연결별 독립 프레이밍을 지원.
  - TCP 서버 모드(`ConnAwareReceiver`): `connection_id` = `remoteAddr` (host:port). `tcp.remote_addr`과 동일한 값으로 설정되며, 기존 `tcp.remote_addr` 메타데이터도 그대로 유지 (하위 호환).
  - TCP 클라이언트 모드(`MessageReceiver`): `connection_id` = `n.ID()` (노드 ID). 단일 연결이므로 고정 식별자로 일관된 메타데이터 구조 제공.
  - framer 노드의 기본 `stream_key_metadata="connection_id"`와 자동 연동되어, 추가 설정 없이 "tcp-in (서버) → framer" 파이프라인에서 연결별 프레이밍 동작.

- **`pkg/framing` 공개 패키지 신설** (SPEC-NODE-002)
  - 시리얼 에이전트 내부(`internal/agent/serial/framing.go`)에 있던 프레이밍 엔진을 `pkg/framing` 공개 패키지로 승격하여 범용 재사용이 가능하도록 함.
  - 6가지 프레이밍 모드 지원: `raw`, `newline`, `length_prefix`, `fixed_size`, `stream`, `frame`.
  - `Framer` 인터페이스, `Options` 구조체, `New` 팩토리 함수, `Drain` API, 모드 상수 (`ModeRaw`, `ModeNewline`, `ModeLengthPrefix`, `ModeFixedSize`, `ModeStream`, `ModeFrame`) 노출.
  - `ScannerConfigurer` 인터페이스를 통해 기존 `SerialConnReader`와의 호환성 유지.
  - sentinel 에러 (`ErrETXMismatch`, `ErrChecksumMismatch`, `ErrMaxSizeExceeded`, `ErrLengthInvalid`) 노출로 에러 분류 지원.
  - 시리얼 에이전트(`internal/agent/serial`)는 `pkg/framing`을 import하여 기존과 동일한 동작을 유지.

- **`framer` 처리 노드 추가** (SPEC-NODE-002)
  - 임의의 바이트 스트림 소스 노드(serial-in, tcp-in, udp-in 등)의 출력을 받아 프로토콜 프레임으로 분리하는 `framer` 처리 노드를 `internal/node/`에 추가.
  - 입력 포트 `in`, 출력 포트 `out`, 에러 포트 `error`의 3포트 구조.
  - 메타데이터 기반 다중 스트림 버퍼 관리 (`connection_id` 등 `stream_key_metadata` 키로 스트림 분리). 키가 없으면 단일 공용 버퍼로 동작.
  - `max_streams`, `stream_idle_timeout` 옵션을 통한 자원 제한 및 유휴 스트림 lazy 축출.
  - 에러 포트를 통한 파싱 에러 분리 (에러 코드: `frame.input.invalid_payload`, `frame.parse.etx_mismatch`, `frame.parse.checksum_mismatch`, `frame.parse.max_size_exceeded`, `frame.buffer.max_streams_exceeded`, `frame.buffer.incomplete_on_stop`).
  - 출력 메시지는 소스 노드 규약(`raw` + `data` 키)을 그대로 따르며, 업스트림 메타데이터 보존 및 `frame.index`, `frame.framer_type`, `frame.stream_key` 메타데이터 추가.
  - 시리얼 에이전트 `framing=frame` 경로와의 완전 동등성을 Parity 테스트(`framer_parity_test.go`)로 검증.
  - Registry 팩토리(`framer_factory.go`)를 통한 빌트인 노드 등록 및 옵션 파싱.

- **에이전트 활성화/비활성화 기능** (SPEC-AGENT-005)
  - `internal/agent/config.go`: `AgentConfig.Enabled *bool` 필드와 `(*AgentConfig).IsEnabled() bool` 메서드 추가. `pkg/flow/node.go`의 `NodeDef.Enabled` 패턴을 재사용하며, `nil` 은 기본값 `true` 로 해석되어 하위 호환성을 보장한다.
  - `internal/agent/serialize.go`: JSON/YAML 직렬화에 `enabled` 필드 추가 (`omitempty`). 기존 저장 데이터는 마이그레이션 없이 그대로 사용 가능하다.
  - `cmd/xflowd/main.go`: 저장소 복원 로직을 테스트 가능한 `restoreAgents` 헬퍼로 추출하고, disabled 상태의 에이전트는 매니저에 등록(Create)만 하고 자동 시작(Start)을 건너뛰도록 변경. 건너뛴 경우 INFO 레벨로 "자동 시작 건너뜀 (disabled)" 로그를 남긴다.
  - `internal/api/handler/agent.go`: `POST /agents/{id}/enable`, `POST /agents/{id}/disable` 엔드포인트 추가. `AgentInfo` 응답에 `enabled: bool` 필드를 항상 포함한다.
  - `internal/api/service/agent_adapter.go`: `EnableAgent`/`DisableAgent` 서비스 메서드와 영속화 실패 시 in-memory 롤백을 수행하는 `setAgentEnabled` 공통 헬퍼 구현. Disable 은 런타임 Stop 을 호출하지 않으며, Enable 은 정지된 에이전트를 자동 Start 하지 않는다 (R3.7, R3.8).
  - `internal/engine/errors.go`: `ErrAgentDisabled` sentinel error 추가.
  - `internal/engine/agent_validation.go`: `validateAgentRefs` 가 플로우 노드가 참조하는 에이전트의 활성화 상태를 검증한다. 비활성화된 에이전트를 참조하는 플로우는 `DeployFlow` 시점에 거부되며, 다수의 검증 실패는 `errors.Join` 으로 한 번에 보고된다. 에러 메시지에는 에이전트 ID, 노드 이름, Enable API 안내가 포함된다.
  - `web/src/types/agent.ts`: `AgentInfo.enabled?: boolean` 필드 추가.
  - `web/src/services/api/agentService.ts`: `enableAgent(id)`, `disableAgent(id)` API 클라이언트 함수 추가.
  - `web/src/hooks/useAgent.ts`: `useEnableAgent`, `useDisableAgent` React Query mutation hook 추가. 성공 시 `['agents']` 및 상세 쿼리 캐시를 무효화한다.
  - `web/src/pages/agents/AgentEnabledBadge.tsx`: 비활성화된 에이전트를 시각적으로 구분하는 회색 배지 컴포넌트 추가. `enabled !== false` 인 경우 렌더링하지 않아 기존 UI 에 노이즈를 주지 않는다.
  - `web/src/pages/agents/AgentActionButtons.tsx`: Enable/Disable 토글 버튼(Power/PowerOff 아이콘) 추가. 비활성화된 에이전트의 수동 Start 시 사용자에게 일시 시작임을 안내하는 확인 다이얼로그 표시.
  - `web/src/pages/agents/AgentListPage.tsx`: 에이전트 이름 옆에 `AgentEnabledBadge` 표시.

- **통합 디바이스 관리 시스템** (SPEC-DEVICE-001)
  - `internal/device/`: 프로토콜 무관 통합 Device/ControllableDevice 인터페이스, DeviceState, CommandSpec, DeviceProvider, DeviceFilter, DeviceMetadata 모델
  - `internal/device/registry.go`: 중앙 DeviceRegistry - RegisterProvider/UnregisterProvider, 필터링, 동시성 안전 설계
  - `internal/device/adapter/nasa.go`: NASADevice를 통합 Device 인터페이스로 래핑하는 어댑터 (ControllableDevice 지원)
  - `internal/agent/samsung/provider.go`: NASAAgent에 DeviceProvider 인터페이스 구현
  - `internal/api/handler/device.go`: 디바이스 REST API 6개 엔드포인트 (목록/상세/실행/메타데이터/명령/상태)
  - `internal/agent/manager.go`: 에이전트 시작/중지 시 DeviceProvider 자동 등록/해제 라이프사이클 훅
  - `internal/api/ws/event_publisher.go`: WebSocket 기반 실시간 디바이스 상태 변경 알림
  - `web/src/pages/devices/DeviceListPage.tsx`: react-grid-layout 기반 디바이스 목록 대시보드 (카드, 필터, 검색, 추가)
  - `web/src/pages/devices/DeviceDetailPanel.tsx`: 디바이스 상세 패널 (상태/속성, CommandSpec 동적 제어 UI, 리모컨)
  - `web/src/pages/agents/AgentDetailPanel.tsx`: 에이전트별 디바이스 CRUD 탭 (추가/제거, 소스 배지)
  - `web/src/hooks/useWebSocket.ts`: WebSocket 연동 실시간 상태 업데이트

### 변경

- **버킷 타임스탬프 벽시계 경계 정렬** (SPEC-STORE-003): `bucketStart = floor(tsMs / intervalMs) * intervalMs` (epoch zero 기준). 1m → 초=0, 5m → 분 0/5/10..., 1h → 분=초=0. TSDB 엔진은 이미 정렬되어 있어 변경 없음.
- **저장소 탭 행별 액션을 마지막 "액션" 컬럼으로 분리**: 타입 컬럼은 정적/동적 배지만, 마지막 컬럼에 [정적변환] [초기화] 버튼 모음
- **전체 초기화 버튼 색상 중립화**: 빨간 톤 → 다른 헤더 버튼과 동일 (destructive 의도는 ConfirmDialog danger variant 가 담당)
- **StoreKeysEditor 키:태그 컬럼 비율 1:3**: `table-fixed` + `<colgroup>` 25%/75%
- **데이터 뷰어 시리즈 multi-select 가시 영역 확장**: `max-h-40 → max-h-[40vh]` (모달 95vh 활용)
- **`UserStoreAgent.Configure` runtime 정책 반영** (SPEC-STORE-003): 이전에는 `agentConfig` 만 갱신하고 inner store 에 미반영 → 정책 필드(`allow_dynamic_keys`, `staticKeys`) 를 runtime 적용 (운영 필드는 restart 필요 유지)
- **`NodeStoreAdapter` lazy resolver 패턴**: 생성 시점 store 스냅샷 → 매 호출마다 resolver 함수로 현재 inner 조회. 에이전트 재시작 후에도 플로우 노드가 재연동 없이 자동으로 새 inner 사용
- `internal/agent/samsung/config.go`: NASA 에이전트 `device_addresses` 설정을 선택 사항으로 변경 (기존: 필수)
- `internal/agent/samsung/agent.go`: processAddDevice/processRemoveDevice에서 req.Params 폴백 읽기 추가
- `web/src/config/agentSchemas.ts`: samsung-nasa 에이전트 스키마에서 device_addresses 필드 제거
- `web/src/config/nodeSchemas.ts`: framer 노드 스키마 추가 (framing_mode, stx, etx, checksum, length_size, max_frame_size, fixed_size, stream_key_metadata, max_streams, stream_idle_timeout 설정)

### 수정

- **readOnly 모드에서 설정 값 가시성 복원** (FormField + StoreKeysEditor + property 편집기 8종)
  - `disabled={readOnly}` → `readOnly={readOnly}` (text/number/textarea)
  - readOnlyClass 단순화: `bg-(--color-bg-elevated)` 토큰 사용
  - 영향: TriggerScheduleEditor, BridgeHttp/Mqtt/ModbusConfig, KeyValueMapEditor, RegisterMapEditor, StringListEditor, TransformPipelineEditor, FormField, StoreKeysEditor
- **정적 키 태그 입력 자동 commit on blur** (SPEC-STORE-003): TagChipsEditor의 keyInput/valInput 이 폼 외부 클릭 시 자동으로 chip 으로 commit. 사용자가 "추가" 버튼 누르지 않고 저장 클릭해도 태그 보존.
- **FormField boolean 기본값 미표시** (SPEC-WEB-005): `value === undefined` 일 때 `field.default` 를 반영하도록 수정. SPEC-STORE-003 의 `allow_dynamic_keys` (default true) 가 기존 config 에 없을 때 unchecked 로 잘못 표시되던 문제 해결.
- **ETX 필드 선택 사항 처리** (`pkg/framing`): `frame` 모드에서 ETX가 빈 값일 때 ETX 검증을 건너뛰도록 수정. ETX 없는 프레임 프로토콜 지원.
- **LengthSize 기본값 보정** (`pkg/framing`): `length_prefix` 모드에서 `length_size` 미지정 시 기본값 2를 적용하도록 수정. 이전에는 0으로 해석되어 프레이밍이 실패함.
- **configInt 문자열 처리** (`internal/node/framer_factory.go`): YAML/JSON에서 정수 설정이 문자열로 전달되는 경우를 처리. `strconv.Atoi` 폴백으로 `"2"` → `2` 변환 지원.
