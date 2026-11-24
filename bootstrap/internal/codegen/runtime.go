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

	cbJobRuntimeHelper = `type cbJob[T any] struct {
	done chan struct{}
	val  T
}

func cbStartJob[T any](fn func() T) *cbJob[T] {
	j := &cbJob[T]{done: make(chan struct{})}
	go func() {
		defer close(j.done)
		j.val = fn()
	}()
	return j
}

func (j *cbJob[T]) wait() T {
	if j == nil {
		var zero T
		return zero
	}
	<-j.done
	return j.val
}

`

	cbVoidJobRuntimeHelper = `type cbVoidJob struct {
	done chan struct{}
}

func cbStartVoidJob(fn func()) *cbVoidJob {
	j := &cbVoidJob{done: make(chan struct{})}
	go func() {
		defer close(j.done)
		fn()
	}()
	return j
}

func (j *cbVoidJob) wait() {
	if j == nil {
		return
	}
	<-j.done
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
	if e.usedJob {
		helpers.WriteString(cbJobRuntimeHelper)
	}
	if e.usedVoidJob {
		helpers.WriteString(cbVoidJobRuntimeHelper)
	}
	return helpers.String()
}
