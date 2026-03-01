package dto

const (
	// defaultPage 는 페이지네이션의 기본 페이지 번호이다.
	defaultPage = 1
	// defaultSize 는 페이지네이션의 기본 페이지 크기이다.
	defaultSize = 20
	// maxSize 는 페이지네이션의 최대 페이지 크기이다.
	maxSize = 100
)

// PaginationParams 는 공통 페이지네이션 쿼리 파라미터이다.
type PaginationParams struct {
	Page int `json:"page" query:"page"` // 기본값: 1, 최소값: 1
	Size int `json:"size" query:"size"` // 기본값: 20, 최소값: 1, 최대값: 100
}

// Normalize 는 Page와 Size에 기본값을 설정한다.
func (p *PaginationParams) Normalize() {
	if p.Page < 1 {
		p.Page = defaultPage
	}
	if p.Size < 1 {
		p.Size = defaultSize
	}
	if p.Size > maxSize {
		p.Size = maxSize
	}
}

// Offset 는 DB 쿼리를 위한 오프셋을 반환한다.
func (p *PaginationParams) Offset() int {
	return (p.Page - 1) * p.Size
}

// ListOptions 는 공통 목록 쿼리 옵션이다.
type ListOptions struct {
	PaginationParams
	Sort   string `json:"sort" query:"sort"`
	Filter string `json:"filter" query:"filter"`
	Status string `json:"status" query:"status"`
}

// FlowCreateRequest 는 플로우 생성을 위한 DTO이다.
type FlowCreateRequest struct {
	Name        string         `json:"name" validate:"required,min=1,max=255"`
	Description string         `json:"description,omitempty"`
	Definition  map[string]any `json:"definition" validate:"required"`
}

// FlowUpdateRequest 는 플로우 업데이트를 위한 DTO이다.
type FlowUpdateRequest struct {
	Name        *string        `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	Description *string        `json:"description,omitempty"`
	Definition  map[string]any `json:"definition,omitempty"`
}

// AgentCreateRequest 는 에이전트 생성을 위한 DTO이다.
type AgentCreateRequest struct {
	Name   string         `json:"name" validate:"required,min=1,max=255"`
	Type   string         `json:"type" validate:"required"`
	Config map[string]any `json:"config,omitempty"`
}

// AgentUpdateRequest 는 에이전트 업데이트를 위한 DTO이다.
type AgentUpdateRequest struct {
	Name   *string        `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	Config map[string]any `json:"config,omitempty"`
}

// ConfigUpdateRequest 는 런타임 설정 업데이트를 위한 DTO이다.
type ConfigUpdateRequest struct {
	Config map[string]any `json:"config" validate:"required"`
}

// AgentExecRequest 는 에이전트 Process 커맨드 실행을 위한 DTO이다.
type AgentExecRequest struct {
	Command string         `json:"command" validate:"required"`
	Params  map[string]any `json:"params,omitempty"`
}
