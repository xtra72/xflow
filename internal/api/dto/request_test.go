package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPaginationParams_Normalize(t *testing.T) {
	tests := []struct {
		name         string
		input        PaginationParams
		expectedPage int
		expectedSize int
	}{
		{
			name:         "zero values get defaults",
			input:        PaginationParams{Page: 0, Size: 0},
			expectedPage: 1,
			expectedSize: 20,
		},
		{
			name:         "negative page gets default",
			input:        PaginationParams{Page: -1, Size: 10},
			expectedPage: 1,
			expectedSize: 10,
		},
		{
			name:         "negative size gets default",
			input:        PaginationParams{Page: 2, Size: -5},
			expectedPage: 2,
			expectedSize: 20,
		},
		{
			name:         "size over max gets capped",
			input:        PaginationParams{Page: 1, Size: 200},
			expectedPage: 1,
			expectedSize: 100,
		},
		{
			name:         "valid values unchanged",
			input:        PaginationParams{Page: 3, Size: 50},
			expectedPage: 3,
			expectedSize: 50,
		},
		{
			name:         "size exactly 100 is fine",
			input:        PaginationParams{Page: 1, Size: 100},
			expectedPage: 1,
			expectedSize: 100,
		},
		{
			name:         "size exactly 1 is fine",
			input:        PaginationParams{Page: 1, Size: 1},
			expectedPage: 1,
			expectedSize: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.input
			p.Normalize()
			assert.Equal(t, tt.expectedPage, p.Page)
			assert.Equal(t, tt.expectedSize, p.Size)
		})
	}
}

func TestPaginationParams_Offset(t *testing.T) {
	tests := []struct {
		name     string
		page     int
		size     int
		expected int
	}{
		{
			name:     "first page",
			page:     1,
			size:     20,
			expected: 0,
		},
		{
			name:     "second page",
			page:     2,
			size:     20,
			expected: 20,
		},
		{
			name:     "third page with size 10",
			page:     3,
			size:     10,
			expected: 20,
		},
		{
			name:     "page 5 size 50",
			page:     5,
			size:     50,
			expected: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PaginationParams{Page: tt.page, Size: tt.size}
			assert.Equal(t, tt.expected, p.Offset())
		})
	}
}

func TestPaginationParams_Offset_AfterNormalize(t *testing.T) {
	p := PaginationParams{Page: 0, Size: 0}
	p.Normalize()
	// Page=1, Size=20 → Offset=0
	assert.Equal(t, 0, p.Offset())
}

func TestListOptions_EmbedsPagination(t *testing.T) {
	opts := ListOptions{
		PaginationParams: PaginationParams{Page: 0, Size: 0},
		Sort:             "created_at",
		Filter:           "active",
		Status:           "running",
	}
	opts.Normalize()

	assert.Equal(t, 1, opts.Page)
	assert.Equal(t, 20, opts.Size)
	assert.Equal(t, "created_at", opts.Sort)
	assert.Equal(t, "active", opts.Filter)
	assert.Equal(t, "running", opts.Status)
}

func TestFlowCreateRequest_Fields(t *testing.T) {
	req := FlowCreateRequest{
		Name:        "test-flow",
		Description: "a test flow",
		Definition:  map[string]any{"nodes": []string{"a", "b"}},
	}
	assert.Equal(t, "test-flow", req.Name)
	assert.Equal(t, "a test flow", req.Description)
	assert.NotNil(t, req.Definition)
}

func TestFlowUpdateRequest_OptionalFields(t *testing.T) {
	name := "updated-flow"
	desc := "updated description"

	// 모든 필드가 nil일 수 있어야 한다
	req := FlowUpdateRequest{}
	assert.Nil(t, req.Name)
	assert.Nil(t, req.Description)
	assert.Nil(t, req.Definition)

	// 일부 필드만 설정 가능해야 한다
	req = FlowUpdateRequest{
		Name:        &name,
		Description: &desc,
	}
	assert.Equal(t, &name, req.Name)
	assert.Equal(t, &desc, req.Description)
	assert.Nil(t, req.Definition)
}

func TestAgentCreateRequest_Fields(t *testing.T) {
	req := AgentCreateRequest{
		Name:   "test-agent",
		Type:   "processor",
		Config: map[string]any{"concurrency": 4},
	}
	assert.Equal(t, "test-agent", req.Name)
	assert.Equal(t, "processor", req.Type)
	assert.Equal(t, map[string]any{"concurrency": 4}, req.Config)
}

func TestAgentUpdateRequest_OptionalFields(t *testing.T) {
	name := "updated-agent"

	req := AgentUpdateRequest{}
	assert.Nil(t, req.Name)
	assert.Nil(t, req.Config)

	req = AgentUpdateRequest{
		Name:   &name,
		Config: map[string]any{"concurrency": 8},
	}
	assert.Equal(t, &name, req.Name)
	assert.NotNil(t, req.Config)
}

func TestConfigUpdateRequest_Fields(t *testing.T) {
	req := ConfigUpdateRequest{
		Config: map[string]any{"debug": true},
	}
	assert.Equal(t, map[string]any{"debug": true}, req.Config)
}
