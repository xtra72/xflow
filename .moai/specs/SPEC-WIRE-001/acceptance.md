# SPEC-WIRE-001: 수용 기준

## 관련 SPEC

- SPEC ID: SPEC-WIRE-001
- 제목: Wire 구조체 Name/Type 필드 추가

---

## 수용 기준 (Acceptance Criteria)

### AC-1: Wire ID UUID 규격 (REQ-1)

**Scenario: NewWire 팩토리로 생성된 Wire의 ID가 UUID 형식이다**

```gherkin
Given NewWire 팩토리 함수가 호출될 때
When sourceNodeID="node-a", sourcePort="output", targetNodeID="node-b", targetPort="input"으로 Wire를 생성하면
Then Wire.ID는 UUID v4 형식(8-4-4-4-12 hex)이어야 한다
```

**Scenario: YAML 정규화 시 빈 ID가 UUID로 채워진다**

```gherkin
Given YAML 플로우 파일에서 Wire의 id가 비어있을 때
When normalizeWireDefaults가 실행되면
Then Wire.ID는 UUID v4 형식으로 자동 생성되어야 한다
And 기존 "<flowName>.wire-<index>" 형식은 더 이상 생성되지 않아야 한다
```

---

### AC-2: Wire Name 자동 생성 (REQ-2)

**Scenario: Wire Name이 노드 이름과 포트로 자동 생성된다**

```gherkin
Given 노드 "mqtt-sub"의 "output" 포트에서 노드 "lg_hvacr02-enc"의 "input" 포트로 연결된 Wire가 있을 때
When normalizeWireNames가 실행되면
Then Wire.Name은 "mqtt-sub.output_to_lg_hvacr02-enc.input"이어야 한다
```

**Scenario: 노드 이름이 비어있을 때 노드 ID를 사용한다**

```gherkin
Given 노드 ID "node-abc123"의 Name이 비어있고 "output" 포트에서 연결된 Wire가 있을 때
When normalizeWireNames가 실행되면
Then Wire.Name에서 소스 노드 부분은 "node-abc123"이어야 한다
```

**Scenario: YAML에서 name이 명시적으로 제공된 경우 덮어쓰지 않는다**

```gherkin
Given YAML 플로우 파일에서 Wire의 name이 "custom-wire-name"으로 지정되어 있을 때
When normalizeWireNames가 실행되면
Then Wire.Name은 "custom-wire-name"이 유지되어야 한다
```

---

### AC-3: Wire Type 기본값 (REQ-3)

**Scenario: NewWire 팩토리의 Type 기본값이 "simple"이다**

```gherkin
Given NewWire 팩토리 함수가 호출될 때
When 별도의 Type 옵션 없이 Wire를 생성하면
Then Wire.Type은 WireSimple("simple")이어야 한다
```

**Scenario: YAML 정규화 시 빈 Type이 "simple"로 채워진다**

```gherkin
Given YAML 플로우 파일에서 Wire의 type이 비어있을 때
When normalizeWireDefaults가 실행되면
Then Wire.Type은 "simple"이어야 한다
```

**Scenario: YAML에서 type이 명시적으로 제공된 경우 유지된다**

```gherkin
Given YAML 플로우 파일에서 Wire의 type이 "simple"로 지정되어 있을 때
When normalizeWireDefaults가 실행되면
Then Wire.Type은 "simple"이 유지되어야 한다
```

---

### AC-4: RuntimeWire 동기화 (REQ-4)

**Scenario: CreateRuntimeWires가 Name과 Type을 복사한다**

```gherkin
Given flow.Wire에 Name="mqtt-sub.output_to_lg_hvacr02-enc.input", Type="simple"이 설정되어 있을 때
When CreateRuntimeWires가 실행되면
Then RuntimeWire.Name은 "mqtt-sub.output_to_lg_hvacr02-enc.input"이어야 한다
And RuntimeWire.Type은 "simple"이어야 한다
```

---

### AC-5: 하위 호환성 (REQ-5)

**Scenario: 기존 YAML 파일이 name/type 없이 정상 파싱된다**

```gherkin
Given 기존 YAML 플로우 파일에 Wire의 name과 type 필드가 없을 때
When UnmarshalFlow가 실행되면
Then 에러 없이 정상적으로 파싱되어야 한다
And Wire.Name은 자동 생성되어야 한다
And Wire.Type은 "simple"이어야 한다
```

**Scenario: 레거시 edges 형식에서도 기본값이 적용된다**

```gherkin
Given 레거시 "edges" 키를 사용하는 YAML 플로우 파일이 있을 때
When normalizeEdgesToWires로 변환 후 normalizeWireDefaults가 실행되면
Then 변환된 Wire의 Type은 "simple"이어야 한다
And 변환된 Wire의 ID는 UUID 형식이어야 한다
```

---

### AC-6: JSON 직렬화/역직렬화 (비기능)

**Scenario: Wire의 JSON 직렬화에 name과 type이 포함된다**

```gherkin
Given Name="a.out_to_b.in", Type="simple"인 Wire가 있을 때
When JSON으로 마샬링하면
Then JSON에 "name":"a.out_to_b.in" 필드가 포함되어야 한다
And JSON에 "type":"simple" 필드가 포함되어야 한다
```

**Scenario: name과 type이 포함된 JSON이 정상 역직렬화된다**

```gherkin
Given {"name":"custom","type":"simple",...} 형태의 JSON Wire 데이터가 있을 때
When JSON에서 Wire로 언마샬링하면
Then Wire.Name은 "custom"이어야 한다
And Wire.Type은 "simple"이어야 한다
```

---

## Quality Gate 기준

| 항목 | 기준 |
|------|------|
| 테스트 커버리지 | 변경 파일 85% 이상 |
| 기존 테스트 | 모든 기존 테스트 통과 |
| go vet | 경고 없음 |
| 빌드 | `go build ./...` 성공 |
| 하위 호환성 | 기존 YAML 예제 파일 정상 로드 |

---

## Definition of Done

- [ ] `WireType` 타입 및 `WireSimple` 상수 정의 완료
- [ ] `Wire` 구조체에 `Name`, `Type` 필드 추가 완료
- [ ] `NewWire` 팩토리에서 `Type` 기본값 설정
- [ ] `normalizeWireDefaults`에서 ID를 UUID로, Type을 "simple"로 기본 설정
- [ ] `normalizeWireNames` 함수 구현 및 정규화 파이프라인에 통합
- [ ] `RuntimeWire`에 `Name`, `Type` 필드 추가 및 복사 로직 구현
- [ ] 단위 테스트 작성 및 통과
- [ ] 기존 테스트 모두 통과
- [ ] `go build ./...` 성공
- [ ] `go test -race ./pkg/flow/... ./internal/engine/...` 통과
