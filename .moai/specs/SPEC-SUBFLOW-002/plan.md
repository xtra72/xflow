# SPEC-SUBFLOW-002 구현 계획 (plan.md)

> 서브플로우 의미론 재설계 — flow-node = 공유 인스턴스 연결(로컬 라이브 브리지)
> 본 문서는 PLAN 단계 산출물이다. 코드는 포함하지 않으며 기술 접근/마일스톤/위험을 정의한다.

> **상태(2026-06-12)**: 계획(planned). OPEN QUESTIONS 전부 RESOLVED(사용자 승인 — spec §5.8). `/moai run SPEC-SUBFLOW-002` 으로 마일스톤 M1~M6 증분 구현한다.

## 1. 기술 접근

### 1.1 핵심 전략 — 원격 브리지 인프라의 in-process 일반화

본 SPEC 의 가장 중요한 통찰은 **로컬 `shared` 모드가 새로운 메커니즘이 아니라, SPEC-SUBFLOW-001 v1.3 의 원격 라이브 브리지를 transport 만 바꿔 일반화한 것**이라는 점이다.

- 원격(`remote://`): `FlowBridgeOpener` → `remote.Server.OpenBridge` → WS 세션 위 `bridge_input`/`bridge_output`.
- 로컬(`shared`): `FlowBridgeOpener` → in-process opener → 엔진 `flows` 맵의 실행 인스턴스 경계 포트에 직접 attach.

두 경로 모두 동일한 `managerBridgeController` 자가치유 supervisor(`start`/`supervise`/`reopen`/`runGeneration`/`pumpOutputs`/`pumpStatus`)를 사용한다. 이미 구현된 supervisor 의 "오프라인 시작 → 도착 시 자동 open → 드롭 시 bounded 백오프 재open" 단일 경로가 **본 SPEC 의 로컬 생명주기(L01~L07)와 의미론이 정확히 일치**한다. 따라서 위험을 최소화하며 재사용한다.

### 1.2 flow-node mode config (`internal/node/flow_node.go`)

- `flowNodeConfigFlowID = "flow_id"` 옆에 `flowNodeConfigMode = "mode"` 상수 추가.
- 값: `shared`|`instance`. 미지정/빈 값 → `shared` 정규화.
- 정규화는 배포 분기 진입 직전 단일 지점에서 수행하여 일관성 보장(M02).
- `flow_id` 가 `remote://` 면 `mode` 와 직교(원격은 종류상 브리지). bare id 일 때만 `mode` 가 `shared`/`instance` 결정.

### 1.3 배포 분기 (`internal/api/service/subflow_expand.go`)

- 현재 `expandSubflowsRec` 의 flow-node 분류:
  - 비-flow-node → 보존.
  - LOCAL(bare id) → `localFlowNodeIDs`(인라인 확장 대상).
  - REMOTE(`remote://`) → LIVE NODE 보존(미확장).
- 변경: LOCAL flow-node 를 `mode` 로 재분류한다.
  - `mode=instance` → 기존 `localFlowNodeIDs`(인라인 확장).
  - `mode=shared` → **REMOTE 와 동일하게 LIVE NODE 보존**(미확장). 단 종류 태그는 `local-shared` 로 구분하여 후속 엔진 통합이 in-process opener 를 선택하게 한다.
- 회귀 안전: `mode=instance` 명시 경로는 기존 `localFlowNodeIDs` 로직을 **건드리지 않는다**(IN01/N01 보존).

### 1.4 로컬 in-process opener (신규)

- 위치: `internal/api/service/` (예: `local_bridge_opener.go`).
- `FlowBridgeOpener` 인터페이스 구현(기존 `server_bridge_opener.go` 의 인터페이스 재사용).
- `OpenBridge(ctx, localFlowID, "", inputPorts, outputPorts)`:
  - 엔진 `flows map[string]*flowRuntime` 에서 `localFlowID` 실행 인스턴스 조회.
  - 미존재(미실행) → transient 오류 반환 → 컨트롤러가 오프라인 대기(L02).
  - 존재 → 경계 포트(`__flow_input__`/`__flow_output__`)에 in-process 채널 attach → `FlowBridge` 핸들 반환.
- `FlowBridge.SendInput` → 인스턴스 입력 경계 포트로 주입.
- `FlowBridge.Outputs` → 인스턴스 출력 경계 포트 tap 채널(유계 버퍼, oldest-drop/coalesce — N03).
- `FlowBridge.Status` → 인스턴스 실행 상태(connected/offline).

### 1.5 매니저 엔진 통합 일반화 (`remote_bridge_node.go`)

- 현재 `rewireRemoteBridges` 는 REMOTE flow-node 만 처리한다.
- 일반화: 살아남은 LIVE NODE 를 종류별 opener 로 매핑.
  - REMOTE → 기존 server opener.
  - `local-shared` → 신규 in-process opener.
- `managerBridgeTable`·`managerBridgeController` 는 opener 인터페이스에만 의존하므로 **컨트롤러 코드 변경 없음**(opener 주입만 분기).
- 브리지 키: 부모별 독립 엔드포인트 보장 위해 `<parentFlowID>:<flowNodeID>:<refFlowID>` (L07).

### 1.6 다중 참조 fan-in/fan-out (인스턴스 경계 attach)

- 같은 참조 플로우 인스턴스의 경계 포트에 **복수 엔드포인트** attach 허용.
- 입력 경계: 일반 와이어 다중 소스와 동일 큐 의미로 fan-in(MR02/MR04).
- 출력 경계: tap fan-out(출력 메시지를 attach 된 모든 엔드포인트 채널로 복제, MR03).
- 인스턴스 측 attach/detach 레지스트리 필요(엔진 또는 in-process opener 가 보유). detach 시 슬롯 정리.

### 1.7 생명주기/자가치유 트리거

- 우선: 엔진 라이프사이클 훅(deploy/start/stop/undeploy 이벤트)으로 attach/detach·재open 트리거.
- 폴백: opener 의 `OpenBridge` 재시도가 미실행 인스턴스에 대해 빠르게 실패하면 컨트롤러 `reopen` 백오프가 자가치유(이미 원격 경로에서 검증됨).
- teardown 격리: 부모 undeploy → 그 부모 엔드포인트만 detach. 참조 인스턴스·타 부모 엔드포인트 불변(L06/L07).

### 1.8 순환 검출 (`subflow_cycle.go`)

- 기존 `DetectFlowReferenceCycle` 그래프에 `shared` 참조 포함(이미 `flow_id` 기준이므로 mode 무관하게 포함됨 — 확인·보강).
- `shared` 는 매니저 인라인 확장이 없어 무한 확장 위험은 없으나, 연결 순환(A↔B)은 무한 패싱·의미 모호이므로 저장·배포 거부(CY01/CY02).

### 1.9 통계 정합

- `shared`: 통계 뷰가 참조 플로우 자체 통계로 연결(임베디드 병합 비활성). 프론트에서 flow-node 통계 클릭 시 참조 플로우 통계로 라우팅하거나 인라인 표시.
- `instance`: 기존 subflow-stats 병합 유지.
- 모드 배지(S03).

### 1.10 프론트

- `nodeSchemas.ts` flow-node 스키마에 `mode` 필드(토글, 기본 shared) 추가. 핸들 계산은 기존 `computePortsForNode`(참조 플로우 inputs/outputs) 유지 — mode 무관(P01).
- 설정 패널: mode 토글(W01) + 편집 반영 안내 툴팁(W03).
- 상태 인디케이터: 원격 브리지 상태 표시기(RU06)를 일반화하여 로컬 shared 에도 적용(미연결/연결됨·실행중/오프라인/오류, W02/W04).
- i18n: `en.json`/`ko.json` 신규 키(mode 라벨·안내·상태).

## 2. 마일스톤 (우선순위 기반, 시간 추정 없음)

> 분할: 브리지 추상화 일반화 → 배포 분기 → 생명주기/자가치유 → 통계 정합 → 웹 UI → 마이그레이션/순환/회귀. 각 마일스톤은 독립 검증 가능 단위.

### 마일스톤 M1 — 우선순위 High: 브리지 추상화 일반화 (in-process opener)
- `FlowBridgeOpener`/`FlowBridge` 인터페이스 재사용 + in-process opener 신규(엔진 `flows` 맵 인스턴스 조회 + 경계 포트 attach).
- `managerBridgeController`/`managerBridgeTable` 을 opener 인터페이스에만 의존하도록 확인(이미 그러함) → 컨트롤러 코드 무변경.
- 검증: in-process opener 단위 테스트(인스턴스 존재→connected, 미존재→transient/offline), 컨트롤러 자가치유 재사용 회귀.
- 영향 파일: `internal/api/service/local_bridge_opener.go`(신규), `internal/api/service/server_bridge_opener.go`(인터페이스 확인), `internal/api/service/remote_bridge_node.go`, `internal/engine/*`(인스턴스 조회 표면).
- 의존: 없음(SPEC-SUBFLOW-001 RB 인프라 전제).

### 마일스톤 M2 — 우선순위 High: 배포 분기 mode
- flow-node `mode` config 키·정규화(미지정→shared).
- `subflow_expand.go` LOCAL flow-node `mode` 재분류: `instance`=인라인 확장(보존), `shared`=LIVE NODE 보존(미확장, `local-shared` 태그).
- `rewireRemoteBridges` 일반화: 종류별 opener 매핑(remote→server, local-shared→in-process).
- 검증: 배포 분기 테스트(shared 미확장, instance 확장, 혼합 IN03), 기존 인라인 확장 회귀(N01).
- 영향 파일: `internal/node/flow_node.go`, `internal/api/service/subflow_expand.go`, `internal/api/service/remote_bridge_node.go`, `internal/api/service/flow_adapter.go`.
- 의존: M1.

### 마일스톤 M3 — 우선순위 High: 생명주기/자가치유 + 다중참조
- 엔진 라이프사이클 훅(또는 폴링) → attach/detach·재open 트리거.
- 오프라인 대기(L02)·시작 감지 자가치유(L03)·정지 끊김(L04)·재시작 자가치유(L05)·teardown 격리(L06/L07).
- 다중 참조 fan-in/fan-out(인스턴스 경계 복수 엔드포인트 attach, MR01~MR04).
- 검증: 오프라인→자가치유, 정지→끊김→재시작 자가치유, 다중참조 fan-in/fan-out, 부모 teardown 격리.
- 영향 파일: `internal/engine/*`(라이프사이클 훅), `internal/api/service/local_bridge_opener.go`(attach 레지스트리), `remote_bridge_node.go`(컨트롤러 재사용).
- 의존: M1·M2.

### 마일스톤 M4 — 우선순위 Medium: 통계 정합
- `shared`=참조 플로우 자체 통계 직접(병합 비활성), `instance`=기존 임베디드 병합 유지.
- 모드 구분 표시(S03).
- 검증: shared 통계 직접 노출 테스트, instance 병합 회귀.
- 영향 파일: 통계 경로(service), 프론트 통계 뷰.
- 의존: M2.

### 마일스톤 M5 — 우선순위 Medium: 웹 UI
- `nodeSchemas.ts` `mode` 필드(토글, 기본 shared).
- 설정 패널 mode 토글(W01) + 편집 반영 안내(W03).
- shared 연결 상태 인디케이터(RU06 일반화, W02/W04).
- i18n 키.
- 검증: Vitest — mode 토글 저장(shared/instance), 상태 인디케이터 렌더, 안내 표시.
- 영향 파일: `web/src/config/nodeSchemas.ts`, flow-node 설정 패널, 상태 인디케이터 컴포넌트, `web/src/lib/i18n/en.json`·`ko.json`.
- 의존: M2(연결 상태 소스).

### 마일스톤 M6 — 우선순위 Low(최종 목표): 마이그레이션·순환·회귀
- 마이그레이션: `mode` 미지정 → shared 동작(MG01), instance 명시로 기존 동작(MG02), remote 불변(MG03).
- 순환: shared/혼합 순환 거부(CY01/CY02, 기존 가드 정합 확인).
- 회귀: instance·remote·flow-node 미포함 플로우 동작 불변, 백프레셔/백오프.
- 검증: 마이그레이션 시나리오, 순환 거부, 전 경로 회귀 스위트.
- 의존: 전 마일스톤.

## 3. 위험 및 대응

| 위험 | 영향 | 대응 |
|------|------|------|
| **동작 변경(breaking)** — 미지정→shared 로 기존 인라인 복사 의존 플로우 동작 변화 | 운영 플로우 무출력/의미 변화 | MG02(instance 명시 경로) 제공·문서화. 배포 시 mode 미지정 LOCAL flow-node 에 경고/안내. 마이그레이션 가이드. |
| **통계 화면 정합** — shared/instance 의미가 화면에서 혼동 | 운영자 오해 | S03 모드 배지·뷰 분리. 통계 정합 테스트로 모드별 동작 고정. |
| **연결 순환(A↔B)** — shared 무한 패싱 | 메시지 폭주 | CY01/CY02 저장·배포 거부(기존 순환 가드 재사용). |
| **다중 fan-in 인터리빙 의미** — 메시지 순서 불명확 | 비결정적 동작 오해 | MR04 — 일반 다중 소스 와이어와 동일 의미로 고정(브리지 무재정렬). 명세·테스트로 못 박음. |
| **참조 플로우 미실행 운영 절차 변화** | 데이터 미흐름 혼동 | L02 오프라인 상태 명확 표시(W02/W04). 자동 시작 안 함을 안내. |
| **자가치유 핫스핀** | CPU 낭비 | N04 — 원격 `reopen` bounded 백오프 경로 재사용. |
| **회귀** — instance/remote 경로 손상 | 기존 기능 파손 | instance/remote 경로 무변경 원칙(분기만 추가) + 회귀 스위트(N01). |
| **백프레셔** — 경계 tap 버퍼 폭증 | 메모리 | N03 — 유계 버퍼 + oldest-drop/coalesce(RB10 일관). |

## 4. TRUST 5 / 테스트 전략

- **Tested**: 브리지 연결/오프라인/자가치유/다중참조 fan-in·fan-out/순환/마이그레이션 시나리오 커버. 85%+ 커버리지. `go test -race ./...`(브리지 동시성).
- **Readable**: mode 분기·opener 종류를 명시적 태그(`local-shared`)로 표현. 한국어 주석(기존 코드 컨벤션).
- **Unified**: 원격/로컬 브리지 동일 인터페이스(`FlowBridgeOpener`/`FlowBridge`)로 일관. gofmt/golangci-lint.
- **Secured**: in-process 브리지는 동일 매니저 프로세스 내이므로 신규 인증 경로 없음. 경계 메시지 페이로드 정책 유지.
- **Trackable**: 브리지 키(`parent:flowNode:ref`)·로그로 추적성. SPEC-SUBFLOW-002 REQ 태그 매핑(spec §6).

### 핵심 테스트 시나리오

1. shared 연결: 참조 플로우 실행 중 → 부모 배포 → 메시지 양방향 패싱.
2. 오프라인→자가치유: 참조 플로우 미실행 → 부모 배포(오프라인 대기) → 참조 플로우 시작 → 자동 연결.
3. 정지→끊김→재시작: 연결 후 참조 플로우 stop → 오프라인 → 재시작 → 자가치유.
4. 다중참조 공유: 두 flow-node 가 같은 flow_id → 단일 인스턴스 fan-in/fan-out.
5. instance 모드: 인라인 확장 보존(회귀).
6. 순환 거부: shared A↔B 저장·배포 거부.
7. 마이그레이션: mode 미지정 → shared 동작.
8. teardown 격리: 부모1 undeploy → 부모2·참조 인스턴스 불변.
