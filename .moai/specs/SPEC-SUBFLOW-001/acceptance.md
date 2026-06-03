# SPEC-SUBFLOW-001 인수 기준 (acceptance.md)

> 플로우 노드 — 플로우 합성(서브플로우) 및 플로우 레벨 입출력 포트
> Given/When/Then 시나리오 기반. 각 시나리오는 spec.md의 EARS 요구사항에 매핑된다.

## 1. 인수 시나리오 (Given-When-Then)

### AC-1: 플로우 포트 추가·저장·재로드 (REQ-SUBFLOW-A01~A05, B01)

- Given: 빈 또는 임의의 플로우 A 를 편집 중이다.
- When: 입력 포트 `in1` 과 출력 포트 `out1` 을 추가하고 저장한 뒤 재로드한다.
- Then:
  - `in1`/`out1` 이 유지된다(id 보존, 방향 유지).
  - 캔버스 좌측 경계에 `in1`, 우측 경계에 `out1` 이 표시된다.
  - 두 포트는 노드 목록(nodes)에 포함되지 않는다(플로우 레벨 엔티티).

### AC-2: 내부 노드를 경계 포트에 와이어 연결 (REQ-SUBFLOW-B02)

- Given: 플로우 A 에 입력 포트 `in1`, 출력 포트 `out1`, 내부 노드 N(예: transform)이 있다.
- When: `in1`(소스) → N(입력), N(출력) → `out1`(싱크) 로 와이어를 연결한다.
- Then: 두 와이어가 정상 생성되며(경계 포트는 와이어 엔드포인트로 동작), 저장/재로드 후 연결이 유지된다.

### AC-3: 포트 이름 변경 시 id 불변 (REQ-SUBFLOW-A03)

- Given: 포트 `in1`(id 고정)에 내부 와이어가 연결되어 있다.
- When: `in1` 의 이름을 `sensorIn` 으로 변경한다.
- Then: 포트 id 는 그대로이고 이름만 바뀌며, 연결된 내부 와이어는 끊기지 않는다.

### AC-4: 포트 삭제 시 와이어 정리 (REQ-SUBFLOW-A04)

- Given: 포트 `out1` 에 내부 노드 와이어가 연결되어 있다.
- When: `out1` 을 삭제한다.
- Then: `out1` 이 제거되고, 그 포트를 엔드포인트로 쓰던 내부 와이어가 함께 제거(또는 dangling 처리)된다.

### AC-5: flow-node 생성과 핸들 표시 (REQ-SUBFLOW-C01, C02, C04)

- Given: 입력 `in1`/출력 `out1` 을 가진 플로우 A 가 저장되어 있고, 별도 플로우 B 를 편집 중이다.
- When: B 에서 flow-node 를 생성하고 피커에서 A 를 선택한다.
- Then:
  - flow-node 의 입력 핸들에 `in1`, 출력 핸들에 `out1` 이 표시된다.
  - 피커 후보에 B 자신은 나타나지 않는다.
  - flow-node config 에 `flow_id = A.id` 가 저장된다.

### AC-6: 배포 인스턴스화·메시지 라우팅 (REQ-SUBFLOW-D01~D05)

- Given: B 가 A 를 참조하는 flow-node 를 포함하고, B 안에서 flow-node.in1 로 보내는 소스와 flow-node.out1 을 받는 싱크가 연결되어 있다.
- When: B 를 배포한다.
- Then:
  - A 의 서브그래프가 네임스페이스(`subflow_<flowNodeID>_*`)로 격리 확장된다.
  - B 에서 flow-node.in1 로 보낸 메시지가 A 내부(`in1`→내부 노드→`out1`)를 거쳐 flow-node.out1 로 나온다.
  - 확장 결과는 영속화되지 않는다(B 저장 정의에는 flow-node 가 그대로 보존).

### AC-7: 다중 인스턴스 격리 (REQ-SUBFLOW-D03)

- Given: B 가 같은 플로우 A 를 참조하는 flow-node 2개(fnA, fnB)를 포함한다.
- When: B 를 배포하고 fnA 로 메시지를 보낸다.
- Then: fnA 인스턴스만 처리하고 fnB 인스턴스는 영향받지 않는다(상태 비공유, 네임스페이스 접두사에 flow-node ID 포함).

### AC-8: 항상 최신 반영 (REQ-SUBFLOW-C03, D04)

- Given: B 를 배포한 적이 있고, 이후 A 의 포트/내부 로직을 수정·저장했다.
- When: B 를 재배포한다.
- Then: 재배포된 B 의 서브그래프가 최신 A 정의를 반영한다(스냅샷 고정 아님). B 를 다시 열면 flow-node 핸들도 최신 A 포트로 갱신된다.

### AC-9: 직접 자기참조 거부 (REQ-SUBFLOW-E01)

- Given: 플로우 A 를 편집 중이다.
- When: A 안에 A 를 참조하는 flow-node(`flow_id = A.id`)를 만들고 저장/배포를 시도한다.
- Then: 저장과 배포가 모두 거부되고 자기참조 에러가 명확히 표시된다.

### AC-10: 간접 순환 거부 (REQ-SUBFLOW-E02, E03, E04)

- Given: A 가 B 를 참조하고, B 가 A 를 참조하도록 구성(A→B→A)한다.
- When: 저장 또는 배포를 시도한다.
- Then:
  - 저장 시점 또는 배포 시점에 순환이 검출되어 거부된다.
  - 에러에 순환 경로(예: A → B → A)가 포함된다.
  - 무한 서브그래프 확장이 수행되지 않는다.

### AC-11: 참조 포트 삭제 시 dangling (REQ-SUBFLOW-C05)

- Given: B 의 flow-node 가 A 의 포트 `out1` 핸들에 부모 와이어로 연결되어 있다.
- When: A 에서 `out1` 을 삭제하고 B 를 다시 연다.
- Then: B 의 해당 와이어가 dangling 으로 표시(경고)되고 정리 경로가 제공된다.

### AC-12: 누락 참조 처리 (REQ-SUBFLOW-F03)

- Given: B 의 flow-node 가 가리키는 `flow_id` 의 플로우가 저장소에 없다.
- When: B 배포를 시도한다.
- Then: 배포가 거부되거나 명확한 "참조 플로우 없음" 에러로 안내된다(서브그래프 확장 정의가 필수이므로 엄격 처리).

### AC-13: 포트 보유 플로우 단독 배포 (REQ-SUBFLOW-F01)

- Given: 입력/출력 포트를 가진 플로우 A 를 flow-node 없이 단독 배포한다.
- When: A 를 배포한다.
- Then: 배포가 성공하고, 포트는 비활성(엔드포인트 dangling)으로 처리되며 내부 노드 실행에는 영향이 없다.

### AC-14: 기존 플로우 동작 불변 (REQ-SUBFLOW-F02)

- Given: 플로우 레벨 포트도 flow-node 도 없는 기존 플로우 정의.
- When: 정상 로드/배포/실행한다.
- Then: 기존과 동일하게 동작한다(회귀 0). 신규 `inputs`/`outputs` 부재는 기본 빈 배열로 해석된다.

### AC-15: 확장 안전 상한 (REQ-SUBFLOW-N01)

- Given: 중첩 깊이 또는 총 확장 노드 수가 안전 상한을 초과하는 구성.
- When: 배포(확장)를 시도한다.
- Then: 확장이 상한에서 중단되고 명확한 에러가 반환된다(OOM/무한 확장 방지).

### AC-16: export/import 보존 (REQ-SUBFLOW-A06)

- Given: 입력/출력 포트와 flow-node 를 가진 플로우.
- When: export 후 다시 import 한다.
- Then: 플로우 레벨 포트(id/name/방향)와 flow-node `flow_id` 가 유지된다(참조 정합 정책은 OPEN Q7 결정에 따름).

## 2. 품질 게이트 (Definition of Done)

- [ ] 모든 EARS 요구사항(REQ-SUBFLOW-A01~F03, N01~N02)에 대응 인수 시나리오 통과.
- [ ] 백엔드: 신규 코드(FlowPort, subflow_expand, cycle_detect, flow-node) TDD, 커버리지 85%+.
- [ ] 백엔드: 기존 변경(flow.go, flow_adapter.go, registry.go) 동작 보존 — 기존 회귀 스위트 100% 통과.
- [ ] 엔진 회귀(옵션 1 채택 시 엔진 코드 불변): 기존 엔진 테스트 전부 통과.
- [ ] `go test -race ./...` 통과, golangci-lint zero, gofmt/goimports 클린.
- [ ] 프론트: 신규 컴포넌트/스토어 Vitest 통과, 경계 포트/피커/flow-node 핸들 동작 검증.
- [ ] 순환/자기참조: 자기참조·A→B→A·A→B→C→A 모두 저장+배포에서 거부, 정상 DAG 는 통과.
- [ ] 다중 인스턴스 격리·항상 최신 반영·메시지 라우팅 end-to-end 검증.
- [ ] dangling/누락 참조/단독 배포/안전 상한 엣지 케이스 검증.
- [ ] LSP 품질 게이트(run): error/type-error/lint-error 0.

## 3. 검증 방법·도구

| 영역 | 도구 | 대상 |
|------|------|------|
| Go 단위 | `go test` + testify | flow_test, flow_adapter_test, cycle_detect_test, subflow_expand_test, registry_test |
| Go 동시성 | `go test -race ./...` | 엔진/확장 라우팅 |
| Go 정적 | golangci-lint, gofmt, goimports | 전 변경 |
| 프론트 단위 | Vitest + Testing Library | editorStore, FlowBoundaryPorts, nodeSchemas(computePortsForNode), FlowPickerModal |
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
