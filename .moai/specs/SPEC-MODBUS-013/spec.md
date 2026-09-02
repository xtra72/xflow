---
id: SPEC-MODBUS-013
title: "MODBUS Client 디바이스 모델 카탈로그 · 레지스터 enable/disable · 블록 병합 읽기"
version: "0.2.0"
status: implemented
created: 2026-08-27
updated: 2026-08-27
author: xtra
priority: P2
phase: "v0.37.0 target"
module: "internal/agent/modbus, web/src/components/property, web/src/pages/agents"
lifecycle: spec-anchored
tags: "modbus, client, device-model, catalog, register-enable, block-coalescing, poll-optimization, bulk-register"
tier: L
---

# SPEC-MODBUS-013: MODBUS Client 디바이스 모델 카탈로그 · 레지스터 enable/disable · 블록 병합 읽기

## HISTORY

| 버전  | 날짜       | 변경 내용 |
| ----- | ---------- | --------- |
| 0.1.0 | 2026-08-27 | 최초 작성. 4개 기능(모델 카탈로그·레지스터 enable/disable·블록 병합 읽기·블록 상한 설정)을 단일 SPEC 으로 묶어 명세. |
| 0.2.0 | 2026-08-27 | 구현 완료(M1~M5). 구현 중 확정된 사항 반영: (a) AC-19 를 "기본 설치 상태"에서 "저장소 동봉 파일 검증"으로 정정(자동 시딩은 미구현 — 설치 시 수동 복사), (b) `list_models` 를 `/agents/{id}/query` 읽기 전용 화이트리스트에 추가(서버·프론트 양쪽 pin 테스트 갱신), (c) 코일/이산입력(fc1·fc2)도 비트 재포장으로 병합 지원, (d) 블록 스케줄러 종료 키를 디바이스 스코프로 전환. |

---

## 배경 (Environment)

GIPAM-115FI 같은 디지털 계전기는 Address Map 이 70여 개 레지스터로 구성되고,
레지스터마다 의미가 달라 개별 설명이 필요하다. 현재 구조에서 이를 등록하면 다음 문제가 발생한다.

**문제 1 — 모델별 재입력.** 동일 기종을 여러 대 등록할 때마다 70여 줄을 매번 붙여넣어야 한다.
`internal/agent/modbus/`·`web/src/components/property/` 어디에도 model/template/profile 개념이 없고,
기존 MODBUS SPEC 12건(001~012, CLI-001)에도 카탈로그성 기능이 없다.

**문제 2 — 부분 사용 불가.** 현장에서 실제로 쓰는 레지스터는 전체의 일부인데,
[RegisterGroupConfig](../../internal/agent/modbus/config.go#L68)에 사용 여부 필드가 없어
안 쓰는 레지스터를 빼려면 행 자체를 삭제해야 한다. 나중에 되살리려면 다시 입력해야 한다.

**문제 3 — 읽기 횟수 폭증.** 레지스터별 설명을 붙이려면 그룹을 레지스터 단위로 쪼개야 하는데,
[pollGroupRead](../../internal/agent/modbus/agent.go#L721)가 그룹마다
[buildReadPDU](../../internal/agent/modbus/device.go#L120)를 1회 호출하므로 **그룹 수 = 트랜잭션 수**가 된다.
GIPAM-115FI 를 레지스터별로 쪼개면 72 트랜잭션이 되고, RS485 9600bps 기준 한 바퀴에 2초 이상이 걸린다.

**문제 4 — 블록 크기 상한 부재.** [protocol.go:67](../../internal/agent/modbus/protocol.go#L67)에
Modbus 규격 상한 `MaxRegistersRead = 125` 만 있고 정책 상한이 없다. GIPAM-115FI 는 자체 상한이
56 이라(PDF p.4 `MAX register read count : 56`) 기종별 상한 지정이 필요하다.

## 가정 (Assumptions)

| # | 가정 | 신뢰도 | 근거 | 틀렸을 때 영향 |
| - | ---- | ------ | ---- | -------------- |
| A1 | 캐시 갱신·타입 오버레이가 주소 기반이므로, 여러 그룹을 한 번에 읽고 주소로 분배해도 하위 동작이 동일하다 | 높음 | `cache.UpdateFromRead(fc, startAddr, data, qty)` ([agent.go:753](../../internal/agent/modbus/agent.go#L753)), 오버레이 키 `FC%d:%d` ([agent.go:1329](../../internal/agent/modbus/agent.go#L1329)) | 병합 후 값 분배 재설계 |
| A2 | 병합은 **읽기(트랜스포트) 계층에만** 적용하고 메시지 방출은 그룹 단위로 유지하면 다운스트림 계약이 100% 보존된다 | 높음 | `sendRegisterEvent(dev, rg, data, ...)` 가 그룹명·quantity 를 그대로 사용 | 메시지 형상 변경 → 노드·대시보드 회귀 |
| A3 | `Enabled *bool` 포인터 필드(nil=사용)로 하위 호환을 확보할 수 있다 | 높음 | 동일 패턴 선례: `DeviceConfig.ShareSession *bool` ([config.go](../../internal/agent/modbus/config.go)) | 값 타입 사용 시 구조체 리터럴의 zero value 가 "미사용"이 되어 무음 회귀 |
| A4 | 일괄등록 열을 **뒤에 추가**(5·6열 → 5·6·7열)하면 기존 붙여넣기 텍스트가 그대로 동작한다 | 높음 | 열 개수 검증이 `n !== 5 && n !== 6` 단일 지점 ([modbusDevicesModel.ts:396](../../web/src/components/property/modbusDevicesModel.ts#L396)) | 기존 사용자 텍스트 전량 무효화 |
| A5 | 모델 카탈로그를 `~/.xflow/models/*.json` 파일 스캔으로 제공하면 재빌드 없이 기종을 늘릴 수 있다 | 중간 | 런타임 디렉터리 `~/.xflow/` 기존 사용(config.yaml, xflow.yaml, data/) | 배포 경로 정책 재검토 |
| A6 | 모델 적용은 프론트에서 편집기 행을 채우는 방식으로 충분하다(서버 `apply_model` 불필요) | 중간 | 기존 저장 경로 `add_device`/`update_device` 가 이미 전체 그룹 배열을 받음 | 서버측 적용 명령 신설 필요 |

## 요구사항 (Requirements, EARS)

### REQ-MODBUS-013-01 — 레지스터 그룹 사용 여부 (Ubiquitous)

시스템은 레지스터 그룹마다 사용 여부(`enabled`)를 보유해야 하며, **기본값은 사용(true)** 이어야 한다.

- **WHERE** 그룹의 `enabled` 가 명시되지 않으면 **THEN** 시스템은 사용으로 간주해야 한다(하위 호환).
- **WHILE** 그룹이 미사용이면 **THEN** 시스템은 해당 그룹을 읽기 계획에서 제외하고, 폴링·캐시 갱신·메시지 방출을 수행하지 않아야 한다.
- 시스템은 미사용 그룹의 정의(주소·개수·타입·설명)를 보존하여 재활성화 시 재입력이 불필요하도록 해야 한다.
- **WHEN** 디바이스의 모든 그룹이 미사용이면 **THEN** 시스템은 해당 디바이스에 대해 어떤 읽기도 수행하지 않아야 하며 오류로 처리하지 않아야 한다.

### REQ-MODBUS-013-02 — 블록 병합 읽기 (State-Driven)

**WHILE** 디바이스가 폴링 중이면 **THEN** 시스템은 사용 중인 그룹들을 물리 읽기 횟수가 최소가 되도록 병합해 읽어야 한다.

- 시스템은 **같은 디바이스 · 같은 폴링 주기 · 같은 function code · 주소 연속**인 그룹만 하나의 블록으로 병합해야 한다.
- 시스템은 주소가 연속하지 않으면(간극 ≥ 1워드) 병합하지 않아야 한다 — 미정의 주소 읽기로 인한 `ILLEGAL DATA ADDRESS` 예외를 회피한다.
- **WHERE** 병합 결과가 블록 상한을 초과하면 **THEN** 시스템은 상한 이하가 되도록 블록을 분할해야 한다.
- 시스템은 블록 읽기 결과를 원래 그룹 경계로 분배하여, **그룹 단위 캐시 갱신·변경 감지·메시지 방출을 병합 이전과 동일하게** 수행해야 한다.
- 시스템은 블록 읽기 1회 실패 시 해당 블록에 속한 모든 그룹을 실패로 처리해야 한다.

### REQ-MODBUS-013-03 — 블록 최대 레지스터 수 설정 (Ubiquitous)

시스템은 블록당 최대 레지스터 수를 설정 가능해야 하며, **기본값은 32** 여야 한다.

- 시스템은 에이전트 레벨 `max_block_registers` 와 디바이스 레벨 오버라이드를 지원해야 한다. 디바이스 값이 없으면 에이전트 값을 상속해야 한다.
- 시스템은 설정값을 `[1, 125]` 로 제한해야 한다(`MaxRegistersRead` 규격 상한).
- **WHEN** 단일 그룹의 `quantity` 가 블록 상한보다 크면 **THEN** 시스템은 그 그룹을 분할하지 않고 그대로 읽어야 한다(기존 동작 보존, 규격 상한 125 만 적용).

### REQ-MODBUS-013-04 — 디바이스 모델 카탈로그 (Event-Driven)

**WHEN** 에이전트가 기동하거나 모델 목록이 요청되면 **THEN** 시스템은 `~/.xflow/models/*.json` 을 스캔해 유효한 모델을 카탈로그로 제공해야 한다.

- 시스템은 파일 단위로 검증하고, 무효 파일은 경고 로그 후 **건너뛰어야** 한다(fail-open — 파일 하나가 전체 카탈로그를 막지 않는다).
- 시스템은 `list_models` exec 명령으로 카탈로그를 반환해야 한다.
- 시스템은 모델 디렉터리가 없으면 빈 카탈로그를 반환하고 오류로 처리하지 않아야 한다.
- 시스템은 GIPAM-115FI 모델을 저장소에 기본 샘플로 동봉해야 한다(`assets/models/gipam-115fi.json`).
  설치 시 이 파일을 모델 디렉터리로 복사한다 — 자동 시딩은 본 SPEC 범위 밖이다.

### REQ-MODBUS-013-05 — 모델 선택 UI (Event-Driven)

**WHEN** 사용자가 디바이스 편집 화면에서 모델을 선택하면 **THEN** 시스템은 해당 모델의 레지스터 정의로 편집기 행을 채워야 한다.

- 시스템은 채우기 전 기존 그룹이 존재하면 **덮어쓰기 여부를 확인**해야 한다.
- 시스템은 채운 결과를 사용자가 검토·수정한 뒤 저장하도록 해야 한다(즉시 저장 금지).
- 시스템은 모델이 지정한 `max_block_registers`·`poll_interval` 기본값을 함께 적용해야 한다.

### REQ-MODBUS-013-06 — 일괄등록 포맷 확장 (Event-Driven)

**WHEN** 사용자가 일괄등록 텍스트를 제출하면 **THEN** 시스템은 7번째 열 `사용` 을 파싱해 그룹의 `enabled` 로 반영해야 한다.

- 시스템은 5열·6열·7열을 모두 허용해야 한다(기존 텍스트 무변경 동작).
- 시스템은 `사용` 열이 비었거나 생략되면 사용(true)으로 간주해야 한다.
- 시스템은 `1/0`, `true/false`, `y/n`, `on/off`(대소문자 무시)를 허용하고, 그 외 값은 행 실패 `invalidEnabled` 로 집계해야 한다.

### REQ-MODBUS-013-07 — 품질·하위 호환 (Ubiquitous, cross-cutting)

- 시스템은 `enabled`·`max_block_registers`·모델을 사용하지 않는 기존 설정에 대해 **병합 이전과 동일한 읽기 시퀀스·메시지**를 산출해야 한다(단, 연속 그룹은 병합되며 이는 메시지 형상에 영향을 주지 않는다).
- 시스템은 기존 MODBUS 테스트 스위트를 전량 통과해야 한다.
- 시스템은 신규 코드에 대해 85% 이상 커버리지를 확보해야 한다.

## 명세 (Specifications)

### 설정 스키마 확장

```yaml
# 에이전트 레벨
max_block_registers: 32          # 신규, 기본 32, 범위 1-125

devices:
  - id: gipam-1
    unit_id: 1
    max_block_registers: 56      # 신규, 선택. 없으면 에이전트 값 상속
    register_groups:
      - name: "상전압 R상"
        function_code: 4
        start_address: 4
        quantity: 2
        data_type: float32
        enabled: true            # 신규, 선택. 없으면 true
```

### 읽기 계획 (Read Plan)

```
buildReadPlan(groups, maxBlock) -> []ReadBlock

1. groups 에서 enabled == false 인 항목 제외
2. (pollInterval, functionCode) 로 버킷 분할
3. 버킷 내 startAddress 오름차순 정렬
4. 인접 그룹 병합 조건 (모두 만족):
     next.startAddress == cur.endAddress + 1     // 간극 0, 엄격 연속
     (next.end - cur.start + 1) <= maxBlock       // 상한 이내
5. 병합 불가 시 새 블록 시작
6. 단일 그룹이 maxBlock 을 넘으면 단독 블록으로 통과 (분할하지 않음)

ReadBlock { FunctionCode, StartAddress, Quantity, Members []RegisterGroupConfig }
```

폴링 경로 대응:

| 코호트 | 기존 | 변경 후 |
| ------ | ---- | ------- |
| `PollInterval == 0` | [pollDevice](../../internal/agent/modbus/agent.go#L702) 가 그룹마다 `pollGroupRead` | 기본 코호트의 블록마다 `pollBlockRead` |
| `PollInterval > 0` | [startGroupLoop](../../internal/agent/modbus/agent.go#L774) 가 그룹마다 goroutine | 같은 주기 블록마다 goroutine |

### 블록 결과 분배

```
data := dev.ReadRegisters(ctx, blockAsGroup)     // 물리 읽기 1회
for _, m := range block.Members {
    off := (m.StartAddress - block.StartAddress) * 2     // 바이트 오프셋
    slice := data[off : off+int(m.Quantity)*2]
    // 이하 기존 pollGroupRead 본문과 동일
    cache.UpdateFromRead(m.FunctionCode, m.StartAddress, slice, m.Quantity)
    a.sendRegisterEvent(dev, m, slice, "interval")
}
```

통계는 멤버 그룹마다 `recordRequestStat` 를 호출해 **기존 그룹별 통계 의미를 보존**하고,
물리 트랜잭션 수는 에이전트 레벨 카운터로 별도 집계한다.

### 모델 JSON 스키마 (`~/.xflow/models/*.json`)

```json
{
  "id": "gipam-115fi",
  "name": "GIPAM-115FI",
  "vendor": "LS ELECTRIC",
  "description": "디지털 복합 계전기 (for GMPC-V)",
  "transport": "rtu",
  "defaults": {
    "max_block_registers": 56,
    "poll_interval": "2s"
  },
  "registers": [
    { "fc": 4, "address": 0,  "count": 1, "data_type": "uint16",  "name": "DI 상태 비트맵 F090" },
    { "fc": 4, "address": 4,  "count": 2, "data_type": "float32", "name": "상전압 R상 [V]" },
    { "fc": 3, "address": 1002, "count": 2, "data_type": "uint32", "poll_interval": "300s", "name": "차단기 통전 시간 [Hour]" }
  ]
}
```

- `id` 는 파일 간 고유. 중복 시 먼저 로드된 파일이 이기고 나머지는 경고 로그 후 스킵.
- `registers[].enabled` 는 선택(기본 true).
- `registers[].poll_interval` 이 없으면 `defaults.poll_interval` 상속.

### 일괄등록 포맷 (v0.3.0)

```
fc,주소,개수,데이터타입,폴링간격,설명,사용
```

| 열 수 | 해석 |
| ----- | ---- |
| 5 | `fc,주소,개수,데이터타입,폴링간격` — 설명 없음, 사용 |
| 6 | `+ 설명` — 사용 (기존 동작) |
| 7 | `+ 사용` |

예시:

```
4,4,2,float32,2s,상전압 R상 [V],1
4,32,2,float32,2s,총 역률,0
```

## Traceability

| 요구사항 | 주요 변경 지점 |
| -------- | -------------- |
| REQ-01 | `config.go` RegisterGroupConfig.Enabled, parseRegisterGroupConfig |
| REQ-02 | `readplan.go` (신규), `agent.go` pollDevice·groupPollLoop·pollBlockRead |
| REQ-03 | `config.go` ModbusConfig.MaxBlockRegisters, DeviceConfig.MaxBlockRegisters |
| REQ-04 | `models.go` (신규), `agent.go` exec `list_models` |
| REQ-05 | `ModbusDevicesEditor.tsx`, `agentService.ts` |
| REQ-06 | `modbusDevicesModel.ts` parseBulkGroups |
| REQ-07 | 기존 테스트 스위트 + 신규 단위 테스트 |

## Non-Goals

- 간극 허용 병합(`max_block_gap`) — 미정의 주소 예외 위험이 있어 별도 SPEC 으로 분리.
- 모델 UI CRUD(생성·편집·삭제) — 본 SPEC 은 읽기 전용 카탈로그.
- 서버측 `apply_model` exec — 프론트 적용으로 충분(A6).
- Gateway(modbusserver) 에이전트 — 본 SPEC 은 modbus-client 한정.
- 쓰기(write) 경로의 블록 병합.
- 모델 파일 자동 시딩(설치 시 `assets/models/*.json` → `~/.xflow/models/` 복사) — 배포 스크립트 영역.
