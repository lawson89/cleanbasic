package codegen

import "strings"

const (
	cbSliceStrRuntimeHelper = `func cbSliceStr(s string, start, end int64) string {
	n := int64(len(s))
	if start < 0 {
		start = 0
	}
	if start > n {
		start = n
	}
	if end < 0 {
		end = 0
	}
	if end > n {
		end = n
	}
	if start >= end {
		return ""
	}
	return s[start:end]
}

`

	cbSliceListRuntimeHelper = `func cbSliceList[T any](s []T, start, end int64) []T {
	n := int64(len(s))
	if start < 0 {
		start = 0
	}
	if start > n {
		start = n
	}
	if end < 0 {
		end = 0
	}
	if end > n {
		end = n
	}
	if start >= end {
		return nil
	}
	return s[start:end]
}

`

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

	cbStrFromCharCodeRuntimeHelper = `func cbStrFromCharCode(code int64) string {
	return string(rune(code))
}

`

	cbMailboxRuntimeHelper = `type CbMailbox[T any] struct {
	ch     chan T
	mu     sync.Mutex
	closed bool
	done   chan struct{}
}

func cbNewMailbox[T any](cap int64) *CbMailbox[T] {
	if cap < 0 {
		cap = 0
	}
	return &CbMailbox[T]{
		ch:   make(chan T, int(cap)),
		done: make(chan struct{}),
	}
}

func (m *CbMailbox[T]) post(v T) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("cannot post to closed mailbox")
	}
	done := m.done
	m.mu.Unlock()
	select {
	case m.ch <- v:
		return nil
	case <-done:
		return errors.New("cannot post to closed mailbox")
	}
}

func (m *CbMailbox[T]) check(out *T) bool {
	select {
	case v := <-m.ch:
		*out = v
		return true
	default:
		return false
	}
}

func (m *CbMailbox[T]) get() (T, error) {
	select {
	case v := <-m.ch:
		return v, nil
	default:
	}
	select {
	case v := <-m.ch:
		return v, nil
	case <-m.done:
		select {
		case v := <-m.ch:
			return v, nil
		default:
			var zero T
			return zero, errors.New("mailbox is closed and empty")
		}
	}
}

func (m *CbMailbox[T]) close() {
	m.mu.Lock()
	if !m.closed {
		m.closed = true
		close(m.done)
	}
	m.mu.Unlock()
}

func (m *CbMailbox[T]) count() int64 {
	return int64(len(m.ch))
}

`
)

func (e *emitter) emitRuntimeHelpers() string {
	if e.usedMailbox {
		e.useImport("errors")
		e.useImport("sync")
	}

	var helpers strings.Builder
	if e.usedSliceStr {
		helpers.WriteString(cbSliceStrRuntimeHelper)
	}
	if e.usedSliceList {
		helpers.WriteString(cbSliceListRuntimeHelper)
	}
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
	if e.usedStrFromCharCode {
		helpers.WriteString(cbStrFromCharCodeRuntimeHelper)
	}
	if e.usedMailbox {
		helpers.WriteString(cbMailboxRuntimeHelper)
	}
	return helpers.String()
}
