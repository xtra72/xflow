package handler

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api/dto"
)

// TestCreate_RegenerateIDsQueryParam 는 ?regenerate_ids=true 쿼리 파라미터가
// CreateFlow 로 전달되는 req.RegenerateIDs 에 반영되는지 검증한다. (requirement 2)
func TestCreate_RegenerateIDsQueryParam(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		bodyFlag bool
		want     bool
	}{
		{name: "쿼리 파라미터 true", path: "/api/v1/flows?regenerate_ids=true", want: true},
		{name: "쿼리 파라미터 없음", path: "/api/v1/flows", want: false},
		{name: "쿼리 파라미터 false", path: "/api/v1/flows?regenerate_ids=false", want: false},
		{name: "body 플래그 true (쿼리 없음)", path: "/api/v1/flows", bodyFlag: true, want: true},
		{name: "쿼리 true 가 body false 를 OR 로 덮어씀", path: "/api/v1/flows?regenerate_ids=true", bodyFlag: false, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured *dto.FlowCreateRequest
			flowMock := &mockFlowManager{
				createFlowFn: func(_ context.Context, req *dto.FlowCreateRequest) (*FlowInfo, error) {
					captured = req
					return &FlowInfo{ID: "new", Name: req.Name, Status: "stored"}, nil
				},
			}
			router := setupFlowRouter(flowMock)

			body := `{"name":"f","definition":{"nodes":[]}`
			if tt.bodyFlag {
				body += `,"regenerate_ids":true`
			}
			body += `}`

			rec := doRequest(t, router, http.MethodPost, tt.path, strings.NewReader(body))
			require.Equal(t, http.StatusCreated, rec.Code)
			require.NotNil(t, captured)
			assert.Equal(t, tt.want, captured.RegenerateIDs)
		})
	}
}
