package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSuccessResponse(t *testing.T) {
	tests := []struct {
		name string
		data any
	}{
		{
			name: "string data",
			data: "hello",
		},
		{
			name: "map data",
			data: map[string]string{"key": "value"},
		},
		{
			name: "nil data",
			data: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := NewSuccessResponse(tt.data)
			assert.True(t, resp.Success)
			assert.Equal(t, tt.data, resp.Data)
			assert.Nil(t, resp.Error)
			assert.Nil(t, resp.Meta)
		})
	}
}

func TestNewSuccessResponseWithMeta(t *testing.T) {
	meta := &Meta{
		RequestID: "req-123",
	}
	resp := NewSuccessResponseWithMeta("data", meta)

	assert.True(t, resp.Success)
	assert.Equal(t, "data", resp.Data)
	assert.Nil(t, resp.Error)
	require.NotNil(t, resp.Meta)
	assert.Equal(t, "req-123", resp.Meta.RequestID)
}

func TestNewSuccessResponseWithMeta_NilMeta(t *testing.T) {
	resp := NewSuccessResponseWithMeta("data", nil)

	assert.True(t, resp.Success)
	assert.Equal(t, "data", resp.Data)
	assert.Nil(t, resp.Meta)
}

func TestNewErrorResponse(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		message string
		details []any
	}{
		{
			name:    "error without details",
			code:    "BAD_REQUEST",
			message: "invalid input",
			details: nil,
		},
		{
			name:    "error with string details",
			code:    "VALIDATION_FAILED",
			message: "validation error",
			details: []any{"field 'name' is required"},
		},
		{
			name:    "error with map details",
			code:    "NOT_FOUND",
			message: "resource not found",
			details: []any{map[string]string{"id": "123"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := NewErrorResponse(tt.code, tt.message, tt.details...)

			assert.False(t, resp.Success)
			assert.Nil(t, resp.Data)
			require.NotNil(t, resp.Error)
			assert.Equal(t, tt.code, resp.Error.Code)
			assert.Equal(t, tt.message, resp.Error.Message)
			if len(tt.details) > 0 {
				assert.Equal(t, tt.details[0], resp.Error.Details)
			} else {
				assert.Nil(t, resp.Error.Details)
			}
		})
	}
}

func TestNewPaginatedResponse(t *testing.T) {
	tests := []struct {
		name           string
		data           []string
		page           int
		size           int
		total          int64
		expectedPages  int
	}{
		{
			name:          "first page",
			data:          []string{"a", "b"},
			page:          1,
			size:          2,
			total:         5,
			expectedPages: 3,
		},
		{
			name:          "exact division",
			data:          []string{"a", "b"},
			page:          1,
			size:          2,
			total:         4,
			expectedPages: 2,
		},
		{
			name:          "single page",
			data:          []string{"a"},
			page:          1,
			size:          10,
			total:         1,
			expectedPages: 1,
		},
		{
			name:          "empty result",
			data:          []string{},
			page:          1,
			size:          10,
			total:         0,
			expectedPages: 0,
		},
		{
			name:          "size zero returns zero pages",
			data:          []string{},
			page:          1,
			size:          0,
			total:         10,
			expectedPages: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := NewPaginatedResponse(tt.data, tt.page, tt.size, tt.total)

			assert.True(t, resp.Success)
			assert.Equal(t, tt.data, resp.Data)
			assert.Nil(t, resp.Error)
			require.NotNil(t, resp.Meta)
			require.NotNil(t, resp.Meta.Pagination)
			assert.Equal(t, tt.page, resp.Meta.Pagination.Page)
			assert.Equal(t, tt.size, resp.Meta.Pagination.Size)
			assert.Equal(t, tt.total, resp.Meta.Pagination.Total)
			assert.Equal(t, tt.expectedPages, resp.Meta.Pagination.TotalPages)
		})
	}
}

func TestAPIResponse_JSONSerialization(t *testing.T) {
	t.Run("success response JSON", func(t *testing.T) {
		resp := NewSuccessResponse(map[string]string{"name": "test"})
		data, err := json.Marshal(resp)
		require.NoError(t, err)

		var parsed map[string]any
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, true, parsed["success"])
		assert.NotNil(t, parsed["data"])
		assert.Nil(t, parsed["error"])
		assert.Nil(t, parsed["meta"])
	})

	t.Run("error response JSON", func(t *testing.T) {
		resp := NewErrorResponse("BAD_REQUEST", "invalid")
		data, err := json.Marshal(resp)
		require.NoError(t, err)

		var parsed map[string]any
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, false, parsed["success"])
		assert.Nil(t, parsed["data"])
		assert.NotNil(t, parsed["error"])
	})

	t.Run("paginated response JSON", func(t *testing.T) {
		resp := NewPaginatedResponse([]string{"a"}, 1, 10, 100)
		data, err := json.Marshal(resp)
		require.NoError(t, err)

		var parsed map[string]any
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, true, parsed["success"])
		assert.NotNil(t, parsed["meta"])
		meta := parsed["meta"].(map[string]any)
		assert.NotNil(t, meta["pagination"])
	})
}

func TestErrorDetail_JSONTags(t *testing.T) {
	detail := ErrorDetail{
		Code:    "TEST",
		Message: "test message",
	}

	data, err := json.Marshal(detail)
	require.NoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, "TEST", parsed["code"])
	assert.Equal(t, "test message", parsed["message"])
	// details는 omitempty이므로 nil일 때 포함되지 않아야 한다
	_, hasDetails := parsed["details"]
	assert.False(t, hasDetails)
}

func TestPaginationMeta_JSONTags(t *testing.T) {
	meta := PaginationMeta{
		Page:       2,
		Size:       20,
		Total:      100,
		TotalPages: 5,
	}

	data, err := json.Marshal(meta)
	require.NoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, float64(2), parsed["page"])
	assert.Equal(t, float64(20), parsed["size"])
	assert.Equal(t, float64(100), parsed["total"])
	assert.Equal(t, float64(5), parsed["total_pages"])
}
