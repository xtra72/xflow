package dto

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFieldError_Fields(t *testing.T) {
	fe := FieldError{
		Field:   "name",
		Message: "is required",
	}
	assert.Equal(t, "name", fe.Field)
	assert.Equal(t, "is required", fe.Message)
}

func TestValidationErrors_Error(t *testing.T) {
	tests := []struct {
		name     string
		errors   ValidationErrors
		expected string
	}{
		{
			name:     "single error",
			errors:   ValidationErrors{{Field: "name", Message: "is required"}},
			expected: "validation failed: name: is required",
		},
		{
			name: "multiple errors",
			errors: ValidationErrors{
				{Field: "name", Message: "is required"},
				{Field: "definition", Message: "is required"},
			},
			expected: "validation failed: name: is required; definition: is required",
		},
		{
			name:     "empty errors",
			errors:   ValidationErrors{},
			expected: "validation failed: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.errors.Error())
		})
	}
}

func TestValidationErrors_ImplementsErrorInterface(t *testing.T) {
	var err error = ValidationErrors{{Field: "name", Message: "is required"}}
	assert.Contains(t, err.Error(), "name")
}

func TestValidateFlowCreate(t *testing.T) {
	tests := []struct {
		name        string
		req         *FlowCreateRequest
		expectValid bool
		errFields   []string
	}{
		{
			name: "valid request",
			req: &FlowCreateRequest{
				Name:       "test-flow",
				Definition: map[string]any{"nodes": []string{"a"}},
			},
			expectValid: true,
		},
		{
			name: "valid request with description",
			req: &FlowCreateRequest{
				Name:        "test-flow",
				Description: "a test flow",
				Definition:  map[string]any{"nodes": []string{"a"}},
			},
			expectValid: true,
		},
		{
			name: "missing name",
			req: &FlowCreateRequest{
				Definition: map[string]any{"nodes": []string{"a"}},
			},
			expectValid: false,
			errFields:   []string{"name"},
		},
		{
			name: "missing definition",
			req: &FlowCreateRequest{
				Name: "test-flow",
			},
			expectValid: false,
			errFields:   []string{"definition"},
		},
		{
			name:        "missing both name and definition",
			req:         &FlowCreateRequest{},
			expectValid: false,
			errFields:   []string{"name", "definition"},
		},
		{
			name: "name too long",
			req: &FlowCreateRequest{
				Name:       strings.Repeat("a", 256),
				Definition: map[string]any{"nodes": []string{"a"}},
			},
			expectValid: false,
			errFields:   []string{"name"},
		},
		{
			name: "name exactly 255 chars is valid",
			req: &FlowCreateRequest{
				Name:       strings.Repeat("a", 255),
				Definition: map[string]any{"nodes": []string{"a"}},
			},
			expectValid: true,
		},
		{
			name:        "nil request",
			req:         nil,
			expectValid: false,
			errFields:   []string{"request"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateFlowCreate(tt.req)
			if tt.expectValid {
				assert.Nil(t, errs)
			} else {
				require.NotNil(t, errs)
				assert.Len(t, errs, len(tt.errFields))
				for i, field := range tt.errFields {
					assert.Equal(t, field, errs[i].Field)
				}
			}
		})
	}
}

func TestValidateAgentCreate(t *testing.T) {
	tests := []struct {
		name        string
		req         *AgentCreateRequest
		expectValid bool
		errFields   []string
	}{
		{
			name: "valid request",
			req: &AgentCreateRequest{
				Name: "test-agent",
				Type: "processor",
			},
			expectValid: true,
		},
		{
			name: "valid request with config",
			req: &AgentCreateRequest{
				Name:   "test-agent",
				Type:   "processor",
				Config: map[string]any{"concurrency": 4},
			},
			expectValid: true,
		},
		{
			name: "missing name",
			req: &AgentCreateRequest{
				Type: "processor",
			},
			expectValid: false,
			errFields:   []string{"name"},
		},
		{
			name: "missing type",
			req: &AgentCreateRequest{
				Name: "test-agent",
			},
			expectValid: false,
			errFields:   []string{"type"},
		},
		{
			name:        "missing both name and type",
			req:         &AgentCreateRequest{},
			expectValid: false,
			errFields:   []string{"name", "type"},
		},
		{
			name: "name too long",
			req: &AgentCreateRequest{
				Name: strings.Repeat("a", 256),
				Type: "processor",
			},
			expectValid: false,
			errFields:   []string{"name"},
		},
		{
			name:        "nil request",
			req:         nil,
			expectValid: false,
			errFields:   []string{"request"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateAgentCreate(tt.req)
			if tt.expectValid {
				assert.Nil(t, errs)
			} else {
				require.NotNil(t, errs)
				assert.Len(t, errs, len(tt.errFields))
				for i, field := range tt.errFields {
					assert.Equal(t, field, errs[i].Field)
				}
			}
		})
	}
}

func TestValidateConfigUpdate(t *testing.T) {
	tests := []struct {
		name        string
		req         *ConfigUpdateRequest
		expectValid bool
		errFields   []string
	}{
		{
			name: "valid request",
			req: &ConfigUpdateRequest{
				Config: map[string]any{"debug": true},
			},
			expectValid: true,
		},
		{
			name:        "missing config",
			req:         &ConfigUpdateRequest{},
			expectValid: false,
			errFields:   []string{"config"},
		},
		{
			name:        "nil config",
			req:         &ConfigUpdateRequest{Config: nil},
			expectValid: false,
			errFields:   []string{"config"},
		},
		{
			name:        "nil request",
			req:         nil,
			expectValid: false,
			errFields:   []string{"request"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateConfigUpdate(tt.req)
			if tt.expectValid {
				assert.Nil(t, errs)
			} else {
				require.NotNil(t, errs)
				assert.Len(t, errs, len(tt.errFields))
				for i, field := range tt.errFields {
					assert.Equal(t, field, errs[i].Field)
				}
			}
		})
	}
}
