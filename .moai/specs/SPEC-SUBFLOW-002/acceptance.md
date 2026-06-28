# SPEC-SUBFLOW-002 인수 기준 (acceptance.md)

> 서브플로우 의미론 재설계 — flow-node = 공유 인스턴스 연결(로컬 라이브 브리지)
> Given/When/Then 시나리오 기반. 각 시나리오는 spec.md 의 EARS 요구사항에 매핑된다.

> **상태(2026-06-12)**: 계획(planned). 아래 인수 기준은 `/moai run SPEC-SUBFLOW-002` 구현의 완료 판정 기준이다.

## 1. 인수 시나리오 (Given-When-Then)

### AC-1: shared 모드 기본값 (REQ-SUBFLOW2-M01, M02)

- Given: LOCAL flow-node 를 생성하고 참조 플로우 A 를 선택했으나 `mode` 를 명시하지 않았다.
- When: flow-node config 를 저장·로드한다.
- Then:
  - flow-node 의 mode 가 `shared` 로 해석된다(미지정 → shared).
  - 설정 패널의 mode 토글이 `공유(shared)` 로 표시된다.

### AC-2: shared 연결·양방향 패싱 (REQ-SUBFLOW2-SH01, SH02, SH03)

- Given: 입력 `in1`/출력 `out1` 경계 포트를 가진 플로우 A 가 **실행 중**이고, A 를 `shared` 로 참조하는 flow-node 를 포함한 부모 B 가 있다(B 안에서 flow-node.in1 로 보내는 소스, flow-node.out1 을 받는 싱크 연결).
- When: B 를 배포한다.
- Then:
  - A 의 서브그래프가 B 안으로 **인라인 확장되지 않는다**(복사본 없음, `subflow_*` 네임스페이스 노드 미생성).
  - flow-node 는 LIVE NODE(브리지 엔드포인트)로 보존된다.
  - flow-node.in1 로 보낸 메시지가 **실행 중인 A 인스턴스**의 `in1` 경계 포트로 들어가 A 내부를 거쳐 `out1` 에서 나와 flow-node.out1 로 emit 된다.
  - A 는 플로우 리스트의 그 단일 인스턴스이며 별도 복사본이 돌지 않는다.

### AC-3: 공유 인스턴스 동일성 (REQ-SUBFLOW2-SH02, MR01)

- Given: 플로우 A 가 실행 중이고, 두 부모 B·C 가 각각 A 를 `shared` 로 참조한다.
- When: B 와 C 를 모두 배포한다.
- Then:
  - A 는 **단일 실행 인스턴스**로 유지된다(B·C 용 복사본이 따로 생기지 않음).
  - B 와 C 의 flow-node 모두 같은 A 인스턴스에 연결된다.

### AC-4: 다중참조 fan-in / fan-out (REQ-SUBFLOW2-MR02, MR03, MR04)

- Given: 플로우 A(입력 `in1`, 출력 `out1`)가 실행 중이고, 한 부모 안에 A 를 `shared` 로 참조하는 flow-node 2개(fn1, fn2)가 있다.
- When: fn1.in1 과 fn2.in1 로 각각 메시지를 보낸다.
- Then:
  - 두 입력 모두 A 인스턴스의 `in1` 경계 포트로 **fan-in** 된다(일반 다중 소스 와이어와 동일 큐 의미).
  - A 의 `out1` 출력은 fn1.out1·fn2.out1 **양쪽**(out1 에 연결된 모든 flow-node)으로 **fan-out** 된다.
  - 인터리빙 순서가 일반 플로우 다중 소스와 동일하며 브리지가 별도 재정렬을 도입하지 않는다.

### AC-5: 미실행 시 오프라인 대기 (REQ-SUBFLOW2-L01, L02)

- Given: 플로우 A 가 **실행되지 않은** 상태이고, A 를 `shared` 로 참조하는 부모 B 가 있다.
- When: B 를 배포한다.
- Then:
  - 배포가 실패하지 않는다.
  - flow-node 가 **브리지 오프라인 상태**로 표시된다(원격 브리지 오프라인과 동일 시각 언어).
  - flow-node 는 데이터를 패싱하지 않는다(무출력).
  - 시스템은 A 를 자동으로 시작하지 않는다.

### AC-6: 시작 감지 자가치유 (REQ-SUBFLOW2-L03)

- Given: AC-5 상태(B 배포됨, A 미실행 → flow-node 오프라인 대기).
- When: 플로우 A 를 (독립적으로) 시작한다.
- Then:
  - 시스템이 A 의 시작을 감지하여 브리지를 **자동 연결**한다(self-healing).
  - flow-node 상태가 `연결됨·실행중` 으로 바뀌고 데이터 패싱이 시작된다(부모 재시작 불필요).

### AC-7: 정지 시 끊김 + 재시작 자가치유 (REQ-SUBFLOW2-L04, L05)

- Given: A 가 실행 중이고 B 의 `shared` flow-node 가 연결되어 데이터가 흐른다.
- When: 플로우 A 를 (독립적으로) 정지(undeploy/stop)한 뒤 다시 시작한다.
- Then:
  - A 정지 시 flow-node 가 **연결 끊김(오프라인)** 으로 표시되고 패싱이 중단된다.
  - A 재시작 시 부모 재시작 없이 브리지가 **자동 재연결**된다(bounded 백오프 재open).

### AC-8: 참조 카운팅 없음·teardown 격리 (REQ-SUBFLOW2-L06, L07)

- Given: 두 부모 B·C 가 같은 실행 중 플로우 A 를 `shared` 로 참조하여 연결되어 있다.
- When: 부모 B 를 undeploy 한다.
- Then:
  - B 의 브리지 엔드포인트만 teardown 된다.
  - A 인스턴스는 계속 실행되며(참조 카운팅 기반 자동 정지 없음), C 의 브리지도 영향받지 않는다.

### AC-9: instance 모드 인라인 확장 보존 (REQ-SUBFLOW2-IN01, IN02)

- Given: 플로우 A 를 `mode: instance` 로 참조하는 flow-node 를 포함한 부모 B 가 있다.
- When: B 를 배포한다.
- Then:
  - A 의 서브그래프가 `subflow_<flowNodeID>_*` 네임스페이스로 인라인 확장된다(SPEC-SUBFLOW-001 그룹 D 동일).
  - 같은 플로우를 참조하는 instance flow-node 2개는 상태 비공유 독립 인스턴스로 확장된다.
  - 기존 동작과 동일하다(회귀 0).

### AC-10: 모드 혼합 배포 (REQ-SUBFLOW2-IN03)

- Given: 한 부모 B 가 `shared` flow-node(→A)와 `instance` flow-node(→C)를 혼합 포함한다(A·C 중 A 는 실행 중).
- When: B 를 배포한다.
- Then:
  - A 참조 flow-node 는 미확장 + 라이브 브리지로 동작한다.
  - C 참조 flow-node 는 인라인 확장된다.
  - 두 처리가 서로 간섭하지 않는다.

### AC-11: 마이그레이션 기본 shared (REQ-SUBFLOW2-MG01, MG02)

- Given: SPEC-SUBFLOW-001 시절 저장된 LOCAL flow-node(`mode` 키 없음)를 포함한 부모 B 가 있고 참조 플로우 A 가 실행 중이다.
- When: B 를 재배포한다.
- Then:
  - flow-node 가 `shared` 로 동작한다(인라인 복사본 대신 A 단일 인스턴스 연결) — 의도된 breaking 변경.
  - 사용자가 flow-node 에 `mode: instance` 를 명시하면 SPEC-SUBFLOW-001 인라인 확장 동작이 그대로 복원된다.

### AC-12: remote 참조 불변 (REQ-SUBFLOW2-MG03, M04)

- Given: `remote://{instance}/{flow}` 를 참조하는 flow-node 를 포함한 부모 B 가 있다.
- When: B 를 배포한다.
- Then:
  - 원격 라이브 브리지가 SPEC-SUBFLOW-001 그룹 RB 와 동일하게 동작한다(본 변경과 무관).
  - `mode` 는 원격 참조 종류와 직교하게 처리된다.

### AC-13: shared 순환 거부 (REQ-SUBFLOW2-CY01, CY02)

- Given: 플로우 A 가 B 를 `shared` 로 참조하고, B 가 A 를 `shared`(또는 instance)로 참조하도록 구성(A↔B)한다.
- When: 저장 또는 배포를 시도한다.
- Then:
  - 저장·배포 양쪽에서 순환이 검출되어 거부된다.
  - 에러에 순환 경로(예: A → B → A)가 포함된다.
  - 직접 자기참조(`flow_id = self`)도 거부된다.

### AC-14: 항상 최신 — 편집 반영 = 재시작 시 (REQ-SUBFLOW2-SH05)

- Given: A 가 실행 중이고 B 의 `shared` flow-node 가 A 에 연결되어 있다.
- When: A 의 내부 로직을 편집·저장한 뒤 A 를 **재시작(재배포)** 한다(B 는 그대로 둔다).
- Then:
  - A 재시작 시 새 정의가 A 실행 인스턴스에 반영된다.
  - B 의 flow-node 는 부모 재시작 없이 갱신된 A 인스턴스에 자동 재연결된다(자가치유).
  - A 를 재시작하기 전에는 편집이 반영되지 않는다(사용자에게 안내됨, W03).

### AC-15: 핸들 파생·dangling (REQ-SUBFLOW2-P01, P02)

- Given: A(입력 `in1`/출력 `out1`)를 `shared` 로 참조하는 flow-node 가 있다.
- When: flow-node 를 렌더하고, 이후 A 에서 `out1` 포트를 삭제한다.
- Then:
  - flow-node 입력 핸들에 `in1`, 출력 핸들에 `out1` 이 표시된다(mode 무관, 참조 플로우 경계 포트 파생).
  - `out1` 삭제 후 그 핸들에 걸린 부모 와이어가 dangling 으로 표시(경고)되고 정리 경로가 제공된다.

### AC-16: 통계 정합 (REQ-SUBFLOW2-S01, S02, S03)

- Given: 부모 B 가 A 를 `shared` 로, C 를 `instance` 로 참조한다.
- When: B 의 통계 화면을 본다.
- Then:
  - `shared`(A) flow-node 통계는 **참조 플로우 A 자체의 통계**로 표시된다(임베디드 병합 없음).
  - `instance`(C) flow-node 통계는 기존 subflow-stats 임베디드 병합으로 표시된다.
  - 두 모드가 화면에서 구분되어(모드 배지) 운영자가 의미를 혼동하지 않는다.

### AC-17: 웹 UI mode 토글·상태·안내 (REQ-SUBFLOW2-W01, W02, W03, W04)

- Given: flow-node 설정 패널을 연다.
- When: mode 토글을 `공유`/`인스턴스` 로 전환하고, 참조 플로우 실행 상태를 바꾼다.
- Then:
  - mode 토글이 동작하고 선택이 config 에 저장된다(기본 공유).
  - `shared` 일 때 연결 상태 인디케이터(미연결/연결됨·실행중/오프라인/오류)가 원격 브리지와 일관된 시각 언어로 표시된다.
  - "참조 플로우 편집은 그 플로우 재시작 시 반영" 안내가 표시된다.
  - 참조 플로우 오프라인 시 flow-node 무출력을 운영자가 식별할 수 있다.

### AC-18: 회귀·라우팅 보존·백프레셔 (REQ-SUBFLOW2-N01, N02, N03, N04)

- Given: flow-node 미포함 플로우, instance flow-node 플로우, remote flow-node 플로우가 각각 있다.
- When: 각각을 배포·실행하고, shared 브리지에 고부하 메시지를 흘린다.
- Then:
  - 세 플로우 모두 기존과 동일하게 동작한다(회귀 0).
  - shared 브리지를 통과한 메시지가 일반 와이어와 동일한 라우팅·큐·카운터 의미로 동작한다.
  - 포트별 유계 버퍼 오버플로 정책(oldest-drop/coalesce)이 적용되어 메모리가 폭증하지 않는다.
  - 오프라인 대기·재연결 시 bounded 백오프로 핫스핀이 발생하지 않는다.

## 2. Definition of Done

- [ ] flow-node `mode`(shared|instance) config + 미지정→shared 정규화(M01~M04).
- [ ] shared 모드: 미확장 + in-process 라이브 브리지 + 양방향 포트 라우팅(SH01~SH05).
- [ ] 다중참조 fan-in/fan-out 단일 인스턴스 공유(MR01~MR04).
- [ ] 생명주기: 자동 시작 없음·오프라인 대기·자가치유·teardown 격리(L01~L07).
- [ ] instance 모드 인라인 확장 보존·모드 혼합(IN01~IN03).
- [ ] 마이그레이션 미지정→shared(breaking)·instance 명시 복원·remote 불변(MG01~MG03).
- [ ] 통계 정합: shared 직접 / instance 병합 / 모드 구분(S01~S03).
- [ ] 순환 거부: shared/혼합(CY01~CY02).
- [ ] 포트 파생·dangling(P01~P02).
- [ ] 웹 UI: mode 토글·상태 인디케이터·편집 안내·오프라인 식별(W01~W04).
- [ ] 비기능: 회귀 0·라우팅 보존·유계 버퍼·백오프(N01~N04).
- [ ] 테스트 커버리지 85%+ · `go test -race ./...` 통과 · golangci-lint 클린.
- [ ] 마이그레이션 가이드·breaking 영향 문서화.
