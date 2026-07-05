// remote_stream.go 는 M8(그룹 J) 브라우저-측 스트리밍 엔드포인트(SSE)를 제공한다
// (@SPEC:SPEC-REMOTE-001 M8, 그룹 J, REQ-J08/J08b/J05/J07).
//
// 전송 선택 — SSE(Server-Sent Events):
//
//	로컬 디테일 패널의 라이브 데이터는 모니터링 WS 허브(/ws)의 단방향 push 와 REST
//	폴링으로 전달된다. 원격 자원별 라이브 스트림(device.state / agent.stats / agent.series)
//	은 본질적으로 서버→브라우저 단방향이므로, 자원별 1엔드포인트로 깔끔히 매핑되는 SSE 를
//	채택한다. 인증/연결 패턴은 로컬 WS(/ws)와 일관된다(Authorization Bearer 또는 ?token=
//	→ JWT 검증 → admin 강제). 브라우저는 EventSource 로 구독하고, 연결을 닫으면 teardown
//	된다(REQ-J08b). v1 의 노드 측 스트림은 1:1(논리 스트림 공유 팬아웃은 서버 streamManager
//	가 처리)이므로 SSE 로 충분하며, 양방향 제어가 필요해지면 WS 로 승격할 수 있다.
//
// 라우트(raw 핸들러 — Context 래퍼 바이패스로 직접 http.ResponseWriter/Flusher 사용):
//
//	GET /api/v1/remote/nodes/{instance_id}/devices/{device_id}/state/stream   → device/state
//	GET /api/v1/remote/nodes/{instance_id}/agents/{agent_id}/stats/stream     → agent/stats
//	GET /api/v1/remote/nodes/{instance_id}/agents/{agent_id}/series/stream    → agent/series
//
// 게이팅(REQ-J05): admin 권한 강제 + IsManaged(503) + 노출 범위(404). teardown(REQ-J08b):
// 브라우저 연결 종료(request ctx) 또는 노드 Done(오프라인/터미널 오류) 시 Unsubscribe 를
// 호출하고 SSE 루프를 종료한다(누수 없음).
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/xtra/xflow/internal/auth"
	"github.com/xtra/xflow/internal/remote"
)

// RemoteStreamService 는 브라우저 스트림 오케스트레이션을 추상화한다(*remote.Server 가
// 만족). 핸들러는 IsManaged/IsResourceExposed 로 게이트한 뒤 SubscribeStream 으로 논리
// 스트림에 소비자를 등록한다.
type RemoteStreamService interface {
	// IsManaged 는 노드가 승인+온라인인지 반환한다(REQ-J05 → 503).
	IsManaged(instanceID string) bool
	// IsResourceExposed 는 자원이 노출 범위 내인지 반환한다(REQ-J05 → 404).
	IsResourceExposed(ctx context.Context, instanceID, kind, id string) (bool, error)
	// SubscribeStream 은 논리 스트림에 소비자를 등록하고 StreamHandle 을 반환한다(REQ-J08).
	SubscribeStream(instanceID, domain, streamAction string, args json.RawMessage) (remote.StreamHandle, error)
}

// streamScope 는 SSE 라우트의 자원 스코프를 구분한다(게이팅 분기 — REQ-J05/L11).
type streamScope int

const (
	// scopeResource 는 per-resource 자원 스코프이다(노출 범위 게이트 — REQ-J05).
	scopeResource streamScope = iota
	// scopeChannel 은 채널-레벨 스코프이다(차트 channelName — 노출 범위 미평가, REQ-L07).
	scopeChannel
	// scopeNode 는 노드-레벨 스코프이다(로그 — 자원 식별자 없음, 노출 범위 미평가, REQ-L06).
	scopeNode
)

// streamRoute 는 SSE 라우트 → (domain, stream_action, 자원 path 파라미터) 매핑이다.
//
// M10(그룹 L): scope 로 게이팅 분기를 결정한다. chart(scopeChannel)/logs(scopeNode)는
// per-resource 노출 범위가 없으므로 IsManaged 만 게이트한다(REQ-L03/L11). idParam 은
// scopeResource 의 자원 id / scopeChannel 의 channel path 변수 이름이다(scopeNode 는 빈값).
type streamRoute struct {
	pattern      string
	domain       string
	streamAction string
	idParam      string
	scope        streamScope
}

// streamRoutes 는 스트림 가능한 라이브 action 의 SSE 라우트 표이다(REQ-J08).
//
// M10(그룹 L): chart(channelName) + logs(노드-레벨)를 추가한다(REQ-L06/L07). 별도 WS
// 경로를 신설하지 않고 기존 SSE 패턴을 재사용한다(REQ-L07 — 제2 WS 핸들러 미신설).
func streamRoutes() []streamRoute {
	return []streamRoute{
		{"/api/v1/remote/nodes/{instance_id}/devices/{device_id}/state/stream", remote.DomainDevice, remote.StreamActionState, "device_id", scopeResource},
		{"/api/v1/remote/nodes/{instance_id}/agents/{agent_id}/stats/stream", remote.DomainAgent, remote.StreamActionStats, "agent_id", scopeResource},
		{"/api/v1/remote/nodes/{instance_id}/agents/{agent_id}/series/stream", remote.DomainAgent, remote.StreamActionSeries, "agent_id", scopeResource},
		// M10: 차트 채널 라이브 스트림(channelName — 노드-레벨, 노출 범위 미평가).
		{"/api/v1/remote/nodes/{instance_id}/charts/{channel}/stream", remote.DomainChart, remote.StreamActionChart, "channel", scopeChannel},
		// M10: 로그 라이브 스트림(노드-레벨, 자원 식별자 없음).
		{"/api/v1/remote/nodes/{instance_id}/logs/stream", remote.DomainMonitor, remote.StreamActionLogs, "", scopeNode},
	}
}

// RemoteStreamHandler 는 원격 라이브 스트림 SSE 엔드포인트를 처리한다.
type RemoteStreamHandler struct {
	svc    RemoteStreamService
	jwtSvc *auth.JWTService
	logger *slog.Logger
}

// NewRemoteStreamHandler 는 RemoteStreamHandler 를 생성한다. jwtSvc 가 nil 이면 인증
// 우회 모드이다(테스트 전용 — 운영에서는 항상 jwtSvc 를 주입한다).
func NewRemoteStreamHandler(svc RemoteStreamService, jwtSvc *auth.JWTService) *RemoteStreamHandler {
	return &RemoteStreamHandler{svc: svc, jwtSvc: jwtSvc, logger: slog.Default()}
}

// WithLogger 는 로거를 설정한다.
func (h *RemoteStreamHandler) WithLogger(logger *slog.Logger) *RemoteStreamHandler {
	if logger != nil {
		h.logger = logger
	}
	return h
}

// RegisterRawHandlers 는 SSE 라우트를 raw 핸들러로 등록한다(server.RegisterRawHandler).
// raw 등록이 필요한 이유: SSE 는 http.Flusher 직접 접근이 필요하므로 Context 래퍼를
// 바이패스한다.
func (h *RemoteStreamHandler) RegisterRawHandlers(register func(pattern string, handler http.HandlerFunc)) {
	for _, rt := range streamRoutes() {
		register("GET "+rt.pattern, h.HandleStream)
	}
}

// testRoleKey 는 인증 우회 모드(jwtSvc nil)에서 테스트가 역할을 주입하는 컨텍스트
// 키이다. 운영에서는 jwtSvc 가 항상 설정되므로 본 경로는 사용되지 않는다(테스트 seam).
type testRoleKey struct{}

// testRoleFromContext 는 인증 우회 모드(jwtSvc nil)에서 테스트가 주입한 역할을 읽는다.
func testRoleFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(testRoleKey{}).(string); ok {
		return v
	}
	return ""
}

// resolveRoute 는 요청에서 매칭되는 streamRoute, instance_id, 자원 id 를 찾는다.
//
// ServeMux 패턴 매칭(운영)과 직접 호출(테스트) 양쪽을 지원하기 위해, 먼저 PathValue 를
// 시도하고 비어 있으면 URL path 세그먼트를 직접 파싱한다. 경로 형태는 일정하므로
// (/api/v1/remote/nodes/{instance}/{kind}/{id}/{action}/stream) 세그먼트 파싱이 안전하다.
func (h *RemoteStreamHandler) resolveRoute(r *http.Request) (streamRoute, string, string, bool) {
	instanceID := r.PathValue("instance_id")
	for _, rt := range streamRoutes() {
		if !routeMatchesPath(rt, r.URL.Path) {
			continue
		}

		// M10: 노드-레벨 로그 스트림은 자원 id 가 없다(scopeNode). instance 만 필요.
		if rt.scope == scopeNode {
			if instanceID == "" {
				pInstance, ok := parseNodeStreamPath(r.URL.Path)
				if !ok {
					continue
				}
				instanceID = pInstance
			}
			if instanceID != "" {
				return rt, instanceID, "", true
			}
			continue
		}

		// scopeResource/scopeChannel — path 변수(자원 id 또는 channelName) 추출.
		id := r.PathValue(rt.idParam)
		if id == "" || instanceID == "" {
			// 직접 호출(PathValue 미설정) — 세그먼트 파싱 폴백.
			pInstance, pID, ok := parseStreamPath(r.URL.Path, rt)
			if !ok {
				continue
			}
			if instanceID == "" {
				instanceID = pInstance
			}
			if id == "" {
				id = pID
			}
		}
		if id != "" && instanceID != "" {
			return rt, instanceID, id, true
		}
	}
	return streamRoute{}, "", "", false
}

// routeMatchesPath 는 라우트 패턴의 고정 suffix(예: /state/stream)가 요청 path 에 있는지
// 확인한다(agent stats/series 구분, kind 세그먼트도 함께 확인).
func routeMatchesPath(rt streamRoute, path string) bool {
	switch rt.scope {
	case scopeNode:
		// M10: 로그(scopeNode)는 .../logs/stream suffix 로 매칭한다(자원 세그먼트 없음).
		suffix := "/" + rt.streamAction + "/stream"
		return len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix
	case scopeChannel:
		// M10: 차트(scopeChannel)는 .../charts/{channel}/stream 형태이다. action(chart)이
		// kind 세그먼트(charts)와 동일하므로 action suffix 가 아니라 /stream suffix +
		// /charts/ 세그먼트 포함으로 매칭한다(REQ-L07).
		if len(path) < len("/stream") || path[len(path)-len("/stream"):] != "/stream" {
			return false
		}
		return containsSegment(path, "/"+pluralKind(rt.domain)+"/")
	default:
		suffix := "/" + rt.streamAction + "/stream"
		if len(path) < len(suffix) || path[len(path)-len(suffix):] != suffix {
			return false
		}
		// kind 세그먼트(devices/agents)도 확인하여 동일 action 의 도메인 혼동을 막는다.
		kindSeg := "/" + pluralKind(rt.domain) + "/"
		return containsSegment(path, kindSeg)
	}
}

// parseStreamPath 는 .../nodes/{instance}/{kind}/{id|channel}/{action}/stream 또는
// (차트의 경우) .../nodes/{instance}/charts/{channel}/stream 형태에서 instance 와
// id/channelName 을 추출한다(직접 호출 폴백).
func parseStreamPath(path string, rt streamRoute) (instanceID, resourceID string, ok bool) {
	segs := splitNonEmpty(path)
	if rt.scope == scopeChannel {
		// 기대: [api v1 remote nodes <instance> charts <channel> stream]
		for i := 0; i+4 < len(segs); i++ {
			if segs[i] == "nodes" && segs[i+2] == pluralKind(rt.domain) &&
				segs[i+4] == "stream" {
				return segs[i+1], segs[i+3], true
			}
		}
		return "", "", false
	}
	// 기대: [api v1 remote nodes <instance> <kind> <id> <action> stream]
	for i := 0; i+4 < len(segs); i++ {
		if segs[i] == "nodes" && segs[i+2] == pluralKind(rt.domain) &&
			segs[i+4] == rt.streamAction {
			return segs[i+1], segs[i+3], true
		}
	}
	return "", "", false
}

// parseNodeStreamPath 는 .../nodes/{instance}/logs/stream 형태에서 instance 를
// 추출한다(scopeNode 직접 호출 폴백 — 자원 id 없음).
func parseNodeStreamPath(path string) (instanceID string, ok bool) {
	segs := splitNonEmpty(path)
	// 기대: [api v1 remote nodes <instance> logs stream]
	for i := 0; i+2 < len(segs); i++ {
		if segs[i] == "nodes" && segs[i+2] == "logs" {
			return segs[i+1], true
		}
	}
	return "", false
}

// pluralKind 는 도메인을 URL 의 복수형 세그먼트로 매핑한다.
func pluralKind(domain string) string {
	switch domain {
	case remote.DomainFlow:
		return "flows"
	case remote.DomainAgent:
		return "agents"
	case remote.DomainDevice:
		return "devices"
	case remote.DomainChart:
		return "charts"
	default:
		return domain
	}
}

// containsSegment 는 path 에 sub 가 포함되는지 확인한다(단순 부분 문자열).
func containsSegment(path, sub string) bool {
	for i := 0; i+len(sub) <= len(path); i++ {
		if path[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// splitNonEmpty 는 path 를 '/' 로 분리하고 빈 세그먼트를 제거한다.
func splitNonEmpty(path string) []string {
	out := make([]string, 0, 10)
	start := -1
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			if start >= 0 {
				out = append(out, path[start:i])
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, path[start:])
	}
	return out
}

// HandleStream 은 SSE 스트림 요청을 처리한다(REQ-J08). 게이팅 후 구독하고 프레임을
// data: 이벤트로 흘린다. 연결 종료/노드 Done 시 teardown 한다(REQ-J08b).
func (h *RemoteStreamHandler) HandleStream(w http.ResponseWriter, r *http.Request) {
	// 1) admin 인증(REQ-F04). jwtSvc 미설정(테스트)은 컨텍스트 역할로 우회.
	if !h.authorizeAdmin(r) {
		http.Error(w, "admin 권한이 필요합니다", http.StatusForbidden)
		return
	}

	rt, instanceID, resourceID, ok := h.resolveRoute(r)
	if !ok {
		http.Error(w, "알 수 없는 스트림 라우트", http.StatusNotFound)
		return
	}

	// 2) 게이팅(REQ-J05/L11): 미관리 → 503. per-resource 스코프만 노출 범위 평가(404).
	if !h.svc.IsManaged(instanceID) {
		http.Error(w, "노드가 관리 대상이 아닙니다(미승인/오프라인)", http.StatusServiceUnavailable)
		return
	}
	// M10: 차트(channel)/로그(node)는 노드-레벨 자원이므로 노출 범위를 평가하지 않는다
	// (REQ-L03/L06/L07). per-resource(device.state/agent.stats/series)만 노출 범위 게이트.
	if rt.scope == scopeResource {
		exposed, err := h.svc.IsResourceExposed(r.Context(), instanceID, kindFor(rt.domain), resourceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !exposed {
			http.Error(w, "대상 자원이 노드의 노출 범위에 없습니다", http.StatusNotFound)
			return
		}
	}

	// 3) SSE 준비(Flusher 필수).
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "스트리밍 미지원", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // 프록시 버퍼링 비활성(즉시 flush).
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// 4) 구독(REQ-J08). args 는 스코프별로 다르다:
	//    - scopeResource: {"id": resourceID}        (device.state/agent.stats/series)
	//    - scopeChannel:  {"channelName": resourceID} (chart — REQ-L07)
	//    - scopeNode:     nil                          (logs — 자원 식별자 없음, REQ-L06)
	args := streamArgs(rt.scope, resourceID)
	handle, err := h.svc.SubscribeStream(instanceID, rt.domain, rt.streamAction, args)
	if err != nil {
		// 헤더는 이미 200 으로 전송됐으므로 SSE 이벤트로 오류를 통지하고 종료한다.
		writeSSEError(w, flusher, mapStreamSubscribeError(err))
		return
	}
	defer handle.Unsubscribe() // teardown(REQ-J08b): 함수 종료 시 노드 구독 정리.

	h.logger.Debug("원격 라이브 스트림 시작",
		"instance_id", instanceID, "domain", rt.domain, "stream_action", rt.streamAction,
		"resource_id", resourceID)

	// 5) 펌프 루프: 프레임 → data: 이벤트. 종료 조건: 노드 Done / 브라우저 연결 종료.
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			// 브라우저 연결 종료 → Unsubscribe(defer) 로 teardown.
			return
		case <-handle.Done:
			// 노드 오프라인/터미널 오류 → 스트림 종료.
			writeSSEEvent(w, flusher, "end", "stream ended")
			return
		case frame, more := <-handle.Frames:
			if !more {
				return
			}
			writeSSEData(w, flusher, frame)
		}
	}
}

// authorizeAdmin 은 요청이 admin 권한인지 확인한다. jwtSvc 설정 시 토큰 검증, 미설정
// (테스트) 시 컨텍스트 역할을 사용한다.
func (h *RemoteStreamHandler) authorizeAdmin(r *http.Request) bool {
	if h.jwtSvc == nil {
		return testRoleFromContext(r.Context()) == "admin"
	}
	token := bearerOrQueryToken(r)
	if token == "" || h.jwtSvc.IsBlacklisted(token) {
		return false
	}
	claims, err := h.jwtSvc.ValidateToken(token)
	if err != nil {
		return false
	}
	return claims.Role == "admin"
}

// bearerOrQueryToken 은 Authorization Bearer 또는 ?token= 에서 토큰을 추출한다(WS 패턴
// 준용 — EventSource 는 헤더 설정이 제한적이라 ?token= 폴백을 제공). 토큰은 로깅하지
// 않는다(REQ-F06).
func bearerOrQueryToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		return authHeader[7:]
	}
	return r.URL.Query().Get("token")
}

// streamArgs 는 스코프별 구독 args 를 구성한다(REQ-J08/L06/L07).
//
//   - scopeResource: {"id": resourceID}          (노드 QuerySource 가 id 로 자원 식별)
//   - scopeChannel:  {"channelName": resourceID}  (차트 — 노드가 channelName 으로 채널 식별)
//   - scopeNode:     nil                            (로그 — 자원 식별자 없음)
func streamArgs(scope streamScope, resourceID string) json.RawMessage {
	switch scope {
	case scopeChannel:
		args, _ := json.Marshal(map[string]string{"channelName": resourceID})
		return args
	case scopeNode:
		return nil
	default:
		args, _ := json.Marshal(map[string]string{"id": resourceID})
		return args
	}
}

// writeSSEData 는 redacted 본문을 data: 이벤트로 전송하고 flush 한다.
func writeSSEData(w http.ResponseWriter, flusher http.Flusher, data json.RawMessage) {
	if len(data) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

// writeSSEEvent 는 명명 이벤트를 전송하고 flush 한다(end/error 통지).
func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, event, msg string) {
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, msg)
	flusher.Flush()
}

// writeSSEError 는 오류를 error 이벤트로 통지하고 flush 한다(구독 실패 — 헤더 전송 후).
func writeSSEError(w http.ResponseWriter, flusher http.Flusher, msg string) {
	writeSSEEvent(w, flusher, "error", msg)
}

// mapStreamSubscribeError 는 구독 오류를 SSE 통지 문자열로 환원한다(REQ-J07).
func mapStreamSubscribeError(err error) string {
	switch {
	case err == nil:
		return ""
	default:
		return "subscribe failed: " + err.Error()
	}
}
