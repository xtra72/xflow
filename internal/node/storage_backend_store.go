package node

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/xtra/xflow/internal/agent/system"
)

// ErrStoreNotConfigured 는 Store 인스턴스가 주입되지 않았을 때 반환된다.
var ErrStoreNotConfigured = errors.New("node: store instance not configured")

// StoreWriter 는 노드에서 Store에 기록하기 위한 인터페이스이다.
// 순환 의존을 방지하기 위해 node 패키지 내에 최소 인터페이스로 정의한다.
type StoreWriter interface {
	Set(ctx context.Context, key string, value any) error
	SetWithTTL(ctx context.Context, key string, value any, ttl time.Duration) error
}

// StoreReader 는 노드에서 Store를 읽기 위한 인터페이스이다.
type StoreReader interface {
	Get(ctx context.Context, key string) (any, bool, error)
	Has(ctx context.Context, key string) (bool, error)
	GetHistory(ctx context.Context, key string) ([]any, error)
}

// StoreMetaWriter 는 data_type/tags 메타데이터를 함께 지정하여 기록할 수 있는
// 선택적(optional) 인터페이스이다. system.NodeStoreAdapter 가 이를 구현한다.
//
// 중요: 옵션 타입은 system.StoreWriteMeta 를 그대로 사용한다. 과거에는 node 패키지에
// 동일 형태의 StoreWriteMeta 를 중복 정의했는데, Go 인터페이스 만족은 메서드 시그니처가
// 정확히 일치해야 하므로(파라미터 타입 포함) 어댑터의 SetWithMeta(system.StoreWriteMeta)
// 가 이 인터페이스를 만족하지 못했다. 그 결과 storage-write 노드의 data_type/field
// 메타가 한 번도 적용되지 못하고(키가 항상 동적 string/unknown), 일반 Set 로 폴백했다.
// node 패키지는 이미 system 을 import 하므로(store_read.go 등) system 타입을 직접 사용한다.
//
// store 가 이 인터페이스를 만족하고 노드에 data_type/field/tags 가 설정된 경우에만
// SetWithMeta 가 사용되며, 그 외에는 기존 StoreWriter.Set/SetWithTTL 로 폴백한다 (하위 호환).
type StoreMetaWriter interface {
	SetWithMeta(ctx context.Context, key string, value any, opts system.StoreWriteMeta) error
}

// 컴파일 타임 보장: 실제 store 어댑터(*system.NodeStoreAdapter)가 StoreMetaWriter 를
// 만족해야 한다. 과거 opts 타입(node.StoreWriteMeta vs system.StoreWriteMeta) 불일치로
// 만족하지 못해 메타 경로가 죽어있던 회귀를 영구 차단한다.
var _ StoreMetaWriter = (*system.NodeStoreAdapter)(nil)

// storeProvider 는 네임스페이스별 Store 어댑터를 제공하는 에이전트의 인터페이스이다.
// UserStoreAgent 가 이 인터페이스를 구현하며, AgentResolver로 해석된 에이전트에서
// 타입 단언을 통해 StoreWriter/StoreReader 에 접근한다.
//
// 반환값은 StoreWriter + StoreReader를 모두 만족하는 NodeStoreAdapter이다.
// 순환 의존을 방지하기 위해 any를 반환하고, 호출 측에서 타입 단언한다.
type storeProvider interface {
	NodeStoreForNamespace(namespace string) any
}

// storageDataTypeAuto 는 data_type 미지정 시 적용되는 "값 타입 추론" sentinel 이다.
// system 계층의 sentinel 을 그대로 재노출하여 노드 쪽 검증/기본값 적용이 한 곳을 보게 한다.
const storageDataTypeAuto = system.DataTypeAuto

// storeBackend 는 storage-write 노드의 키-값 저장소(Store) 백엔드이다.
//
// 매핑 규약 (중립 어휘 → Store 개념):
//   - series_key   → Store 키
//   - values[].name → field (같은 키 안에서 시리즈를 가르는 축)
//   - tags          → 시리즈 태그 (모든 값에 공유 적용)
//
// timestamp 는 Store 어댑터가 쓰기 시점에 직접 스탬프하므로 이 백엔드에서는 사용하지
// 않는다 (storage-write 의 timestamp_path 는 시계열 백엔드 전용이다).
type storeBackend struct {
	writer StoreWriter
	ttl    time.Duration
}

// backendName 은 에러 메시지에 쓰일 백엔드 이름이다.
func (b *storeBackend) backendName() string { return "store" }

// resolve 는 Store 에이전트에서 네임스페이스별 쓰기 핸들을 확보한다.
func (b *storeBackend) resolve(underlying any, cfg *storageWriteConfig) error {
	provider, ok := underlying.(storeProvider)
	if !ok {
		return fmt.Errorf("agent does not implement storeProvider")
	}
	instance := provider.NodeStoreForNamespace(cfg.namespace)
	if instance == nil {
		return fmt.Errorf("store agent returned nil store for namespace %q", cfg.namespace)
	}
	writer, ok := instance.(StoreWriter)
	if !ok {
		return fmt.Errorf("store agent returned incompatible type for StoreWriter")
	}
	b.writer = writer
	b.ttl = cfg.ttl
	return nil
}

// write 는 배치의 각 측정값을 Store 에 개별 키-메트릭 쓰기로 기록한다.
//
// store 가 StoreMetaWriter 를 만족하면 SetWithMeta 로 field / tags 를 함께
// 적용하고, 그렇지 않으면 Set/SetWithTTL 로 폴백한다.
//
// data_type 은 항상 auto(값 타입 추론)이다. 노드 설정에서 타입을 받지 않는 대신
// 첫 쓰기 값의 Go 타입으로 시리즈 타입을 고정한다 — 타입을 넘기지 않으면 store 가
// "동적 = string" 정책으로 등록해 숫자 측정값까지 문자열로 저장하고, 그러면 차트와
// 집계가 그 시리즈를 통째로 버린다.
func (b *storeBackend) write(ctx context.Context, batch storageBatch) error {
	if b.writer == nil {
		return ErrStoreNotConfigured
	}
	for _, v := range batch.values {
		if mw, ok := b.writer.(StoreMetaWriter); ok {
			opts := system.StoreWriteMeta{
				DataType:   storageDataTypeAuto,
				Field: v.name,
				Tags:       batch.tags,
				TTL:        b.ttl,
			}
			if err := mw.SetWithMeta(ctx, batch.seriesKey, v.value, opts); err != nil {
				return err
			}
			continue
		}
		if b.ttl > 0 {
			if err := b.writer.SetWithTTL(ctx, batch.seriesKey, v.value, b.ttl); err != nil {
				return err
			}
			continue
		}
		if err := b.writer.Set(ctx, batch.seriesKey, v.value); err != nil {
			return err
		}
	}
	return nil
}
