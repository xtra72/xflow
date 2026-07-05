package system

import (
	"context"
	"errors"
	"time"
)

// NodeStoreAdapter 는 system.Store 인터페이스를 node.StoreWriter / node.StoreReader
// 인터페이스로 변환하는 어댑터이다.
//
// Store.Get() 는 (StoreEntry, error) 를 반환하지만
// node.StoreReader.Get() 는 (any, bool, error) 를 기대한다.
// ErrKeyNotFound 를 (nil, false, nil) 로 변환하여 인터페이스 차이를 해소한다.
//
// 내부적으로 resolver 함수를 통해 매 호출마다 현재 유효한 Store 를 조회한다.
// 이는 UserStoreAgent 재시작(Stop → Start) 으로 내부 inner 가 새 인스턴스로
// 교체되더라도, 플로우 노드가 들고 있는 기존 Adapter 가 자동으로 새 Store 에
// 라우팅되도록 보장하기 위함이다. 생성 시점의 Store 스냅샷을 잡으면 재시작
// 이후 쓰기/읽기가 모두 정지된 옛 inner 로 향해 실패하는 회귀가 발생한다.
type NodeStoreAdapter struct {
	resolver func() Store
	// agentResolver 는 메타데이터(data_type/tags) 설정에 필요한 현재 *StoreAgent 를 돌려준다.
	// SetWithMeta 경로에서만 사용되며, nil 이면 메타 설정 없이 일반 쓰기로 폴백한다.
	// resolver 와 동일하게 매 호출마다 현재 유효한 StoreAgent 를 얻어 에이전트 재시작에
	// 안전하도록 lazy 하게 동작한다.
	agentResolver func() *StoreAgent
	// namespace 는 SetWithMeta 가 사용자 관점 key 를 staticKeys 에 등록할 때 사용한다.
	// staticKeys 는 네임스페이스 접두사가 없는 사용자 key 를 키로 쓰므로, 노드가 전달한
	// key 를 그대로 등록하면 NamespacedStore 의 쓰기 경로(coerce/검증)와 일치한다.
	namespace string
}

// NewNodeStoreAdapter 는 고정된 system.Store 를 감싸는 NodeStoreAdapter 를 생성한다.
// 하위 호환을 위해 기존 시그니처를 유지하며, 내부적으로는 전달된 store 를
// 그대로 반환하는 resolver 로 래핑한다. 생명주기 전환이 없는 테스트 및
// 단순 사용처에 적합하다.
//
// 이 경로로 생성된 Adapter 는 agentResolver 가 없으므로 SetWithMeta 는 일반 쓰기로
// 폴백한다 (data_type/tags 미적용). 메타 지정이 필요하면 NewLazyNodeStoreAdapterWithAgent 를 사용한다.
func NewNodeStoreAdapter(store Store) *NodeStoreAdapter {
	return &NodeStoreAdapter{resolver: func() Store { return store }}
}

// NewLazyNodeStoreAdapter 는 매 호출마다 resolver 를 호출하여 현재 Store 를
// 동적으로 얻는 Adapter 를 반환한다. 에이전트 재시작 등으로 내부 Store 가
// 교체되어도 resolver 가 최신 인스턴스를 돌려주면 동일한 Adapter 로 계속
// 올바른 Store 에 접근할 수 있다.
//
// resolver 는 nil 을 반환해서는 안 된다. 호출자는 resolver 가 어떤 동시성
// 보장(락 등) 아래에서 안전하게 현재 Store 를 돌려주도록 구현해야 한다.
func NewLazyNodeStoreAdapter(resolver func() Store) *NodeStoreAdapter {
	return &NodeStoreAdapter{resolver: resolver}
}

// NewLazyNodeStoreAdapterWithAgent 는 NewLazyNodeStoreAdapter 에 더해 메타데이터
// (data_type/tags) 설정을 위한 agentResolver 와 namespace 를 함께 받는다.
// store-write 노드의 data_type/tags 지정 기능(SetWithMeta)을 활성화하려면 이 생성자를 사용한다.
//
// agentResolver 는 nil 을 반환해서는 안 되며, resolver 와 동일한 동시성 보장 아래에서
// 현재 유효한 *StoreAgent 를 돌려주어야 한다.
func NewLazyNodeStoreAdapterWithAgent(resolver func() Store, agentResolver func() *StoreAgent, namespace string) *NodeStoreAdapter {
	return &NodeStoreAdapter{
		resolver:      resolver,
		agentResolver: agentResolver,
		namespace:     namespace,
	}
}

// Set 은 node.StoreWriter 인터페이스를 구현한다.
func (a *NodeStoreAdapter) Set(ctx context.Context, key string, value any) error {
	return a.resolver().Set(ctx, key, value)
}

// SetWithTTL 은 node.StoreWriter 인터페이스를 구현한다.
func (a *NodeStoreAdapter) SetWithTTL(ctx context.Context, key string, value any, ttl time.Duration) error {
	return a.resolver().SetWithTTL(ctx, key, value, ttl)
}

// StoreWriteMeta 는 SetWithMeta 호출 시 부여할 메타데이터 옵션이다.
// node.StoreWriteMeta 와 동일한 형태이며, 노드 패키지의 순환 의존을 피하기 위해
// system 계층에도 동일 구조를 둔다 (어댑터가 node 인터페이스를 구현).
type StoreWriteMeta struct {
	// DataType 은 빈 문자열이 아니면 키를 그 data_type 으로 등록/고정한다.
	// 6종 enum(int/float/string/boolean/bytes/json) 검증은 호출자(노드 Configure)의 책임이다.
	DataType string
	// MetricType 은 빈 문자열이 아니면 키의 metric_type 으로 적용한다.
	// 빈 문자열이면 기존 metric_type(또는 unknown)을 보존한다 — metric_type 변경 안 함.
	// 정규식 ^[a-zA-Z0-9_-]+$ 검증은 호출자(노드 Configure/Process)의 책임이며,
	// 본 어댑터는 전달된 값을 신뢰하고 SetKeyMeta 로 적용한다.
	MetricType string
	// Tags 는 비어있지 않으면 키에 태그를 부여한다.
	Tags map[string]string
	// TTL 은 0 보다 크면 값 쓰기에 TTL 을 적용한다.
	TTL time.Duration
}

// @spec SPEC-STORE-004
// SetWithMeta 는 data_type / metric_type / tags 를 지정하여 값을 기록한다 (node.StoreMetaWriter 구현).
//
// M2(쓰기 경로 시리즈화): metric_type/tags 는 더 이상 "키 메타 덮어쓰기"가 아니라
// **시리즈 식별자 구성**에 사용된다. (key, metric_type, sorted(tags)) 를 SeriesID 로 정규화·
// 인코딩하여 그 인코딩 문자열을 저장/레지스트리 키로 쓴다. 즉:
//   - 같은 key + 다른 metric_type → 다른 시리즈 키 → 독립 시리즈 (N1/E2/AC-1).
//   - 같은 key + 다른 tags(정규화 후) → 다른 시리즈 키 → 독립 시리즈 (N2/E3/AC-2).
//   - tags 순서만 다름 → 같은 시리즈 키(인코딩이 정렬) → 동일 시리즈 갱신 (U2/AC-3).
//   - data_type 만 다름 → 같은 시리즈 키(data_type 은 식별 차원 아님) → 동일 시리즈 (U3/N3/AC-4).
//   - 메타 미지정 → 기본 시리즈 (key, "unknown", {}) (AC-16).
//
// 레이어 경계: node 패키지는 시리즈 개념을 모른다. 노드는 StoreWriteMeta(metric/tags) 만
// 전달하고, 시리즈 인코딩/라우팅은 system 계층(본 어댑터) 의 책임이다.
//
// 처리 순서 (네임스페이스 일관성 보존):
//  1. SeriesID 인코딩 → seriesKey 산출. 이후 모든 등록/쓰기/메타 적용은 seriesKey 를 사용한다.
//  2. DataType 지정 시 → 값 쓰기 전에 StoreAgent.SetKeyDataType(seriesKey) 으로 타입 고정.
//  3. 값 쓰기 → 일반 Set/SetWithTTL 경로(NamespacedStore)에 seriesKey 를 사용자 key 로 넘긴다.
//     NamespacedStore 가 네임스페이스 접두사·gatekeeper 검증·coercion 을 일관되게 처리한다.
//     최종 VolatileStore 키 = namespace + ":" + seriesKey (prefix 바깥, 시리즈 인코딩 안쪽).
//  4. metric_type + tags 를 SetKeyMeta(seriesKey) 로 적용한다. seriesKey 는 이미 metric/tags 를
//     식별 차원으로 흡수했으므로, 여기서는 GET /keys 응답·필터를 위한 메타 라벨로 함께 저장한다.
//     - SeriesID 정규화 결과(metric=unknown 보정, tags 깊은 복사)를 그대로 적용한다.
//
// agentResolver 가 없으면(NewNodeStoreAdapter 등으로 생성된 경우) 메타/시리즈 적용을 건너뛰고
// 일반 쓰기(bare key)로 폴백하여 하위 호환을 보장한다.
func (a *NodeStoreAdapter) SetWithMeta(ctx context.Context, key string, value any, opts StoreWriteMeta) error {
	// agentResolver 가 없으면 시리즈 라우팅 불가 → 일반 쓰기(bare key)로 폴백.
	if a.agentResolver == nil {
		return a.writeValue(ctx, key, value, opts.TTL)
	}

	ag := a.agentResolver()

	// 0) key_tag: 자동 요소 생성 시 지정 태그 값을 키로 사용한다(store 에이전트 설정).
	//    - 쓰기 태그에 지정 태그(예: "name")가 있고 값이 비어있지 않으면 그 값을 키로 사용.
	//    - 지정 태그가 없거나 값이 비면 기존 키(호출자 제공, 예: 생성된 device_id)로 폴백.
	//    - 오버라이드 시 원래 키(생성된 id)를 "id" 태그로 보존한다: 관리용 식별 유지 +
	//      동명(name 중복) 요소가 하나의 시리즈로 잘못 병합되는 것을 방지(id 가 시리즈 차원).
	if ag != nil {
		if kt := ag.KeyTag(); kt != "" {
			if tv, ok := opts.Tags[kt]; ok && tv != "" {
				newTags := make(map[string]string, len(opts.Tags)+1)
				for k, v := range opts.Tags {
					newTags[k] = v
				}
				if _, exists := newTags["id"]; !exists {
					newTags["id"] = key
				}
				opts.Tags = newTags
				key = tv
			}
		}
	}

	// 1) SeriesID 구성·정규화·인코딩. metric/tags 가 식별 차원이 된다.
	//    이후 모든 키 연산(등록/쓰기/메타)은 이 인코딩 키를 사용자 key 로 사용한다.
	series := SeriesID{Key: key, MetricType: opts.MetricType, Tags: opts.Tags}.Normalize()
	seriesKey := EncodeSeriesKey(series)

	// 2) DataType 지정 시 값 쓰기 전에 시리즈 키를 그 타입으로 등록/고정한다.
	if opts.DataType != "" && ag != nil {
		ag.SetKeyDataType(seriesKey, DataType(opts.DataType))
	}

	// 3) 값 쓰기 (일반 경로 — 네임스페이스/검증/coercion 일관 적용). seriesKey 를 키로 사용.
	if err := a.writeValue(ctx, seriesKey, value, opts.TTL); err != nil {
		return err
	}

	// 4) 시리즈 메타(metric_type + tags) 라벨 적용. 정규화된 값(unknown 보정·깊은 복사)을 쓴다.
	//    GET /keys 시리즈 행과 조회 filter 가 이 메타를 사용한다.
	if ag != nil {
		ag.SetKeyMeta(seriesKey, series.MetricType, series.Tags)
	}

	return nil
}

// @spec SPEC-STORE-004
// GetSeries 는 (key, metric_type, tags) 시리즈의 현재값을 조회한다.
// 시리즈 인코딩 키로 라우팅하여 단일 시리즈의 값을 가져온다. metric 빈 값/ nil tags 는
// 기본 시리즈로 정규화된다. 키가 없으면 (nil, false, nil) 을 반환한다.
func (a *NodeStoreAdapter) GetSeries(ctx context.Context, key, metricType string, tags map[string]string) (any, bool, error) {
	seriesKey := EncodeSeriesKey(SeriesID{Key: key, MetricType: metricType, Tags: tags})
	return a.Get(ctx, seriesKey)
}

// writeValue 는 TTL 유무에 따라 Set 또는 SetWithTTL 로 값을 기록하는 공통 헬퍼이다.
func (a *NodeStoreAdapter) writeValue(ctx context.Context, key string, value any, ttl time.Duration) error {
	if ttl > 0 {
		return a.resolver().SetWithTTL(ctx, key, value, ttl)
	}
	return a.resolver().Set(ctx, key, value)
}

// Get 은 node.StoreReader 인터페이스를 구현한다.
// Store.Get() 의 (StoreEntry, error) 를 (any, bool, error) 로 변환한다.
// ErrKeyNotFound 발생 시 (nil, false, nil) 을 반환한다.
func (a *NodeStoreAdapter) Get(ctx context.Context, key string) (any, bool, error) {
	entry, err := a.resolver().Get(ctx, key)
	if err != nil {
		if errors.Is(err, ErrKeyNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return entry.Value, true, nil
}

// Has 는 node.StoreReader 인터페이스를 구현한다.
func (a *NodeStoreAdapter) Has(ctx context.Context, key string) (bool, error) {
	return a.resolver().Has(ctx, key)
}

// Delete 는 키를 삭제한다(Follow-up A: xflow.store.delete 지원).
// 현재 유효한 네임스페이스 스토어로 위임한다. 존재하지 않는 키 삭제 동작은
// 하위 Store 구현의 규약을 따른다.
func (a *NodeStoreAdapter) Delete(ctx context.Context, key string) error {
	return a.resolver().Delete(ctx, key)
}

// GetHistory 는 주어진 키의 값 변경 히스토리를 반환한다.
// 반환 값은 []any 형태로 이전 값들만 담는다 (최신순).
func (a *NodeStoreAdapter) GetHistory(ctx context.Context, key string) ([]any, error) {
	entries, err := a.resolver().GetHistory(ctx, key)
	if err != nil {
		return nil, err
	}
	result := make([]any, len(entries))
	for i, e := range entries {
		result[i] = e.Value
	}
	return result, nil
}

// QueryHistory 는 HistoryQuery 조건에 따른 시계열 엔트리를 반환한다.
// 반환 형식은 payload 에 그대로 실릴 수 있도록 "value"/"timestamp" 키를 갖는
// map 슬라이스로 통일한다. timestamp 는 프로젝트 전체 컨벤션에 따라 epoch
// 밀리초(int64, UnixMilli)로 직렬화된다. 결과는 최신순이며, 현재값을 포함한다.
// ErrKeyNotFound 발생 시 (nil, nil)을 반환하여 노드 계층에서 "값 없음"으로 처리한다.
func (a *NodeStoreAdapter) QueryHistory(ctx context.Context, key string, query HistoryQuery) ([]map[string]any, error) {
	entries, err := a.resolver().QueryHistory(ctx, key, query)
	if err != nil {
		if errors.Is(err, ErrKeyNotFound) {
			return nil, nil
		}
		return nil, err
	}
	result := make([]map[string]any, len(entries))
	for i, e := range entries {
		result[i] = map[string]any{
			"value":     e.Value,
			"timestamp": e.Timestamp.UnixMilli(),
		}
	}
	return result, nil
}

// GetMetadata 는 주어진 키의 메타데이터를 map[string]any 형태로 반환한다.
// 반환되는 키: "store_count", "store_created_at", "store_updated_at", "store_oldest_at"
func (a *NodeStoreAdapter) GetMetadata(ctx context.Context, key string) (map[string]any, error) {
	store := a.resolver()
	entry, err := store.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	oldestAt := entry.CreatedAt

	// 히스토리가 존재하면 가장 오래된 항목의 타임스탬프를 사용한다
	if entry.HistoryCount > 0 {
		history, hErr := store.GetHistory(ctx, key)
		if hErr == nil && len(history) > 0 {
			// 히스토리는 최신순이므로 마지막 항목이 가장 오래된 것
			oldestAt = history[len(history)-1].Timestamp
		}
	}

	return map[string]any{
		"store_count":      entry.HistoryCount,
		"store_created_at": entry.CreatedAt,
		"store_updated_at": entry.UpdatedAt,
		"store_oldest_at":  oldestAt,
	}, nil
}
