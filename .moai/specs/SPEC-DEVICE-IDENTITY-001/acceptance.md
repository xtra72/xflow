---
id: SPEC-DEVICE-IDENTITY-001
title: 인수 기준 - 디바이스 ID 체계 통일 (4 Phase)
version: 0.2.0
status: in_progress
created: 2026-05-25
updated: 2026-05-26
author: xtra
priority: high
---

# SPEC-DEVICE-IDENTITY-001 Acceptance: 디바이스 ID 체계 통일 인수 기준

본 문서는 SPEC-DEVICE-IDENTITY-001 의 인수 기준을 정의한다. 각 Phase 별 검증 가능한 deliverable 과 Given-When-Then 시나리오, 그리고 마이그레이션 데이터 무결성 검증 시나리오를 포함한다.

## TAG Traceability

- @ACCEPTANCE:SPEC-DEVICE-IDENTITY-001
- 관련: @SPEC:SPEC-DEVICE-IDENTITY-001 (spec.md), @PLAN:SPEC-DEVICE-IDENTITY-001 (plan.md)

---

## 1. Phase A 인수 기준 (M1, M2)

### A-AC1: `Device.UID()` 메서드 노출 및 UUID 보장

- **Given**: xflow 인스턴스가 v0.18.6+ 의 `DeviceIDRepository` 설정으로 부팅됨.
- **When**: 임의의 에이전트 (LGCNP/LGAP/LGCP/NASA/Century/Modbus/Samsung) 가 디바이스를 등록.
- **Then**:
  - 해당 디바이스의 `UID()` 호출은 빈 문자열이 아닌 UUID v4 형식 문자열을 반환한다.
  - 같은 디바이스에 대한 `UID()` 의 반복 호출은 항상 동일한 UUID 를 반환한다 (idempotent).
  - 다른 디바이스의 `UID()` 는 서로 다른 UUID 를 반환한다 (uniqueness).
- **검증 방법**: 통합 테스트 — 7개 HVAC 에이전트 + 비-HVAC 에이전트 각각에 대해 UUID 발급 및 일관성 검증.

### A-AC2: `DeviceIDRepository` 미설정 시 graceful degradation

- **Given**: `DeviceIDRepository` 가 미설정 (`SetDeviceIDRepository` 호출되지 않음).
- **When**: 에이전트가 디바이스를 등록.
- **Then**:
  - 부팅 시 경고 로그가 stderr 에 출력됨 (`"WARN: DeviceIDRepository not configured, UID() will return empty"`).
  - 디바이스의 `UID()` 호출은 빈 문자열을 반환한다 (graceful degradation).
  - 디바이스 등록 자체는 실패하지 않는다 (Phase A 의 graceful 동작).
  - `xflowd_device_uid_missing_total` 메트릭이 디바이스 수만큼 증가.
- **검증 방법**: 단위 테스트 — repository nil 상태에서 device 생성 후 UID() 검증.

### A-AC3: emit payload 의 `uid` 필드 노출

- **Given**: HVAC 에이전트가 정상 작동 중이며 `DeviceIDRepository` 설정됨.
- **When**: 에이전트가 디바이스 상태 변경을 emit.
- **Then**:
  - emit payload JSON 에 `uid` 필드가 string (UUID) 값으로 존재한다.
  - emit payload JSON 에 기존 `id` 필드 (composite) 도 함께 존재한다 (병기).
  - 두 필드는 동일 디바이스를 가리키며 일관된다.
- **검증 방법**: E2E 테스트 — 에이전트 emit 을 capture 하여 payload 의 두 필드 모두 검증.

### A-AC4: API 응답의 `uid` 필드 노출

- **Given**: xflow 인스턴스가 정상 작동.
- **When**: 클라이언트가 `GET /api/v1/devices` 또는 `GET /api/v1/devices/{composite_id}` 호출.
- **Then**:
  - 응답 JSON 의 각 디바이스 객체에 `uid` 필드 (UUID) 가 포함된다.
  - 응답 JSON 의 각 디바이스 객체에 기존 `id` 필드 (composite) 도 포함된다.
- **검증 방법**: 통합 테스트 — 실제 HTTP 응답에서 두 필드 검증.

### A-AC5: SPEC-INVENTORY-001 v0.2.0 호환 정렬

- **Given**: inventory 노드가 `devices` source 로 설정됨.
- **When**: 인벤토리 emit 발생.
- **Then**:
  - SPEC-INVENTORY-001 v0.2.0 의 `device_uuid` 필드와 본 SPEC 의 `uid` 필드가 동일 UUID 값을 가진다.
  - Phase A 단계에서는 두 키 모두 emit (호환 alias).
- **검증 방법**: 통합 테스트 — inventory emit payload 의 두 필드 일치성 검증.

---

## 2. Phase B 인수 기준 (M3, M4, M5, M6)

### B-AC1: callback 시그니처 UUID 전달

- **Given**: Phase B 가 배포된 xflow 인스턴스.
- **When**: 디바이스 상태 변경이 발생하여 등록된 callback 이 호출됨.
- **Then**:
  - callback 의 두 번째 인자 (deviceUID) 가 UUID v4 형식이다.
  - callback 의 두 번째 인자가 composite 형식 (`agent:local_id`) 이 아니다.
- **검증 방법**: 단위 테스트 — mock callback 으로 인자 검증.

### B-AC2: WebSocket event payload UUID 1급

- **Given**: 클라이언트가 WebSocket 으로 `device.state_changed` event 구독.
- **When**: 디바이스 상태 변경 발생.
- **Then**:
  - event payload 의 디바이스 식별 필드는 `uid` (UUID) 가 1급 필드.
  - 호환 기간 동안 `id` (composite) 도 병기 (Phase B 단계).
  - Phase D 에서는 `uid` 만.
- **검증 방법**: E2E 테스트 — WebSocket client 로 event 수신 후 payload 검증.

### B-AC3: REST URL UUID resolver

- **Given**: 디바이스 `(agent=lgcnp, name=indoor-1, uid=<uuid>)`.
- **When**: 클라이언트가 다음 URL 들 호출:
  - `GET /api/v1/devices/<uuid>`
  - `GET /api/v1/devices/lgcnp/indoor-1`
  - `GET /api/v1/devices/lgcnp:81` (composite alias)
- **Then**:
  - 처음 두 URL 은 정상 응답 (HTTP 200).
  - 세 번째 URL (composite) 은 정상 응답이되 `Deprecation: true` 와 `Sunset: <date>` 헤더 포함.
  - 모두 동일한 디바이스 객체 반환.
- **검증 방법**: 통합 테스트 — 세 URL 의 응답 비교 및 헤더 검증.

### B-AC4: name 기반 명시 resolver

- **Given**: 디바이스 `(agent=lgcnp, name=indoor-1)`.
- **When**: 클라이언트가 `GET /api/v1/devices:resolve?agent=lgcnp&name=indoor-1` 호출.
- **Then**:
  - 응답에 해당 디바이스의 UUID 와 메타데이터가 포함된다.
- **검증 방법**: 통합 테스트.

### B-AC5: 잘못된 reference 의 404

- **Given**: 존재하지 않는 reference.
- **When**: 클라이언트가 `GET /api/v1/devices/invalid-xyz-format` 호출.
- **Then**:
  - HTTP 404 응답.
  - 에러 메시지가 명시적 (`"device reference invalid-xyz-format not found; expected UUID, agent/name, or legacy agent:local_id"`).
- **검증 방법**: 통합 테스트.

### B-AC6: 로그 형식 `agent/name`

- **Given**: Phase B 가 배포된 xflow 인스턴스.
- **When**: 디바이스 관련 로그 라인 출력.
- **Then**:
  - 디바이스 표시가 `agent/name` 결합 형식 (예: `device "lgcnp/indoor-1" offline`).
  - UUID 가 필요한 경우 별도 구조화 필드 (`device_uid=<uuid>`).
  - raw composite (`lgcnp:81`) 의 직접 로그 표시는 점진적으로 제거됨.
- **검증 방법**: 통합 테스트 — 로그 capture 후 형식 검증.

### B-AC7: yaml 의 두 형식 허용

- **Given**: yaml 설정 파일이 다음 디바이스 참조를 포함:
  ```yaml
  flow:
    pinned:
      - "lgcnp/indoor-1"
      - "a58ba668-5741-..."
      - "lgcnp:81"
  ```
- **When**: xflow 부팅.
- **Then**:
  - 부팅 성공.
  - 세 참조 모두 동일 UUID 로 정규화됨.
  - composite 참조 (`lgcnp:81`) 사용에 대한 경고 로그.
  - 정상 작동 (호환).
- **검증 방법**: 통합 테스트 — yaml 파싱 및 정규화 검증.

### B-AC8: yaml 의 매칭 실패 시 부팅 실패

- **Given**: yaml 에 매칭 불가능한 참조 (`"invalid-format-xyz"`).
- **When**: xflow 부팅.
- **Then**:
  - 부팅 실패.
  - 에러 메시지에 `ErrInvalidDeviceReference` 와 위치 정보 (yaml 파일 경로, 라인 번호) 포함.
- **검증 방법**: 통합 테스트.

### B-AC9: Deprecation 메트릭 노출

- **Given**: 클라이언트가 composite alias 로 REST 호출.
- **When**: composite alias 사용 발생.
- **Then**:
  - `xflowd_device_composite_use_total{source="rest"}` counter 가 증가.
  - `/metrics` 엔드포인트에서 노출.
- **검증 방법**: 통합 테스트 — 메트릭 카운터 검증.

---

## 3. Phase C 인수 기준 (M7, M8)

### C-AC1: 영속 메타데이터 dry-run

- **Given**: 영속 메타데이터 파일에 100개의 composite key 엔트리 존재.
- **When**: `xflowd migrate device-ids --dry-run --metadata-dir <path>` 실행.
- **Then**:
  - stdout 에 변환 계획 출력 (100개 매핑).
  - 실제 파일은 변경되지 않는다.
  - 종료 코드 0.
- **검증 방법**: 통합 테스트 — 파일 hash 비교 (변경 없음).

### C-AC2: 영속 메타데이터 정상 마이그레이션

- **Given**: 영속 메타데이터 파일에 100개의 composite key 엔트리 존재. DeviceIDRepository 에 모든 매핑 존재.
- **When**: `xflowd migrate device-ids --metadata-dir <path>` 실행.
- **Then**:
  - 백업 파일 (`<file>.bak.<timestamp>`) 생성.
  - 원본 파일의 모든 키가 UUID 로 변환됨.
  - 엔트리 수 변화 없음 (100개).
  - 각 엔트리의 메타데이터 (tags, location 등) 완전 보존.
  - hash 검증 통과 (변환 전후 메타데이터 hash 동일).
  - 종료 코드 0.
- **검증 방법**: 통합 테스트 — 변환 전후의 메타데이터 entry-by-entry 비교.

### C-AC3: 매핑 실패 시 strict 모드

- **Given**: 영속 메타데이터에 composite key 가 100개, DeviceIDRepository 에 95개만 매핑 (5개 누락).
- **When**: `xflowd migrate device-ids --strict --metadata-dir <path>` 실행.
- **Then**:
  - 마이그레이션 실패 (전체 fail, 부분 변경 없음).
  - stderr 에 5개 누락된 composite key 목록.
  - 원본 파일 변경 없음.
  - 종료 코드 비-0.
- **검증 방법**: 통합 테스트.

### C-AC4: 매핑 실패 시 non-strict 모드

- **Given**: 위와 동일한 상황.
- **When**: `xflowd migrate device-ids --metadata-dir <path>` (strict 없음).
- **Then**:
  - 95개 엔트리 변환 성공, 5개 skip.
  - stderr 에 skip 된 5개 경고.
  - 종료 코드 0 또는 부분 성공 코드.
- **검증 방법**: 통합 테스트.

### C-AC5: 마이그레이션 중 실패 시 복원

- **Given**: 마이그레이션 중 atomic rename 실패 (예: 디스크 full).
- **When**: 도구 실행 중 에러 발생.
- **Then**:
  - 원본 파일이 백업으로부터 자동 복원됨 (또는 백업이 보존되어 수동 복원 가능).
  - stderr 에 명확한 에러 메시지.
- **검증 방법**: 통합 테스트 — 디스크 full mock 으로 시나리오 재현.

### C-AC6: 시계열 backfill 진행률 출력

- **Given**: 시계열 DB 에 1,000,000 개의 composite tag series 존재.
- **When**: `xflowd migrate tsdb-tags --batch-size 1000` 실행.
- **Then**:
  - stdout 에 진행률 (`Processed 50000/1000000 series (5%)...`) 주기적 출력.
  - 완료 시 검증 쿼리 결과 출력.
- **검증 방법**: 통합 테스트 — Influx mock 서버 또는 staging 환경.

### C-AC7: 시계열 backfill 검증

- **Given**: backfill 완료.
- **When**: 검증 쿼리 실행 (composite tag count vs UUID tag count).
- **Then**:
  - 두 count 가 일치 (또는 명시적 차이로 skip 된 series 수 만큼).
  - 모든 measurement 에서 일관됨.
- **검증 방법**: Influx flux 쿼리 자동 실행.

### C-AC8: 호환 기간 동안 두 tag 병기

- **Given**: Phase C 가 배포된 xflow 인스턴스.
- **When**: 새 디바이스 데이터가 시계열 DB 에 기록됨.
- **Then**:
  - 동일 series 에 `id=<composite>` 와 `uid=<uuid>` 두 tag 모두 존재.
- **검증 방법**: 통합 테스트 — Influx 쿼리로 tag 확인.

---

## 4. Phase D 인수 기준 (M9, M10)

### D-AC1: `Device.ID()` UUID 반환 (Breaking)

- **Given**: xflowd v1.0+ 부팅 완료.
- **When**: 임의의 디바이스의 `ID()` 호출.
- **Then**:
  - 반환 값이 UUID v4 형식.
  - composite 형식 (`agent:local_id`) 이 아님.
- **검증 방법**: 단위 테스트.

### D-AC2: REST composite alias 거부

- **Given**: xflowd v1.0+ 정상 작동.
- **When**: 클라이언트가 `GET /api/v1/devices/lgcnp:81` 호출.
- **Then**:
  - HTTP 404 응답.
  - 에러 메시지가 마이그레이션 안내 포함 (`"composite reference is removed; use UUID or agent/name"`).
- **검증 방법**: 통합 테스트.

### D-AC3: emit 의 deprecated `id` 필드 제거

- **Given**: xflowd v1.0+ 의 에이전트.
- **When**: 디바이스 emit 발생.
- **Then**:
  - emit payload 에 `uid` 만 존재 (composite `id` 필드 부재).
- **검증 방법**: E2E 테스트.

### D-AC4: yaml composite 거부

- **Given**: yaml 설정에 `pinned: ["lgcnp:81"]` 포함.
- **When**: xflowd v1.0+ 부팅.
- **Then**:
  - 부팅 실패.
  - 에러 메시지: `"yaml: legacy composite reference 'lgcnp:81' is removed in v1.0; use 'lgcnp/indoor-1' or UUID"`.
- **검증 방법**: 통합 테스트.

### D-AC5: 영속 메타데이터 composite 잔존 시 부팅 실패

- **Given**: 영속 메타데이터에 composite key 가 잔존 (마이그레이션 미수행).
- **When**: xflowd v1.0+ 부팅.
- **Then**:
  - 부팅 실패.
  - 에러 메시지에 마이그레이션 명령 안내 (`"run 'xflowd migrate device-ids' before starting xflowd v1.0+"`).
- **검증 방법**: 통합 테스트.

### D-AC6: `DeviceIDRepository` 미설정 시 부팅 실패

- **Given**: `DeviceIDRepository` 미설정.
- **When**: xflowd v1.0+ 부팅.
- **Then**:
  - 부팅 실패.
  - 에러 메시지: `"DeviceIDRepository is required in v1.0+; configure persistent storage"`.
- **검증 방법**: 통합 테스트.

### D-AC7: `xflowd preflight` 정상 PASS

- **Given**: 모든 마이그레이션 완료, DeviceIDRepository 설정.
- **When**: `xflowd preflight --config <path>` 실행.
- **Then**:
  - 종료 코드 0.
  - stdout 에 `PASSED` 와 각 점검 항목의 상태.
- **검증 방법**: 통합 테스트.

### D-AC8: `xflowd preflight` 실패 시 actionable 메시지

- **Given**: 영속 메타데이터에 12개의 composite key 잔존.
- **When**: `xflowd preflight` 실행.
- **Then**:
  - 종료 코드 비-0.
  - stdout 에 `FAILED` 와 각 실패 항목에 대한 명시적 수정 명령 (`"Run: xflowd migrate device-ids --metadata-dir ..."`).
- **검증 방법**: 통합 테스트.

### D-AC9: `--skip-preflight` 긴급 복구 경로

- **Given**: preflight 가 실패하는 상태.
- **When**: `xflowd --skip-preflight` 부팅.
- **Then**:
  - 경고 로그 출력 (`"WARN: preflight skipped, system may behave unexpectedly"`).
  - 부팅 계속 (긴급 복구 경로).
- **검증 방법**: 통합 테스트.

---

## 4-1. Phase D 인수 기준 (v0.2.0 신규 — Soft Deprecation cleanup + Frontend + 인프라 deprecation)

### D-AC8: V1 callback wrapper 코드 부재

- **Given**: Phase D (xflowd v1.0) 빌드.
- **When**: `internal/agent/device_callback.go` 와 5 HVAC 에이전트 파일을 grep 검증.
- **Then**:
  - `AdaptLegacyCallback` 함수 정의 부재.
  - `DeviceStateChangeCallback` v1 타입 정의 부재 (V2 만 존재).
  - 컴파일 가능 (호환 wrapper 없이도 모든 호출처 정상 작동).
- **검증 방법**: `grep -rn 'AdaptLegacyCallback' internal/` empty + `go build ./...` 성공.

### D-AC9 (v0.2.0): 5 HVAC 에이전트 V1 callback 필드 부재

- **Given**: Phase D 빌드.
- **When**: 5 HVAC 에이전트 (LGCNP/LGAP/LGCP/NASA/Century/Modbus/Samsung) 의 구조체 정의를 grep 검증.
- **Then**:
  - `onDeviceStateChange` v1 필드 부재 또는 V2 시그니처 (`func(agent, uid string, ...)`) 만 존재.
  - `SetDeviceStateChangeCallback` v1 메서드 부재.
- **검증 방법**: `grep -rn 'onDeviceStateChange' internal/agent/{lg,samsung,century,modbus}/` 결과가 V2 만 포함.

### D-AC10: REST composite alias 404

- **Given**: xflowd v1.0+ 정상 작동.
- **When**: 클라이언트가 `GET /api/v1/devices/lgcnp:81` 호출.
- **Then**:
  - HTTP 404 응답 (composite alias dispatch 자체가 제거됨).
  - `Deprecation` 헤더 부재 (alias handler 가 없으므로).
- **검증 방법**: 통합 테스트.

### D-AC11: yaml composite 즉시 부팅 실패

- **Given**: yaml 에 `pinned: ["lgcnp:81"]` 포함.
- **When**: xflowd v1.0+ 부팅.
- **Then**:
  - 부팅 즉시 실패 (Deprecation 경고 단계 없이).
  - 에러: `ErrInvalidDeviceReference` 와 yaml 위치 정보.
- **검증 방법**: 통합 테스트.

### D-AC12: 로그 composite 형식 부재

- **Given**: xflowd v1.0+ 정상 작동 중.
- **When**: 디바이스 관련 로그 라인 capture.
- **Then**:
  - `device="lgcnp:81"` 또는 `device_id="lgcnp:81"` 형식의 raw composite 로그 라인 부재.
  - `agent/name` 형식 또는 UUID 만 표시.
- **검증 방법**: 통합 테스트 — 로그 capture 후 grep.

### D-AC13: `xflowd_device_composite_use_total` 메트릭 부재

- **Given**: xflowd v1.0+ 정상 작동.
- **When**: `GET /metrics` 호출.
- **Then**:
  - 응답 본문에 `xflowd_device_composite_use_total` 메트릭 family 부재.
- **검증 방법**: 통합 테스트.

### D-AC14: `xflowd_tsdb_dual_tag_total` 메트릭 부재

- **Given**: xflowd v1.0+ 정상 작동, InfluxDBAgent 활성.
- **When**: `GET /metrics` 호출.
- **Then**:
  - 응답 본문에 `xflowd_tsdb_dual_tag_total` 메트릭 family 부재.
- **검증 방법**: 통합 테스트.

### D-AC15: InfluxDB write 시 composite tag 미부착

- **Given**: xflowd v1.0+ 의 InfluxDBAgent.
- **When**: 디바이스 데이터 write.
- **Then**:
  - point 의 tags 에 `uid=<uuid>` 만 존재.
  - `device_id=<composite>` tag 자동 부착 안 됨.
  - `DualTagEmit` 설정이 yaml 에 있어도 무시되거나 부팅 실패 (옵션 제거 시).
- **검증 방법**: 통합 테스트 — Influx mock 으로 written points 검증.

### D-AC16: inventory emit `device_uuid` 키 부재

- **Given**: xflowd v1.0+ 의 inventory 노드.
- **When**: inventory emit 발생.
- **Then**:
  - emit payload 의 각 디바이스 객체에 `uid` 키만 존재.
  - `device_uuid` 키 부재.
- **검증 방법**: 통합 테스트.

### D-AC17: Frontend `device_id` 참조 부재 (M11)

- **Given**: xflow v1.0 frontend (web/src/).
- **When**: 다음 검증 수행:
  - `grep -rn 'device_id' web/src/` → 0건.
  - 디바이스 상세 페이지 라우트 확인 → `/devices/<uuid>` 형식.
  - inventory 노드 output desc 확인 → `device_uuid` 참조 제거 확인.
  - TypeScript 타입 정의 확인 → `Device` 인터페이스에 `id` (composite) 필드 부재.
- **Then**:
  - 모든 검증 통과.
- **검증 방법**: 수동 grep + e2e 라우트 테스트 + TypeScript 컴파일 검증.

### D-AC18: migrate 명령 deprecated noop

- **Given**: xflowd v1.0+ 빌드.
- **When**: `xflowd migrate device-ids --metadata-dir <path>` 또는 `xflowd migrate tsdb-tags ...` 실행.
- **Then**:
  - stdout 에 "v1.0 환경에는 마이그레이션 대상 없음 — composite 형식은 이미 제거되었습니다" 메시지.
  - exit code 0.
  - 실제 마이그레이션 동작 수행 안 함.
- **검증 방법**: CLI 통합 테스트.

---

## 5. 마이그레이션 데이터 무결성 검증 시나리오

### MIG-AC1: 영속 메타데이터 round-trip 검증

- **Given**: 메타데이터 파일 V1 (composite keys, 100 entries with rich metadata).
- **When**:
  1. `xflowd migrate device-ids` 실행 → V2 (UUID keys).
  2. (가상) 역방향 변환 도구로 V2 → V3.
- **Then**:
  - V3 의 entry 수 = V1 의 entry 수.
  - V3 의 각 entry 메타데이터 = V1 의 대응 entry 메타데이터 (tags, location, group, labels, pinned 등 모두 일치).
- **검증 방법**: 통합 테스트 — Go 의 `reflect.DeepEqual` 또는 `go-cmp` 사용.

### MIG-AC2: 시계열 backfill 후 쿼리 일치

- **Given**: 시계열 DB 의 backfill 완료.
- **When**: 동일 시간 범위에 대해 두 쿼리 실행:
  - `from(...) |> filter(fn: (r) => r.id == "lgcnp:81")`
  - `from(...) |> filter(fn: (r) => r.uid == "a58ba668-...")`
- **Then**:
  - 두 쿼리의 결과 series 가 동일 (point count 일치, 값 일치).
- **검증 방법**: 자동 검증 쿼리 (`xflowd migrate tsdb-tags --verify-after`).

### MIG-AC3: 마이그레이션 atomicity

- **Given**: 영속 메타데이터 파일.
- **When**: 마이그레이션 중 SIGTERM 으로 중단.
- **Then**:
  - 원본 파일이 정상 상태 (백업에서 복원 또는 atomic rename 미실행으로 변경 없음).
  - 부분 변경 없음 (모든 entry 가 composite 이거나 모든 entry 가 UUID).
- **검증 방법**: 통합 테스트 — kill -SIGTERM 시뮬레이션.

### MIG-AC4: 마이그레이션 idempotency

- **Given**: 이미 UUID key 로 변환된 메타데이터 파일.
- **When**: `xflowd migrate device-ids` 재실행.
- **Then**:
  - 변환 대상이 없음을 인식하고 no-op 으로 종료 (또는 idempotent 한 동작).
  - 종료 코드 0.
- **검증 방법**: 통합 테스트.

### MIG-AC5: 백업의 복원 가능성

- **Given**: 마이그레이션 완료, 백업 파일 (`<file>.bak.<ts>`) 존재.
- **When**: 운영자가 백업 파일로 수동 복원 (`cp <file>.bak.<ts> <file>`).
- **Then**:
  - 복원된 파일이 마이그레이션 전 상태와 byte-for-byte 동일.
- **검증 방법**: hash 비교.

---

## 6. 회귀 방지 시나리오

### REG-AC1: 호환 기간 동안 기존 클라이언트의 정상 작동

- **Given**: Phase B 까지 배포된 xflow 인스턴스. 외부 클라이언트가 v0.x 의 composite 사용.
- **When**: 외부 클라이언트가 기존 REST URL / WebSocket event 형식으로 호출.
- **Then**:
  - 모든 호출 정상 작동.
  - Deprecation 헤더만 추가됨 (기능 변화 없음).
- **검증 방법**: E2E 테스트 — 외부 클라이언트 시뮬레이션.

### REG-AC2: SPEC-INVENTORY-001 호환

- **Given**: SPEC-INVENTORY-001 v0.2.0 의 `device_uuid` 사용 노드 설정.
- **When**: Phase A/B 가 배포된 후 inventory emit.
- **Then**:
  - `device_uuid` 필드가 정상 emit (값은 본 SPEC 의 `uid` 와 동일).
  - 사용자 노드의 작동 변화 없음.
- **검증 방법**: 통합 테스트.

### REG-AC3: HVAC 에이전트의 emit 시계열 연속성

- **Given**: Phase A 배포 전후의 시계열 DB.
- **When**: Phase A 배포 후 동일 디바이스의 데이터 기록.
- **Then**:
  - 시계열 DB 에서 동일 series 가 단절 없이 연속 (composite tag 가 유지되므로).
  - 새 데이터에 `uid` tag 도 추가됨 (Phase A 단계는 기록만, Phase C 에서 backfill).
- **검증 방법**: Influx 쿼리.

---

## 7. 성능 인수 기준

### PERF-AC1: `UID()` 호출 오버헤드

- **Given**: 디바이스의 `UID()` 가 캐시된 UUID 를 반환.
- **When**: 100만 회 `UID()` 호출.
- **Then**:
  - 평균 호출당 100 ns 이하 (단순 map lookup 또는 field access).
- **검증 방법**: Go benchmark.

### PERF-AC2: yaml resolver 성능

- **Given**: 1000개의 디바이스 참조 (혼합 형식).
- **When**: yaml 파싱.
- **Then**:
  - 전체 정규화 100 ms 이하.
- **검증 방법**: Go benchmark.

### PERF-AC3: 마이그레이션 도구 처리량

- **Given**: 영속 메타데이터 파일 (10,000 entries).
- **When**: `xflowd migrate device-ids` 실행.
- **Then**:
  - 5초 이내 완료 (DeviceIDRepository 가 in-memory 또는 빠른 file-backed).
- **검증 방법**: 통합 테스트 with timing.

---

## 8. Definition of Done (Phase 별)

### Phase A DoD
- [ ] A-AC1 ~ A-AC5 모두 통과.
- [ ] 통합 테스트 커버리지 85% 이상.
- [ ] 운영 클러스터 1개 이상 배포 후 메트릭 검증.

### Phase B DoD
- [ ] B-AC1 ~ B-AC9 모두 통과.
- [ ] REG-AC1, REG-AC2 회귀 테스트 통과.
- [ ] Deprecation 메트릭 노출 및 운영 대시보드 등록.
- [ ] 외부 클라이언트 마이그레이션 안내 공식 문서 공개.

### Phase C DoD
- [ ] C-AC1 ~ C-AC8 모두 통과.
- [ ] MIG-AC1 ~ MIG-AC5 모든 무결성 시나리오 통과.
- [ ] PERF-AC3 처리량 기준 충족.
- [ ] staging 환경 마이그레이션 리허설 1회 이상 성공.
- [ ] 마이그레이션 가이드 공식 문서 공개.

### Phase D DoD (v0.2.0 갱신 — xflowd v1.0 통합 메이저)
- [ ] D-AC1 ~ D-AC7, D-AC9 (긴급 복구 경로) 모두 통과 (composite 제거 기본).
- [ ] D-AC8, D-AC9 (v0.2.0), D-AC10 ~ D-AC18 모두 통과 (v0.2.0 신규 — Soft Deprecation cleanup + Frontend + 인프라 deprecation).
- [ ] **Frontend `device_id` 참조 0건** (M11 충족, D-AC17).
- [ ] **Phase B Soft Deprecation 인프라 코드 부재** (D-AC8, D-AC9, D-AC10, D-AC11, D-AC12, D-AC16):
  - V1 callback wrapper / 5 HVAC 에이전트 v1 callback 필드 / REST composite alias / yaml composite parse / 로그 composite 형식 / inventory device_uuid alias 모두 부재.
- [ ] **Phase C dual-tag emit 코드 부재** (D-AC14, D-AC15):
  - `influxdb_dualtag.go` / `DualTagEmit` 옵션 / `xflowd_tsdb_dual_tag_total` 메트릭 / composite tag 자동 부착 모두 부재.
- [ ] **`xflowd_device_composite_use_total` 메트릭 부재** (D-AC13).
- [ ] **migrate 명령 deprecated noop** (D-AC18).
- [ ] **greenfield 환경 가정 sanity check**: composite 메트릭 검증이 미필요한 환경임을 운영 가이드에 명시 (`A6` 가정 충족 확인).
- [ ] 환경별 호환 기간 충족:
  - greenfield: M11 frontend 준비 + staging 검증 완료 직후 진행 가능.
  - brownfield 1-2 외부 통합: 1~2개월 호환 기간 경과.
  - brownfield 다수 외부: 6개월 호환 기간 경과 + Deprecation 메트릭이 0 또는 무시 가능 수준.
- [ ] xflowd v1.0 release notes 의 Breaking Change 명시.
- [ ] 운영자 사전 통보 및 마이그레이션 안내 (brownfield 환경 한정).

---

## 9. Status: in_progress (Phase A + B + C1/C2/C3 완료, Phase D xflowd v1.0 통합 메이저 잔여)

각 Phase 별로 본 인수 기준이 자동화된 테스트로 변환되어야 하며, CI 에서 모든 시나리오가 검증되어야 한다.

v0.2.0 갱신 사항:
- D-AC8 ~ D-AC18 신설 (Phase D 의 Soft Deprecation cleanup + Frontend + 인프라 deprecation 인수 기준).
- Phase D DoD 환경별 분기 (greenfield/brownfield) 명시.
- `A6` 가정 (greenfield 환경 확정) 에 따른 sanity check 항목 추가.
