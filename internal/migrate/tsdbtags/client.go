// client.go — read-only schema discovery interface and adapter constructors.
//
// SchemaClient 는 마이그레이션 도구가 Influx 서버에 요구하는 최소 read-only
// 인터페이스이다. v2 / v3 어댑터가 본 인터페이스를 구현하며, 테스트는 fixture
// 기반 mock 으로 대체한다 (실제 Influx 서버 접근 없음).
package tsdbtags

import (
	"context"
	"errors"
)

// SchemaClient 는 InfluxDB 의 schema 조회 (measurement / tag key / tag value)
// 만 노출하는 read-only 인터페이스이다.
//
// 본 인터페이스는 의도적으로 write API 를 노출하지 않는다 — C2 도구는
// Influx 에 write 하지 않는다는 안전 불변식 (M8 + acceptance C-AC7) 의
// 컴파일 타임 보장.
type SchemaClient interface {
	// Version 은 클라이언트가 사용하는 InfluxDB 버전 ("v2" 또는 "v3") 을 반환한다.
	Version() Target

	// Ping 은 서버 연결 가능성을 검증한다 (read-only health check).
	Ping(ctx context.Context) error

	// ListMeasurements 는 bucket / database 의 모든 measurement 명을 반환한다.
	ListMeasurements(ctx context.Context) ([]string, error)

	// ListTagKeys 는 특정 measurement 의 모든 tag key 를 반환한다.
	ListTagKeys(ctx context.Context, measurement string) ([]string, error)

	// ListTagValues 는 특정 (measurement, tagKey) 의 모든 tag value 를 반환한다.
	//
	// 호출자는 본 메서드로 composite pattern 매칭을 수행한다.
	ListTagValues(ctx context.Context, measurement, tagKey string) ([]string, error)

	// Close 는 클라이언트 리소스를 해제한다.
	Close() error
}

// ErrUnsupportedTarget 는 알 수 없는 Target 이 전달되었을 때 반환된다.
var ErrUnsupportedTarget = errors.New("지원하지 않는 target")
