# SPEC-SUBFLOW-001 인수 기준 (acceptance.md)

> 플로우 노드 — 플로우 합성(서브플로우) 및 플로우 레벨 입출력 포트
> Given/When/Then 시나리오 기반. 각 시나리오는 spec.md의 EARS 요구사항에 매핑된다.

> **구현 상태(2026-06-05, 브랜치 `feature/subflow-node`)**: M1(포트 모델)·M2(엔진 확장/순환검출)·M3(flow-node 프론트)·M4(경계 포트 UI) **구현 완료**. 아래 인수 시나리오의 인수 기준 자체는 보존하며, 구현으로 충족된 항목에는 "구현됨" 주석을 덧붙인다(신규 요구사항 추가 없음).
>
> 구현 시 확정된 동작과 일치하도록 참고할 점:
> - 경계 포트는 센티넬 노드(`__flow_input__`/`__flow_output__`)를 와이어 엔드포인트로 사용하며, 합성 React Flow 노드로 렌더된다(저장 `nodes` 제외, 정의 inputs/outputs 로만 영속).
> - 배포 확장은 어댑터 사전 확장 + 직접 재배선이며 dangling 와이어는 경고 후 드랍한다.
> - 순환/누락 참조/상한(깊이 8·노드 5000)은 저장·배포 양쪽에서 엄격 처리한다.

## 1. 인수 시나리오 (Given-When-Then)

### AC-1: 플로우 포트 추가·저장·재로드 (REQ-SUBFLOW-A01~A05, B01) — 구현됨

- Given: 빈 또는 임의의 플로우 A 를 편집 중이다.
- When: 입력 포트 `in1` 과 출력 포트 `out1` 을 추가하고 저장한 뒤 재로드한다.
- Then:
  - `in1`/`out1` 이 유지된다(id 보존, 방향 유지).
  - 캔버스 좌측 경계에 `in1`, 우측 경계에 `out1` 이 표시된다.
  - 두 포트는 노드 목록(nodes)에 포함되지 않는다(플로우 레벨 엔티티).

### AC-2: 내부 노드를 경계 포트에 와이어 연결 (REQ-SUBFLOW-B02) — 구현됨

- Given: 플로우 A 에 입력 포트 `in1`, 출력 포트 `out1`, 내부 노드 N(예: transform)이 있다.
- When: `in1`(소스) → N(입력), N(출력) → `out1`(싱크) 로 와이어를 연결한다.
- Then: 두 와이어가 정상 생성되며(경계 포트는 와이어 엔드포인트로 동작), 저장/재로드 후 연결이 유지된다.

### AC-3: 포트 이름 변경 시 id 불변 (REQ-SUBFLOW-A03) — 구현됨

- Given: 포트 `in1`(id 고정)에 내부 와이어가 연결되어 있다.
- When: `in1` 의 이름을 `sensorIn` 으로 변경한다.
- Then: 포트 id 는 그대로이고 이름만 바뀌며, 연결된 내부 와이어는 끊기지 않는다.

### AC-4: 포트 삭제 시 와이어 정리 (REQ-SUBFLOW-A04) — 구현됨

- Given: 포트 `out1` 에 내부 노드 와이어가 연결되어 있다.
- When: `out1` 을 삭제한다.
- Then: `out1` 이 제거되고, 그 포트를 엔드포인트로 쓰던 내부 와이어가 함께 제거(또는 dangling 처리)된다.

### AC-5: flow-node 생성과 핸들 표시 (REQ-SUBFLOW-C01, C02, C04) — 구현됨

- Given: 입력 `in1`/출력 `out1` 을 가진 플로우 A 가 저장되어 있고, 별도 플로우 B 를 편집 중이다.
- When: B 에서 flow-node 를 생성하고 피커에서 A 를 선택한다.
- Then:
  - flow-node 의 입력 핸들에 `in1`, 출력 핸들에 `out1` 이 표시된다.
  - 피커 후보에 B 자신은 나타나지 않는다.
  - flow-node config 에 `flow_id = A.id` 가 저장된다.

### AC-6: 배포 인스턴스화·메시지 라우팅 (REQ-SUBFLOW-D01~D05) — 구현됨

- Given: B 가 A 를 참조하는 flow-node 를 포함하고, B 안에서 flow-node.in1 로 보내는 소스와 flow-node.out1 을 받는 싱크가 연결되어 있다.
- When: B 를 배포한다.
- Then:
  - A 의 서브그래프가 네임스페이스(`subflow_<flowNodeID>_*`)로 격리 확장된다.
  - B 에서 flow-node.in1 로 보낸 메시지가 A 내부(`in1`→내부 노드→`out1`)를 거쳐 flow-node.out1 로 나온다.
  - 확장 결과는 영속화되지 않는다(B 저장 정의에는 flow-node 가 그대로 보존).

### AC-7: 다중 인스턴스 격리 (REQ-SUBFLOW-D03) — 구현됨

- Given: B 가 같은 플로우 A 를 참조하는 flow-node 2개(fnA, fnB)를 포함한다.
- When: B 를 배포하고 fnA 로 메시지를 보낸다.
- Then: fnA 인스턴스만 처리하고 fnB 인스턴스는 영향받지 않는다(상태 비공유, 네임스페이스 접두사에 flow-node ID 포함).

### AC-8: 항상 최신 반영 (REQ-SUBFLOW-C03, D04) — 구현됨

- Given: B 를 배포한 적이 있고, 이후 A 의 포트/내부 로직을 수정·저장했다.
- When: B 를 재배포한다.
- Then: 재배포된 B 의 서브그래프가 최신 A 정의를 반영한다(스냅샷 고정 아님). B 를 다시 열면 flow-node 핸들도 최신 A 포트로 갱신된다.

### AC-9: 직접 자기참조 거부 (REQ-SUBFLOW-E01) — 구현됨

- Given: 플로우 A 를 편집 중이다.
- When: A 안에 A 를 참조하는 flow-node(`flow_id = A.id`)를 만들고 저장/배포를 시도한다.
- Then: 저장과 배포가 모두 거부되고 자기참조 에러가 명확히 표시된다.

### AC-10: 간접 순환 거부 (REQ-SUBFLOW-E02, E03, E04) — 구현됨

- Given: A 가 B 를 참조하고, B 가 A 를 참조하도록 구성(A→B→A)한다.
- When: 저장 또는 배포를 시도한다.
- Then:
  - 저장 시점 또는 배포 시점에 순환이 검출되어 거부된다.
  - 에러에 순환 경로(예: A → B → A)가 포함된다.
  - 무한 서브그래프 확장이 수행되지 않는다.

### AC-11: 참조 포트 삭제 시 dangling (REQ-SUBFLOW-C05) — 구현됨

- Given: B 의 flow-node 가 A 의 포트 `out1` 핸들에 부모 와이어로 연결되어 있다.
- When: A 에서 `out1` 을 삭제하고 B 를 다시 연다.
- Then: B 의 해당 와이어가 dangling 으로 표시(경고)되고 정리 경로가 제공된다.

### AC-12: 누락 참조 처리 (REQ-SUBFLOW-F03) — 구현됨

- Given: B 의 flow-node 가 가리키는 `flow_id` 의 플로우가 저장소에 없다.
- When: B 배포를 시도한다.
- Then: 배포가 거부되거나 명확한 "참조 플로우 없음" 에러로 안내된다(서브그래프 확장 정의가 필수이므로 엄격 처리).

### AC-13: 포트 보유 플로우 단독 배포 (REQ-SUBFLOW-F01) — 구현됨

- Given: 입력/출력 포트를 가진 플로우 A 를 flow-node 없이 단독 배포한다.
- When: A 를 배포한다.
- Then: 배포가 성공하고, 포트는 비활성(엔드포인트 dangling)으로 처리되며 내부 노드 실행에는 영향이 없다.

### AC-14: 기존 플로우 동작 불변 (REQ-SUBFLOW-F02) — 구현됨

- Given: 플로우 레벨 포트도 flow-node 도 없는 기존 플로우 정의.
- When: 정상 로드/배포/실행한다.
- Then: 기존과 동일하게 동작한다(회귀 0). 신규 `inputs`/`outputs` 부재는 기본 빈 배열로 해석된다.

### AC-15: 확장 안전 상한 (REQ-SUBFLOW-N01) — 구현됨

- Given: 중첩 깊이 또는 총 확장 노드 수가 안전 상한을 초과하는 구성.
- When: 배포(확장)를 시도한다.
- Then: 확장이 상한에서 중단되고 명확한 에러가 반환된다(OOM/무한 확장 방지).

### AC-16: export/import 보존 (REQ-SUBFLOW-A06) — 구현됨

- Given: 입력/출력 포트와 flow-node 를 가진 플로우.
- When: export 후 다시 import 한다.
- Then: 플로우 레벨 포트(id/name/방향)와 flow-node `flow_id` 가 유지된다(참조 정합 정책 = **id 참조 유지, 참조 플로우 동봉 안 함** — 확정).

## 1-bis. 인수 시나리오 (v1.2 — 원격 서브플로우 참조) — ⏳ 예정

> 전제: SPEC-REMOTE-001 매니저(서버 모드)에 원격 노드 N(승인∧온라인)이 등록되어 있고, N 은 입력 `in1`/출력 `out1` 을 가진 self-contained 플로우 RF 를 노출(exposure)한다. 매니저에서 LOCAL 플로우 P 를 편집한다.

### AC-R1: 원격 노드의 플로우를 서브플로우로 선택 (REQ-SUBFLOW-RU01~RU05, R01)

- Given: 매니저에서 LOCAL 플로우 P 를 편집 중이고, 원격 노드 N 이 승인∧온라인이며 플로우 RF 를 노출한다.
- When: P 에서 flow-node 를 생성하고 피커에서 **원격 노드 N** 을 선택한 뒤 N 의 플로우 RF 를 선택한다.
- Then:
  - flow-node config 의 `flow_id` 가 `remote://{N.instance_id}/{RF.id}` 정규형으로 저장된다.
  - flow-node 에 **원격 배지**와 출처 노드(N) 식별이 표시된다.
  - flow-node 입력 핸들에 `in1`, 출력 핸들에 `out1` 이 표시된다(query 프록시 `flow`/`get` 으로 원격 RF 포트 취득, READ-ONLY).
  - bare id LOCAL 선택은 기존대로 동작한다(하위 호환).

### AC-R2: 배포 시 원격 정의 fetch·인라인 확장·실행 (REQ-SUBFLOW-R02~R05)

- Given: P 가 `remote://{N}/{RF}` 를 참조하는 flow-node 를 포함하고, P 안에서 flow-node.in1 로 보내는 소스와 flow-node.out1 을 받는 싱크가 연결되어 있다. N 은 온라인이다.
- When: 매니저에서 P 를 배포한다.
- Then:
  - 매니저가 query 프록시 `flow`/`get`(`DispatchQuery(N, "flow", "get", RF)`)으로 RF 의 **현재(최신)** 정의를 fetch 한다.
  - RF 서브그래프가 네임스페이스(`subflow_<flowNodeID>_*`)로 격리 인라인 확장되고, flow-node.in1→내부→flow-node.out1 로 메시지가 라우팅된다.
  - 확장 서브그래프는 **매니저(배포 인스턴스)에서 실행**된다(원격 노드는 실행 피어 아님).
  - 확장 결과는 영속화되지 않는다(P 저장 정의에는 `remote://` 참조 보존).

### AC-R3: 항상 최신 반영 (REQ-SUBFLOW-R03)

- Given: P 를 배포한 적이 있고, 이후 N 에서 RF 의 포트/내부 로직을 수정·저장했다.
- When: P 를 재배포한다.
- Then: 재배포된 P 의 서브그래프가 **최신 RF 정의**를 반영한다(매 배포 fetch, 스냅샷 고정 아님). P 를 다시 열면 flow-node 핸들도 최신 RF 포트로 갱신된다.

### AC-R4: 노드 오프라인 시 배포 실패 (REQ-SUBFLOW-R06)

- Given: P 가 `remote://{N}/{RF}` flow-node 를 포함하는데, 배포 시점에 N 이 **오프라인**이다.
- When: P 를 배포한다.
- Then: 배포가 **거부**되고 "원격 노드 오프라인"(503 계열) 명확한 에러가 반환된다. **stale/empty 정의를 무음으로 사용하지 않는다**.

### AC-R5: 원격 플로우 누락 시 배포 실패 (REQ-SUBFLOW-R06)

- Given: P 가 `remote://{N}/{RF}` 를 참조하는데, N 에 RF 가 더 이상 존재하지 않거나 노출 범위 밖이다(N 은 온라인).
- When: P 를 배포한다.
- Then: 배포가 거부되고 "원격 플로우 없음/미노출"(502/노출 거부) 에러가 명확히 안내된다.

### AC-R6: 비-서버 모드/해석기 부재 거부 (REQ-SUBFLOW-R07)

- Given: 원격 참조 flow-node 를 가진 P 를, server 모드가 아니거나 `RemoteFlowFetcher` 가 주입되지 않은 인스턴스에서 배포한다.
- When: P 를 배포한다.
- Then: 배포가 거부되고 "원격 서브플로우 해석 불가(서버 모드 아님/해석기 미구성)" 에러가 반환된다.

### AC-R7: 중첩 원격 참조 거부 (REQ-SUBFLOW-R08, R09)

- Given: P 가 `remote://{N}/{RF}` 를 참조하는데, fetch 된 RF 정의가 **그 자체로 flow-node(로컬/원격 불문)를 포함**한다.
- When: P 를 배포한다.
- Then: 배포가 거부되고 "원격 서브플로우는 중첩 서브플로우를 포함할 수 없음" 에러가 반환된다(분산 순환/무한 확장 차단).

### AC-R8: 시크릿/agent·device 제약 표면화 (REQ-SUBFLOW-RC01~RC03)

- Given: RF 가 시크릿이 필요한 노드 또는 N-전용 agent/device 참조를 포함한다.
- When: P 를 배포(또는 피커에서 RF 선택)한다.
- Then:
  - 운영자에게 **제약 경고**가 표면화된다(OQ-R5 확정 시: 시크릿 보유 노드·미해결 agent/device 참조 나열).
  - (실행) 매니저에서 redacted 시크릿/미해결 원격 참조로 인해 해당 노드는 동작하지 않으며, 이는 self-contained/비시크릿 로직 한정 제약으로 문서화된다.
  - 원격 노드는 라이브 실행 피어가 아님이 보장된다(정의 임베딩).

## 2. 품질 게이트 (Definition of Done)

- [ ] 모든 EARS 요구사항(REQ-SUBFLOW-A01~F03, N01~N02)에 대응 인수 시나리오 통과.
- [ ] 백엔드: 신규 코드(플로우 포트=`Port` 재사용, subflow_expand, subflow_cycle, flow-node, boundary) TDD, 커버리지 85%+.
- [ ] 백엔드: 기존 변경(flow.go, serialize.go, flow_adapter.go, registry.go) 동작 보존 — 기존 회귀 스위트 100% 통과.
- [ ] 엔진 회귀(옵션 1 채택 시 엔진 코드 불변): 기존 엔진 테스트 전부 통과.
- [ ] `go test -race ./...` 통과, golangci-lint zero, gofmt/goimports 클린.
- [ ] 프론트: 신규 컴포넌트/스토어 Vitest 통과, 경계 포트/피커/flow-node 핸들 동작 검증.
- [ ] 순환/자기참조: 자기참조·A→B→A·A→B→C→A 모두 저장+배포에서 거부, 정상 DAG 는 통과.
- [ ] 다중 인스턴스 격리·항상 최신 반영·메시지 라우팅 end-to-end 검증.
- [ ] dangling/누락 참조/단독 배포/안전 상한 엣지 케이스 검증.
- [ ] LSP 품질 게이트(run): error/type-error/lint-error 0.

### v1.2 (원격 서브플로우 참조) — ⏳ 예정

- [ ] OQ-R1~R5 확정(사용자 확인) 후 Run 진입.
- [ ] 신규 EARS(REQ-SUBFLOW-R01~R09, RU01~RU05, RC01~RC03)에 대응 인수 시나리오(AC-R1~R8) 통과.
- [ ] 백엔드 신규(`remote://` 파서, `RemoteFlowFetcher`, `ExpandSubflows` 원격 분기) TDD, 커버리지 85%+. fake fetcher 주입 테스트.
- [ ] 실패 매핑(오프라인 503/타임아웃 504/누락 502 → 배포 거부, stale/empty 무음 금지), 비-서버 모드 거부, 중첩 원격 참조 거부.
- [ ] bare id LOCAL 참조 회귀 0(기존 동작 불변).
- [ ] 프론트 신규(원격 노드 선택기·`useFlowsTarget` 재사용·`remote://` 저장·원격 배지·원격 핸들 계산) Vitest.
- [ ] 제약(시크릿/agent·device/실행 지역성) 표면화 또는 문서화(OQ-R5 확정 반영).
- [ ] SPEC-REMOTE-001 query 프록시(`flow`/`get`) 연동 통합 검증.

## 3. 검증 방법·도구

| 영역 | 도구 | 대상 |
|------|------|------|
| Go 단위 | `go test` + testify | flow round-trip, flow_adapter_test, subflow_cycle_test, subflow_expand_test, registry |
| Go 동시성 | `go test -race ./...` | 엔진/확장 라우팅 |
| Go 정적 | golangci-lint, gofmt, goimports | 전 변경 |
| 프론트 단위 | Vitest + Testing Library | editorStore, boundary, subflowPorts, nodeSchemas(computePortsForNode), FlowPortPanel |
| 통합/회귀 | 기존 엔진·어댑터 회귀 + end-to-end 배포 | 단독+합성, 메시지 라우팅, 기존 플로우 불변 |
| 품질 | TRUST 5, LSP 게이트 | run 단계 zero-error |

## 4. 추적성 매핑

| 인수 시나리오 | EARS 요구사항 |
|---------------|---------------|
| AC-1 | A01, A02, A05, B01, B04 |
| AC-2 | B02 |
| AC-3 | A03 |
| AC-4 | A04 |
| AC-5 | C01, C02, C04 |
| AC-6 | D01, D02, D04, D05 |
| AC-7 | D03 |
| AC-8 | C03, D04 |
| AC-9 | E01 |
| AC-10 | E02, E03, E04 |
| AC-11 | C05 |
| AC-12 | F03 |
| AC-13 | F01 |
| AC-14 | F02 |
| AC-15 | N01 |
| AC-16 | A06 |
| (전반) | A07, B03, B05, D06, N02 |
| AC-R1 (v1.2) | R01, RU01, RU02, RU03, RU04, RU05 |
| AC-R2 (v1.2) | R02, R04, R05 |
| AC-R3 (v1.2) | R03 |
| AC-R4 (v1.2) | R06 |
| AC-R5 (v1.2) | R06, R05 |
| AC-R6 (v1.2) | R07 |
| AC-R7 (v1.2) | R08, R09 |
| AC-R8 (v1.2) | RC01, RC02, RC03 |
