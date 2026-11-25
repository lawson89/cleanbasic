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

func (j *cbJob[T]) isDone() bool {
	if j == nil {
		return true
	}
	select {
	case <-j.done:
		return true
	default:
		return false
	}
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

func (j *cbVoidJob) isDone() bool {
	if j == nil {
		return true
	}
	select {
	case <-j.done:
		return true
	default:
		return false
	}
}

`

	cbJobTimeoutRuntimeHelper = `func (j *cbJob[T]) waitTimeout(ms int64) (T, error) {
	if j == nil {
		var zero T
		return zero, nil
	}
	select {
	case <-j.done:
		return j.val, nil
	default:
	}
	if ms <= 0 {
		var zero T
		return zero, fmt.Errorf("job timed out after %d ms", ms)
	}
	timer := time.NewTimer(time.Duration(ms) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-j.done:
		return j.val, nil
	case <-timer.C:
		var zero T
		return zero, fmt.Errorf("job timed out after %d ms", ms)
	}
}

`

	cbVoidJobTimeoutRuntimeHelper = `func (j *cbVoidJob) waitTimeout(ms int64) error {
	if j == nil {
		return nil
	}
	select {
	case <-j.done:
		return nil
	default:
	}
	if ms <= 0 {
		return fmt.Errorf("job timed out after %d ms", ms)
	}
	timer := time.NewTimer(time.Duration(ms) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-j.done:
		return nil
	case <-timer.C:
		return fmt.Errorf("job timed out after %d ms", ms)
	}
}

`

	cbCancelTokenRuntimeHelper = `type cbCancelToken struct {
	canceled _cb_atomic.Bool
	once     sync.Once
	done     chan struct{}
}

func cbNewCancelToken() *cbCancelToken {
	return &cbCancelToken{done: make(chan struct{})}
}

func (t *cbCancelToken) cancel() {
	if t == nil {
		return
	}
	t.once.Do(func() {
		t.canceled.Store(true)
		close(t.done)
	})
}

func (t *cbCancelToken) isCanceled() bool {
	if t == nil {
		return false
	}
	return t.canceled.Load()
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
	if e.usedJobTimeout {
		e.useImport("fmt")
		e.useImport("time")
		helpers.WriteString(cbJobTimeoutRuntimeHelper)
	}
	if e.usedVoidJobTimeout {
		e.useImport("fmt")
		e.useImport("time")
		helpers.WriteString(cbVoidJobTimeoutRuntimeHelper)
	}
	if e.usedCancelToken {
		e.useImport("sync")
		e.useGoImport("sync/atomic", "_cb_atomic")
		helpers.WriteString(cbCancelTokenRuntimeHelper)
	}
	return helpers.String()
}
