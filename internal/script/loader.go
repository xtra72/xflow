package script

import (
	"context"
	"fmt"
	"os"
)

// StoreAccessor 는 Store 기반 스크립트 로딩을 위한 인터페이스이다.
// 순환 의존을 피하기 위해 script 패키지 내에 정의한다.
type StoreAccessor interface {
	Get(ctx context.Context, key string) (any, error)
	Set(ctx context.Context, key string, value any) error
	Delete(ctx context.Context, key string) error
	Has(ctx context.Context, key string) (bool, error)
}

// LoaderOption 은 ScriptLoader 생성 옵션 함수 타입이다.
type LoaderOption func(*ScriptLoader)

// WithStoreAccessor 는 ScriptLoader에 StoreAccessor를 설정한다.
func WithStoreAccessor(store StoreAccessor) LoaderOption {
	return func(l *ScriptLoader) {
		l.store = store
	}
}

// ScriptLoader 는 다양한 소스에서 스크립트를 로드하는 구조체이다.
type ScriptLoader struct {
	store StoreAccessor
}

// NewScriptLoader 는 새 ScriptLoader를 생성한다.
func NewScriptLoader(opts ...LoaderOption) *ScriptLoader {
	l := &ScriptLoader{}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// LoadScript 는 ScriptSource에서 스크립트 내용을 로드하여 반환한다.
// SourceInline: Content 필드 반환
// SourceFile: Path 경로의 파일을 읽어 반환
// SourceStore: StoreAccessor를 통해 StoreKey로 조회하여 반환
func (l *ScriptLoader) LoadScript(ctx context.Context, source ScriptSource) (string, error) {
	switch source.Type {
	case SourceInline:
		return l.loadInline(source)
	case SourceFile:
		return l.loadFile(source)
	case SourceStore:
		return l.loadStore(ctx, source)
	default:
		return "", &ScriptError{
			Err:      ErrInvalidScriptSource,
			ScriptID: source.Name,
			Detail:   fmt.Sprintf("unsupported source type: %d", source.Type),
		}
	}
}

// loadInline 은 인라인 소스에서 스크립트 내용을 반환한다.
func (l *ScriptLoader) loadInline(source ScriptSource) (string, error) {
	if source.Content == "" {
		return "", &ScriptError{
			Err:      ErrInvalidScriptSource,
			ScriptID: source.Name,
			Detail:   "empty script content",
		}
	}
	return source.Content, nil
}

// loadFile 은 파일에서 스크립트 내용을 읽어 반환한다.
func (l *ScriptLoader) loadFile(source ScriptSource) (string, error) {
	if source.Path == "" {
		return "", &ScriptError{
			Err:      ErrInvalidScriptSource,
			ScriptID: source.Name,
			Detail:   "empty file path",
		}
	}

	data, err := os.ReadFile(source.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", &ScriptError{
				Err:      ErrScriptNotFound,
				ScriptID: source.Name,
				Detail:   fmt.Sprintf("file not found: %s", source.Path),
			}
		}
		return "", &ScriptError{
			Err:      ErrScriptNotFound,
			ScriptID: source.Name,
			Detail:   fmt.Sprintf("failed to read file: %s", err.Error()),
		}
	}

	return string(data), nil
}

// loadStore 는 StoreAccessor를 통해 스크립트 내용을 조회하여 반환한다.
func (l *ScriptLoader) loadStore(ctx context.Context, source ScriptSource) (string, error) {
	if l.store == nil {
		return "", &ScriptError{
			Err:      ErrInvalidScriptSource,
			ScriptID: source.Name,
			Detail:   "store accessor not configured",
		}
	}

	if source.StoreKey == "" {
		return "", &ScriptError{
			Err:      ErrInvalidScriptSource,
			ScriptID: source.Name,
			Detail:   "empty store key",
		}
	}

	val, err := l.store.Get(ctx, source.StoreKey)
	if err != nil {
		return "", &ScriptError{
			Err:      ErrScriptNotFound,
			ScriptID: source.Name,
			Detail:   fmt.Sprintf("store key not found: %s", source.StoreKey),
		}
	}

	// 문자열 또는 []byte로 변환
	switch v := val.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		return "", &ScriptError{
			Err:      ErrInvalidScriptSource,
			ScriptID: source.Name,
			Detail:   fmt.Sprintf("store value is not a string: %T", val),
		}
	}
}
