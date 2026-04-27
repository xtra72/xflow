# SPEC-DEBUG-001 수락 기준

## 관련 SPEC

- SPEC ID: SPEC-DEBUG-001
- 제목: Output 노드를 Debug 노드로 통합

---

## 수락 시나리오

### AC-1: 템플릿 설정 시 템플릿 출력 (REQ-1)

```gherkin
Given debug 노드가 config.template = "온도: {{.temperature}}C" 로 설정되어 있고
  And payload에 {"temperature": 25.5, "humidity": 60} 이 포함되어 있을 때
When 메시지가 Process를 통과하면
Then 출력에 "온도: 25.5C" 가 포함되어야 한다
```

### AC-2: 템플릿 미설정 시 기본 포맷 출력 (REQ-1)

```gherkin
Given debug 노드가 config.template 없이 설정되어 있고
  And 메시지 ID가 "msg-001"이고 payload에 {"key": "value"} 가 있을 때
When 메시지가 Process를 통과하면
Then 출력이 "message id=msg-001 payload=map[key:value]" 형식이어야 한다
```

### AC-3: 출력 대상 - logger (REQ-2)

```gherkin
Given debug 노드가 config.output = "logger", config.level = "info" 로 설정되어 있을 때
When 메시지가 Process를 통과하면
Then 노드의 로거를 통해 info 레벨로 메시지가 기록되어야 한다
```

### AC-4: 출력 대상 - terminal (REQ-2)

```gherkin
Given debug 노드가 config.output = "terminal" 로 설정되어 있을 때
When 메시지가 Process를 통과하면
Then os.Stdout에 메시지가 직접 출력되어야 한다
```

### AC-5: 출력 대상 - file (REQ-2)

```gherkin
Given debug 노드가 config.output = "file", config.file = "/tmp/debug.log" 로 설정되어 있을 때
When 메시지가 Process를 통과하면
Then "/tmp/debug.log" 파일에 메시지가 기록되어야 한다
  And 파일에 줄바꿈이 포함되어야 한다
```

### AC-6: 출력 대상 - editor (REQ-2)

```gherkin
Given debug 노드에 DebugSink가 주입되어 있고
  And config.output = "editor" 로 설정되어 있을 때
When 메시지가 Process를 통과하면
Then DebugSink.SendDebug가 노드 ID와 출력 메시지로 호출되어야 한다
```

### AC-7: 선택적 필드 - 전체 출력 (REQ-3)

```gherkin
Given debug 노드가 config.fields 없이 설정되어 있고
  And payload에 {"a": 1, "b": 2, "c": 3} 이 있을 때
When 메시지가 Process를 통과하면
Then 출력에 a, b, c 모든 필드가 포함되어야 한다
```

### AC-8: 선택적 필드 - 지정 필드만 출력 (REQ-3)

```gherkin
Given debug 노드가 config.fields = ["a", "c"] 로 설정되어 있고
  And payload에 {"a": 1, "b": 2, "c": 3} 이 있을 때
When 메시지가 Process를 통과하면
Then 출력에 a, c 필드만 포함되어야 한다
  And b 필드는 출력에 포함되지 않아야 한다
```

### AC-9: prefix 설정 시 적용 (REQ-4)

```gherkin
Given debug 노드가 config.prefix = "[센서]" 로 설정되어 있을 때
When 메시지가 Process를 통과하면
Then 출력이 "[센서]" 로 시작해야 한다
```

### AC-10: prefix 미설정 시 노드 이름 사용 (REQ-4)

```gherkin
Given debug 노드의 이름이 "my-debug"이고 config.prefix가 설정되지 않았을 때
When 메시지가 Process를 통과하면
Then 출력이 노드 이름을 prefix로 사용해야 한다
```

### AC-11: output 타입 하위 호환성 (REQ-5)

```gherkin
Given 레지스트리에서 "output" 타입으로 노드를 생성할 때
When NewDebugNode 팩토리가 호출되면
Then DebugNode 인스턴스가 정상 생성되어야 한다
  And output 노드의 기존 config 키(prefix, template, file)가 모두 동작해야 한다
```

### AC-12: 기존 debug config 호환성 (REQ-5)

```gherkin
Given debug 노드가 config.level = "warn", config.file = "/tmp/test.log" 로 설정되어 있을 때
When 메시지가 Process를 통과하면
Then warn 레벨로 로깅되어야 한다
  And 파일에 기록되어야 한다
  And 기존 debug 노드와 동일한 출력 포맷이어야 한다
```

### AC-13: 기존 output config 호환성 (REQ-5)

```gherkin
Given "output" 타입의 노드가 config.template = "{{.value}}", config.prefix = "[out]" 로 설정되어 있을 때
When 메시지가 Process를 통과하면
Then 템플릿이 정상 실행되어야 한다
  And prefix가 "[out]"으로 적용되어야 한다
```

### AC-14: Pass-through 동작 (REQ-6)

```gherkin
Given debug 노드가 임의의 설정으로 구성되어 있고
  And 입력 메시지의 payload가 {"key": "value"} 이고 metadata가 {"source": "test"} 일 때
When 메시지가 Process를 통과하면
Then 반환된 메시지는 입력 메시지와 동일해야 한다
  And payload와 metadata가 변경되지 않아야 한다
```

### AC-15: 동시 접근 스레드 안전성 (REQ-7)

```gherkin
Given debug 노드가 config.output = "file" 로 설정되어 있을 때
When 여러 고루틴에서 동시에 Process를 호출하면
Then 데이터 경합(race condition)이 발생하지 않아야 한다
  And 파일 출력이 손상되지 않아야 한다
```

### AC-16: output.go 파일 삭제 (REQ-8)

```gherkin
Given SPEC-DEBUG-001 구현이 완료되었을 때
When 프로젝트 파일 구조를 확인하면
Then internal/node/output.go 파일이 존재하지 않아야 한다
  And internal/node/output_test.go 파일이 존재하지 않아야 한다
  And go test -race ./internal/node/... 가 통과해야 한다
```

---

## 품질 게이트

### Definition of Done

- [ ] 모든 수락 시나리오(AC-1 ~ AC-16) 통과
- [ ] `go test -race ./internal/node/...` 통과
- [ ] 코드 커버리지 85%+ (`go test -cover ./internal/node/...`)
- [ ] `go vet ./internal/node/...` 경고 없음
- [ ] output.go, output_test.go 삭제 완료
- [ ] 레지스트리에서 "output" -> NewDebugNode 별칭 등록 확인
- [ ] 기존 example YAML 파일에서 "output" 타입 사용 시 정상 동작 확인

### 검증 방법

| 검증 항목 | 도구/방법 |
|-----------|-----------|
| 단위 테스트 | `go test -race -v ./internal/node/...` |
| 커버리지 | `go test -cover -coverprofile=coverage.out ./internal/node/...` |
| 경합 감지 | `go test -race ./internal/node/...` |
| 정적 분석 | `go vet ./internal/node/...` |
| 하위 호환성 | 기존 example YAML 로딩 테스트 |
| 통합 테스트 | debug 노드를 포함한 플로우 실행 |

---

## 추적성 태그

- SPEC-DEBUG-001/REQ-1 -> AC-1, AC-2
- SPEC-DEBUG-001/REQ-2 -> AC-3, AC-4, AC-5, AC-6
- SPEC-DEBUG-001/REQ-3 -> AC-7, AC-8
- SPEC-DEBUG-001/REQ-4 -> AC-9, AC-10
- SPEC-DEBUG-001/REQ-5 -> AC-11, AC-12, AC-13
- SPEC-DEBUG-001/REQ-6 -> AC-14
- SPEC-DEBUG-001/REQ-7 -> AC-15
- SPEC-DEBUG-001/REQ-8 -> AC-16
