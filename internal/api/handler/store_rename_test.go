package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent/system"
)

// TestHTTP_RenameKey_MovesAllSeries 는 실제 store 에이전트로 키의 모든 시리즈가
// 새 키로 이동함을 end-to-end 로 검증한다.
func TestHTTP_RenameKey_MovesAllSeries(t *testing.T) {
	ag, adapter := newSeriesStoreAgent(t, "store-s")
	seedThreeSeries(t, adapter) // room 아래 3개 시리즈
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/store/store-s/keys/room/rename",
		strings.NewReader(`{"new_key":"living"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	// 기존 키의 시리즈는 모두 새 키로 이동한다.
	assert.Equal(t, 0, remainingSeriesCount(t, router, "store-s", "room"),
		"기존 키(room) 시리즈는 남지 않아야 한다")
	assert.Equal(t, 3, remainingSeriesCount(t, router, "store-s", "living"),
		"새 키(living)로 3개 시리즈 모두 이동")
}

// TestHTTP_RenameKey_Conflict 는 대상 키 충돌 시 409 를 반환함을 검증한다.
func TestHTTP_RenameKey_Conflict(t *testing.T) {
	ag := &fakeStoreAgent{
		fakeAgentCommon: newFakeAgent("store-a", "my-store", "store"),
		renameFn: func(_ context.Context, _, _, _ string) (int, error) {
			return 0, system.ErrKeyExists
		},
	}
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/store/my-store/keys/room/rename",
		strings.NewReader(`{"new_key":"living"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code, "body=%s", rec.Body.String())
}

// TestHTTP_RenameKey_NotFound 는 이동 대상 시리즈가 0개면 404 를 반환함을 검증한다.
func TestHTTP_RenameKey_NotFound(t *testing.T) {
	ag := &fakeStoreAgent{
		fakeAgentCommon: newFakeAgent("store-a", "my-store", "store"),
		renameFn: func(_ context.Context, _, _, _ string) (int, error) {
			return 0, nil
		},
	}
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/store/my-store/keys/missing/rename",
		strings.NewReader(`{"new_key":"living"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code, "body=%s", rec.Body.String())
}

// TestHTTP_RenameKey_MissingNewKey 는 new_key 누락 시 400 을 반환함을 검증한다.
func TestHTTP_RenameKey_MissingNewKey(t *testing.T) {
	ag := &fakeStoreAgent{
		fakeAgentCommon: newFakeAgent("store-a", "my-store", "store"),
	}
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/store/my-store/keys/room/rename",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
}
