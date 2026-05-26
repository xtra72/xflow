// detect.go — InfluxDB 버전 자동 감지.
//
// auto 모드에서 도구는 다음 휴리스틱으로 v2/v3 를 추정한다:
//  1. GET <url>/ping (또는 /api/v2/ping) — v2 가 응답하면 X-Influxdb-Version 헤더에
//     "v2.x" 형식. v3 는 다른 형식 (또는 헤더 없음) 으로 응답.
//  2. 헤더 매칭 실패 시 fallback: v3 로 가정 (최신 버전 선호).
//
// 본 함수는 실제 HTTP 클라이언트 (net/http) 만 사용하며 influx SDK 에는
// 의존하지 않는다. read-only 안전 가드.
package tsdbtags

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// DefaultDetectTimeout 은 버전 감지 HTTP 호출의 기본 타임아웃이다.
const DefaultDetectTimeout = 5 * time.Second

// DetectTarget 은 <url>/ping 응답의 X-Influxdb-Version 헤더로 target 을 추정한다.
//
// 반환값:
//   - TargetV2: 헤더가 "v2." 또는 "2." 로 시작.
//   - TargetV3: 헤더가 "v3." 또는 "3." 로 시작, 또는 헤더 없이 200/204 응답.
//   - 에러: 네트워크 실패 (auto-detect 사용자에게 명확한 에러 메시지 제공).
//
// 본 함수는 인증을 사용하지 않는다 (ping endpoint 는 일반적으로 public).
// 인증이 필요한 환경에서는 운영자가 --target 을 명시해야 한다.
func DetectTarget(ctx context.Context, url string) (Target, error) {
	client := &http.Client{Timeout: DefaultDetectTimeout}

	// 일반적 endpoint 후보 (v1/v2 의 /ping 과 /api/v2/ping 모두 시도).
	endpoints := []string{
		strings.TrimRight(url, "/") + "/ping",
		strings.TrimRight(url, "/") + "/api/v2/ping",
	}

	var lastErr error
	for _, ep := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, ep, nil)
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("ping %s: status %d", ep, resp.StatusCode)
			continue
		}
		// 200 / 204 / 404 (v3 의 일부 endpoint) 모두 헤더 검사 후 판단.
		ver := resp.Header.Get("X-Influxdb-Version")
		if ver != "" {
			return classifyVersion(ver), nil
		}
		// 헤더 없으면 v3 가정 (v3 는 항상 헤더를 노출하지 않음).
		return TargetV3, nil
	}
	return TargetAuto, fmt.Errorf("influx 버전 자동 감지 실패: %w (운영자가 --target 을 명시하세요)", lastErr)
}

// classifyVersion 은 X-Influxdb-Version 헤더 값 (예: "v2.7.0") 을 Target 으로 변환.
func classifyVersion(version string) Target {
	v := strings.TrimPrefix(strings.ToLower(version), "v")
	switch {
	case strings.HasPrefix(v, "2."):
		return TargetV2
	case strings.HasPrefix(v, "3."):
		return TargetV3
	case strings.HasPrefix(v, "1."):
		// v1.x 응답 — 본 도구는 v1 을 지원하지 않으나 헤더는 v2 와 호환되는
		// 경우가 많다. 명시적 미지원 신호 (운영자가 --target 명시 필요).
		return TargetAuto
	default:
		// 알 수 없는 형식 — fallback.
		return TargetV3
	}
}
