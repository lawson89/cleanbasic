package codegen

import "strings"

const (
	cbDictGetRuntimeHelper = `func cbDictGet[K comparable, V any](m map[K]V, k K, def V) V {
	if v, ok := m[k]; ok {
		return v
	}
	return def
}

`

	cbDictContainsRuntimeHelper = `func cbDictContains[K comparable, V any](m map[K]V, k K) bool {
	_, ok := m[k]
	return ok
}

`

	cbStrToIntRuntimeHelper = `func cbStrToInt(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

`
)

func (e *emitter) emitRuntimeHelpers() string {
	var helpers strings.Builder
	if e.usedDictGet {
		helpers.WriteString(cbDictGetRuntimeHelper)
	}
	if e.usedDictContains {
		helpers.WriteString(cbDictContainsRuntimeHelper)
	}
	if e.usedStrToInt {
		e.useImport("strconv")
		helpers.WriteString(cbStrToIntRuntimeHelper)
	}
	return helpers.String()
}
