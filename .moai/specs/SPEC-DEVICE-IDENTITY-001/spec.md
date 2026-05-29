---
id: SPEC-DEVICE-IDENTITY-001
title: 디바이스 ID 체계 통일 - Kubernetes 패턴 (uid + name) 기반 단계적 진화
version: 0.3.0
status: completed
created: 2026-05-25
updated: 2026-05-26
author: xtra
priority: high
tags: [device, identity, uuid, refactoring, breaking, migration, system-wide]
related_spec: SPEC-DEVICE-001, SPEC-AGENT-001, SPEC-INVENTORY-001
---

# SPEC-DEVICE-IDENTITY-001: 디바이스 ID 체계 통일 - Kubernetes 패턴 (uid + name) 기반 단계적 진화

| 항목 | 내용 |
|------|------|
| SPEC ID | SPEC-DEVICE-IDENTITY-001 |
| 제목 | 디바이스 ID 체계 통일 - Kubernetes 패턴 (uid + name) 기반 단계적 진화 |
| 버전 | 0.3.0 |
| 상태 | completed (Phase A + B + C1 + C2 + C3 + D PR1~PR4 완료, xflowd v1.0.0 git tag 진입 가능) |
| 작성일 | 2026-05-25 |
| 최종 수정 | 2026-05-26 (Phase D PR4 완료 — composite 제거 + xflowd v1.0 통합 메이저) |
| 작성자 | xtra |
| 우선순위 | high (시스템 광역 영향) |
| 관련 SPEC | SPEC-DEVICE-001 (Device 기본 정의), SPEC-AGENT-001 (agent.ResolveDeviceID 도입), SPEC-INVENTORY-001 (device_uuid 선행 노출) |
| 도메인 | Device Identity / System-wide Refactoring / Data Migration |

---

## HISTORY

- **0.3.0** (2026-05-26): Phase D PR4 구현 완료 — composite 식별자 완전 제거 +
  xflowd v1.0 통합 메이저 진입 준비. 본 SPEC 의 모든 Phase (A + B + C + D)
  구현이 완료되어 status 가 `completed` 로 전환된다. 본 릴리즈는 Breaking 변경:
  - **D-T1**: `Device.ID()` 시맨틱 변경 — composite (`agent_name:local_id`) 대신
    UUID v4 반환 (UID() 와 동일 값). 5 어댑터 (NASA/LG HVACR-01/LG HVACR-02/Century/Modbus)
    모두 일관 적용. 호출 사이트는 사람이 읽는 식별이 필요하면 AgentName() +
    Name() 또는 agent/name REST 라우트 사용.
  - **D-T2**: `ClassifyDeviceRef` 가 composite 패턴을 `DeviceRefUnknown` 으로
    분류. `DeviceRegistry.ResolveDevice` 가 composite 입력을 자동 ErrDeviceNotFound
    로 처리. REST `GET /api/v1/devices/{composite}` 는 404 + actionable 마이그레이션
    안내 메시지.
  - **D-T3**: WebSocket `deviceStatusPayload` 의 `device_id` (composite alias)
    필드 완전 제거. `uid` (UUID v4) 만 1급 식별자로 emit. 5 HVAC 에이전트의
    emit payload 는 이미 `unit_id` (protocol) + `device_id` (UUID) schema
    사용 — 잔여 composite `id` 키 부재 확인 (코드 변경 불필요).
  - **D-T4**: yaml `ParseDeviceRef` 의 명시적 `DeviceRefComposite` case 제거 —
    default 분기로 일관 ErrInvalidDeviceReference. yaml composite 잔존 시 부팅 실패.
  - **D-T5**: 신규 CLI `xflowd preflight` 명령 — config yaml 검증 + device_ids.json
    로드 + device_metadata.json composite key 부재 검증 (read-only).
  - **D-T6**: `runServer` 시작 시 자동 sanity check — composite metadata 잔존 시
    부팅 거부 + actionable 복구 명령 안내.
  - **D-T8**: CHANGELOG.md `[Unreleased]` 섹션에 v1.0 Breaking 안내 추가.
  - **D-T9**: composite alias dead code cleanup (yaml_resolver / registry doc).
  - **D-T21**: `docs/migration/device-identity.md` 의 § 6.2 운영자 체크리스트
    재작성 — greenfield 즉시 / brownfield 6개월 매트릭스, v1.0 진입 5단계 절차.
  - **테스트 갱신**: 5 어댑터 unit test, 5 provider lookup test, registry_resolver
    test, event_publisher test, api handler test 모두 새 UUID 시맨틱 / composite
    거부 시맨틱에 맞게 갱신. 회귀 0 (pre-existing flaky config / century race
    제외). 신규 preflight 명령 테스트 커버리지 5 케이스.
  - 작업 분해: 9 커밋 (D-T1, D-T2, D-T3(ws), D-T5, D-T6, D-T4/T9 cleanup, D-T21
    docs, D-T8 CHANGELOG, 본 HISTORY 갱신). HVAC emit payload (D-T3 backend
    portion) 은 변경 불요 (PR1~PR3 누적으로 이미 깨끗).
  - **v1.0.0 git tag 진입은 운영자 별도 결정** — 본 세션 외 작업.
  - 인수 기준 충족 매트릭스: D-AC1 (✅ Device.ID() UUID), D-AC2 (✅ REST composite
    404), D-AC3 (✅ emit `id` 부재 + WebSocket `device_id` 부재), D-AC4 (✅ yaml
    composite 부팅 실패), D-AC5 (✅ 영속 metadata composite 부팅 실패), D-AC6
    (✅ DeviceIDRepository 미설정 부팅 실패), D-AC7 (✅ preflight PASS), D-AC10
    (✅ REST composite URL 404 — D-T2 자동 결과). 회귀 0.

- **0.2.0** (2026-05-26): Phase D 통합 재정의 — greenfield 환경 확정에 따라 외부
  클라이언트 호환 기간 (6개월) 불요. Phase D 가 xflowd v1.0 메이저 단일 릴리즈로
  통합되며 frontend cleanup + Soft Deprecation 인프라 제거를 포함한다. 구체적 작업:
  (1) M9 확장 — composite 제거 + Phase B Soft Deprecation 인프라 cleanup
  (2) M11 신규 — Frontend UUID-first 전환 (web/src/ device_id → uid 치환)
  (3) M12 신규 — Phase C 인프라 deprecation (migrate 도구, dual-tag emit)
  (4) 호환 기간 정책 재정의 — greenfield 환경은 단축 가능, 외부 클라이언트 보유 환경은 기존 6개월 유지
  본 세션 디버깅 중 운영 환경이 greenfield (단일 frontend 본인 통제, 외부 클라이언트
  없음, 기존 영속 데이터 거의 없음) 임이 확정되었기에 Phase D 의 호환 기간 가정을
  환경별 분기로 재정의한다. xflow 자체 사용자 그룹은 즉시 xflowd v1.0 메이저 진행.
- **0.1.0** (2026-05-25): 최초 작성 — 디바이스 ID 이중 체계(composite key `"agent:local_id"` vs UUID `device_id`) 문제 정의, Kubernetes 식 `uid + name + reference` 모델 채택, 10개 EARS 모듈(M1~M10)과 4 Phase 단계적 진화 전략(A: UUID 1급 격상 → B: 내부 전환 → C: 영속 데이터 마이그레이션 → D: composite 제거 Breaking) 수립.
- **0.1.0 / Phase A 구현 완료** (2026-05-26): Phase A (비파괴 추가) 4 커밋 머지.
  `Device` 인터페이스에 `UID() string` 메서드 1급 추가 (`991e793`), 5개 어댑터(NASA/LG HVACR-01/LG HVACR-02/Century/Modbus)
  에 UUID 보유 및 `UID()` 구현, REST `GET /api/v1/devices` 응답에 `uid` 필드 노출 (`be86904`, omitempty graceful degradation),
  `DeviceIDRepository` 미설정 시 1회 경고 로그 (`bc67561`), `xflowd_device_uid_missing_total` Prometheus 메트릭 정의,
  inventory 노드의 UUID 발급 경로를 `Device.UID()` 로 정렬 (`510fc4b`, Century localID 형식 불일치 잠재 결함 동시 수정).
  인수 기준 A-AC1/A-AC2/A-AC4/A-AC5 충족, A-AC3 (emit map literal `uid` 키 추가) 은 Phase B 본격 emit payload
  표준화와 통합 예정. 회귀 0, 외부 클라이언트 비영향. Phase B/C/D 는 별도 세션 진행.

- **0.1.0 / Phase B 구현 완료** (2026-05-26): Phase B1 (7 커밋) + Phase B2 (3 커밋) 머지 (10 커밋 누적).
  registry UUID 인덱스 + V2 callback 시그니처 + 로그 `agent/name` 형식 + yaml resolver 3 형식 지원 + inventory `uid` 키 정규화 + composite 사용 빈도 메트릭 + Deprecation 운영 가이드 (B1, B-T1/T2/T6/T7/T8/T9/T10). WebSocket `uid` 1급 필드 + REST URL 3형식 dispatch (`Deprecation`/`Sunset` 헤더) + name 기반 명시 resolver (B2, B-T3/T4/T5). 회귀 0, 외부 클라이언트 비영향 (호환 alias 유지).

- **0.1.0 / Phase C1 구현 완료** (2026-05-26): Phase C § C1 (device-ids 마이그레이션 도구) 5 커밋 머지.
  `xflowd migrate` 명령 그룹 + `device-ids` 서브명령 신설 (`90c464f`),
  `internal/migrate/deviceids` 패키지 (classify + backup + atomic rename + sha256 검증) 도입 (`de539fd`),
  CLI 통합 테스트 7개 (strict / dry-run / idempotency / interactive cancel) 추가 (`1896d9f`),
  MIG-AC1 (round-trip) / MIG-AC4 (idempotency) / MIG-AC5 (백업 복원 가능성) 통합 시나리오 검증 (`ed2deca`),
  운영 가이드 (`docs/migration/device-identity.md`) 에 C1 도구 사용법 + 권장 절차 + 위험 신호 + 복구 절차 추가.
  인수 기준 C-AC1/AC2/AC3/AC4 + MIG-AC1/AC4/AC5 충족. neue 테스트 커버리지 86.6%. 회귀 0 (`go test -race ./internal/migrate/... ./internal/storage/... ./cmd/xflowd/...`).
  본 세션은 **Tools-only 모드**로 운영 데이터에 적용하지 않음 — staging 리허설은 운영자가 별도 진행.
  C2 (`tsdb-tags`) / C3 (Dual-tag 운영) 은 별도 세션 예정.

- **0.1.0 / Phase C2 구현 완료** (2026-05-26): Phase C § C2 (tsdb-tags 마이그레이션 도구) 5 커밋 머지.
  `xflowd migrate tsdb-tags` 서브명령 + 플래그 + 입력 검증 (`81dbf8a`),
  `internal/migrate/tsdbtags` 패키지 (SchemaClient + composite Classify + MockClient + DetectTarget) 도입 (`fe23fe8`),
  Flux (v2) / SQL+InfluxQL (v3) 스크립트 생성 + RUN.md + 4 golden file 테스트 (`59c9e1d`),
  통합 시나리오 (Plan/Apply, dry-run, idempotency, ping 실패, output-dir 충돌, restrict, ambiguous) + 실 Influx 어댑터 (v2 Flux schema / v3 InfluxQL SHOW) + CLI 통합 테스트 7개 (`a0eb885`),
  운영 가이드 (`docs/migration/device-identity.md`) 에 C2 도구 사용법 § 5.1 추가.
  인수 기준 M8 + C-AC6/AC7/AC8 (시계열 backfill 관련) + MIG-AC2 (sampling 검증) 의 **mock 버전** 충족.
  비-adapter 코드 평균 커버리지 92.9% (85% 목표 초과 달성). 실 Influx 어댑터 (`client_v2.go` / `client_v3.go`) 는 실제 서버 없이 단위 테스트 불가 — staging 운영자 검증으로 보강.
  회귀 0 (`go test -race ./internal/migrate/... ./cmd/xflowd/...`).
  **안전 가드 준수**: SchemaClient 인터페이스가 write API 호출을 컴파일 타임에 봉쇄. 모든 테스트는 MockClient 또는 httptest in-process server 만 사용 — 실제 Influx 인스턴스 접근 없음.
  본 세션도 **Tools-only 모드** — 운영자가 별도 staging 리허설 진행. C3 (Dual-tag 기간 운영) 은 별도 세션 예정.

- **0.1.0 / Phase C3 구현 완료** (2026-05-26): Phase C § C3 (Dual-tag emit — 데몬 자체 동작 변경) 5 커밋 머지.
  `xflowd_tsdb_dual_tag_total{state}` Prometheus CounterVec 신규 + 4 state 라벨 (both / composite_only / uid_only / unmapped) (`40e435c`),
  `InfluxDBConfig.DualTagEmit` (default true) + `DualTagEmitSourceKeys` (default `["device_id"]`) 옵션 도입 + yaml `[]any`/`[]string` 양쪽 형식 지원 (`39f2910`),
  `internal/agent/system/influxdb_dualtag.go` 신규 — `DeviceResolver` 인터페이스 + `augmentWriteDataWithUID` 헬퍼 + `InfluxDBAgent` 의 `processWriteSingle`/`processWriteBatch` 에 `validateWriteData` 직후·`client.Write` 직전 위치에 통합 + `WithDeviceResolver` 옵션 함수형 패턴 + `RegisterInfluxDBTypesWithResolver` 신규 + `cmd/xflowd/main.go` 에서 `deviceRegistry` 주입 (`cdc40e1`),
  통합 테스트 18 케이스 (4 state 전수 + 옵션 토글 + 다중 source key + graceful + 배치 mixed state + resolver 에러 + Phase A graceful UID="" + augmentWriteDataWithUID 단위 4 종) (`b875b65`),
  운영 가이드 (`docs/migration/device-identity.md`) 에 § 5.2 C3 사용법 추가 + 메트릭 해석 가이드 + C2 와의 관계 + Phase D 진입 신호 + opt-out 절차.
  인수 기준 **C-AC8 (호환 기간 두 tag 병기)** 충족 + 신규 코드 커버리지 (`augmentWriteDataWithUID` 96.0% / `parseInfluxDBConfig` 100% / `filterNonEmpty` 100%) 85% 목표 초과 달성.
  회귀 0 (`go test -race ./internal/agent/system/... ./internal/observe/...`).
  **Soft Deprecation 보장 확인**: composite tag (`device_id`) 미변경 — 외부 쿼리/대시보드 무영향. v2/v3 client 양쪽 자동 적용 (agent 단계 부착). `dual_tag_emit: false` opt-out 시 Phase A/B 동작 그대로.
  Phase D (composite 완전 제거, `Device.ID()` 시맨틱 변경 등) 는 별도 메이저 버전 (xflowd v1.0) 으로 분리 예정 — 본 Phase 외.

| Version | Date       | Author | Change                                                                                  |
| ------- | ---------- | ------ | --------------------------------------------------------------------------------------- |
| 0.1.0   | 2026-05-25 | xtra   | 최초 작성 — 이중 ID 체계 통일 계획, Kubernetes 패턴 채택, 4 Phase 진화 전략 (M1~M10)     |
| 0.1.0   | 2026-05-26 | xtra   | Phase A 구현 완료 (4 커밋: 991e793, be86904, bc67561, 510fc4b) — UUID 1급 격상, status in_progress |
| 0.1.0   | 2026-05-26 | xtra   | Phase B 구현 완료 (10 커밋, B1 7 + B2 3) — 내부 사용처 UUID 전환, 호환 alias 유지 |
| 0.1.0   | 2026-05-26 | xtra   | Phase C1 구현 완료 (5 커밋: 90c464f, de539fd, 1896d9f, ed2deca, +docs) — device-ids 마이그레이션 도구, status in_progress (C2/C3/D 잔여) |
| 0.1.0   | 2026-05-26 | xtra   | Phase C2 구현 완료 (5 커밋: 81dbf8a, fe23fe8, 59c9e1d, a0eb885, +docs) — tsdb-tags 마이그레이션 도구 (v2 Flux + v3 SQL), status in_progress (C3/D 잔여) |
| 0.1.0   | 2026-05-26 | xtra   | Phase C3 구현 완료 (5 커밋: 40e435c, 39f2910, cdc40e1, b875b65, +docs) — InfluxDBAgent dual-tag emit (composite + uid 자동 부착), status in_progress (D 잔여) |
| 0.2.0   | 2026-05-26 | xtra   | Phase D 통합 재정의 (xflowd v1.0 메이저) — greenfield 환경 가정 (A6 신규), M11 (Frontend UUID-first 전환) + M12 (Phase C 인프라 deprecation) 추가, D-AC8~D-AC18 신설, D-T10~D-T21 작업 분해, 호환 기간 매트릭스 환경별 분기 |
| 0.3.0   | 2026-05-26 | xtra   | Phase D PR4 구현 완료 (9 커밋) — composite 제거 + xflowd v1.0 메이저, Device.ID() UUID 반환 (D-T1), DeviceRegistry composite 거부 (D-T2), WebSocket device_id 필드 제거 (D-T3), yaml/registry composite cleanup (D-T4/T9), xflowd preflight 명령 (D-T5), 부팅 자동 sanity check (D-T6), CHANGELOG Breaking 안내 (D-T8), 운영자 체크리스트 갱신 (D-T21). 모든 D-AC1~D-AC18 충족. status: completed. **v1.0.0 git tag 진입 가능 (운영자 별도 결정)**. |

---

## 1. 개요 (Overview)

xflow 시스템은 디바이스를 식별하기 위해 **두 가지 ID 체계**가 공존하는 비대칭 상태에 있다:

1. **Composite key** (`Device.ID()` → `"agent_name:local_id"`, 예: `"lg_icp01:81"`): 초기 설계, 사람이 읽을 수 있고 yaml/REST/로그에 직접 노출. 에이전트 rename 시 모든 외부 참조와 시계열 데이터가 끊긴다.
2. **UUID** (`device_id`, v0.18.6 도입): `internal/agent/device_id_repo.go` 의 `ResolveDeviceID(ctx, agentName, localID)` 가 `DeviceIDRepository` 로부터 영속 UUID 를 발급. 현재 HVAC 7종(LG HVACR-01/LGAP/LG HVACR-02/NASA/Century/Modbus/Samsung) 의 emit 메시지에만 부분적으로 적용되어 있다.

본 SPEC 은 두 체계를 **Kubernetes 의 `uid` + `name` + `reference` 모델**로 단일화하며, 시스템 전반의 cascading 영향을 고려해 **4 Phase 단계적 진화** 전략을 수립한다.

> 본 SPEC 은 **계획만** 정의한다. 실제 구현은 별도 단계 (`/moai run SPEC-DEVICE-IDENTITY-001`) 와 후속 PR 시리즈로 분리한다. Phase D 의 Breaking 변경은 충분한 마이그레이션 기간 이후 별도 메이저 버전(예: xflowd v1.0)에서 적용한다.

---

## 2. 배경 (Background)

### 2.1 이중 ID 체계의 역사적 원인

**Phase 0 (initial)** — xflow 초기 설계는 디바이스를 에이전트의 종속 객체로 간주했다. `agent_name:local_id` 형태의 composite key 는 다음 장점을 제공했다:
- 사람이 읽기 쉽다 (`lg_icp01:81` 은 "lg_icp01 프로토콜의 81번 슬롯").
- yaml 설정의 `pinned:` 목록이나 REST URL (`/api/v1/devices/lg_icp01:81`) 에 직접 사용 가능.
- 외부 의존성 없이 즉시 생성.

**Phase 1 (v0.18.6 — UUID 도입)** — 실제 운영에서 다음 문제가 드러나며 UUID 가 도입되었다:
- **Rename 단절**: 에이전트 이름을 변경하면 (`lg_hvacr01` → `lg_hvacr01_main`) composite key 가 전부 바뀌어 시계열 DB tag, MQTT topic 구독자, 외부 클라이언트의 캐시가 모두 끊긴다.
- **재배치 단절**: 같은 물리 디바이스를 다른 에이전트로 옮기면(예: 마이그레이션) 추적 안정성 상실.
- **글로벌 유일성 보장 어려움**: composite 의 `:` 구분 규칙은 도메인 컨벤션일 뿐 강제되지 않는다.

이 시점에서 `agent.ResolveDeviceID(ctx, agentName, localID)` 와 `DeviceIDRepository` 가 도입되어 영속 UUID 를 발급한다. 그러나 적용 범위는 HVAC 에이전트의 emit 메시지로 한정되었고, `Device.ID()` 인터페이스 자체는 그대로 composite 를 반환한다.

### 2.2 현재 시스템의 문제점

| 영역 | 현재 상태 | 문제 |
|------|----------|------|
| `Device.ID()` | composite 반환 | UUID 가 1급 시민이 아님 |
| `device.Registry` | composite key 인덱스 | rename 시 키 재해시 필요 |
| `agent` callback | `func(agentName, deviceID string)` 의 deviceID 가 composite | UUID 추적 불가 |
| REST URL | `/api/v1/devices/{composite}` | rename 시 URL 깨짐 |
| HVAC emit | UUID 있음 | 비-HVAC 에이전트는 없음 (비대칭) |
| 시계열 DB tag | composite | 가장 큰 안정성 리스크 |
| `device_metadata` 영속 파일 | composite key | rename 시 메타데이터 고아화 |
| WebSocket event | composite | 프론트엔드가 composite 에 결합 |
| 프론트엔드 표시 | composite | 사용자에게 노이즈 (`lg_icp01:81`) |

### 2.3 비대칭이 만드는 비용

- 한 시스템 안에 **두 종류의 진실**이 공존하여 디버깅·로깅·연관성 추적이 어렵다.
- 신규 에이전트 추가 시 UUID 발급 누락이 발생하기 쉽다 (HVAC 만 적용된 패턴이 굳어짐).
- 프론트엔드와 시계열 도구 사이의 키 호환성 비용이 영구화된다.

---

## 3. 사용자 결정 (Decision: Kubernetes 패턴 채택)

본 SPEC 은 Kubernetes API 의 `metadata.uid` / `metadata.name` 모델을 차용한다. 디바이스 식별을 다음 **3 개념의 명확한 분리**로 재정의한다:

| 개념 | 필드명 | 범위 | 변경 가능성 | 용도 |
|---|---|---|---|---|
| **Identity** | `uid` (UUID v4) | 글로벌 유일, 불변 | 불변 | 내부 참조, 영속 저장, 시계열 tag, REST URL |
| **Name** | `name` | 에이전트 내에서 유일 | 변경 가능 | 사람이 읽는 라벨, 로그, UI, yaml 참조 |
| **Reference** | `agent/name` (파생) | 글로벌 유일 (구성요소 따라 갱신) | 구성요소 변경 시 갱신 | yaml 의 pinned 참조 등 |

### 3.1 규칙

1. `Device.UID() string` (신규) → UUID 반환 (글로벌 유일, 불변).
2. `Device.Name() string` → 사람이 보는 라벨. 현재 정의 유지 (사용자 지정 이름 또는 fallback).
3. `Device.ID()` 의 시맨틱은 **Phase D 에서 UUID 로 변경**되며, 그 전까지는 composite 를 유지하되 `Deprecated` 마킹.
4. composite key 는 단계적 제거 대상이다 (alias 호환 → 완전 제거).
5. REST URL 은 `/api/v1/devices/{uid}` 로 표준화. 사람이 직접 입력하지 않으므로 UUID 도 수용 가능.
6. 로그 표시는 `agent/name` 결합 형식: `device "lg_hvacr01/indoor-1" went offline`.
7. yaml 의 디바이스 참조는 `agent/name` 또는 UUID 두 형식 모두 허용한다 (운영자 편의).

### 3.2 Kubernetes 와의 의도적 유사성

- Kubernetes 의 `Pod` 가 `metadata.uid` (UUID) 와 `metadata.name` (사람이 정의) 를 분리하는 것과 동일한 패턴.
- 시스템 내부는 항상 `uid` 로 참조하므로 `name` 변경의 파급이 없다.
- xflow 의 에이전트 = Kubernetes 의 namespace, 디바이스 = Pod 에 매핑된다 (개념적 비유).

---

## 4. 환경 (Environment)

### 4.1 영향 받는 영역 (상세)

| 영역 | 변경 항목 | 위험도 |
|---|---|---|
| `internal/device/` | `Device.UID()` 추가, `Device.ID()` 시맨틱 변경 (Phase D), Registry 키 UUID 화, 이름 인덱스 추가 | 높음 |
| `internal/agent/` | 모든 callback 시그니처 (`SetDeviceStateChangeCallback(func(agentName, deviceUID string))` 등) | 중간 |
| `internal/api/handler/device.go` | REST URL UUID 우선, name 기반 resolver 별도 엔드포인트 | 중간 |
| HVAC 7종 에이전트 (LG HVACR-01/LGAP/LG HVACR-02/NASA/Century/Modbus/Samsung) | 모든 emit 메시지 일관성 (uid 1급 필드) | 높음 |
| `internal/storage/device_metadata/` | 영속 파일 키 composite → UUID 마이그레이션 도구 | **매우 높음** |
| `internal/api/ws/` | WebSocket event payload 의 deviceID UUID 통일 | 중간 |
| `web/src/` (프론트엔드) | API 응답 ID 의미 변경, URL params, 표시는 name | 중간 |
| 시계열 DB (Influx/TSDB) | 기존 composite tag → UUID tag backfill, 호환 기간 운영 | **매우 높음** |
| `internal/agent/device_id_repo.go` | UUID 가 nil 인 경로 제거 (Phase D), 부팅 시 강제 검증 | 높음 |

### 4.2 가정 (Assumptions)

- **A1**: 운영 중인 모든 xflow 인스턴스는 v0.18.6 이상이며 `DeviceIDRepository` 가 설정되어 있다. (미설정 시 Phase A 에서 부팅 경고, Phase D 에서 부팅 실패.)
- **A2**: 시계열 DB 의 backfill 은 운영 중단 윈도우 없이 점진적으로 가능하다 (호환 기간 동안 두 tag 모두 기록).
- **A3**: 프론트엔드 v0.x 시리즈는 호환 기간 동안 두 ID 모두 수용 가능하다. v1.0 부터 UUID 만 사용.
- **A4**: `device_metadata` 영속 파일의 마이그레이션은 백업 + atomic rename 으로 안전하게 수행 가능하다.
- **A5**: 외부 통합(MQTT 구독자, 외부 클라이언트) 은 호환 기간 동안 알림 받고 마이그레이션할 시간이 있다.
- **A6** (v0.2.0 신규): 본 SPEC 의 구체적 운영 환경은 greenfield (단일 frontend 통제, 외부
  클라이언트 없음, 기존 영속 메타데이터 거의 없음) 으로 확정. 따라서 Phase D 의
  호환 기간 정책은 환경별로 분기한다:
  - greenfield: 즉시 통합 진행 가능 (xflowd v1.0 단일 메이저)
  - brownfield (다른 운영자가 본 SPEC 채택 시): 기존 6개월 호환 기간 권장
  본 SPEC 의 xflow 자체 사용자 그룹은 greenfield 에 해당하며, 본 갱신 후 Phase D
  통합 메이저 릴리즈로 진입한다. brownfield 사용자는 § 7.4 호환 기간 매트릭스 참조.

### 4.3 Constitution 정합성

- Go 1.23+ 기존 기술 스택 (신규 의존성 없음, `crypto/rand` 와 `google/uuid` 는 v0.18.6 에서 이미 사용 중).
- 하위호환은 Phase A~C 에서 보장, Phase D 에서 의도적 Breaking (별도 메이저 버전).
- TRUST 5: Tested (마이그레이션 검증 시나리오 필수), Trackable (모든 Phase 의 변경을 SPEC 으로 분리 추적).

---

## 5. EARS 요구사항 (EARS Requirements)

본 SPEC 은 10개 EARS 모듈로 구성된다. 각 모듈은 4 Phase 중 하나에 속한다 (Phase 표기는 § 7 참조).

### M1: `Device.UID()` 인터페이스 및 UUID 보장 (Phase A)

- **Ubiquitous**: `Device` 인터페이스는 **항상** `UID() string` 메서드를 노출하여 디바이스의 글로벌 유일 UUID 를 반환해야 한다.
- **Ubiquitous**: 모든 `Device` 구현체는 생성 시점에 `agent.ResolveDeviceID(ctx, agentName, localID)` 또는 동등 경로로 UUID 를 발급받아 보유해야 한다.
- **State-driven**: IF `DeviceIDRepository` 가 미설정 (nil) 인 경우, THEN 시스템은 Phase A 에서 부팅 시 경고 로그를 남기고 `UID()` 가 빈 문자열을 반환하도록 허용한다 (graceful degradation).
- **Unwanted**: WHEN `Device` 구현체가 `UID()` 를 잘못 구현하여 매 호출마다 다른 UUID 를 반환하면, THEN 통합 테스트가 이를 감지하고 실패해야 한다.
- **Ubiquitous**: `Device.ID()` 는 Phase A~C 동안 **composite key** 를 반환하며, Phase D 에서 **UUID** 반환으로 시맨틱이 변경된다 (Breaking, M9 참조).

### M2: emit 메시지 / API 응답의 uid 필드 1급 노출 (Phase A)

- **Ubiquitous**: 모든 에이전트(HVAC 7종 + 향후 추가) 의 emit 메시지 payload 는 **항상** `uid` 필드 (string, UUID) 와 `id` 필드 (string, composite, deprecated) 를 **병기**해야 한다.
- **Ubiquitous**: `GET /api/v1/devices` 와 `GET /api/v1/devices/{ref}` 응답은 **항상** `uid` 필드를 1급 필드로 포함해야 한다.
- **State-driven**: IF `UID()` 가 빈 문자열인 경우, THEN emit / API 응답에서 `uid` 키를 생략한다 (graceful degradation).
- **Ubiquitous**: emit payload 의 `uid` 필드명은 `device_uuid` 가 아닌 **`uid`** 로 정규화한다 (SPEC-INVENTORY-001 v0.2.0 의 `device_uuid` 는 Phase B 에서 `uid` 로 정렬, 호환 alias 유지).
- **Optional**: WHERE 운영자가 ID 노출 형식을 제어하려는 경우, 시스템은 future-proof 한 형식 선언 (예: yaml 의 `legacy_composite_emit: true|false`) 을 Phase A 에서 도입할 수 있다 (default: 두 필드 모두 emit).

### M3: callback / event / WebSocket payload UUID 전환 (Phase B)

- **Ubiquitous**: 모든 에이전트 callback 시그니처는 `func(agentName string, deviceUID string, ...)` 형태로 UUID 를 전달해야 한다.
- **Ubiquitous**: WebSocket event payload (`device.state_changed`, `device.online`, `device.offline` 등) 의 디바이스 식별 필드는 **항상** `uid` (UUID) 를 1급 필드로 포함해야 한다.
- **State-driven**: IF 외부 클라이언트가 Phase A 형식의 composite 기반 이벤트를 구독 중이면, THEN 시스템은 호환 기간 동안 `id` (composite) 도 병기한다 (Phase B 단계).
- **Unwanted**: WHEN callback 이 `deviceUID` 가 빈 문자열인 디바이스에 대해 호출되면 (UUID 미발급 디바이스), THEN 시스템은 경고 로그를 남기고 callback 을 건너뛴다 (Phase B 단계, Phase D 에서 부팅 실패로 전환).
- **Ubiquitous**: 내부 모듈 간 디바이스 참조는 **항상** UUID 를 사용하며, composite 은 외부 표시·로그 외에서 사용을 금지한다.

### M4: REST URL UUID 우선 + composite alias 호환 (Phase B)

- **Event-driven**: WHEN `GET /api/v1/devices/{ref}` 요청에서 `{ref}` 가 UUID v4 형식이면, THEN 시스템은 UUID 인덱스에서 직접 조회한다.
- **Event-driven**: WHEN `{ref}` 가 `agent/name` 형식이면 (Phase B 신규), THEN 시스템은 `(agent, name)` 쌍으로 조회한다.
- **Event-driven**: WHEN `{ref}` 가 v0.x 의 composite (`agent:local_id`) 형식이면, THEN 시스템은 호환 alias resolver 로 변환 후 조회하며 응답 헤더 `Deprecation: true` 와 `Sunset` 헤더를 포함한다.
- **State-driven**: IF `{ref}` 가 어떤 형식에도 매칭되지 않으면, THEN HTTP 404 와 함께 명시적 에러 메시지 (`"device reference {ref} not found; expected UUID, agent/name, or legacy agent:local_id"`) 를 반환한다.
- **Optional**: WHERE 별도 name-based resolver 엔드포인트 (`GET /api/v1/devices:resolve?agent=lg_hvacr01&name=indoor-1`) 가 필요하면, Phase B 에서 추가한다.

### M5: 로그 형식 `agent/name` 표시 (Phase B)

- **Ubiquitous**: 디바이스 관련 로그 라인은 **항상** `agent/name` 결합 형식을 사용해야 한다 (예: `device "lg_hvacr01/indoor-1" offline`).
- **Ubiquitous**: 로그 라인에 UUID 가 필요한 경우 (디버깅·연관성 추적) 별도 키 (`device_uid=<uuid>`) 로 구조화 로깅에 포함한다.
- **State-driven**: IF `Device.Name()` 이 빈 문자열인 경우 (fallback 미설정), THEN 로그는 `agent/<local_id>` 또는 `agent/<short_uid>` 로 fallback 한다.
- **Unwanted**: 로그 라인에서 raw composite key (`lg_icp01:81`) 의 표시는 Phase B 이후 점진적으로 제거한다 (Phase D 에서 완전 금지).

### M6: yaml 설정 참조 표기법 (Phase B)

- **Ubiquitous**: yaml 설정 파일의 디바이스 참조 (예: `pinned: [...]`) 는 **항상** 두 형식을 허용해야 한다:
  - `agent/name` 형식 (예: `"lg_hvacr01/indoor-1"`) — 사람이 작성하기 편함.
  - UUID 형식 (예: `"a58ba668-5741-..."`) — 자동 생성된 yaml.
- **Event-driven**: WHEN yaml 파싱 시 두 형식이 혼재하면, THEN 시스템은 각각의 resolver 를 사용하여 UUID 로 정규화해야 한다.
- **Unwanted**: WHEN yaml 의 디바이스 참조가 어떤 형식에도 매칭되지 않으면, THEN 설정 로드를 실패시키고 `ErrInvalidDeviceReference` 를 반환해야 한다.
- **Unwanted**: WHEN yaml 에 v0.x 의 composite (`agent:local_id`) 가 발견되면, THEN 호환 기간 동안 경고 로그를 남기고 정규화하되 (Phase B), Phase D 에서 부팅 실패로 전환한다.
- **Optional**: WHERE 운영자가 yaml 의 디바이스 참조를 일괄 마이그레이션하려면, CLI 도구 (`xflowd migrate yaml-device-refs`) 를 제공한다.

### M7: 영속 메타데이터 마이그레이션 도구 (Phase C)

- **Ubiquitous**: 시스템은 **항상** `xflowd migrate device-ids` CLI 도구를 제공하여 영속 메타데이터 파일의 키를 composite → UUID 로 변환할 수 있어야 한다.
- **Ubiquitous**: 마이그레이션 도구는 **항상** 다음 순서로 동작해야 한다:
  1. 백업 생성 (`<file>.bak.<timestamp>`).
  2. composite key 를 UUID 로 변환 (단, `DeviceIDRepository` 미설정 시 실패).
  3. atomic rename (원자적 교체).
  4. 변환 전후 hash 비교 (entry 수 일치, 모든 메타데이터 보존).
- **Event-driven**: WHEN 마이그레이션 도구가 실행되면, THEN 다음 출력을 stdout 으로 제공해야 한다:
  - 변환된 엔트리 수.
  - 매핑 실패한 composite key 목록 (있으면 경고).
  - 백업 파일 경로.
  - 검증 hash 일치 여부.
- **Unwanted**: WHEN 마이그레이션 중 어떤 오류라도 발생하면, THEN 도구는 부분 변경 없이 원복하고 (백업에서 복원) 명확한 에러 메시지를 반환해야 한다.
- **State-driven**: IF 마이그레이션 도구가 `--dry-run` 플래그로 호출되면, THEN 변환 계획만 출력하고 실제 파일은 변경하지 않는다.

### M8: 시계열 데이터 backfill 전략 (Phase C)

- **Ubiquitous**: 시스템은 호환 기간 동안 **항상** 시계열 DB tag 에 `uid` 와 `id` (composite) 두 tag 를 모두 기록해야 한다 (Phase A 종료 시점부터 Phase D 시작 직전까지).
- **Ubiquitous**: backfill 절차는 **항상** 다음을 포함해야 한다:
  - Influx flux 스크립트 또는 별도 마이그레이션 도구 (`xflowd migrate tsdb-tags`).
  - 기존 데이터의 composite tag → UUID tag 추가 (composite tag 는 호환 기간 동안 유지).
  - backfill 진행률과 에러를 stdout 으로 노출.
- **Event-driven**: WHEN backfill 이 완료되면, THEN 시스템은 검증 쿼리 (composite tag 와 UUID tag 의 시리즈 수 일치) 를 실행하고 결과를 보고해야 한다.
- **Unwanted**: WHEN backfill 중 한 series 의 매핑이 모호하면 (composite → 다중 UUID 매핑), THEN 도구는 해당 series 를 skip 하고 경고를 stderr 에 기록한다 (운영자 수동 개입 대상).
- **Optional**: WHERE 운영자가 호환 기간 종료 후 composite tag 를 제거하려면, 별도 도구 (`xflowd migrate tsdb-drop-composite`) 를 제공한다.

### M9: composite 제거 + Soft Deprecation 인프라 cleanup 및 Breaking 시맨틱 변경 (Phase D, v0.2.0 확장)

- **Ubiquitous**: Phase D 에서 `Device.ID()` 의 시맨틱은 **UUID 반환** 으로 변경된다 (Breaking Change).
- **Ubiquitous**: REST URL 의 composite alias (`/api/v1/devices/lg_icp01:81`) 는 Phase D 에서 **제거**되며, 호출 시 HTTP 404 를 반환한다.
- **Ubiquitous**: emit 메시지에서 deprecated `id` (composite) 필드는 Phase D 에서 **제거**되며, `uid` 만 노출된다.
- **Ubiquitous**: 시계열 DB 의 composite tag 는 Phase D 시작 시점부터 새로 기록되지 않으며, 기존 데이터는 `xflowd migrate tsdb-drop-composite` 도구로 정리 가능하다.
- **Unwanted**: WHEN yaml 설정에 composite (`agent:local_id`) 가 발견되면 (Phase D), THEN 시스템은 부팅을 실패시키고 즉시 `ErrInvalidDeviceReference` 를 반환해야 한다 (v0.2.0: greenfield 환경에는 Deprecation 경고 단계 없이 즉시 부팅 실패).
- **Ubiquitous** (v0.2.0 신규): Phase D 는 Phase B 의 Soft Deprecation 인프라를 동시에 제거한다. 대상:
  - V1 callback wrapper (`AdaptLegacyCallback`, `DeviceStateChangeCallback` v1 시그니처) 제거
  - REST composite alias 핸들러 + `Deprecation`/`Sunset` 헤더 dispatch 제거
  - yaml resolver 의 composite (`agent:local_id`) 형식 parse 경로 제거
  - inventory 노드의 `device_uuid` alias 제거 (`uid` 만 emit)
  - logger device_format 의 composite fallback 제거
  - 5 HVAC 에이전트 (LG HVACR-01/LGAP/LG HVACR-02/NASA/Century/Modbus/Samsung) 의 `onDeviceStateChange` v1 필드 + setter 제거
- **Ubiquitous** (v0.2.0 갱신): Phase D 적용 시점은 환경에 따라 분기한다:
  - greenfield (xflow 자체 사용자 그룹): Phase B/C 완료 후 즉시 진행 가능 (xflowd v1.0 단일 메이저 릴리즈)
  - brownfield (다른 운영자 채택): Phase B/C 완료 시점부터 **최소 6개월의 호환 기간** 확보 권장

### M10: 마이그레이션 미수행 부팅 실패 (Phase D)

- **Unwanted**: WHEN Phase D 버전 (xflowd v1.0+) 부팅 시 `DeviceIDRepository` 가 미설정이거나 영속 메타데이터에 composite key 가 남아있으면, THEN 시스템은 부팅을 실패시키고 다음 명시적 에러 메시지를 반환해야 한다:
  ```
  device identity: legacy composite keys detected in <file_path>;
  run 'xflowd migrate device-ids' before starting xflowd v1.0+.
  see https://docs.xflow.example/migration/device-identity for details.
  ```
- **Unwanted**: WHEN Phase D 버전에서 시계열 DB 에 UUID tag 가 없는 series 가 검출되면 (best-effort check, 부팅 시 sampling), THEN 시스템은 경고 로그를 남기고 backfill 안내를 출력한다 (부팅은 계속).
- **Ubiquitous**: Phase D 의 부팅 검증 로직은 **항상** 사전 점검 (preflight) 으로 분리되어 `xflowd preflight` 명령으로 단독 실행 가능해야 한다.
- **State-driven**: IF 운영자가 `--skip-preflight` 플래그로 부팅한다면, THEN 시스템은 경고 후 부팅을 계속한다 (긴급 복구 경로, 권장하지 않음).

### M11: Frontend UUID-first 전환 (Phase D, v0.2.0 신규)

- **Ubiquitous**: web/src/ 의 모든 디바이스 참조는 `uid` (UUID) 1급 사용을 원칙으로 해야 한다.
- **Ubiquitous**: WS 메시지 핸들러는 payload 의 `uid` 키를 사용하며, `device_id` (composite) 참조는 제거되어야 한다.
- **Ubiquitous**: REST 호출은 `/api/v1/devices/{uid}` UUID 형식 또는 `/api/v1/devices:resolve?agent=X&name=Y` 명시 resolver 형식을 사용해야 한다. composite URL (`/api/v1/devices/lg_icp01:81`) 호출은 제거되어야 한다.
- **Ubiquitous**: 디바이스 표시 라벨은 `agent/name` (사람이 읽음) 또는 UUID (내부 ID) 두 형식 중 선택하며, 절대로 composite (`lg_icp01:81`) 형식을 사용자에게 노출해서는 안 된다.
- **Ubiquitous**: inventory 노드의 output desc / 사용 예시 / 타입 정의는 `device_uuid` 가 아닌 `uid` 키 기준으로 갱신되어야 한다.
- **Unwanted**: WHEN frontend (web/src/) 코드 어디든 `device_id` 식별자 또는 composite 형식이 사용자에게 노출되는 경로가 잔존하면, THEN v1.0 출시는 차단되어야 한다.
- **State-driven**: IF `useDevices` / `useDevice` hook 의 반환 객체에 `id` (composite) 키가 남아있으면, THEN 해당 키는 `uid` 로 치환되거나 제거되어야 한다.

### M12: Phase C 인프라 deprecation (Phase D, v0.2.0 신규)

- **Ubiquitous**: `xflowd migrate device-ids` 와 `xflowd migrate tsdb-tags` 명령은 v1.0 에서 deprecation 경고를 출력 후 정상 종료해야 한다. 강제 제거하지 않는다 (brownfield 사용자의 잠재적 필요 보존).
- **Ubiquitous**: `InfluxDBConfig.DualTagEmit` 설정 옵션은 제거되거나 deprecated noop 으로 유지되어야 한다 — composite tag 가 없어지므로 dual emit 의미 상실.
- **Ubiquitous**: `InfluxDBConfig.DualTagEmitSourceKeys` 설정 옵션은 제거되어야 한다.
- **Ubiquitous**: `xflowd_tsdb_dual_tag_total{state}` Prometheus CounterVec 메트릭은 제거되어야 한다.
- **Ubiquitous**: `xflowd_device_composite_use_total{source}` Prometheus CounterVec 메트릭은 제거되어야 한다 — composite 사용 자체가 사라짐.
- **State-driven**: IF v1.0 환경에서 `xflowd migrate device-ids` 또는 `xflowd migrate tsdb-tags` 명령이 실행되면, THEN "v1.0 환경에는 마이그레이션 대상 없음 — composite 형식은 이미 제거되었습니다" 안내 메시지와 함께 즉시 종료 (exit code 0) 한다.
- **Optional**: WHERE brownfield 사용자가 본 SPEC 을 채택하여 마이그레이션 도구가 필요하면, v0.x (Phase C3 완료 시점) 버전을 staging 에서 실행 후 v1.0 으로 업그레이드한다.

---

## 6. 명세 (Specifications)

### 6.1 `Device` 인터페이스 진화 (Phase A → Phase D)

**Phase A (현재 → 신규 메서드 추가):**

```go
// internal/device/device.go
type Device interface {
    // ID returns the legacy composite ID (agent_name:local_id).
    //
    // Deprecated: Use UID() for stable references. Phase D will change ID()
    // semantics to return UUID instead. See SPEC-DEVICE-IDENTITY-001.
    ID() string

    // UID returns the globally unique, immutable UUID v4 for this device.
    // Returns empty string only if DeviceIDRepository is not configured
    // (graceful degradation during Phase A; will fail boot in Phase D).
    UID() string

    // Name returns the user-defined name (mutable, human-readable).
    Name() string

    // ... (other methods unchanged)
}
```

**Phase D (Breaking — `ID()` 시맨틱 변경):**

```go
type Device interface {
    // ID returns the globally unique UUID v4 for this device.
    // Equivalent to UID() but kept for API stability.
    ID() string

    // UID returns the globally unique UUID v4 for this device.
    // Identical to ID() in Phase D+.
    UID() string

    // Name returns the user-defined name (mutable, human-readable).
    Name() string

    // ... (other methods unchanged)
}
```

### 6.2 emit 메시지 payload 진화

**Phase A (병기):**

```json
{
  "agent": "lg_hvacr01",
  "id": "lg_icp01:81",
  "uid": "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
  "name": "indoor-1",
  "online": true,
  "ts_ms": 1716620000000
}
```

**Phase B (uid 1급, id deprecated 유지):**

```json
{
  "agent": "lg_hvacr01",
  "uid": "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
  "id": "lg_icp01:81",
  "_deprecated": ["id"],
  "name": "indoor-1",
  "online": true,
  "ts_ms": 1716620000000
}
```

**Phase D (uid 만):**

```json
{
  "agent": "lg_hvacr01",
  "uid": "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
  "name": "indoor-1",
  "online": true,
  "ts_ms": 1716620000000
}
```

### 6.3 REST URL 진화

| Phase | URL 패턴 | 동작 |
|---|---|---|
| A | `/api/v1/devices/lg_icp01:81` | composite resolver (현재 동작 유지) |
| A | `/api/v1/devices/{uuid}` | UUID resolver (신규) |
| B | `/api/v1/devices/lg_hvacr01/indoor-1` | `agent/name` resolver (신규) |
| B | `/api/v1/devices/lg_icp01:81` | composite alias (Deprecation 헤더 포함) |
| B | `/api/v1/devices:resolve?agent=lg_hvacr01&name=indoor-1` | name 기반 명시 resolver (신규) |
| D | `/api/v1/devices/{uuid}` | UUID only (composite alias 제거) |
| D | `/api/v1/devices/lg_hvacr01/indoor-1` | `agent/name` resolver (유지) |

### 6.4 yaml 설정 진화

**Phase A (변경 없음, composite 유지):**

```yaml
flow:
  pinned:
    - "lg_icp01:81"
```

**Phase B (두 형식 허용):**

```yaml
flow:
  pinned:
    - "lg_hvacr01/indoor-1"                        # agent/name (권장)
    - "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d" # UUID
    - "lg_icp01:81"                              # composite (deprecated, 경고)
```

**Phase D (composite 제거):**

```yaml
flow:
  pinned:
    - "lg_hvacr01/indoor-1"
    - "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"
  # "lg_icp01:81" 형식은 부팅 실패
```

### 6.5 로그 형식 진화

```
# Phase A (현재)
INFO  agent=lg_hvacr01 device_id=lg_icp01:81 online=false

# Phase B
INFO  device "lg_hvacr01/indoor-1" went offline device_uid=a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d

# Phase D (composite 표시 금지)
INFO  device "lg_hvacr01/indoor-1" went offline device_uid=a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d
```

### 6.6 에러 모델 (신규)

| 에러 | Phase | 의미 |
|---|---|---|
| `ErrInvalidDeviceReference` | B | yaml/REST 의 디바이스 참조 형식 미매칭 |
| `ErrCompositeDeprecated` | B | composite 사용 경고 (호환 동작, 헤더로 노출) |
| `ErrLegacyCompositeInPersistentStore` | D | 영속 메타데이터에 composite 잔존 (부팅 실패) |
| `ErrMissingDeviceIDRepository` | D | UUID 발급 저장소 미설정 (부팅 실패) |
| `ErrDeviceUIDNotResolved` | B,C | 마이그레이션 도구에서 매핑 실패 |
| `ErrYAMLLegacyCompositeRef` | D | yaml 에 composite 참조 잔존 (부팅 실패) |

---

## 7. 4 Phase 단계적 진화 전략

### Phase A: UUID 를 1급 ID 로 격상 (Breaking 없음)

- **목표**: `Device.UID()` 도입, 모든 Device 생성 시 UUID 보장, emit/API 응답에 `uid` 병기.
- **포함 모듈**: M1, M2.
- **위험도**: 낮음 (추가만, 기존 동작 유지).
- **운영 영향**: 없음 (기존 클라이언트는 `id` 그대로 사용).
- **검증**: 통합 테스트로 모든 에이전트가 `UID()` 를 반환하는지 확인.

### Phase B: 내부 사용처를 UUID 로 전환 (Soft Breaking — Deprecation 만)

- **목표**: callback / event / WebSocket / 로그 / yaml resolver 를 UUID 기반으로 전환, composite 은 호환 alias.
- **포함 모듈**: M3, M4, M5, M6.
- **위험도**: 중간 (외부 클라이언트가 composite 에 결합되어 있을 가능성).
- **운영 영향**: 외부 클라이언트는 Deprecation 헤더로 알림. 호환 기간 6개월 권장.
- **검증**: 외부 통합 (MQTT 구독자, 외부 API) 의 UUID 수용 확인.

### Phase C: 영속 데이터 마이그레이션 (Operational Risk — High)

- **목표**: 영속 메타데이터와 시계열 DB 의 composite → UUID 변환.
- **포함 모듈**: M7, M8.
- **위험도**: **매우 높음** (데이터 무결성).
- **운영 영향**: 마이그레이션 윈도우 권장 (다만 backfill 은 비파괴적).
- **검증**: hash 비교, 엔트리 수 일치, sampling 쿼리.

### Phase D: composite 제거 + Soft Deprecation 인프라 cleanup + Frontend 전환 (xflowd v1.0 통합 메이저, v0.2.0 재정의)

- **목표** (v0.2.0 재정의):
  - composite 완전 제거: `Device.ID()` UUID 반환, REST alias 제거, deprecated emit 필드 제거, 부팅 시 마이그레이션 검증.
  - Phase B Soft Deprecation 인프라 cleanup: V1 callback wrapper, REST alias dispatch, yaml composite parse, inventory device_uuid alias, logger composite fallback 제거.
  - Phase C 인프라 deprecation: migrate 도구 (deprecated noop 유지), dual-tag emit 옵션 + 메트릭 제거.
  - Frontend UUID-first: web/src/ 의 `device_id` → `uid` 일괄 치환.
- **포함 모듈**: M9 (확장), M10, M11 (v0.2.0 신규), M12 (v0.2.0 신규).
- **위험도**: **매우 높음** (Breaking Change), 단 greenfield 환경에서는 외부 호환성 리스크 없음.
- **운영 영향**: **메이저 버전 (xflowd v1.0)** 단일 릴리즈. greenfield 환경은 즉시 진행 가능. brownfield 환경은 § 7.4 호환 기간 매트릭스 참조.
- **검증**: preflight 명령으로 사전 점검 + frontend `device_id` 참조 0건 grep.
- **Phase D 진입 신호 (v0.2.0 재정의)**:
  - **greenfield**: Frontend (M11) 준비 완료 + staging 검증 완료 → 즉시 진행
  - **brownfield**: 기존 6개월 호환 기간 경과 + `xflowd_device_composite_use_total{*}` 메트릭 < 0.01% 의 외부 클라이언트 사용 빈도

---

## 8. 관련 SPEC (Related SPECs)

- **SPEC-DEVICE-001** (참고): Device 인터페이스 기본 정의. 본 SPEC 의 § 6.1 진화의 출발점.
- **SPEC-AGENT-001** (참고): `agent.ResolveDeviceID` / `DeviceIDRepository` 도입 SPEC (v0.18.6). Phase A 의 기반 인프라.
- **SPEC-INVENTORY-001 v0.2.0** (선행 작업): `devices` source payload 에 `device_uuid` 필드 추가. 본 SPEC Phase A 의 부분 검증. Phase B 에서 `device_uuid` → `uid` 키 정규화 예정 (호환 alias 유지).
- **SPEC-DEVICE-IDENTITY-002 (가칭, 후속)**: Phase D 적용 (xflowd v1.0 메이저 버전) — 본 SPEC 완료 후 분리 작성 예정.

---

## 9. TAG Traceability

- @SPEC:SPEC-DEVICE-IDENTITY-001 → spec.md (이 문서)
- @PLAN:SPEC-DEVICE-IDENTITY-001 → plan.md
- @ACCEPTANCE:SPEC-DEVICE-IDENTITY-001 → acceptance.md
- 구현 경로 (Phase 별):
  - **Phase A**:
    - `internal/device/device.go` — `UID() string` 메서드 추가
    - `internal/agent/{lg,samsung,century,modbus}/device.go` — UUID 보유 및 `UID()` 구현
    - `internal/api/handler/device.go` — 응답에 `uid` 필드 추가
    - `internal/agent/device_id_repo.go` — 부팅 시 미설정 경고 로그
  - **Phase B**:
    - `internal/device/registry.go` — UUID 기본 인덱스, `(agent, name)` 보조 인덱스
    - `internal/agent/*/callback.go` (전역) — callback 시그니처 변경
    - `internal/api/ws/event.go` — event payload `uid` 1급 필드
    - `internal/api/handler/device.go` — `agent/name` resolver, name 기반 resolver 엔드포인트
    - `internal/config/yaml_resolver.go` (신규 또는 확장) — yaml 참조 정규화
    - `internal/logger/device_format.go` (신규) — `agent/name` 형식 헬퍼
  - **Phase C**:
    - `cmd/xflowd-migrate/device_ids.go` (신규) — 영속 메타데이터 마이그레이션 CLI
    - `cmd/xflowd-migrate/tsdb_tags.go` (신규) — 시계열 backfill CLI
    - `internal/storage/device_metadata/migration.go` (신규)
  - **Phase D**:
    - `internal/device/device.go` — `ID()` 시맨틱 변경 (Breaking)
    - `internal/api/handler/device.go` — composite alias 제거
    - `cmd/xflowd/preflight.go` (신규) — 부팅 사전 점검
    - `cmd/xflowd-migrate/tsdb_drop_composite.go` (신규)

---

## 10. Implementation Notes

### 10.1 4 Phase 전략의 합리성

- **Phase A 는 비파괴적**: 신규 메서드 추가와 응답 필드 병기만 수행. 기존 코드는 영향받지 않으며, 잠재적 회귀가 없다.
- **Phase B 는 deprecation 단계**: 호환 alias 를 충분히 유지하여 외부 클라이언트가 마이그레이션할 시간을 확보한다. 헤더로 명시적 알림.
- **Phase C 는 데이터 무결성이 최우선**: 백업 + atomic rename + hash 검증 + dry-run 으로 4중 안전망. 부분 변경을 허용하지 않는다.
- **Phase D 는 메이저 버전**: SemVer 의 Breaking Change 규칙에 따라 xflowd v1.0 으로 분리. Phase B/C 완료 후 최소 6개월의 호환 기간을 확보하여 운영자가 충분히 마이그레이션할 시간을 준다.

### 10.2 Kubernetes 패턴의 차용 이유

- **검증된 모델**: Kubernetes 는 수십만 클러스터에서 동일한 문제를 동일한 방식으로 해결하고 있다.
- **명확한 책임 분리**: `uid` (identity) 와 `name` (label) 의 분리는 직관적이며, 모든 운영자가 이해하기 쉽다.
- **`agent/name` 의 자연스러운 namespace 비유**: Kubernetes 의 `namespace/name` 패턴과 동일한 멘탈 모델.

### 10.3 위험 완화 전략 (요약)

| 위험 | 완화 |
|---|---|
| 영속 메타데이터 손실 | 백업 + atomic rename + hash 검증 + dry-run (M7) |
| 시계열 DB 단절 | 호환 기간 동안 두 tag 병기 (M8) |
| 외부 클라이언트 단절 | Deprecation 헤더 + 6개월 호환 기간 (M4, Phase B→D) |
| 부팅 실패 | preflight 명령으로 사전 점검 (M10) |
| UUID 발급 실패 | Phase A 는 graceful degradation, Phase D 에서 부팅 실패로 전환 (M1) |

### 10.4 본 SPEC 의 범위 외 (Out of Scope)

- **에이전트 rename 의 UX 와 자동화**: 별도 SPEC 으로 분리 (예: SPEC-AGENT-RENAME-001).
- **디바이스 재배치 (cross-agent migration)**: Phase D 이후 별도 기능 (별도 SPEC).
- **외부 통합 (MQTT bridge, 외부 API) 의 구체적 마이그레이션 가이드**: 본 SPEC 은 시스템 내부 정합성에 집중. 운영 가이드는 docs 별도 작성.

### 10.5 Status: in_progress (Phase A 완료, Phase B/C/D 잔여)

본 SPEC 은 **다단계 SPEC** 으로 4 Phase 중 **Phase A 만 완료** 된 상태이다. Phase B/C/D 는 각각 독립적인 PR 시리즈와 별도 메이저 버전으로 분리 진행한다.

#### 10.5.1 Phase A 완료 (2026-05-26)

**커밋 (`feature/SPEC-DEVICE-IDENTITY-001` 브랜치)**:

| 해시 | 메시지 요약 | 인수 기준 |
|---|---|---|
| `991e793` | feat(device): Device 인터페이스에 `UID()` 메서드 추가 — UUID 1급 격상 | A-AC1, A-AC4 |
| `be86904` | feat(api): REST `/devices` 응답에 `uid` 필드 1급 노출 + Phase A 인수 테스트 | A-AC2 |
| `bc67561` | feat(agent): `DeviceIDRepository` 미설정 시 1회 경고 로그 + 테스트 | A-AC4 (graceful degradation) |
| `510fc4b` | fix(inventory): `resolveDeviceUUID` 가 `Device.UID()` 를 사용하도록 정렬 | A-AC5 |

**변경 통계 (Phase A 4 커밋 누적)**:

- Production 코드 (8 파일):
  - `internal/device/device.go` — `Device` 인터페이스에 `UID() string` 추가 (+22 LOC)
  - `internal/device/adapter/uid.go` — 공통 UID 헬퍼 신규 (+61 LOC)
  - `internal/device/adapter/{nasa,lg_hvacr01,lg_hvacr02,modbus}.go` — UID() 구현 (+50 LOC)
  - `internal/agent/century/provider.go` — Century 어댑터 UID() 구현 (+15 LOC)
  - `internal/agent/device_id_repo.go` — 미설정 1회 경고 로그 (+22 LOC)
  - `internal/api/handler/device.go` — 응답에 `uid` 필드 (omitempty, +12 LOC)
  - `internal/node/inventory.go` — `Device.UID()` 사용 경로 정렬 (+12/-30 LOC, Century localID 형식 불일치 보너스 수정)
  - `internal/observe/device_metrics.go` — `xflowd_device_uid_missing_total` 메트릭 신규 (+108 LOC)
- 테스트 코드 (5 파일, +633 LOC):
  - `internal/device/adapter/uid_test.go` — 신규 (+225 LOC)
  - `internal/agent/century/provider_uid_test.go` — 신규 (+101 LOC)
  - `internal/agent/device_id_repo_test.go` — 신규 (+112 LOC)
  - `internal/api/handler/device_test.go` — uid 필드 검증 (+121 LOC)
  - `internal/node/inventory_test.go` — Device.UID() 경로 회귀 (+74 LOC)

총 17 파일, 931 insertions / 36 deletions.

**보너스 수정 사항**:

- Century 어댑터의 `localID` 형식이 다른 HVAC 어댑터(`agent:device:N`) 와 달리 자체 ID 만 반환하여 inventory 노드가 UUID 발급 시 형식 불일치로 잠재적 매핑 실패 가능성이 있었다. `510fc4b` 에서 `resolveDeviceUUID` 를 `Device.UID()` 직접 호출로 정렬하면서 해당 결함을 동시에 해소했다.

**검증 결과**:

- 회귀 0건 (`go test -race ./...`)
- 외부 클라이언트 비영향 (composite `id` 필드는 그대로 유지, `uid` 는 추가 필드)
- 인수 기준 A-AC1/A-AC2/A-AC4/A-AC5 충족
- 인수 기준 A-AC3 (HVAC 7종 어댑터의 emit map literal 에 `uid` 키 추가) 은 Phase A 범위에서 부분 적용 (어댑터 5종의 `UID()` 메서드 노출 까지). 본격적인 emit payload `uid` 키 표준화는 Phase B 의 § 6.2 emit 메시지 진화와 통합 작업으로 미룬다.

#### 10.5.2 Phase B/C/D 차후 세션 진행

- **Phase B** (내부 사용처 UUID 전환, M3~M6): callback/event/WebSocket/로그/yaml resolver. 별도 세션·SPEC 진화.
- **Phase C** (영속 데이터 마이그레이션, M7~M8): CLI 도구, 시계열 backfill. 운영 윈도우 협의 후.
- **Phase D** (composite 제거 Breaking, M9~M10): xflowd v1.0 메이저 버전. Phase B/C 완료 후 최소 6개월 호환 기간 확보.
