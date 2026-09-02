# SPEC-MODBUS-007 구현 계획 (plan.md)

> 연관: `spec.md`(요구사항 M1~M5), `acceptance.md`(AC-01~08).
> 시간 예측 없음 — 우선순위/의존성 기반 마일스톤으로 표기.

## 1. 기술 스택

| 레이어    | 스택                                                                                     |
|-----------|------------------------------------------------------------------------------------------|
| 백엔드    | Go 1.23+, `internal/node` 노드 프레임워크(`*BaseNode`, `Process(ctx, msg) ([]msg, error)`) |
| 프론트    | React + TypeScript, `web/src/config/nodeSchemas.ts` + `web/src/components/property/`      |
| 재사용    | `modbus_common.go` area 상수/`validRegisterAreas`, `command_set` op-list + payload 오버라이드 패턴, `ModbusCommandSetEditor` / `KeyValueMapEditor` / `MODBUS_DATA_TYPE_OPTIONS` |

## 2. 의존성

- **SPEC-MODBUS-006** (`modbus-read` 출력 계약): 입력 스키마 `{success, values[], agent_type}`, 엔트리 `{index, area, address, count, values[]|raw, data_type?}`. 본 노드는 이 계약 위에 구축된다.
- **핵심 제약(계약에서 유래)**: read-op 출력 엔트리에는 `unit_id`가 없다 → From 매칭은 `(area, address, count)` 기준, device_id는 To측 할당값.

## 3. 아키텍처 방향

- `modbus-remap`은 **stateless 변환 노드**다. 라이브 에이전트/RegisterMap을 참조하지 않고, message payload만 소비/생산한다(`mapping.go`와 동일 계층).
- 변환 의미는 `device_view.go:resolveSegment`(오프셋 + 완전-포함-아니면-거부)를 **참조**하되, 엔트리 전체 정확 매칭으로 단순화한다(코드 직접 재사용 아님).
- config 기본 op-list(`rules`/`templates`)를 RLock으로 읽고, payload에 동일 키 존재 시 대체(`modbus_read.go` `command_set` 선례).

## 4. 태스크 분해

### Backend

- **B1 (우선순위 High)**: `registry.go` `registerBuiltins()`에 `{"modbus-remap", NewModbusRemapNode, "modbus", "레지스터 주소/영역/디바이스 재매핑"}` 1행 추가.
- **B2 (우선순위 High)**: `internal/node/modbus_remap.go` 신규 — `NewModbusRemapNode(def, opts...) (Node, error)`, `*BaseNode` 임베드, `Init` / `Configure` / `Process` / `Shutdown`.
- **B3 (우선순위 High)**: config 파싱 — `rules []map[string]any`(source_area/source_address/count/target_unit_id/target_area?/target_address) + `templates []map[string]any`(area/offset). `toOpList` 유사 헬퍼로 op-list 정규화.
- **B4 (우선순위 High)**: `Process` remap 로직 — 입력 `values[]` 파싱 → 각 규칙에 대해 엔트리 전체 정확 매칭(`area+address+count`) → 매칭 시 오프셋 재주소화 + area/unit_id 재작성 + 값 보존 → 재매핑 `values[]` 방출. 미매칭 규칙은 거부(규칙별 error, `success=false`).
- **B5 (우선순위 Medium)**: 템플릿 인스턴스화 헬퍼 — `(device_id, start, count)` 주입 → 구체 규칙(`target_address = start + offset`, `target_area = area`, `target_unit_id = device_id`).
- **B6 (우선순위 Medium)**: payload 오버라이드 — RLock으로 config 기본 `rules`/`templates` 읽기, payload 키 존재 시 대체.
- **B7 (우선순위 Medium)**: `modbus_common.go` `validRegisterAreas`/area 상수 재사용으로 area 유효성 검증.
- **B8 (우선순위 High)**: Go 테이블 테스트 — AC-01~06 커버.

### Frontend

- **F1 (우선순위 High)**: `nodeSchemas.ts`에 `modbus-remap` 엔트리(description/inputDesc/outputDesc/configSchema.fields/defaultPorts) 추가(`modbus-read` ~1703-1731 모델).
- **F2 (우선순위 High)**: `FormField.tsx`에 신규 `field.type` 분기(~327-449 패턴) 추가 — `RegisterRemapEditor` 렌더.
- **F3 (우선순위 High)**: `RegisterRemapEditor.tsx` 신규 — 좌(From)/우(To) 2-테이블. From 컬럼: area/address/count. To 컬럼: device_id(unit_id)/area/address. 템플릿 정의(area/offset) + 적용(device_id/start/count) 컨트롤. `ModbusCommandSetEditor`(1행=1규칙 테이블) + `KeyValueMapEditor`(2-컬럼) 결합, `MODBUS_DATA_TYPE_OPTIONS` 재사용, `{value, onChange, readOnly}` + `toRows`/`toEmit` 규약 준수.
- **F4 (우선순위 Medium)**: `tsc --noEmit` / web build 클린 확인.

## 5. 마일스톤 (우선순위 기반)

- **1차 목표 (Primary)**: B1~B4 + B8 — 백엔드 노드 등록 + 엔트리 전체 remap(주소+area+device_id) + 정확 매칭 거부 + 테스트. (핵심 remap 동작 완성.)
- **2차 목표 (Secondary)**: B5~B7 — 템플릿 인스턴스화 + payload 오버라이드 + area 검증.
- **최종 목표 (Final)**: F1~F4 — nodeSchemas 엔트리 + FormField 분기 + RegisterRemapEditor 2-테이블 UI.
- **선택 목표 (Optional)**: UI 미리보기(From→To 매핑 시각화) 등 편의 기능.

## 6. 핵심 기술 제약

1. **오프셋 변환 수학**: `targetAddr = targetStart + (srcAddr − srcStart)`. 엔트리 전체 remap에서 `srcAddr == entry.address`이므로 결과는 `target_address`, `count` 보존.
2. **positional-array 재작성**: 값 배열/raw는 그대로 전달(변형 금지), 주소/영역/디바이스 메타만 재작성.
3. **엔트리 전체 정확 매칭**: `(area, address, count)` 정확 일치 없으면 규칙 거부(부분 매핑 금지).
4. **area/unit_id 재매핑**: target_area 미지정 시 source_area 유지; unit_id는 To측 할당(입력 엔트리에 unit_id 없음).
5. **템플릿 인스턴스화**: `(area, offset)` + `(device_id, start, count)` → 구체 규칙. config 내부 저장만.

## 7. 리스크 및 대응

| 리스크                                        | 영향 | 대응                                                                 |
|-----------------------------------------------|------|----------------------------------------------------------------------|
| 입력 엔트리에 unit_id 부재 → From 매칭 혼선   | 중   | From 매칭은 area+address+count로만 명세 고정, device_id는 To측 값으로 문서/코드 일치 |
| positional 값 배열 오변형                     | 중   | 값 보존 불변식 + 테이블 테스트(AC-04)로 검증                          |
| area 변경 시 read-only/boolean area 처리      | 중   | `validRegisterAreas`/`booleanAreas` 재사용, boolean area 값 표현 보존(AC 엣지) |
| 템플릿 인스턴스화 오프셋 부호/경계 오류       | 중   | 템플릿 적용 결과를 구체 규칙으로 전개 후 동일 remap 경로로 처리(AC-02) |
| payload 오버라이드와 config 기본 혼용         | 낮음 | `command_set` 선례 그대로(RLock 읽기 → payload 대체), 회귀 테스트     |
| 프론트 2-테이블 라운드트립(toRows/toEmit) 손실 | 낮음 | 기존 에디터 규약 준수 + `tsc --noEmit` 클린                          |

## 8. 검증 접근

- 백엔드: `go build ./...`, `go test ./internal/node/...` (AC-01~06).
- 등록 확인: 노드가 `/nodes`(registry) 목록에 `modbus-remap`으로 노출.
- 프론트: web build / `tsc --noEmit` 클린, 노드 config 폼에서 From→To 편집 동작(AC-07~08).
