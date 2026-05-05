// @spec SPEC-STORE-003 v0.3.0
//
// store_query_meta.go 는 Phase D (API 응답 객체 배열) 가 사용하는 UserStoreAgent
// 메타데이터 패스스루를 제공한다.
//
// 본 파일은 Phase B/C 의 누락 보정용이다 — 내부 *StoreAgent 에는 이미
// `StaticKeysSnapshot() map[string]StaticKeyMeta` 가 있지만 (`store.go` 참조),
// API 핸들러가 거치는 *UserStoreAgent (agent.Agent 구현) 에는 동일 패스스루가
// 없어 Phase D 의 BREAKING 응답 모델 (data_type, metric_type, registration 노출)
// 을 구현할 수 없었다.
//
// Phase D 가 핸들러 측에서 `StaticKeysSnapshot()` 으로 마이그레이션을 완료하면
// `store.go` 의 `StaticKeyTags()` shim 은 제거 가능하다.
package system

// StaticKeysSnapshot 은 정적 키 → StaticKeyMeta 전체 깊은 복사본을 반환한다.
//
// 정적 키가 하나도 없거나 inner StoreAgent 가 초기화되지 않은 상태이면 빈 맵을
// 반환한다. 반환 맵의 Tags 는 모두 깊은 복사본이며, 호출자가 수정해도 에이전트
// 내부 상태에 영향이 없다.
//
// API 핸들러(GET /store/{name}/keys) 가 응답 객체 배열을 빌드할 때 사용한다.
//
// @spec SPEC-STORE-003 v0.3.0
func (a *UserStoreAgent) StaticKeysSnapshot() map[string]StaticKeyMeta {
	a.mu.RLock()
	inner := a.inner
	a.mu.RUnlock()
	if inner == nil {
		return map[string]StaticKeyMeta{}
	}
	return inner.StaticKeysSnapshot()
}
