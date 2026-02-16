package dto

import (
	"fmt"
	"strings"
)

// FieldError 는 단일 필드 유효성 검증 에러를 나타낸다.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationErrors 는 필드 유효성 검증 에러 컬렉션이다.
type ValidationErrors []FieldError

// Error 는 error 인터페이스를 구현한다.
func (ve ValidationErrors) Error() string {
	parts := make([]string, 0, len(ve))
	for _, fe := range ve {
		parts = append(parts, fmt.Sprintf("%s: %s", fe.Field, fe.Message))
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

// ValidateFlowCreate 는 FlowCreateRequest를 검증한다.
func ValidateFlowCreate(req *FlowCreateRequest) ValidationErrors {
	if req == nil {
		return ValidationErrors{{Field: "request", Message: "is required"}}
	}

	var errs ValidationErrors

	if req.Name == "" {
		errs = append(errs, FieldError{Field: "name", Message: "is required"})
	} else if len(req.Name) > 255 {
		errs = append(errs, FieldError{Field: "name", Message: "must be at most 255 characters"})
	}

	if req.Definition == nil {
		errs = append(errs, FieldError{Field: "definition", Message: "is required"})
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// ValidateAgentCreate 는 AgentCreateRequest를 검증한다.
func ValidateAgentCreate(req *AgentCreateRequest) ValidationErrors {
	if req == nil {
		return ValidationErrors{{Field: "request", Message: "is required"}}
	}

	var errs ValidationErrors

	if req.Name == "" {
		errs = append(errs, FieldError{Field: "name", Message: "is required"})
	} else if len(req.Name) > 255 {
		errs = append(errs, FieldError{Field: "name", Message: "must be at most 255 characters"})
	}

	if req.Type == "" {
		errs = append(errs, FieldError{Field: "type", Message: "is required"})
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// ValidateConfigUpdate 는 ConfigUpdateRequest를 검증한다.
func ValidateConfigUpdate(req *ConfigUpdateRequest) ValidationErrors {
	if req == nil {
		return ValidationErrors{{Field: "request", Message: "is required"}}
	}

	var errs ValidationErrors

	if req.Config == nil {
		errs = append(errs, FieldError{Field: "config", Message: "is required"})
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}
