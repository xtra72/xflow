package flow

import "errors"

// 패키지 수준 에러 정의
var (
	// ErrInvalidPath 는 dot 표기법 경로가 유효하지 않을 때 반환된다.
	ErrInvalidPath = errors.New("invalid path")

	// ErrInvalidStateTransition 은 유효하지 않은 상태 전이를 시도할 때 반환된다.
	ErrInvalidStateTransition = errors.New("invalid state transition")

	// ErrInvalidFlowState 는 알 수 없는 FlowState 문자열을 파싱할 때 반환된다.
	ErrInvalidFlowState = errors.New("invalid flow state")

	// ErrDuplicateNodeID 는 동일 ID의 노드가 이미 존재할 때 반환된다.
	ErrDuplicateNodeID = errors.New("duplicate node id")

	// ErrDuplicateNodeName 은 동일 이름의 노드가 이미 존재할 때 반환된다.
	ErrDuplicateNodeName = errors.New("duplicate node name")

	// ErrNodeNotFound 는 지정된 ID/이름의 노드를 찾을 수 없을 때 반환된다.
	ErrNodeNotFound = errors.New("node not found")

	// ErrWireNotFound 는 지정된 ID의 와이어를 찾을 수 없을 때 반환된다.
	ErrWireNotFound = errors.New("wire not found")

	// ErrDuplicateWireID 는 동일 ID의 와이어가 이미 존재할 때 반환된다.
	ErrDuplicateWireID = errors.New("duplicate wire id")

	// ErrInvalidWireSource 는 와이어 소스 노드/포트가 유효하지 않을 때 반환된다.
	ErrInvalidWireSource = errors.New("invalid wire source")

	// ErrInvalidWireTarget 은 와이어 타겟 노드/포트가 유효하지 않을 때 반환된다.
	ErrInvalidWireTarget = errors.New("invalid wire target")

	// ErrUnsupportedFormat 은 지원하지 않는 파일 확장자일 때 반환된다.
	ErrUnsupportedFormat = errors.New("unsupported format")

	// ErrFlowNameRequired 는 Flow 이름이 비어있을 때 반환된다.
	ErrFlowNameRequired = errors.New("flow name required")
)
