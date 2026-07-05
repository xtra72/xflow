// query_cache.go 는 M8(그룹 J) 서버 측 READ 응답 단기 TTL 캐시를 구현한다
// (@SPEC:SPEC-REMOTE-001 M8, spec §5.10, REQ-J16).
//
// 목적: 동일 자원에 대한 짧은 시간 내 반복 READ 질의(예: 디테일 패널 새로고침, 다중
// 탭)를 노드 왕복 없이 서비스하여 노드/관리채널 부하를 줄인다. 캐시는 노드가 이미
// redaction 한 본문만 저장한다(REQ-J06 — 시크릿 비저장). 게이팅(승인/노출 범위)은
// 캐시 적중 여부와 무관하게 매 요청마다 재평가된다(REQ-J05 — 캐시는 게이팅을 우회하지
// 않음; 평가는 DispatchQuery 호출자/핸들러 책임).
//
// 라이브 우회(REQ-J16): 스트림 가능 action(device.state/agent.stats/agent.series —
// IsStreamableAction)과 명시적 라이브 데이터는 캐시하지 않는다(항상 노드 재호출).
// 이는 DispatchQuery 가 IsStreamableAction 으로 판정한다.
//
// 무효화(REQ-J16): 그룹 D/I 변경 명령(Dispatch)이 성공하면 해당 노드의 캐시 엔트리를
// 무효화한다(stale 방지). 노드 단위 무효화는 보수적이지만 안전하다(자원 단위 키를
// 변경 args 에서 항상 신뢰 추출할 수 없으므로 — node-scoped 가 최소 안전 보장).
//
// 동시성/유계(bounded): RWMutex 보호 + 최대 크기 sweep 으로 무한 증가를 방지한다.
package remote

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// DefaultQueryCacheTTL 은 READ 응답 캐시의 기본 TTL 이다(REQ-J16). 짧게 유지하여
// stale 노출을 최소화하면서도 폭주성 반복 질의를 흡수한다. 라이브 action 은 우회한다.
const DefaultQueryCacheTTL = 2 * time.Second

// DefaultQueryCacheMaxEntries 는 캐시 최대 엔트리 수이다(유계 — 메모리 폭증 방지).
// 초과 시 만료/오래된 엔트리를 sweep 하여 상한을 유지한다.
const DefaultQueryCacheMaxEntries = 1024

// cacheEntry 는 캐시된 redacted 본문과 만료 시각이다.
type cacheEntry struct {
	data      json.RawMessage
	expiresAt time.Time
}

// queryCache 는 {instance_id, domain, query_action, args-hash} 키의 단기 TTL 캐시이다.
type queryCache struct {
	ttl        time.Duration
	maxEntries int

	mu      sync.RWMutex
	entries map[string]cacheEntry
	// keyNode 는 캐시 키 → 출처 instance_id 역인덱스이다(노드 단위 무효화용).
	keyNode map[string]string
}

// newQueryCache 는 TTL 과 최대 크기로 캐시를 생성한다. ttl<=0 이면 기본값을 사용한다.
func newQueryCache(ttl time.Duration, maxEntries int) *queryCache {
	if ttl <= 0 {
		ttl = DefaultQueryCacheTTL
	}
	if maxEntries <= 0 {
		maxEntries = DefaultQueryCacheMaxEntries
	}
	return &queryCache{
		ttl:        ttl,
		maxEntries: maxEntries,
		entries:    make(map[string]cacheEntry),
		keyNode:    make(map[string]string),
	}
}

// queryCacheKey 는 캐시 키를 구성한다. args 는 정규화하지 않고 바이트 해시하므로,
// 동일 의미의 다른 직렬화(키 순서 등)는 별도 키가 될 수 있다(보수적 — stale 방지).
func queryCacheKey(instanceID, domain, queryAction string, args json.RawMessage) string {
	h := sha256.New()
	_, _ = h.Write([]byte(instanceID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(domain))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(queryAction))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(args))
	return hex.EncodeToString(h.Sum(nil))
}

// get 은 키에 대한 미만료 캐시 본문을 반환한다. 미존재/만료 시 (nil, false).
func (c *queryCache) get(key string) (json.RawMessage, bool) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(e.expiresAt) {
		// 만료 — 지연 삭제(다음 put/sweep 에서 정리되거나 여기서 즉시 제거).
		c.mu.Lock()
		if cur, still := c.entries[key]; still && cur.expiresAt.Equal(e.expiresAt) {
			delete(c.entries, key)
			delete(c.keyNode, key)
		}
		c.mu.Unlock()
		return nil, false
	}
	return e.data, true
}

// putForNode 는 출처 노드와 함께 redacted 본문을 캐시한다(노드 단위 무효화 인덱스 포함).
func (c *queryCache) putForNode(instanceID, key string, data json.RawMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.maxEntries {
		c.sweepLocked()
	}
	c.entries[key] = cacheEntry{data: data, expiresAt: time.Now().Add(c.ttl)}
	if instanceID != "" {
		c.keyNode[key] = instanceID
	}
}

// put 은 출처 노드 정보 없이 본문을 캐시한다(테스트/유틸 — 노드 무효화 인덱스 없음).
func (c *queryCache) put(key string, data json.RawMessage) {
	c.putForNode("", key, data)
}

// invalidateNode 는 한 노드의 모든 캐시 엔트리를 제거한다(변경 성공 후 — REQ-J16).
func (c *queryCache) invalidateNode(instanceID string) {
	if instanceID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, node := range c.keyNode {
		if node == instanceID {
			delete(c.entries, key)
			delete(c.keyNode, key)
		}
	}
}

// size 는 현재 엔트리 수를 반환한다(테스트/관측).
func (c *queryCache) size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// sweepLocked 는 만료 엔트리를 제거하고, 그래도 상한 이상이면 임의 엔트리를 제거해
// 상한 미만으로 줄인다(유계 보장). 호출자는 mu 를 보유해야 한다.
func (c *queryCache) sweepLocked() {
	now := time.Now()
	for key, e := range c.entries {
		if now.After(e.expiresAt) {
			delete(c.entries, key)
			delete(c.keyNode, key)
		}
	}
	// 만료 정리 후에도 상한 이상이면 강제 축출한다(상한의 절반까지 — 잦은 sweep 방지).
	target := c.maxEntries / 2
	if target < 1 {
		target = 1
	}
	for key := range c.entries {
		if len(c.entries) <= target {
			break
		}
		delete(c.entries, key)
		delete(c.keyNode, key)
	}
}
