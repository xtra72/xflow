# SPEC-MODBUS-013 구현 계획 (plan.md)

## 기술 접근

4개 요구사항을 **의존 순서**로 쌓는다: `Enabled` 필드가 읽기 계획의 입력이 되고,
읽기 계획이 블록 상한을 소비하며, 모델 카탈로그가 이 둘을 채우는 데이터 소스가 된다.

핵심 설계 원칙 두 가지:

1. **병합은 트랜스포트 계층에만.** `ReadBlock` 은 물리 읽기 단위일 뿐이고,
   캐시 갱신·변경 감지·메시지 방출은 기존과 동일하게 **그룹 단위**로 남는다.
   따라서 노드·대시보드·플로우 등 다운스트림 계약이 전혀 바뀌지 않는다.
2. **읽기 계획은 순수 함수.** `buildReadPlan(groups, maxBlock) []ReadBlock` 을
   부수효과 없는 함수로 분리해 단위 테스트로 전량 검증한다. 폴링 루프는 계획을 소비만 한다.

## 재사용 지도 (file:line)

| 대상 | 위치 | 재사용 방식 |
| ---- | ---- | ----------- |
| 그룹 설정 구조체 | `internal/agent/modbus/config.go:68` | `Enabled *bool` 필드 추가 |
| 포인터 옵셔널 선례 | `internal/agent/modbus/config.go` `DeviceConfig.ShareSession *bool` | 동일 패턴 그대로 |
| 그룹 파서 | `internal/agent/modbus/config.go:415` `parseRegisterGroupConfig` | `enabled` 키 파싱 추가 |
| 디바이스 파서 | `internal/agent/modbus/config.go:327` `parseDeviceConfig` | `max_block_registers` 추가 |
| 기본 폴링 루프 | `internal/agent/modbus/agent.go:702` `pollDevice` | 그룹 순회 → 블록 순회 |
| 그룹 읽기 본문 | `internal/agent/modbus/agent.go:721` `pollGroupRead` | 멤버 분배 루프로 재구성 |
| 그룹 전용 루프 | `internal/agent/modbus/agent.go:774` `startGroupLoop` / `:795` `groupPollLoop` | 키를 그룹명 → 블록키로 |
| 물리 읽기 | `internal/agent/modbus/device.go:120` `ReadRegisters` | 블록을 합성 그룹으로 넘겨 그대로 사용 |
| 규격 상한 | `internal/agent/modbus/protocol.go:67` `MaxRegistersRead=125` | 상한 클램프 기준 |
| 캐시 갱신 | `internal/agent/modbus/agent.go:753` `cache.UpdateFromRead` | 주소 기반이라 무변경 |
| 타입 오버레이 | `internal/agent/modbus/agent.go:1329` `FC%d:%d` 키 | 무변경 |
| 일괄등록 파서 | `web/src/components/property/modbusDevicesModel.ts:377` `parseBulkGroups` | 7열 분기 추가 |
| 열 분리 | `web/src/components/property/modbusDevicesModel.ts:353` `splitCells` | 무변경 |
| 데이터타입 목록 | `web/src/config/agentSchemas.ts:193` | 모델 검증에 재사용 |
| 디바이스 편집기 | `web/src/components/property/ModbusDevicesEditor.tsx` | 모델 선택기 + 사용 체크박스 |

## 파일 계획

| 파일 | 구분 | 내용 |
| ---- | ---- | ---- |
| `internal/agent/modbus/readplan.go` | 신규 | `ReadBlock`, `buildReadPlan`, `effectiveMaxBlock` |
| `internal/agent/modbus/readplan_test.go` | 신규 | 병합·분할·비활성 제외 단위 테스트 |
| `internal/agent/modbus/models.go` | 신규 | 모델 JSON 스키마·디렉터리 로더·검증 |
| `internal/agent/modbus/models_test.go` | 신규 | 로더 fail-open·중복 id·검증 테스트 |
| `internal/agent/modbus/config.go` | 수정 | `Enabled *bool`, `MaxBlockRegisters` 2개소 |
| `internal/agent/modbus/agent.go` | 수정 | `pollBlockRead`, 루프 2곳, `list_models` exec |
| `internal/agent/modbus/list_models.go` | 신규 | exec 핸들러 |
| `web/src/components/property/modbusDevicesModel.ts` | 수정 | 7열 파싱, `enabled` 필드 |
| `web/src/components/property/ModbusDevicesEditor.tsx` | 수정 | 사용 체크박스 + 모델 선택기 |
| `web/src/services/api/agentService.ts` | 수정 | `list_models` 호출 |
| `web/src/lib/i18n/{ko,en}.json` | 수정 | 신규 라벨·오류 문자열 |
| `assets/models/gipam-115fi.json` | 신규 | GIPAM-115FI 샘플 모델 (72 레지스터) |

## 마일스톤 (우선순위 기반, 시간 예측 없음)

### M1 (Priority High) — 레지스터 사용 여부
- `RegisterGroupConfig.Enabled *bool` + `IsEnabled()` 헬퍼
- `parseRegisterGroupConfig` 에 `enabled` 키 파싱 (누락 → nil → 사용)
- `pollDevice` 에서 비활성 그룹 스킵 (병합 전 단계에서 먼저 동작 확보)
- `list_devices` 응답에 `enabled` 반영
- 검증: 기존 MODBUS 테스트 전량 통과 + 신규 테스트

### M2 (Priority High) — 읽기 계획 + 블록 병합
- `readplan.go` 순수 함수 작성 및 단위 테스트 선행
- `ModbusConfig.MaxBlockRegisters`(기본 32) + `DeviceConfig.MaxBlockRegisters`(상속) + `[1,125]` 클램프
- `pollBlockRead` 구현 — 물리 읽기 1회 → 멤버별 슬라이스 분배
- `pollDevice` / `groupPollLoop` 를 블록 기반으로 전환, `groupStops` 키 변경
- 물리 트랜잭션 카운터 추가
- 검증: 병합 전후 메시지 형상 동일성 (특성 테스트)

### M3 (Priority Medium) — 모델 카탈로그 백엔드
- `models.go` 로더: `~/.xflow/models/*.json` 스캔, 파일 단위 fail-open
- 검증 규칙: id/name 필수, fc 1-4, address 0-65535, count ≥1, data_type 허용목록
- `list_models` exec 핸들러
- `assets/models/gipam-115fi.json` 작성 (본 SPEC 대화에서 확정한 72 레지스터)

### M4 (Priority Medium) — 프론트 통합
- 일괄등록 7열 파싱 + `invalidEnabled` 오류 sentinel + i18n
- 편집기 행에 사용 체크박스(기본 체크)
- 모델 선택기 + 덮어쓰기 확인 다이얼로그
- 검증: vitest + 기존 프론트 테스트 전량 통과

### M5 (Priority Low) — 문서·마감
- `internal/agent/modbus/README.md` 갱신
- CHANGELOG 항목
- 커버리지 85% 확인

## 아키텍처 방향

```
config (enabled, max_block_registers)
        │
        ▼
buildReadPlan()  ──순수함수──▶  []ReadBlock
        │
        ▼
pollBlockRead()  ──물리 읽기 1회──▶  data
        │
        ├─ member[0] slice ─▶ cache.UpdateFromRead ─▶ sendRegisterEvent
        ├─ member[1] slice ─▶ ...
        └─ member[N] slice ─▶ ...
                                   ▲
                          여기부터는 기존 코드와 동일
```

## 위험 및 대응

| 위험 | 영향 | 대응 |
| ---- | ---- | ---- |
| `Enabled` 를 값 타입으로 두면 구조체 리터럴 zero value 가 "미사용"이 되어 기존 그룹이 무음 정지 | 치명 | `*bool` + `IsEnabled()` (nil=true). 기존 `ShareSession *bool` 선례와 동일 |
| `groupStops` 키를 그룹명→블록키로 바꾸면 런타임 재구성(`processSetConfig`)이 루프를 못 찾아 고아 goroutine 발생 | 높음 | 키 생성 함수를 단일 지점으로 모으고, 재구성 시 전체 정지 후 재계획 |
| 병합 후 슬라이스 오프셋 계산 오류로 값이 어긋남 | 높음 | `buildReadPlan` 단위 테스트 + 분배 오프셋 특성 테스트를 M2 착수 전에 작성 |
| 모델 JSON 하나가 깨져 카탈로그 전체가 비는 회귀 | 중간 | 파일 단위 fail-open, 경고 로그, 나머지 파일 계속 로드 |
| 일괄등록 7열 추가가 기존 6열 텍스트를 깨뜨림 | 중간 | 열 개수 5/6/7 모두 허용, 6열 경로는 기존 테스트로 고정 |
| GIPAM 모델의 F090~F093 비트 정의 미확인 | 낮음 | 주소·개수·타입만 확정하고 설명 문자열은 추정 표기, 원본 대조 후 갱신 |

## Non-Goals

`spec.md` § Non-Goals 참조. 특히 간극 허용 병합(`max_block_gap`)과 모델 UI CRUD 는
본 SPEC 범위 밖이며, 필요 시 SPEC-MODBUS-014 로 분리한다.
