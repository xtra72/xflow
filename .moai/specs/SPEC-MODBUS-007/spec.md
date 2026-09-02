---
id: SPEC-MODBUS-007
title: "MODBUS Register Remapper 노드 (주소/영역/디바이스 재매핑)"
version: "0.3.0"
status: draft
created: 2026-08-02
updated: 2026-08-02
author: xtra
priority: P2
phase: "v0.1.0 target"
module: "internal/node + web/src/config + web/src/components/property"
lifecycle: spec-first
tags: "modbus, remap, register, node, flow, template, from-to"
depends_on:
  - SPEC-MODBUS-006
---

# SPEC-MODBUS-007: MODBUS Register Remapper 노드 (주소/영역/디바이스 재매핑)

## HISTORY

| 버전  | 날짜       | 변경 내용                                                                                         |
|-------|------------|--------------------------------------------------------------------------------------------------|
| 0.1.0 | 2026-08-02 | 초안 작성 (Draft) — `modbus-remap` 노드 명세. 사용자 승인 확정 설계 반영: (1) 변환축 = 주소+area+device_id 전부, (2) 규칙 대상 단위 = read-op 엔트리 전체(정확 매칭), (3) 템플릿 = 노드 config 내부 저장(command_set 패턴). M1~M5 EARS 5모듈. |
| 0.2.0 | 2026-08-02 | 구현 중 개선 반영: (1) From 측 `source_unit_id`(선택) 추가 — 입력 엔트리에 `unit_id`가 있으면(remap→remap 체이닝) 매칭에 포함, 없으면 unit_id 제외하고 area+address+count로 매칭, (2) 다중 To(`targets[]`) — 하나의 source 규칙이 여러 대상으로 팬아웃(대상마다 출력 엔트리 1개). 레거시 단일 `target_*`는 하위 호환(1-원소 targets로 정규화). M2 갱신. |
| 0.3.0 | 2026-08-02 | UX·템플릿 재설계: (1) 규칙 = 컴팩트 리스트+등록/수정 팝업(영역 C/D/H/I 약어) + 일괄등록, (2) 템플릿 = 이름 있는 **다중 규칙 패턴**(상대 오프셋), 노드 config 별도 `templates` 섹션 저장, (3) 적용 = 시작 주소+device_id 지정 → 프론트가 구체 규칙들을 `rules`에 즉시 생성(materialize), (4) 백엔드는 rules-only 런타임(templates inert, 런타임 전개 제거 — 이중 적용 방지). M4 재작성. 원 SPEC AC-02(백엔드 템플릿 인스턴스화)는 의도적 폐기. |

---

## 1. 개요 (Overview)

xflow는 방금 MODBUS 노드 패밀리(`modbus-write` / `modbus-read` / `modbus-control`, SPEC-MODBUS-006)를 추가했다. 본 SPEC은 이 패밀리에 신규 flow 노드 **`modbus-remap`**(카테고리 `modbus`)을 추가한다.

`modbus-remap`은 `modbus-read` 노드가 방출하는 positional `values[]` 페이로드를 소비하여, 각 read-op 엔트리를 **remap 규칙**에 따라 **재주소화(re-address)**한다. 하나의 remap 규칙은 다음 변환을 수행한다.

- **From(소스) 매칭**: `(area, address, count)` — read-op 엔트리 전체를 정확 매칭한다.
- **To(타깃) 재작성**: `(target device_id/unit_id, target area, target address)` — 타깃 주소는 오프셋 기반(`targetAddr = targetStart + (srcAddr − srcStart)`)으로 산출한다.

출력은 downstream `modbus-write`가 소비 가능한 스키마(`{success, values[]}`, 엔트리별 `unit_id`/`area`/`address` 포함)를 유지한다.

**템플릿**은 `(area, address offset)`만 정의한 부분 규칙으로, 노드 config 내부에 op-list(`command_set` 패턴)로 저장한다. 적용 시 `(device_id, start address, count)`를 주입하여 구체 remap 규칙으로 인스턴스화한다. 별도 스토리지/API·재사용 named 템플릿은 도입하지 않는다.

Web UI는 좌(From)/우(To) 2-테이블 From→To 편집기로 규칙을 편집한다.

본 SPEC은 문서만 산출하며 구현 코드를 포함하지 않는다.

### 1.1 사용자 확정 설계 결정 (Locked)

1. **변환 축 = 주소 + area + device_id 전부**: remap 규칙의 To측은 타깃 `area`(예: `holding_registers`→`input_registers`) **및** `device_id`/`unit_id` **및** 주소(오프셋)를 모두 재지정할 수 있다. (M2에서 확정 요구사항, Optional 아님.)
2. **규칙 대상 단위 = read-op 엔트리 전체**: 규칙 소스는 `modbus-read` `values[]` 엔트리 하나 전체를 `(area, address, count)` **정확 매칭**한다. 엔트리 내부 서브-범위 슬라이싱은 없다. 완전-포함-아니면-거부 규칙은 "정확 area+address+count 일치 엔트리가 없으면 거부"로 단순화된다. 오프셋 변환 수학은 유지되며, 엔트리 전체 remap에서는 엔트리 base 주소가 타깃 start로 매핑되고 count는 보존된다.
3. **템플릿 저장 = 노드 config 내부에만**: 템플릿은 노드 자체 config에 op-list(`command_set` 패턴)로 저장한다. 별도 스토리지·API·전역 named 템플릿 없음.

---

## 2. 환경 (Environment)

| 항목                | 현재 상태 / 제약                                                                                     |
|---------------------|------------------------------------------------------------------------------------------------------|
| 백엔드 언어         | Go 1.23+                                                                                              |
| 노드 등록           | `internal/node/registry.go` `registerBuiltins()` — 1행 `{typeName, factory, category, description}`  |
| 노드 인터페이스     | `Process(ctx, msg message.Message) ([]message.Message, error)`; `return (nil, err)` → error port     |
| 노드 라이프사이클   | `Init` / `Configure` / `Process` / `Shutdown`, `*BaseNode` 임베드                                    |
| 모델 노드           | `internal/node/mapping.go`(최소 노드), `modbus_read.go` / `modbus_common.go`(패밀리)                 |
| 영역 상수           | `modbus_common.go`: `coils` / `discrete_inputs` / `holding_registers` / `input_registers`, `validRegisterAreas`, `areaToFunctionCode`(FC 1-4) |
| unit_id 규약        | `unit_id: 0` = 서버 공유 컨테이너                                                                    |
| 변환 수학 선례      | `internal/agent/modbusserver/device_view.go` `resolveSegment`: `targetAddr = mapStart + (srcAddr − srcStart)` + 완전-포함-아니면-거부(`ErrAddressNotMapped`) — 의미만 참조(라이브 RegisterMap 대상, 직접 재사용 불가) |
| 페이로드 오버라이드 | config 기본 op-list를 RLock으로 읽고 payload 키 존재 시 대체(`modbus_read.go:146-152` `command_set` 패턴) |
| 프론트엔드 스키마   | `web/src/config/nodeSchemas.ts` — 1 키 엔트리(`modbus-read` ~1703-1731)                              |
| 프론트엔드 필드     | `web/src/components/property/FormField.tsx` — `field.type` 분기(~327-449)                            |
| 편집기 모델         | `ModbusCommandSetEditor.tsx`(1행=1op 테이블), `KeyValueMapEditor`(2-컬럼), `MODBUS_DATA_TYPE_OPTIONS` 재사용, `{value, onChange, readOnly}` + `toRows`/`toEmit` 규약 |

### 2.1 입력 계약 (Grounding — `modbus-read` 출력)

`modbus-read` 노드는 다음 페이로드를 방출한다.

```
{
  success:    bool,
  agent_type: "server" | "client",
  values: [
    { index, area, address, count, values[] | raw, data_type? }
  ]
}
```

- `values[]`는 read-op별 positional 배열이며, op 레벨 `(area, address, count)`로 주소화된다(평탄한 address→value 맵이 아님).
- **주의(핵심)**: 출력 엔트리에는 `unit_id`가 **없다**(unit_id는 read-op **입력** 레벨에만 존재). 따라서:
  - **From 매칭**은 `(area, address, count)` 기준으로만 수행한다.
  - **device_id**는 remap 규칙의 **To측 할당값**이다(입력에서 추출하는 값이 아님).
- 출력 호환: `modbus-remap`의 출력은 downstream `modbus-write`의 `command_set` op 소비 스키마(`{area, address, count, data_type?, unit_id, values?}`)와 호환되어야 한다.

---

## 3. 가정 (Assumptions)

- (A1) 상류에 `modbus-read` 노드가 존재하며 위 §2.1 스키마를 방출한다. `modbus-remap`은 이 스키마를 신뢰 입력으로 취급한다.
- (A2) 하나의 remap 규칙은 정확히 하나의 read-op 엔트리에 대응한다(엔트리 전체 단위, §1.1-2).
- (A3) 템플릿/규칙 op-list는 config 기본값으로 주어지며, 런타임에 payload 키로 오버라이드 가능하다(§2.1 command_set 패턴).
- (A4) `data_type` / boolean area(coils/discrete_inputs) 의 값 표현은 입력 엔트리의 표현을 그대로 보존하며 remap이 값 자체를 변형하지 않는다(주소/영역/디바이스만 재작성).

---

## 4. 요구사항 (Requirements — EARS)

요구사항 모듈은 5개(M1~M5)로 제한한다.

### M1 — 입력 소비 (modbus-read 출력 계약)

- **Ubiquitous**: 시스템은 항상 `modbus-read` 출력 스키마(`{success, values[], agent_type}`)를 입력으로 수용해야 한다.
- **Event-driven**: WHEN 메시지가 도착 THEN `values[]`의 각 엔트리를 `(area, address, count, values[]|raw, data_type?)`로 파싱한다.
- **State-driven**: IF 입력 `success == false` 이면 THEN 시스템은 해당 실패 상태를 출력에 승계하여 표기해야 한다(성공으로 왜곡 금지).
- **Unwanted**: 시스템은 `modbus-read` 스키마가 아닌 페이로드에 대해 추측성 매핑을 수행하지 않아야 하며, error port(`return (nil, err)`)로 라우팅해야 한다.

### M2 — 규칙별 remap (주소 + area + device_id 재작성)

- **Ubiquitous**: 각 remap 규칙은 `(source area, source address, count[, source_unit_id])` → `targets[]`(1..N개의 `{target device_id/unit_id, target area?, target address}`)로 정의된다.
- **Event-driven**: WHEN 규칙이 매칭 엔트리에 적용 THEN `targets`의 각 대상마다 출력 엔트리 1개를 팬아웃하며, 각 타깃 엔트리는 `unit_id = target device_id`, `area = target area`, `address = target address + (srcAddr − source address)`로 재작성되고 `count`와 값 배열(positional)은 보존된다.
- **State-driven (unit_id 선택 매칭)**: IF 규칙이 `source_unit_id`를 지정하고 입력 엔트리에 `unit_id` 필드가 존재하면(상류가 또 다른 `modbus-remap`인 체이닝 경우 — 본 노드 출력 엔트리는 unit_id를 포함) THEN 엔트리 `unit_id`가 `source_unit_id`와 같아야 매칭한다. IF 입력 엔트리에 `unit_id`가 없으면(일반 `modbus-read` 경우) THEN unit_id를 제외하고 `(area, address, count)`로만 매칭한다. IF `source_unit_id`가 생략되면 unit_id로 제약하지 않는다.
- **State-driven (area 재작성)**: IF 타깃이 `target_area`를 소스와 다르게 지정하면(예: `holding_registers`→`input_registers`) THEN area 변경을 적용해야 하며, 생략 시 source area를 유지한다.
- **Optional (하위 호환)**: 가능하면 `targets` 없이 최상위 `target_unit_id`/`target_area`/`target_address`만 있는 레거시 단일-타깃 규칙을 1-원소 `targets`로 정규화해 수용한다.
- **Unwanted**: 시스템은 remap 과정에서 입력 값(values/raw) 자체를 변형하지 않아야 한다(주소·영역·디바이스만 재작성).

### M3 — 완전-포함-아니면-거부 (엔트리 전체 정확 매칭)

- **Ubiquitous**: 규칙 소스는 read-op 엔트리 하나 전체를 `(area, address, count)` **정확 매칭**한다(엔트리 내부 서브-범위 없음).
- **State-driven**: IF 규칙 소스 `(area, address, count)`와 정확히 일치하는 엔트리가 존재하면 THEN 해당 엔트리에만 remap을 적용한다.
- **Unwanted**: 시스템은 어떤 엔트리와도 정확 일치하지 않는 규칙에 대해 부분 매핑을 수행하지 않아야 하며, 해당 규칙을 거부해야 한다(`ErrAddressNotMapped` 의미 준거).
- **Event-driven**: WHEN 미매칭/경계 위반 감지 THEN 규칙별 명확한 error를 생성하고 전체 `success = false`로 표기하며 결과를 error port로 라우팅한다.

### M4 — 템플릿(다중 규칙 패턴) 정의/적용 + config-기본/payload-오버라이드

- **Ubiquitous**: 템플릿은 **이름 있는 다중 규칙 패턴**이며(각 패턴 규칙은 base 대비 상대 오프셋으로 `source_area`·`source_offset`·`count`·`targets[{target_area?, target_offset, target_unit_offset?}]`를 정의), 노드 config 내부 **별도 `templates` 섹션**에 저장된다.
- **Event-driven (적용=materialize)**: WHEN 사용자가 템플릿에 `(start address, device_id)`를 적용 THEN 프론트 에디터가 패턴의 각 규칙을 구체 규칙으로 전개해(`source_address = start + source_offset`, 각 타깃 `target_address = start + target_offset`, `target_unit_id = device_id + target_unit_offset`) `rules` 배열에 **즉시 생성(materialize)**하며, 이후 개별 규칙으로 편집 가능하다. '시작 주소 지정만으로 여러 규칙 일괄 등록'을 만족한다.
- **State-driven (백엔드 rules-only)**: IF 노드가 실행되면 THEN 백엔드는 `rules`만 런타임 처리하고 `templates`는 전개하지 않는다(inert) — 에디터 materialize와 백엔드 전개의 이중 적용을 방지한다.
- **State-driven (오버라이드)**: IF 페이로드에 `rules` 오버라이드가 존재하면 THEN config 기본값 대신 페이로드 값을 사용한다(RLock으로 config 기본 읽기 → payload 존재 시 대체). 템플릿은 payload 오버라이드 대상이 아니다.
- **Unwanted**: 시스템은 재사용 named/전역 템플릿 스토리지나 별도 API를 도입하지 않아야 한다(노드 config 내부 `templates` 섹션 저장으로 한정).

### M5 — 결과 스키마 + From→To 2-테이블 Web UI

- **Ubiquitous**: 출력은 재주소화된 `{success, values[]}` 형태로, downstream `modbus-write` `command_set` 소비 스키마와 호환되어야 한다(엔트리별 `unit_id`/`area`/`address` 포함).
- **State-driven**: IF 하나 이상의 규칙이 실패하면 THEN `success = false`와 규칙별 error를 표기해야 한다.
- **Unwanted**: 시스템은 성공/실패를 혼동해선 안 되며, 일부 실패를 성공으로 보고하지 않아야 한다.
- **Optional**: 가능하면 Web UI는 좌(From)/우(To) 2-테이블 From→To 형식으로 규칙을 편집 가능하게 제공한다.

---

## 5. 명세 (Specifications)

### 5.1 노드 타입

- 타입 키: `modbus-remap`, 카테고리 `modbus`.
- 등록: `registry.go` `registerBuiltins()`에 `{"modbus-remap", NewModbusRemapNode, "modbus", "레지스터 주소/영역/디바이스 재매핑"}` 1행.

### 5.2 config 스키마 (제안)

```
rules:     []map[string]any   // 각 규칙: {source_unit_id?, source_area, source_address, count, targets:[{target_unit_id, target_area?, target_address}]}
templates: []map[string]any   // 각 템플릿(다중 규칙 패턴): {name, rules:[{source_unit_id?, source_area, source_offset, count, targets:[{target_area?, target_offset, target_unit_offset?}]}]}
```

- 규칙 필드: `source_unit_id`(선택, unit-id 선택 매칭), `source_area`, `source_address`, `count`, `targets`(1..N개 `{target_unit_id(device_id), target_area?(생략 시 source_area 유지), target_address(타깃 start)}`). 레거시 최상위 `target_*`는 1-원소 targets로 정규화(하위 호환).
- 템플릿 필드: `name` + `rules[]`(상대 오프셋 패턴). 각 패턴 규칙: `source_offset`/`target_offset`(base 대비 상대), `target_unit_offset`(device_id 대비 상대). **적용(materialize)**: `(start, device_id)` 지정 → `source_address = start + source_offset`, `target_address = start + target_offset`, `target_unit_id = device_id + target_unit_offset`로 전개해 `rules`에 추가. **백엔드는 templates를 런타임 처리하지 않음(inert)** — 에디터 전용.
- 템플릿 필드: `area`, `offset`. 적용 파라미터: `(device_id, start_address, count)`.
- 페이로드 오버라이드: payload에 `rules` 또는 `templates` 키 존재 시 config 기본값 대체.

### 5.3 출력 스키마

```
{
  success: bool,
  values: [
    { index, area(=target_area), address(=target_address), count, values[]|raw, data_type?, unit_id(=target_device_id), error? }
  ]
}
```

### 5.4 제약

- 오프셋 변환: `targetAddr = targetStart + (srcAddr − srcStart)`; 엔트리 전체 remap에서 `srcAddr == entry.address`(base)이므로 `targetAddr == target_address`, `count` 보존.
- 매칭: 엔트리 전체 정확 매칭(area+address+count); 미매칭 규칙 거부.
- 값 보존: positional 값 배열/raw는 그대로 전달, 변형 금지.

---

## 6. 추적성 (Traceability)

| 요구사항 모듈 | 관련 파일(예정)                                                                 | 인수 기준 |
|---------------|---------------------------------------------------------------------------------|-----------|
| M1            | `internal/node/modbus_remap.go`(Process 입력 파싱)                              | AC-01, AC-05 |
| M2            | `internal/node/modbus_remap.go`(remap 로직) + `modbus_common.go`(area 상수)     | AC-01, AC-04 |
| M3            | `internal/node/modbus_remap.go`(정확 매칭 + 거부)                               | AC-03 |
| M4            | `internal/node/modbus_remap.go`(config/템플릿 파싱 + payload 오버라이드)        | AC-02, AC-06 |
| M5            | `nodeSchemas.ts` + `FormField.tsx` + `RegisterRemapEditor.tsx`                   | AC-07, AC-08 |

---

## 7. 범위 밖 (Non-Goals)

- 재사용 named/전역 템플릿 스토리지 및 관련 API.
- read-op 엔트리 내부 서브-범위 슬라이싱.
- remap 시 값(values/raw) 자체 변환(스케일링·타입 변환 등).
- 라이브 RegisterMap 직접 조작(입력은 message payload에 한정).
