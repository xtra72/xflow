package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// finishableResponseWriter 는 net/http 의 동작을 모사하는 테스트용 ResponseWriter 이다.
//
// 실제 net/http2responseWriter 는 핸들러가 끝난 뒤 Header()/WriteHeader()/Write() 가
// 호출되면 "Header called after Handler finished" 패닉을 일으킨다.
// 단위 테스트에서 이 패닉을 결정적으로 재현하기 위해, finish() 호출 이후의
// 모든 ResponseWriter 접근을 패닉으로 표면화한다.
//
// 또한 finished 플래그는 메인 goroutine(타임아웃 경로)과 백그라운드 goroutine
// (지연된 핸들러)에서 함께 접근되므로 mutex 로 보호하여 -race 오탐을 막는다.
type finishableResponseWriter struct {
	mu       sync.Mutex
	header   http.Header
	finished bool
	code     int
	body     []byte
}

func newFinishableResponseWriter() *finishableResponseWriter {
	return &finishableResponseWriter{header: make(http.Header)}
}

func (w *finishableResponseWriter) Header() http.Header {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.finished {
		panic("Header called after Handler finished")
	}
	return w.header
}

func (w *finishableResponseWriter) WriteHeader(code int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.finished {
		panic("WriteHeader called after Handler finished")
	}
	w.code = code
}

func (w *finishableResponseWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.finished {
		panic("Write called after Handler finished")
	}
	w.body = append(w.body, b...)
	return len(b), nil
}

// finish 는 net/http 가 핸들러를 완료 처리하는 시점을 모사한다.
func (w *finishableResponseWriter) finish() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.finished = true
}

func (w *finishableResponseWriter) statusCode() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.code
}

// TestTimeout_BackgroundHandlerWriteAfterFinish 는 타임아웃/취소 발생 후
// 백그라운드 핸들러 goroutine 이 뒤늦게 응답을 쓰려고 할 때
// 패닉이나 data race 가 발생하지 않음을 검증한다(표 기반).
//
// 수정 전: 백그라운드 goroutine 의 hctx.JSON() 이 finished 상태의 ResponseWriter 에
// 접근하여 "Header called after Handler finished" 패닉 발생.
// 수정 후: Timeout 미들웨어가 hctx 를 finished 로 마킹하므로 백그라운드 write 는 no-op.
func TestTimeout_BackgroundHandlerWriteAfterFinish(t *testing.T) {
	tests := []struct {
		name string
		// makeCtx 는 타임아웃(deadline 초과) 또는 취소(client 끊김) 상황을 만든다.
		makeCtx func() (context.Context, context.CancelFunc)
		// wantMainStatus 는 타임아웃 시 메인 경로가 기록해야 하는 상태 코드이다.
		// 0 이면 메인 경로가 응답을 쓰지 않음을 의미한다(취소 케이스).
		wantMainStatus int
	}{
		{
			name: "deadline_exceeded_writes_408",
			makeCtx: func() (context.Context, context.CancelFunc) {
				// 즉시 초과되는 짧은 deadline
				return context.WithTimeout(context.Background(), 5*time.Millisecond)
			},
			wantMainStatus: http.StatusRequestTimeout,
		},
		{
			name: "client_cancel_no_write",
			makeCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				// 즉시 취소(클라이언트 연결 끊김 모사)
				cancel()
				return ctx, func() {}
			},
			wantMainStatus: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parentCtx, cancel := tt.makeCtx()
			defer cancel()

			w := newFinishableResponseWriter()
			req := httptest.NewRequest(http.MethodGet, "/agents", nil).WithContext(parentCtx)
			hctx := newHTTPContext(w, req)

			// 핸들러는 타임아웃/취소보다 늦게 끝나며, 종료 직전에 JSON 응답을 시도한다.
			// 이는 AgentHandler.List 가 ctx.JSON() 을 호출하는 상황을 모사한다.
			handlerStarted := make(chan struct{})
			handlerProceed := make(chan struct{})
			handlerDone := make(chan struct{})

			handler := func(ctx Context) error {
				close(handlerStarted)
				<-handlerProceed // 타임아웃이 먼저 발생하도록 대기
				defer close(handlerDone)
				return ctx.JSON(http.StatusOK, map[string]string{"hello": "world"})
			}

			// Timeout 미들웨어로 감싼다. d 는 충분히 짧게 하여 parentCtx 만료 또는
			// 자체 deadline 으로 즉시 타임아웃 경로를 타게 한다.
			wrapped := Timeout(10 * time.Millisecond)(handler)

			// 메인 경로 실행: 타임아웃이 먼저 발생하여 즉시 반환되어야 한다.
			mainErrCh := make(chan error, 1)
			go func() {
				mainErrCh <- wrapped(hctx)
			}()

			<-handlerStarted

			var mainErr error
			select {
			case mainErr = <-mainErrCh:
			case <-time.After(2 * time.Second):
				t.Fatal("Timeout 미들웨어가 반환되지 않음 (deadlock)")
			}

			// 메인 경로(Timeout 미들웨어)가 반환된 직후 net/http 가
			// 핸들러를 완료 처리하는 시점을 모사한다.
			w.finish()

			// 이제 백그라운드 핸들러 goroutine 을 진행시킨다.
			// 수정 전: 여기서 ctx.JSON() → finished writer 접근 → 패닉.
			// 수정 후: finished 가드로 no-op.
			require.NotPanics(t, func() {
				close(handlerProceed)
				select {
				case <-handlerDone:
				case <-time.After(2 * time.Second):
					t.Fatal("백그라운드 핸들러가 종료되지 않음")
				}
			}, "백그라운드 핸들러 write 는 패닉 없이 안전하게 처리되어야 한다")

			// Timeout 미들웨어는 에러를 직접 렌더하므로 nil 을 반환해야 한다
			// (handleError 중복 렌더 방지).
			assert.NoError(t, mainErr, "Timeout 미들웨어는 응답을 직접 쓰고 nil 을 반환해야 한다")

			if tt.wantMainStatus != 0 {
				assert.Equal(t, tt.wantMainStatus, w.statusCode(),
					"deadline 초과 시 메인 경로가 408 을 기록해야 한다")
			}
		})
	}
}

// TestTimeout_ConcurrentWriteRace 는 핸들러 goroutine 의 응답 write 와
// 타임아웃 경로의 finish 마킹이 동시에 일어날 때 data race 가 없음을 검증한다.
// `go test -race` 로 실행해야 의미가 있다.
func TestTimeout_ConcurrentWriteRace(t *testing.T) {
	const iterations = 50

	for i := 0; i < iterations; i++ {
		w := newFinishableResponseWriter()
		// deadline 을 0 에 가깝게 하여 핸들러 진입과 타임아웃이 경합하도록 한다.
		req := httptest.NewRequest(http.MethodGet, "/agents", nil)
		hctx := newHTTPContext(w, req)

		handler := func(ctx Context) error {
			// 즉시 JSON write 를 시도하여 타임아웃 경로와 경합시킨다.
			return ctx.JSON(http.StatusOK, map[string]int{"i": i})
		}

		wrapped := Timeout(time.Nanosecond)(handler)

		require.NotPanics(t, func() {
			_ = wrapped(hctx)
			// 메인 반환 직후 완료 처리 모사.
			w.finish()
		})
	}
}
