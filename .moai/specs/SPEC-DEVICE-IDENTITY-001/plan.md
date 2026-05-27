---
id: SPEC-DEVICE-IDENTITY-001
title: 구현 계획 - 디바이스 ID 체계 통일 (Kubernetes 패턴, 4 Phase)
version: 0.2.0
status: in_progress
created: 2026-05-25
updated: 2026-05-26
author: xtra
priority: high
---

# SPEC-DEVICE-IDENTITY-001 Plan: 디바이스 ID 체계 통일 (4 Phase 단계적 진화)

본 문서는 SPEC-DEVICE-IDENTITY-001 의 4 Phase 전략을 작업 단위로 분해하고, 각 Phase 의 위험도·롤백 전략·호환 기간 운영 가이드를 정의한다.

## TAG Traceability

- @PLAN:SPEC-DEVICE-IDENTITY-001
- 관련: @SPEC:SPEC-DEVICE-IDENTITY-001 (spec.md), @ACCEPTANCE:SPEC-DEVICE-IDENTITY-001 (acceptance.md)

---

## 0. 기본 원칙

- **점진성**: 각 Phase 는 독립 PR 시리즈로 분리되어 main 에 통합 가능해야 한다 (마지막에 거대한 빅뱅 머지 금지).
- **가역성**: Phase A/B 는 언제든 revert 가능 (코드만 변경). Phase C 는 백업으로 복원 가능. Phase D 는 별도 메이저 버전.
- **검증성**: 각 Phase 별 명확한 인수 기준 (acceptance.md) 과 자동화된 회귀 테스트.
- **관측성**: 호환 기간 동안 Deprecation 사용 빈도를 메트릭으로 노출하여 클라이언트 마이그레이션 진척도 파악.

---

## 1. Phase A: UUID 1급 격상 (비파괴)

**우선순위**: 매우 높음 (다른 모든 Phase 의 전제)
**위험도**: 낮음 (추가만, 기존 동작 유지)
**범위**: M1, M2

### 1.1 작업 분해

- **A-T1**: `internal/device/device.go` 에 `UID() string` 메서드 추가 (인터페이스 확장).
- **A-T2**: 모든 `Device` 구현체에 `UID()` 구현 — 생성 시 `agent.ResolveDeviceID(...)` 호출로 UUID 보유.
  - 대상: `internal/agent/lg/device.go` (LG HVACR-01, LGAP, LGCP), `internal/agent/samsung/device.go` (NASA), `internal/agent/century/device.go`, `internal/agent/modbus/device.go`.
  - 비-HVAC 에이전트 (예: serial, socket) 도 동일 패턴 적용.
- **A-T3**: `internal/agent/device_id_repo.go` 에서 미설정 경고 로그 추가 (graceful degradation 유지).
- **A-T4**: emit 메시지 payload 에 `uid` 필드 추가 (모든 에이전트 일관).
- **A-T5**: `GET /api/v1/devices` / `/api/v1/devices/{id}` 응답에 `uid` 필드 추가.
- **A-T6**: SPEC-INVENTORY-001 v0.2.0 의 `device_uuid` 와의 호환 정렬 — 본 Phase 에서 `uid` 도 함께 emit (Phase B 에서 키 통일 마이그레이션).
- **A-T7**: 통합 테스트 — 모든 에이전트가 UUID 를 반환하는지 verify (table-driven).
- **A-T8**: 메트릭 노출 — `xflowd_device_uid_missing_total` counter (UUID 미발급 디바이스 수, Phase D 에서 부팅 실패 전조).

### 1.2 검증

- 단위 테스트: `UID()` 호출 시 stable UUID 반환 (재호출 동일 값).
- 통합 테스트: 모든 에이전트의 emit payload 에 `uid` 필드 존재.
- 회귀 테스트: 기존 클라이언트가 `id` 필드로 작동 (호환 보장).

### 1.3 롤백 전략

- 코드만 변경되므로 git revert 로 즉시 롤백 가능.
- `UID()` 메서드는 무해 (호출되지 않으면 영향 없음).

### 1.4 완료 정의 (DoD)

- 모든 에이전트의 emit / API 응답에 `uid` 필드가 일관되게 노출됨.
- `xflowd_device_uid_missing_total` 메트릭이 정상 0 또는 운영자가 인식한 의도적 미설정 외 0.
- 통합 테스트 통과.

---

## 2. Phase B: 내부 사용처 UUID 전환 (Soft Deprecation)

**우선순위**: 높음
**위험도**: 중간 (외부 클라이언트 호환성)
**범위**: M3, M4, M5, M6

### 2.1 작업 분해

- **B-T1**: `internal/device/registry.go` — UUID 를 기본 인덱스로, `(agent, name)` 을 보조 인덱스로 변경.
- **B-T2**: 모든 callback 시그니처 변경 — `func(agentName, deviceID string)` → `func(agentName, deviceUID string)`.
  - 대상: `SetDeviceStateChangeCallback`, `SetDeviceOnlineCallback`, `SetDeviceOfflineCallback` 등 (grep 결과로 전수 조사).
- **B-T3**: WebSocket event payload 의 디바이스 식별을 `uid` 1급 필드로 변경 (composite 은 deprecated 병기).
- **B-T4**: `internal/api/handler/device.go` — REST URL resolver 진화:
  - UUID resolver (Phase A 에서 이미 추가).
  - `agent/name` resolver 신규 (Phase B).
  - composite resolver 는 alias 로 유지하되 `Deprecation: true`, `Sunset: <future-date>` 헤더 포함.
- **B-T5**: `GET /api/v1/devices:resolve?agent=X&name=Y` 엔드포인트 신규.
- **B-T6**: 로그 형식 변경 — `agent/name` 결합 표시, `device_uid` 구조화 필드.
  - `internal/logger/device_format.go` 헬퍼 신규.
  - 전역 grep + 수정 (legacy 형식 점진적 제거).
- **B-T7**: yaml 설정 resolver — `agent/name` / UUID / composite (deprecated warning) 모두 허용.
  - `internal/config/yaml_resolver.go` 신규 또는 확장.
- **B-T8**: SPEC-INVENTORY-001 v0.3.0 (예정) — `device_uuid` 키를 `uid` 로 정규화 (호환 alias 유지).
- **B-T9**: Deprecation 메트릭 — `xflowd_device_composite_use_total{source=...}` counter (composite alias 사용 빈도 추적).
- **B-T10**: 운영 문서 — Deprecation 안내 페이지 (`docs/migration/device-identity.md`).

### 2.2 검증

- 단위 테스트: yaml resolver 의 모든 형식 매칭.
- 통합 테스트: WebSocket event payload 의 `uid` 1급 필드 검증.
- E2E 테스트: 기존 composite 기반 클라이언트가 alias 로 정상 동작.
- 메트릭 관찰: Deprecation 카운터의 외부 사용 빈도 모니터링.

### 2.3 롤백 전략

- B-T1 (registry 인덱스 변경) 은 데이터 영향 없음 (메모리 인덱스만), git revert 로 즉시 롤백.
- B-T6 (로그 형식) 은 외부에 노출되지만 클라이언트 파싱 의존성이 낮으면 안전.
- callback 시그니처 변경 (B-T2) 은 외부 플러그인이 있는 경우 영향 — 호환 wrapper 제공 가능.

### 2.4 호환 기간 운영 가이드

- **6개월 호환 기간** 확보를 권장 (Phase B 시작 → Phase D 시작).
- Deprecation 헤더는 모든 composite alias 사용에 자동 포함.
- 매 분기 메트릭 리포트로 외부 클라이언트 마이그레이션 진척도 추적.
- 운영자는 Deprecation 메트릭이 0 이 될 때까지 Phase D 적용을 미룬다.

### 2.5 완료 정의 (DoD)

- 내부 모듈 간 디바이스 참조가 모두 UUID 기반.
- 외부 REST/WS 클라이언트는 Deprecation 헤더 받는 상태.
- 로그 형식이 `agent/name` 표준화.
- 운영 문서 (마이그레이션 가이드) 공개.

---

## 3. Phase C: 영속 데이터 마이그레이션 (Operational High-Risk)

**우선순위**: 높음 (Phase D 의 전제)
**위험도**: 매우 높음 (데이터 무결성)
**범위**: M7, M8

### 3.1 작업 분해

#### 3.1.1 영속 메타데이터 마이그레이션 (M7)

- **C-T1**: `cmd/xflowd-migrate/main.go` 신규 — CLI 진입점.
- **C-T2**: `cmd/xflowd-migrate/device_ids.go` 신규 — `xflowd migrate device-ids` 서브커맨드.
- **C-T3**: 마이그레이션 절차 구현:
  1. 대상 파일 백업 (`<file>.bak.<timestamp>`).
  2. composite key 를 UUID 로 변환 (DeviceIDRepository 조회).
  3. atomic rename 으로 교체.
  4. hash 검증 (엔트리 수, 메타데이터 보존).
- **C-T4**: `--dry-run` 플래그 — 변환 계획만 출력.
- **C-T5**: 매핑 실패 처리 — composite → UUID 매핑 부재 시 stderr 경고, 전체 마이그레이션은 부분 변경 없이 fail.
- **C-T6**: 통합 테스트 — 다양한 메타데이터 파일 시나리오 (정상, 깨진 파일, 누락 매핑) 검증.
- **C-T7**: 운영 문서 — 마이그레이션 절차, 백업 정책, 복구 방법.

#### 3.1.2 시계열 DB Backfill (M8)

- **C-T8**: 호환 기간 동안 두 tag 병기 동작 활성화 — 기존 emit 경로에서 `id` (composite) 와 `uid` 모두 tag 로 기록.
  - 대상: Influx writer, TSDB writer 모듈.
- **C-T9**: `cmd/xflowd-migrate/tsdb_tags.go` 신규 — `xflowd migrate tsdb-tags` 서브커맨드.
  - Influx flux 스크립트 또는 native Go 클라이언트로 backfill.
  - 진행률 stdout 노출.
- **C-T10**: 검증 쿼리 — composite tag 와 UUID tag 의 시리즈 수 일치 확인.
- **C-T11**: 모호 매핑 처리 — composite → 다중 UUID 매핑 시 skip + stderr 경고.
- **C-T12**: 운영 문서 — backfill 절차, 검증 방법, 복구 가이드.

### 3.2 검증

- 단위 테스트: 마이그레이션 도구의 dry-run / 실제 실행 / 백업 / 복원 시나리오.
- 통합 테스트: 실제 영속 파일 샘플로 round-trip 검증.
- 운영 검증: staging 환경에서 사전 리허설.

### 3.3 롤백 전략

- **영속 메타데이터**: 백업 파일 (`<file>.bak.<timestamp>`) 에서 복원.
- **시계열 DB**: composite tag 는 backfill 후에도 유지하므로 UUID tag 만 drop 하면 원상복귀 가능.
- 운영자는 마이그레이션 전 별도 백업 절차 (스냅샷, 외부 백업) 도 권장.

### 3.4 호환 기간 운영 가이드

- 영속 메타데이터 마이그레이션은 **운영 중단 윈도우 없이** 실행 가능 (read-write lock 으로 atomic rename).
- 시계열 backfill 은 트래픽 영향 적도록 chunk 단위로 실행 (--batch-size 옵션).
- Phase C 완료 후 새로 기록되는 데이터는 두 tag 모두 포함.

### 3.5 완료 정의 (DoD)

- 모든 운영 인스턴스에서 영속 메타데이터가 UUID key 로 변환됨.
- 시계열 DB 에 UUID tag 가 backfill 되어 검증 쿼리 통과.
- 마이그레이션 도구가 v0.x 의 일부로 배포됨.

---

## 4. Phase D: composite 제거 + Soft Deprecation cleanup + Frontend 전환 (xflowd v1.0 통합 메이저, v0.2.0 재정의)

**우선순위**: 높음 (greenfield 환경에서는 Phase C3 완료 직후 진행 가능)
**위험도**: 매우 높음 (Breaking Change), 단 greenfield 환경에서는 외부 호환성 리스크 없음
**범위**: M9 (확장), M10, M11 (v0.2.0 신규), M12 (v0.2.0 신규)

### 4.1 작업 분해

#### 4.1.1 composite 제거 (M9 본체)

- **D-T1**: `internal/device/device.go` — `Device.ID()` 시맨틱 변경 (UUID 반환).
- **D-T2**: `internal/api/handler/device.go` — composite alias 제거 (HTTP 404).
- **D-T3**: emit 메시지 payload 에서 deprecated `id` 필드 제거.
- **D-T4**: yaml resolver — composite (`agent:local_id`) 형식 거부 (부팅 실패, `ErrInvalidDeviceReference` 즉시 반환, Deprecation 경고 단계 생략).
- **D-T5**: `cmd/xflowd/preflight.go` 신규 — 부팅 사전 점검 명령.
  - 영속 메타데이터 composite 잔존 검사.
  - `DeviceIDRepository` 설정 검사.
  - yaml 의 composite 참조 검사.
- **D-T6**: `--skip-preflight` 플래그 — 긴급 복구 경로 (경고 후 부팅).
- **D-T7**: `cmd/xflowd-migrate/tsdb_drop_composite.go` 신규 — composite tag 정리 도구 (brownfield 사용자 보존 용도).
- **D-T8**: 마이그레이션 가이드 업데이트 — v0.x → v1.0 업그레이드 문서. greenfield/brownfield 환경별 분기 명시.
- **D-T9**: 메이저 버전 release notes — Breaking Change 명시.

#### 4.1.2 Phase B Soft Deprecation 인프라 cleanup (M9 확장, v0.2.0 신규)

- **D-T10**: V1 callback wrapper 완전 제거 — `internal/agent/device_callback.go` 의 `AdaptLegacyCallback`, `DeviceStateChangeCallback` v1 타입 / 시그니처 제거.
- **D-T11**: 5 HVAC 에이전트 (LG HVACR-01/LGAP/LGCP/NASA/Century/Modbus/Samsung) 의 `onDeviceStateChange` v1 필드 + setter (`SetDeviceStateChangeCallback` v1) 제거. V2 callback 시그니처만 유지.
- **D-T12**: REST URL composite alias dispatch + `Deprecation`/`Sunset` 헤더 핸들러 제거 — `internal/api/handler/device.go` 의 composite 인식 분기 삭제.
- **D-T13**: yaml resolver 의 composite (`agent:local_id`) parse 경로 제거 — composite 형식 받으면 즉시 에러 (D-T4 와 통합 가능). `internal/config/yaml_resolver.go`.
- **D-T14**: logger device_format 의 composite fallback 제거 — `internal/logger/device_format.go` 의 raw composite 표시 경로 삭제.
- **D-T15**: `internal/observe/device_composite_metrics.go` 전체 삭제 (또는 메트릭 정의 제거) — `xflowd_device_composite_use_total{source}` CounterVec.
- **D-T16**: `internal/observe/tsdb_dual_tag_metrics.go` 전체 삭제 (또는 deprecated marking + 비등록) — `xflowd_tsdb_dual_tag_total{state}` CounterVec.
- **D-T17**: `InfluxDBAgent` 의 `dual_tag_emit` 옵션 + augmentation 로직 제거:
  - `internal/agent/system/influxdb_agent.go` 의 `processWriteSingle`/`processWriteBatch` 에서 `augmentWriteDataWithUID` 호출 제거.
  - `internal/agent/system/influxdb_dualtag.go` 전체 삭제 (또는 deprecated noop).
  - `internal/agent/system/influxdb_config.go` 의 `DualTagEmit` + `DualTagEmitSourceKeys` + 파싱 로직 제거.
  - `cmd/xflowd/main.go` 의 `RegisterInfluxDBTypesWithResolver` → `RegisterInfluxDBTypes` 로 정리.
- **D-T18**: inventory 노드의 `device_uuid` alias 제거 — `internal/node/inventory.go` 가 `uid` 키만 emit.

#### 4.1.3 Phase C 인프라 deprecation (M12, v0.2.0 신규)

- **D-T19**: `migrate device-ids` 와 `migrate tsdb-tags` CLI 명령 deprecation 경고 추가 — 실행 시 "v1.0 환경에는 마이그레이션 대상 없음" 메시지 후 즉시 exit 0. 강제 제거하지 않고 deprecated noop 유지 (brownfield 사용자 잠재적 필요).

#### 4.1.4 Frontend UUID-first 전환 (M11, v0.2.0 신규)

- **D-T20**: web/src/ 일괄 치환 작업:
  - `useDevices`, `useDevice` 훅의 반환 타입 / 사용처에서 `id` (composite) 사용 → `uid` 우선.
  - WS 메시지 핸들러의 `device_id` payload 참조 → `uid` 사용.
  - device 표시 컴포넌트 (장비 카드, 리스트, 상세 페이지 등) 의 label / key prop / route param 갱신.
  - REST 호출 URL 형식 갱신: `/api/v1/devices/${composite}` → `/api/v1/devices/${uid}` 또는 `:resolve?agent=X&name=Y`.
  - inventory 노드 output desc / 사용 예시 / 타입 정의 갱신 — `device_uuid` → `uid`.
  - TypeScript 타입 정의에서 `device_id: string` 제거 또는 `uid: string` 으로 치환.

#### 4.1.5 운영 가이드 정비

- **D-T21**: 운영 가이드 `docs/migration/device-identity.md` 의 호환 기간 섹션 재작성:
  - greenfield 환경: 즉시 진행 가능 (외부 클라이언트 없음).
  - brownfield 환경: 기존 6개월 호환 기간 권장.
  - § 5.1 (device-ids 도구) / § 5.2 (C3 dual-tag) 사용 안내에 "v1.0 에서는 deprecated" 명시.

### 4.2 Phase D Release 절차 (v0.2.0 신규, § 4.5 신설)

**사전 체크리스트:**

- [ ] Frontend (web/src/) 가 `uid` 사용 (M11 충족 — D-T20 완료, grep `device_id` 0건)
- [ ] 모든 외부 클라이언트 (있다면) 마이그레이션 완료 (brownfield 환경 한정)
- [ ] staging 환경에서 v1.0 빌드 검증 완료 (smoke test, e2e test 통과)
- [ ] composite 미사용 확인 (`xflowd_device_composite_use_total{*} = 0`)
- [ ] preflight 명령 (`xflowd preflight`) 통과

**릴리즈 순서:**

1. **Phase B/C 인프라 cleanup 커밋** (D-T10 ~ D-T18) — 외부 동작 변경 없음, 내부 리팩터링.
2. **Frontend 치환 커밋** (D-T20) — web/src/ `device_id` → `uid` 일괄 치환. 단독 PR 로 분리 권장.
3. **Phase C 인프라 deprecation 커밋** (D-T19) — migrate 도구 noop 화.
4. **composite 제거 메이저 커밋** (D-T1 ~ D-T9) — `Device.ID()` 시맨틱 변경, REST alias 404, emit `id` 필드 제거, yaml 부팅 실패, preflight 추가. **Breaking Change 진입점.**
5. **메이저 버전 태그** (`v1.0.0`) — Conventional Commits 의 `!` 마커 또는 `BREAKING CHANGE:` footer 명시.
6. **CHANGELOG breaking 안내** — `v1.0.0` 섹션에 마이그레이션 가이드 링크 + 호환성 표.

### 4.3 검증

- E2E 테스트: composite 형식의 모든 입력 (yaml, REST URL, emit) 이 거부됨.
- 부팅 테스트: preflight 실패 시나리오 (composite 잔존, repository 미설정) 가 부팅 실패로 이어짐.
- 마이그레이션 안내 메시지가 명확하고 actionable.
- Frontend e2e: `device_id` 식별자 참조 0건 (`grep -rn 'device_id' web/src/` empty).
- 메트릭 등록 부재 확인: `xflowd_device_composite_use_total`, `xflowd_tsdb_dual_tag_total` 가 `/metrics` 응답에 없음.

### 4.4 롤백 전략

- Phase D 는 **별도 메이저 버전 (xflowd v1.0)** 으로 분리되므로 운영자는 v0.x 로 다운그레이드 가능.
- 다만 마이그레이션이 완료된 영속 데이터는 v0.x 로 자연스럽게 작동 (composite 표시는 변환 도구로 역방향 가능 — brownfield 사용자 한정, 별도 도구 제공 검토).
- greenfield 환경: 단일 frontend 만 통제하므로 frontend 도 같이 롤백하면 안전.

### 4.5 호환 기간 운영 가이드 (v0.2.0 환경별 분기)

| 환경 | Phase B/C 완료 → Phase D 진입까지 | 비고 |
|---|---|---|
| greenfield | 즉시 가능 (외부 클라이언트 없음) | xflow 자체 사용자 그룹에 해당. M11 frontend 준비 + staging 검증만 충족하면 진행. |
| brownfield with 1-2 외부 통합 | 1~2개월 (조율 가능 시) | 통합 파트너와 사전 협의. Deprecation 메트릭으로 진척 관찰. |
| brownfield with 다수 외부 | 6개월 (기존 정책) | 공개 API/dashboard 보유 시. Deprecation 메트릭 0 또는 무시 가능 수준까지 대기. |

### 4.6 완료 정의 (DoD)

- xflowd v1.0 가 composite 없이 정상 작동.
- 모든 운영 인스턴스가 preflight 통과.
- 마이그레이션 가이드 공식 문서로 공개.
- Frontend (web/src/) 의 `device_id` 참조 0건.
- Phase B Soft Deprecation 인프라 코드 부재 (`AdaptLegacyCallback`, composite alias handler, yaml composite parse, inventory `device_uuid` alias, logger composite fallback).
- Phase C dual-tag emit 코드 부재 (`influxdb_dualtag.go`, `DualTagEmit` 옵션, `xflowd_tsdb_dual_tag_total` 메트릭).
- `xflowd_device_composite_use_total` 메트릭 부재.

---

## 5. 마이그레이션 도구 설계 (Phase C 핵심)

### 5.1 `xflowd migrate device-ids` (영속 메타데이터)

**사용법:**
```
xflowd migrate device-ids \
    --metadata-dir <path> \
    --device-id-repo <path> \
    [--dry-run] \
    [--backup-suffix .bak.<timestamp>] \
    [--strict]
```

**동작 흐름:**
1. 메타데이터 파일 enumeration.
2. 각 파일에 대해:
   - 백업 생성.
   - 각 entry 의 composite key 를 UUID 로 변환 (DeviceIDRepository 조회).
   - 변환 결과를 임시 파일에 작성.
   - hash 검증 (entry 수 일치).
   - atomic rename 으로 교체.
3. 결과 리포트 출력 (변환 수, skip 수, 에러 수).

**실패 처리:**
- 매핑 실패한 composite key 가 있고 `--strict` 면 전체 fail (부분 변경 없음).
- 매핑 실패가 있고 `--strict` 가 아니면 skip 하고 경고 (변환 가능한 것만 적용).
- 어떤 단계라도 실패하면 백업에서 복원.

**dry-run 출력 예시:**
```
[DRY-RUN] Scanning /var/lib/xflow/device_metadata...
[DRY-RUN] File 1: device_meta_lg_hvacr01.json (legacy file: device_meta_lgcnp.json)
[DRY-RUN]   - "lg_icp01:81" (legacy "lgcnp:81") → "a58ba668-5741-..."
[DRY-RUN]   - "lg_icp01:82" (legacy "lgcnp:82") → "b66cb779-6852-..."
[DRY-RUN]   - "lg_icp01:83" (legacy "lgcnp:83") → SKIP (no UUID mapping)
[DRY-RUN] Summary: 2 converted, 1 skipped, 0 errors.
```

### 5.2 `xflowd migrate tsdb-tags` (시계열 백필)

**사용법:**
```
xflowd migrate tsdb-tags \
    --tsdb-url <url> \
    --bucket <name> \
    --device-id-repo <path> \
    [--batch-size 1000] \
    [--from <timestamp>] \
    [--to <timestamp>] \
    [--dry-run]
```

**동작 흐름:**
1. 시계열 DB 에서 composite tag 를 가진 series 검색.
2. batch 단위로 처리:
   - 각 composite tag 를 UUID 로 변환.
   - 동일 series 에 UUID tag 추가 (composite 유지).
   - 진행률 stdout.
3. 검증 쿼리 — composite tag count vs UUID tag count.

**검증 쿼리 예시 (Influx flux):**
```flux
from(bucket: "xflow")
  |> range(start: <from>)
  |> group(columns: ["_measurement"])
  |> filter(fn: (r) => exists r.id and not exists r.uid)
  |> count()
```

### 5.3 `xflowd preflight` (Phase D 부팅 사전 점검)

**사용법:**
```
xflowd preflight \
    --config <path> \
    [--metadata-dir <path>]
```

**점검 항목:**
1. `DeviceIDRepository` 설정 여부.
2. 영속 메타데이터에 composite key 잔존 여부.
3. yaml 설정에 composite 참조 잔존 여부.
4. (best-effort) 시계열 DB 의 UUID tag 존재 sampling.

**출력 (success):**
```
xflowd preflight: PASSED
  - DeviceIDRepository: configured (storage://device_ids.json)
  - Metadata files: 3/3 use UUID keys
  - YAML config: 0 legacy composite refs
  - TSDB sampling: UUID tags present in last 5 series sampled
```

**출력 (fail):**
```
xflowd preflight: FAILED
  ✗ Metadata file /var/lib/xflow/device_meta_lg_hvacr01.json (or legacy device_meta_lgcnp.json) contains 12 legacy composite keys.
    → Run: xflowd migrate device-ids --metadata-dir /var/lib/xflow
  ✗ YAML config /etc/xflow/flow.yaml line 42 contains "lg_icp01:81" (or legacy "lgcnp:81") composite reference.
    → Replace with "lg_hvacr01/indoor-1" or UUID.
```

---

## 6. 호환 기간 운영 가이드 (전체, v0.2.0 환경별 분기)

### 6.1 시점별 운영 행동

| 시점 | 운영 행동 |
|---|---|
| Phase A 출시 직후 | 운영 클러스터 업그레이드. emit 의 `uid` 필드 확인. 외부 클라이언트는 영향 없음. |
| Phase B 출시 직후 | Deprecation 헤더 모니터링. 외부 클라이언트 마이그레이션 안내 (brownfield 한정). 호환 기간 카운트다운 시작. |
| Phase C 출시 직후 | staging 에서 마이그레이션 리허설. 프로덕션 마이그레이션 (백업 → dry-run → 실제 실행 → 검증). |
| Phase B/C 완료 후 | 환경별 분기 — § 6.2 참조. |
| Phase D 출시 (xflowd v1.0) | 운영자는 사전에 preflight 통과 + frontend 준비 확인 후 업그레이드. |
| Phase D 후 1개월 | composite tag 정리 (`tsdb-drop-composite`, brownfield 한정). |

### 6.2 환경별 호환 기간 매트릭스 (v0.2.0 신규)

| 환경 | Phase B/C 완료 → D 진입 까지 | 비고 |
|---|---|---|
| greenfield | 즉시 가능 (외부 클라이언트 없음) | xflow 자체 사용자 그룹에 해당. M11 frontend 준비 + staging 검증만 충족하면 진행. |
| brownfield with 1-2 외부 통합 | 1~2개월 (조율 가능 시) | 통합 파트너와 사전 협의. Deprecation 메트릭으로 진척 관찰. |
| brownfield with 다수 외부 | 6개월 (기존 정책) | 공개 API/dashboard 보유 시. Deprecation 메트릭 0 또는 무시 가능 수준까지 대기. |

---

## 7. 기술 부채 / 후속 작업

- **에이전트 rename UX**: 별도 SPEC (예: SPEC-AGENT-RENAME-001) 으로 분리. 본 SPEC 완료 후 자연스럽게 가능해짐.
- **디바이스 cross-agent migration**: Phase D 이후 별도 기능.
- **MQTT bridge / 외부 통합 가이드**: docs 별도 작성.
- **프론트엔드 v1.0 출시**: Phase B 호환 alias 활용 후 v1.0 에서 UUID-first UI 전환.

---

## 8. Status: in_progress (Phase A + B + C1/C2/C3 완료, Phase D xflowd v1.0 통합 메이저 잔여)

Phase A ~ C3 완료 (31 커밋 누적 in `feature/SPEC-DEVICE-IDENTITY-001` 브랜치).
Phase D 는 v0.2.0 재정의에 따라 **xflowd v1.0 통합 메이저 단일 릴리즈** 로 진행 — composite 제거 + Phase B Soft Deprecation 인프라 cleanup + Phase C 인프라 deprecation + Frontend UUID-first 전환 (D-T1 ~ D-T21).

각 Phase 의 완료 후 다음 Phase 의 작업 분해를 재검토하여 plan.md 갱신.
