# SPEC-DEBUG-001 구현 계획

## 관련 SPEC

- SPEC ID: SPEC-DEBUG-001
- 제목: Output 노드를 Debug 노드로 통합

---

## 개발 방법론

- **모드**: hybrid
- **기존 코드 (debug.go, registry.go)**: DDD (ANALYZE-PRESERVE-IMPROVE)
- **새 기능 (template, output dest, fields)**: TDD (RED-GREEN-REFACTOR)
- **테스트 커버리지 목표**: 85%+

---

## 마일스톤

### M0: 분석 및 기존 동작 보존 (Priority: Critical)

**목표**: 기존 debug/output 노드의 동작을 완전히 이해하고 특성화 테스트 작성

1. `debug.go`, `output.go` 코드 분석 및 동작 매핑
2. `debug_test.go`에 기존 debug 노드 특성화(characterization) 테스트 보강
3. `output_test.go`의 기존 테스트를 `debug_test.go`로 이관 (output 노드 동작 기준)
4. 모든 특성화 테스트가 통과하는지 확인

**완료 기준**: 기존 debug/output 동작을 커버하는 특성화 테스트 통과

### M1: 메시지 템플릿 및 Prefix 기능 (Priority: High)

**목표**: REQ-1, REQ-4 구현

1. **RED**: template 설정 시 Go text/template으로 출력하는 테스트 작성 (실패 확인)
2. **GREEN**: DebugNode에 `tmpl *template.Template`, `prefix string` 필드 추가
3. Configure에서 `template`, `prefix` config 키 파싱 구현
4. Process에서 template 실행 로직 구현
5. prefix 적용 로직 구현 (미설정 시 노드 이름 사용)
6. **REFACTOR**: 출력 포맷팅 로직 정리

**완료 기준**: template/prefix 관련 테스트 통과, 기존 특성화 테스트 유지

### M2: 출력 대상 선택 (Priority: High)

**목표**: REQ-2 구현

1. **RED**: 각 출력 대상(logger/terminal/file/editor) 테스트 작성
2. **GREEN**: `outputDest string` 필드 추가 및 Configure에서 파싱
3. Process에서 출력 대상 분기 로직 구현:
   - `logger`: 기존 로거 사용 (기본값)
   - `terminal`: `os.Stdout`에 직접 출력
   - `file`: 파일에 출력 (기존 file 출력 로직 활용)
   - `editor`: DebugSink 인터페이스 호출
4. DebugSink 인터페이스 정의
5. **REFACTOR**: 출력 전략 패턴 적용

**완료 기준**: 모든 출력 대상 테스트 통과

### M3: 선택적 필드 출력 (Priority: Medium)

**목표**: REQ-3 구현

1. **RED**: fields 설정에 따른 payload 필터링 테스트 작성
2. **GREEN**: `fields []string` 필드 추가 및 Configure에서 파싱
3. Process에서 fields 필터링 로직 구현
4. 빈 fields/nil fields 시 전체 payload 출력 확인
5. **REFACTOR**: 필터링 로직을 별도 함수로 분리

**완료 기준**: fields 필터링 테스트 통과, 기존 테스트 유지

### M4: 하위 호환성 확보 (Priority: High)

**목표**: REQ-5 구현

1. registry.go에서 "output" 팩토리를 `NewDebugNode`로 변경
2. "output" 타입으로 노드 생성 시 정상 동작 테스트
3. output 노드의 config 키(prefix, template, file)가 debug 노드에서 동작 확인
4. 기존 debug 노드 config 키(level, file)가 변경 없이 동작 확인

**완료 기준**: 하위 호환성 테스트 통과, 기존/신규 config 키 모두 동작

### M5: 정리 및 최종 검증 (Priority: Medium)

**목표**: REQ-8, 최종 품질 확인

1. `output.go` 파일 삭제
2. `output_test.go` 파일 삭제 (M0에서 이관 완료 확인 후)
3. 전체 테스트 실행 (`go test -race ./internal/node/...`)
4. 코드 커버리지 85%+ 확인
5. `go vet` 및 린트 검사 통과 확인

**완료 기준**: output.go/output_test.go 삭제, 전체 테스트 통과, 커버리지 85%+

---

## 기술 접근 방식

### 아키텍처

- 기존 `DebugNode` 구조체를 확장하여 output 노드의 기능을 흡수
- 출력 대상 분기는 `outputDest` 필드 기반 switch 문으로 구현
- DebugSink는 인터페이스로 정의하여 노드 외부에서 주입 가능하도록 설계
- fields 필터링은 Process 초반에 payload를 복사한 후 필터링하여 원본 메시지 보존

### 주요 설계 결정

| 결정 사항 | 선택 | 근거 |
|-----------|------|------|
| output 노드 처리 | 별칭(alias) | 하위 호환성 유지, 코드 중복 제거 |
| DebugSink 주입 | 인터페이스 | 테스트 용이성, 에디터 패널 구현 분리 |
| fields 필터링 위치 | Process 내부 | 단순성, 노드 라이프사이클 패턴 일관성 |
| 출력 대상 기본값 | logger | 기존 debug 노드 동작 유지 |

### 리스크 및 대응

| 리스크 | 영향 | 대응 |
|--------|------|------|
| 기존 플로우 YAML 호환성 깨짐 | High | M0에서 특성화 테스트로 기존 동작 보존 확인 |
| template 파싱 오류 | Medium | Configure 단계에서 template 유효성 검증, 오류 시 노드 초기화 실패 |
| DebugSink nil 참조 | Medium | editor 출력 시 DebugSink nil 체크, nil이면 로거 폴백 |
| 동시 파일 쓰기 경합 | Low | 기존 sync.RWMutex 패턴 유지 |

---

## 추적성 태그

- SPEC-DEBUG-001/REQ-1 -> M1
- SPEC-DEBUG-001/REQ-2 -> M2
- SPEC-DEBUG-001/REQ-3 -> M3
- SPEC-DEBUG-001/REQ-4 -> M1
- SPEC-DEBUG-001/REQ-5 -> M4
- SPEC-DEBUG-001/REQ-6 -> 전체 마일스톤
- SPEC-DEBUG-001/REQ-7 -> 전체 마일스톤
- SPEC-DEBUG-001/REQ-8 -> M5
