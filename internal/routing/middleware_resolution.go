package routing

import "github.com/nimburion/nimburion/pkg/http/router"

func ApplyMiddlewareDirectives(base, additions, disabled []string) []string {
	result := append([]string(nil), base...)

	for _, middlewareName := range additions {
		if containsMiddleware(result, middlewareName) {
			continue
		}
		result = append(result, middlewareName)
	}

	if len(disabled) == 0 {
		return result
	}

	filtered := make([]string, 0, len(result))
	for _, middlewareName := range result {
		if containsMiddleware(disabled, middlewareName) {
			continue
		}
		filtered = append(filtered, middlewareName)
	}

	return filtered
}

func BuildMiddlewareChain(names []string, registry map[string]func() router.MiddlewareFunc) ([]router.MiddlewareFunc, error) {
	chain := make([]router.MiddlewareFunc, 0, len(names))
	for _, middlewareName := range names {
		factory, ok := registry[middlewareName]
		if !ok {
			return nil, ErrUnknownMiddleware(middlewareName)
		}
		chain = append(chain, factory())
	}
	return chain, nil
}

func containsMiddleware(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

type unknownMiddlewareError string

func (e unknownMiddlewareError) Error() string {
	return "unknown middleware: " + string(e)
}

func ErrUnknownMiddleware(name string) error {
	return unknownMiddlewareError(name)
}
