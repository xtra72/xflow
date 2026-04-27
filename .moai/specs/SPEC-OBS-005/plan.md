---
id: SPEC-OBS-005
type: plan
version: "1.0.0"
created: "2026-04-06"
updated: "2026-04-06"
---

# SPEC-OBS-005 구현 계획: Debug 노드 통합 (output 노드 흡수)

## 1. 구현 전략

### 접근 방식

기존 `debug.go`를 확장하여 `output.go`의 기능을 흡수한다. output 노드의 템플릿 렌더링, 접두사 출력 기능을 debug 노드에 통합하고, 새로운 출력 대상 선택 및 필드 선택 기능을 추가한다. output.go 파일은 삭제하고 registry에서 alias 매핑으로 하위 호환성을 보장한다.

### 핵심 설계 결정

1. **기존 구조체 확장**: DebugNode 구조체에 `outputDest`, `tmpl`, `prefix`, `fields` 필드를 추가하여 output 기능을 흡수
2. **기본값으로 호환성 보장**: 새 필드의 기본값이 기존 debug 노드 동작을 그대로 재현
3. **output 타입 alias**: registry에서 `"output"` -> `NewDebugNode`로 매핑하여 기존 플로우 YAML 호환
4. **editor 출력 대상**: 초기 구현은 slog 로거에 `debug_output` 속성 추가 방식으로 구현하여 향후 WebSocket 디버그 패널 연동 포인트 확보

---

## 2. 마일스톤

### Primary Goal: DebugNode 확장 및 output 기능 흡수

**대상 파일**:
- `internal/node/debug.go` (수정: 대폭 확장)

**작업 내용**:
1. `debugTemplateContext` 구조체 정의
   - ID, Payload, Metadata, Timestamp 필드
2. DebugNode 구조체 확장
   - `outputDest string` 추가 (기본: "logger")
   - `tmpl *template.Template` 추가 (기본: nil)
   - `prefix string` 추가 (기본: "")
   - `fields []string` 추가 (기본: nil)
3. `Configure()` 확장
   - `output` 키 파싱 및 유효성 검증 ("logger", "terminal", "file", "editor")
   - `template` 키 파싱 및 `text/template` 컴파일 (Configure 시점에 1회)
   - `prefix` 키 파싱
   - `fields` 키 파싱 ([]string 또는 []any -> []string 변환)
   - `output="file"` + `file=""` 조합 에러 검증
4. `filterFields()` 함수 구현
   - payload 필드 선택 (일반 키)
   - metadata 필드 선택 (`meta:` 접두사 키)
   - 빈 fields 시 원본 반환
5. `formatMessage()` 메서드 구현
   - 템플릿이 있으면 템플릿 렌더링 (실패 시 JSON 폴백)
   - prefix가 있으면 `[prefix] <payload-json>` 형식
   - 둘 다 없으면 기존 debug 포맷 `message id=... payload=... metadata=...`
6. `writeOutput()` 메서드 구현
   - `"logger"`: BaseNode.Logger()로 level에 따라 출력
   - `"terminal"`: os.Stdout에 직접 출력
   - `"file"`: 열린 파일에 출력
   - `"editor"`: BaseNode.Logger()로 출력 + `"debug_output": true` 속성
7. `Process()` 리라이트
   - fields 필터링 -> 메시지 포맷팅 -> 출력 대상에 쓰기 -> pass-through
8. `Init()` / `Shutdown()` 수정
   - `output="file"` 외에는 파일 열기/닫기 생략

### Secondary Goal: 파일 삭제 및 Registry 업데이트

**대상 파일**:
- `internal/node/output.go` (삭제)
- `internal/node/output_test.go` (삭제)
- `internal/node/registry.go` (수정)

**작업 내용**:
1. `output.go` 삭제
2. `output_test.go` 삭제
3. `registry.go` 수정:
   - `{"output", NewOutputNode, ...}` -> `{"output", NewDebugNode, "debug", "메시지를 포맷팅하여 출력 (debug 노드 alias)"}`
   - `{"debug", NewDebugNode, ...}` description 업데이트
   - `NewOutputNode` import 제거

### Final Goal: 테스트 구현

**대상 파일**:
- `internal/node/debug_test.go` (수정: 대폭 확장)

**작업 내용**:
1. 기존 debug 테스트 유지/수정
2. 설정 검증 테스트
   - Configure 기본값 적용 검증
   - Configure 전체 필드 파싱 검증
   - 유효하지 않은 output 값 에러 검증
   - output="file" + file="" 에러 검증
   - 유효하지 않은 template 에러 검증
3. 출력 대상 테스트
   - logger 출력 테스트 (level별)
   - terminal 출력 테스트 (stdout 캡처)
   - file 출력 테스트 (임시 파일)
   - editor 출력 테스트 (debug_output 속성 검증)
4. 템플릿 테스트
   - 정상 템플릿 렌더링
   - 템플릿 실행 실패 시 JSON 폴백
   - 템플릿 컨텍스트 필드 검증 (ID, Payload, Metadata, Timestamp)
5. 필드 선택 테스트
   - payload 키 선택
   - metadata 키 선택 (meta: 접두사)
   - 혼합 선택 (payload + metadata)
   - 빈 fields 시 전체 출력
   - fields 필터가 템플릿 컨텍스트에 반영되는지 검증
6. 하위 호환성 테스트
   - 기존 debug 설정으로 Configure/Process 동작 검증
   - 기존 output 설정으로 Configure/Process 동작 검증
   - output 타입 registry alias 동작 검증
7. pass-through 테스트
   - 입력 메시지가 변경 없이 출력되는지 검증

### Optional Goal: 예제 플로우 업데이트

**대상 파일**:
- `examples/flows/` 하위 debug/output 사용 예제 (해당 시)

**작업 내용**:
1. 기존 output 노드를 사용하는 예제가 있으면 debug 노드로 변경 또는 그대로 유지 (alias로 동작)
2. 새 기능(템플릿, 필드 선택)을 활용하는 예제 추가 검토

---

## 3. 리스크 및 대응

| 리스크 | 영향 | 대응 |
|--------|------|------|
| 기존 output 플로우 호환 실패 | 사용자 플로우 동작 불가 | registry alias + 하위 호환성 테스트로 사전 검증 |
| 템플릿 컴파일 에러가 Configure에서 발생 | 노드 시작 실패 | Configure에서 명확한 에러 메시지 반환 |
| editor 출력이 실제 WebSocket 연동 안 됨 | 기능 기대 불일치 | slog 속성 방식으로 구현하고 향후 연동 포인트 명시 |
| fields 필터링 성능 오버헤드 | 고빈도 메시지 처리 시 지연 | 빈 fields 시 필터링 건너뛰기 (fast path) |

---

## 4. 수정 대상 파일 요약

| 파일 | 작업 | 설명 |
|------|------|------|
| `internal/node/debug.go` | 수정 | 통합 DebugNode (템플릿, 출력 대상, 필드 선택 추가) |
| `internal/node/debug_test.go` | 수정 | 통합 테스트 (모든 모드, 호환성) |
| `internal/node/output.go` | 삭제 | output 노드 제거 |
| `internal/node/output_test.go` | 삭제 | output 노드 테스트 제거 |
| `internal/node/registry.go` | 수정 | output -> NewDebugNode alias |

---

## 5. 전문가 상담 권장

### expert-backend 상담 권장

이 SPEC은 Go 백엔드 구현(노드 리라이트, text/template 통합, 동시성 처리)을 포함하므로, 구현 단계(`/moai run SPEC-OBS-005`)에서 expert-backend 에이전트의 상담을 권장한다.

상담 포인트:
- filterFields 함수의 성능 최적화 (고빈도 메시지 처리 시나리오)
- editor 출력 대상의 향후 WebSocket 연동 설계 방향
- template.Execute의 동시성 안전성 검증

---

*문서 버전: 1.0.0*
*최종 수정: 2026-04-06*
*작성: MoAI SPEC Builder (manager-spec)*
