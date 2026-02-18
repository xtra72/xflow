package cli

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// resourceListResponse 는 API 리소스 목록 응답 구조체이다.
type resourceListResponse struct {
	Items []resourceItem `json:"items"`
}

// resourceItem 는 API 리소스 항목이다.
type resourceItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// cacheEntry 는 자동완성 캐시 항목이다.
type cacheEntry struct {
	items     []string
	fetchedAt time.Time
}

// AutoCompleter 는 readline 호환 탭 자동완성 엔진이다.
// Cobra 명령어 트리 기반 정적 완성과 API 리소스 동적 완성을 제공한다.
type AutoCompleter struct {
	rootCmd  *cobra.Command
	client   **Client
	cache    map[string]*cacheEntry
	cacheTTL time.Duration
	mu       sync.RWMutex
}

// resourceTypeMap 은 명령어 이름에서 API 리소스 타입으로의 매핑이다.
var resourceTypeMap = map[string]string{
	"flow":   "flows",
	"agent":  "agents",
	"node":   "nodes",
	"plugin": "plugins",
}

// resourceSubcommands 는 리소스 인자를 받는 서브커맨드 목록이다.
var resourceSubcommands = map[string]bool{
	"get":    true,
	"delete": true,
	"deploy": true,
	"update": true,
}

// NewAutoCompleter 는 새로운 AutoCompleter 를 생성한다.
// rootCmd 는 Cobra 루트 커맨드, client 는 API 클라이언트 더블 포인터,
// cacheTTL 은 리소스 캐시 유효 기간이다.
func NewAutoCompleter(rootCmd *cobra.Command, client **Client, cacheTTL time.Duration) *AutoCompleter {
	return &AutoCompleter{
		rootCmd:  rootCmd,
		client:   client,
		cache:    make(map[string]*cacheEntry),
		cacheTTL: cacheTTL,
	}
}

// Do 는 readline.AutoCompleter 인터페이스를 구현한다.
// 현재 입력 줄과 커서 위치를 받아 자동완성 후보를 반환한다.
func (ac *AutoCompleter) Do(line []rune, pos int) (newLine [][]rune, length int) {
	// 커서 위치까지의 텍스트만 사용
	lineStr := string(line[:pos])

	// 토큰 분리
	tokens, partial := splitLineForCompletion(lineStr)

	var candidates []string

	// 플래그 완성 (-- 로 시작하는 경우)
	if strings.HasPrefix(partial, "--") {
		cmd := ac.findCommand(tokens)
		candidates = ac.completeFlags(cmd, partial)
	} else if len(tokens) >= 2 && resourceSubcommands[tokens[len(tokens)-1]] {
		// 리소스 인자 완성 (예: flow get <partial>)
		// tokens 에서 부모 명령어 찾기
		parentCmd := ac.findParentCommand(tokens)
		if parentCmd != nil {
			resourceType := ac.resolveResourceType(parentCmd)
			if resourceType != "" {
				candidates = ac.completeResource(resourceType, partial)
			}
		}
		// 리소스 타입이 없으면 명령어 완성 시도
		if candidates == nil {
			candidates = ac.completeCommands(tokens, partial)
		}
	} else {
		// 명령어/서브커맨드 완성
		candidates = ac.completeCommands(tokens, partial)
	}

	// 후보를 [][]rune 형태로 변환
	for _, c := range candidates {
		// partial 을 제거한 접미사만 반환
		suffix := c[len(partial):]
		newLine = append(newLine, []rune(suffix))
	}

	return newLine, len(partial)
}

// completeCommands 는 명령어/서브커맨드 후보를 반환한다.
// tokens 는 이미 완성된 토큰들, partial 은 현재 입력 중인 접두사이다.
func (ac *AutoCompleter) completeCommands(tokens []string, partial string) []string {
	// 현재 위치의 명령어 찾기
	cmd := ac.rootCmd
	for _, token := range tokens {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == token {
				cmd = sub
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}

	// 서브커맨드 목록에서 접두사 매칭
	var candidates []string
	for _, sub := range cmd.Commands() {
		name := sub.Name()
		// Cobra 자동 생성 명령어 제외
		if name == "help" || name == "completion" {
			continue
		}
		if strings.HasPrefix(name, partial) {
			candidates = append(candidates, name)
		}
	}

	sort.Strings(candidates)
	return candidates
}

// completeFlags 는 플래그 후보를 반환한다.
// cmd 는 현재 명령어, partial 은 "--" 로 시작하는 접두사이다.
func (ac *AutoCompleter) completeFlags(cmd *cobra.Command, partial string) []string {
	seen := make(map[string]bool)
	var candidates []string

	addFlag := func(f *pflag.Flag) {
		flagName := "--" + f.Name
		if strings.HasPrefix(flagName, partial) && !seen[flagName] {
			seen[flagName] = true
			candidates = append(candidates, flagName)
		}
	}

	// 현재 명령어의 로컬 플래그
	cmd.Flags().VisitAll(addFlag)

	// 현재 명령어의 persistent 플래그
	cmd.PersistentFlags().VisitAll(addFlag)

	// 부모 명령어의 persistent 플래그 (inherited)
	cmd.InheritedFlags().VisitAll(addFlag)

	sort.Strings(candidates)
	return candidates
}

// completeResource 는 API 에서 리소스 이름을 조회하여 자동완성 후보를 반환한다.
// resourceType 은 API 리소스 타입 (예: "flows"), prefix 는 접두사이다.
// API 호출 실패 시 빈 목록을 반환한다 (에러 없음).
func (ac *AutoCompleter) completeResource(resourceType, prefix string) []string {
	// 캐시 확인
	items, ok := ac.getCached(resourceType)
	if !ok {
		// 캐시 미스: API 에서 리소스 목록 조회
		items = ac.fetchResources(resourceType)
		if items != nil {
			ac.setCache(resourceType, items)
		}
	}

	// 접두사 필터링
	var candidates []string
	for _, item := range items {
		if strings.HasPrefix(item, prefix) {
			candidates = append(candidates, item)
		}
	}

	return candidates
}

// resolveResourceType 은 명령어에서 API 리소스 타입을 추론한다.
// 매핑되지 않는 명령어는 빈 문자열을 반환한다.
func (ac *AutoCompleter) resolveResourceType(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	return resourceTypeMap[cmd.Name()]
}

// getCached 는 캐시에서 리소스 목록을 조회한다.
// TTL 이 만료된 항목은 무시한다.
func (ac *AutoCompleter) getCached(key string) ([]string, bool) {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	entry, ok := ac.cache[key]
	if !ok {
		return nil, false
	}

	// TTL 만료 확인
	if time.Since(entry.fetchedAt) > ac.cacheTTL {
		return nil, false
	}

	return entry.items, true
}

// setCache 는 캐시에 리소스 목록을 저장한다.
func (ac *AutoCompleter) setCache(key string, items []string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.cache[key] = &cacheEntry{
		items:     items,
		fetchedAt: time.Now(),
	}
}

// fetchResources 는 API 에서 리소스 목록을 조회한다.
// 클라이언트가 nil 이거나 API 호출 실패 시 nil 을 반환한다.
func (ac *AutoCompleter) fetchResources(resourceType string) []string {
	if ac.client == nil || *ac.client == nil {
		return nil
	}

	client := *ac.client
	path := "/api/v1/" + resourceType

	var listResp resourceListResponse
	if err := client.Get(path, &listResp); err != nil {
		return nil
	}

	var names []string
	for _, item := range listResp.Items {
		if item.Name != "" {
			names = append(names, item.Name)
		} else if item.ID != "" {
			names = append(names, item.ID)
		}
	}

	return names
}

// findCommand 는 토큰 목록에서 현재 명령어를 찾는다.
// 플래그가 아닌 토큰만 명령어 경로로 사용한다.
func (ac *AutoCompleter) findCommand(tokens []string) *cobra.Command {
	cmd := ac.rootCmd
	for _, token := range tokens {
		if strings.HasPrefix(token, "-") {
			continue // 플래그는 무시
		}
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == token {
				cmd = sub
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	return cmd
}

// findParentCommand 는 토큰 목록에서 리소스 타입을 가진 부모 명령어를 찾는다.
// 예: ["flow", "get"] -> flow 커맨드를 반환
func (ac *AutoCompleter) findParentCommand(tokens []string) *cobra.Command {
	cmd := ac.rootCmd
	for _, token := range tokens {
		if strings.HasPrefix(token, "-") {
			continue
		}
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == token {
				// 리소스 서브커맨드이면 부모를 반환
				if resourceSubcommands[token] {
					return cmd
				}
				cmd = sub
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return cmd
}

// splitLineForCompletion 는 입력 줄을 완성된 토큰과 현재 입력 중인 부분으로 분리한다.
// 마지막 공백 이후가 partial (현재 입력 중), 그 이전이 tokens (완성된 토큰)이다.
func splitLineForCompletion(line string) (tokens []string, partial string) {
	// 후행 공백이 있으면 새 토큰 입력 시작
	if line == "" {
		return nil, ""
	}

	if strings.HasSuffix(line, " ") {
		// 공백으로 끝나면 이전 토큰들은 완성, 새 토큰 시작
		allTokens := strings.Fields(line)
		return allTokens, ""
	}

	// 공백으로 끝나지 않으면 마지막 토큰이 partial
	allTokens := strings.Fields(line)
	if len(allTokens) == 0 {
		return nil, ""
	}
	if len(allTokens) == 1 {
		return nil, allTokens[0]
	}

	return allTokens[:len(allTokens)-1], allTokens[len(allTokens)-1]
}
