# SPEC-OBS-003: 인수 기준

## 개요

노드별 로그 출력 라우팅 기능의 인수 기준을 정의한다. 모든 시나리오는 Given-When-Then 형식으로 작성한다.

---

## 시나리오 1: ParseLogOutput - stdout 형식 파싱

```gherkin
Given log_output 값이 "stdout"일 때
When ParseLogOutput을 호출하면
Then LogOutputTarget.UseStdout은 true이다
And LogOutputTarget.FilePath는 빈 문자열이다
And 에러는 nil이다
```

## 시나리오 2: ParseLogOutput - 파일 전용 형식 파싱

```gherkin
Given log_output 값이 "/var/log/xflow/node.log"일 때
When ParseLogOutput을 호출하면
Then LogOutputTarget.UseStdout은 false이다
And LogOutputTarget.FilePath는 "/var/log/xflow/node.log"이다
And 에러는 nil이다
```

## 시나리오 3: ParseLogOutput - stdout+file 형식 파싱

```gherkin
Given log_output 값이 "stdout+/var/log/xflow/node.log"일 때
When ParseLogOutput을 호출하면
Then LogOutputTarget.UseStdout은 true이다
And LogOutputTarget.FilePath는 "/var/log/xflow/node.log"이다
And 에러는 nil이다
```

## 시나리오 4: ParseLogOutput - 빈 문자열 처리

```gherkin
Given log_output 값이 빈 문자열일 때
When ParseLogOutput을 호출하면
Then LogOutputTarget.UseStdout은 false이다
And LogOutputTarget.FilePath는 빈 문자열이다
And 에러는 nil이다
```

## 시나리오 5: ParseLogOutput - 잘못된 형식 에러

```gherkin
Given log_output 값이 "stdout+"일 때 (stdout+ 뒤에 빈 경로)
When ParseLogOutput을 호출하면
Then 에러가 반환된다
And 에러 메시지에 잘못된 형식임을 알리는 내용이 포함된다
```

---

## 시나리오 6: resolveNodeLogOutput - 노드 레벨 설정 우선

```gherkin
Given 노드 config["log_output"]이 "/var/log/xflow/node-a.log"이고
And 플로우 config.log_output이 "/var/log/xflow/flow.log"이고
And 서버 기본값이 "stdout"일 때
When resolveNodeLogOutput을 호출하면
Then 반환된 LogOutputTarget.FilePath는 "/var/log/xflow/node-a.log"이다
And explicit 플래그는 true이다
```

## 시나리오 7: resolveNodeLogOutput - 플로우 레벨 폴백

```gherkin
Given 노드 config["log_output"]이 설정되지 않았고
And 플로우 config.log_output이 "/var/log/xflow/flow.log"일 때
When resolveNodeLogOutput을 호출하면
Then 반환된 LogOutputTarget.FilePath는 "/var/log/xflow/flow.log"이다
And explicit 플래그는 true이다
```

## 시나리오 8: resolveNodeLogOutput - 서버 기본값 폴백

```gherkin
Given 노드 config["log_output"]이 설정되지 않았고
And 플로우 config.log_output이 빈 문자열이고
And 서버 기본값이 "/var/log/xflow/server.log"일 때
When resolveNodeLogOutput을 호출하면
Then 반환된 LogOutputTarget.FilePath는 "/var/log/xflow/server.log"이다
And explicit 플래그는 true이다
```

## 시나리오 9: resolveNodeLogOutput - 설정 없음

```gherkin
Given 노드 config["log_output"]이 설정되지 않았고
And 플로우 config.log_output이 빈 문자열이고
And 서버 기본값이 빈 문자열일 때
When resolveNodeLogOutput을 호출하면
Then explicit 플래그는 false이다
```

---

## 시나리오 10: FlowConfig - LogOutput 필드 JSON 직렬화

```gherkin
Given FlowConfig에 LogOutput이 "/var/log/xflow/mqtt.log"로 설정되어 있을 때
When Flow를 JSON으로 직렬화하면
Then JSON 출력에 "log_output": "/var/log/xflow/mqtt.log" 키-값이 포함된다
```

## 시나리오 11: FlowConfig - LogOutput 빈 문자열 omitempty

```gherkin
Given FlowConfig에 LogOutput이 빈 문자열일 때
When Flow를 JSON으로 직렬화하면
Then JSON 출력에 "log_output" 키가 포함되지 않는다
```

## 시나리오 12: FlowConfig - LogOutput YAML 역직렬화

```gherkin
Given YAML 파일에 다음 설정이 포함되어 있을 때:
  config:
    log_level: "info"
    log_output: "/var/log/xflow/mqtt.log"
When FlowFromYAML로 역직렬화하면
Then FlowConfig.LogOutput은 "/var/log/xflow/mqtt.log"이다
And FlowConfig.LogLevel은 "info"이다
```

## 시나리오 13: FlowConfig - LogOutput 미포함 YAML 역호환성

```gherkin
Given 기존 YAML 파일에 log_output 키가 없을 때
When FlowFromYAML로 역직렬화하면
Then FlowConfig.LogOutput은 빈 문자열이다
And 다른 필드는 정상적으로 파싱된다
```

---

## 시나리오 14: DeployFlow - 파일 출력 라우팅 등록

```gherkin
Given Observer가 설정된 Engine이 있고
And 노드 config["log_output"]이 "/tmp/test-node.log"인 Flow가 있을 때
When DeployFlow를 호출하면
Then "/tmp/test-node.log" 파일이 생성된다
And observer.Streams.Routes("node.<노드이름>")에 파일 writer가 등록된다
And flowRuntime.closers에 파일 핸들이 추가된다
```

## 시나리오 15: DeployFlow - stdout+file 이중 출력

```gherkin
Given Observer가 설정된 Engine이 있고
And 노드 config["log_output"]이 "stdout+/tmp/test-dual.log"인 Flow가 있을 때
When DeployFlow를 호출하면
Then "/tmp/test-dual.log" 파일이 생성된다
And observer.Streams.Routes("node.<노드이름>")에 파일 writer와 stdout writer가 모두 등록된다
```

## 시나리오 16: DeployFlow - stdout 전용 (명시적)

```gherkin
Given Observer가 설정된 Engine이 있고
And 노드 config["log_output"]이 "stdout"인 Flow가 있을 때
When DeployFlow를 호출하면
Then observer.Streams.Routes("node.<노드이름>")는 nil이다 (기본 stdout 사용)
And flowRuntime.closers에 추가된 핸들이 없다
```

## 시나리오 17: DeployFlow - 파일 열기 실패 시 에러

```gherkin
Given Observer가 설정된 Engine이 있고
And 노드 config["log_output"]이 존재하지 않는 읽기 전용 경로일 때
When DeployFlow를 호출하면
Then 에러가 반환된다
And 에러 메시지에 실패한 파일 경로와 원인이 포함된다
And 플로우는 배포되지 않는다
```

## 시나리오 18: DeployFlow - 디렉토리 자동 생성

```gherkin
Given Observer가 설정된 Engine이 있고
And 노드 config["log_output"]이 "/tmp/xflow-test/subdir/node.log"이고
And "/tmp/xflow-test/subdir/" 디렉토리가 존재하지 않을 때
When DeployFlow를 호출하면
Then "/tmp/xflow-test/subdir/" 디렉토리가 자동 생성된다
And "/tmp/xflow-test/subdir/node.log" 파일이 생성된다
And DeployFlow가 성공한다
```

## 시나리오 19: DeployFlow - 다중 노드 파일 열기 실패 시 정리

```gherkin
Given Observer가 설정된 Engine이 있고
And 첫 번째 노드의 log_output은 "/tmp/test-node1.log" (정상 경로)이고
And 두 번째 노드의 log_output은 잘못된 경로일 때
When DeployFlow를 호출하면
Then 에러가 반환된다
And 첫 번째 노드를 위해 열린 파일 핸들이 닫힌다 (정리 완료)
```

---

## 시나리오 20: StopFlow - 파일 핸들 정리

```gherkin
Given log_output으로 파일이 설정된 노드가 있는 Flow가 배포되어 실행 중일 때
When StopFlow를 호출하면
Then flowRuntime.closers의 모든 파일 핸들이 Close()된다
And closers 슬라이스가 nil로 초기화된다
```

## 시나리오 21: UndeployFlow - 파일 핸들 정리

```gherkin
Given log_output으로 파일이 설정된 노드가 있는 Flow가 배포된 상태일 때
When UndeployFlow를 호출하면
Then flowRuntime.closers의 모든 파일 핸들이 Close()된다
And StreamRouter에서 해당 컴포넌트의 라우트가 제거된다
```

---

## 시나리오 22: 계층적 상속 - 플로우 레벨 기본값

```gherkin
Given 플로우 config.log_output이 "/var/log/xflow/flow.log"이고
And 3개 노드 중 1개만 config["log_output"]이 설정되어 있을 때
When DeployFlow를 호출하면
Then log_output이 설정된 노드는 자신의 설정 파일로 출력된다
And log_output이 미설정된 2개 노드는 플로우 레벨 "/var/log/xflow/flow.log"로 출력된다
```

## 시나리오 23: BaseNode 미변경 확인

```gherkin
Given BaseNode 구조체의 현재 필드 목록이 있을 때
When SPEC-OBS-003 구현이 완료된 후
Then BaseNode 구조체의 필드는 변경되지 않았다
And 노드는 자신의 로그 출력 대상을 알지 못한다
```

---

## 시나리오 24: 파일 append 모드 확인

```gherkin
Given 이미 내용이 있는 로그 파일이 존재하고
And 해당 파일을 log_output으로 사용하는 Flow를 배포할 때
When 노드에서 로그를 기록하면
Then 기존 파일 내용이 보존되고 새로운 로그가 뒤에 추가된다
```

## 시나리오 25: 동시성 안전성

```gherkin
Given log_output으로 파일이 설정된 여러 노드가 있는 Flow가 실행 중일 때
When go test -race 플래그로 테스트를 실행하면
Then 데이터 레이스가 감지되지 않는다
```

---

## 품질 게이트

| 기준 | 목표 |
|------|------|
| 테스트 커버리지 | `internal/engine/log_output.go` 85% 이상 |
| 테스트 커버리지 | `pkg/flow/flow.go` (FlowConfig 변경) 기존 커버리지 유지 |
| Race Detector | `go test -race ./internal/engine/... ./pkg/flow/...` 통과 |
| 기존 테스트 | `go test ./...` 전체 통과 (역호환성) |
| 린트 | `golangci-lint run ./internal/engine/... ./pkg/flow/...` 통과 |
| 코드 리뷰 | `resolveNodeLogLevel`과 동일한 패턴 사용 확인 |

---

## 검증 방법

### 단위 테스트

- `internal/engine/log_output_test.go`: `ParseLogOutput`, `resolveNodeLogOutput`, `openLogFile`
- `pkg/flow/flow_test.go`: `FlowConfig.LogOutput` 필드 포함 테스트
- `pkg/flow/serialize_test.go`: JSON/YAML 직렬화 역호환성 테스트

### 통합 테스트

- `internal/engine/engine_test.go`: `DeployFlow` -> 로그 출력 -> `StopFlow` 전체 흐름
- 실제 파일 생성/삭제를 포함한 통합 시나리오 (`t.TempDir()` 활용)

### 수동 검증

- 예제 Flow YAML (`examples/flows/mqtt-metrics.yaml`)에 `log_output` 추가하여 테스트
- `xflow flow deploy` 명령으로 실제 배포 후 파일 생성 확인

---

## Definition of Done

- [ ] `ParseLogOutput` 함수 구현 및 테스트 통과
- [ ] `resolveNodeLogOutput` 함수 구현 및 테스트 통과
- [ ] `FlowConfig.LogOutput` 필드 추가 및 직렬화 테스트 통과
- [ ] `flowRuntime.closers` 필드 추가
- [ ] `DeployFlow` 로그 출력 라우팅 통합
- [ ] `StopFlow`/`UndeployFlow` 파일 핸들 정리
- [ ] 디렉토리 자동 생성 (`os.MkdirAll`)
- [ ] 에러 발생 시 rollback (이미 열린 파일 정리)
- [ ] `go test -race ./...` 통과
- [ ] 테스트 커버리지 85% 이상
- [ ] 기존 테스트 전체 통과
