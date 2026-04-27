package system

import "fmt"

// resolveAlias resolves a file alias to its sandboxed path.
func (f *FileAgentImpl) resolveAlias(alias string) (string, error) {
	val, ok := f.targetFiles.Load(alias)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrAliasNotFound, alias)
	}
	return val.(string), nil
}

// resolvePath resolves either alias or direct path from command params.
// If the params contain an alias, it is resolved via targetFiles. Otherwise
// the direct path is validated through the sandbox.
func (f *FileAgentImpl) resolvePath(params fileCommandParams) (string, error) {
	if params.Alias != "" {
		return f.resolveAlias(params.Alias)
	}
	if params.Path == "" {
		return "", fmt.Errorf("%w: path or alias is required", ErrInvalidCommandFormat)
	}
	return f.resolveSandboxPath(params.Path)
}
