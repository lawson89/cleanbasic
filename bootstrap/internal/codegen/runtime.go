package codegen

import "strings"

const (
	cbErrorRuntimeHelper = `type CbError struct {
	msg  string
	code int64
	file string
	line int64
}

func (e CbError) Error() string { return e.msg }

`

	cbListGetRuntimeHelper = `func cbListGet[T any](s []T, idx int64) T {
	var zero T
	if idx < 0 || int(idx) >= len(s) {
		return zero
	}
	return s[idx]
}

`

	cbStrGetRuntimeHelper = `func cbStrGet(s string, idx int64) byte {
	if idx < 0 || int(idx) >= len(s) {
		return 0
	}
	return s[idx]
}

`

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

	cbBoxRuntimeHelpers = `func cbBox[T any](v T) *T { return &v }
func cbUnbox[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}
func cbEnsureBox[T any](p **T) *T {
	if *p == nil {
		*p = new(T)
	}
	return *p
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
		return CbError{msg: "cannot post to closed mailbox"}
	}
	done := m.done
	m.mu.Unlock()
	select {
	case m.ch <- v:
		return nil
	case <-done:
		return CbError{msg: "cannot post to closed mailbox"}
	}
}

func (m *CbMailbox[T]) postTimeout(v T, ms int64) (bool, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return false, CbError{msg: "cannot post to closed mailbox"}
	}
	done := m.done
	m.mu.Unlock()
	if ms <= 0 {
		select {
		case m.ch <- v:
			return true, nil
		case <-done:
			return false, CbError{msg: "cannot post to closed mailbox"}
		default:
			return false, nil
		}
	}
	timer := time.NewTimer(time.Duration(ms) * time.Millisecond)
	defer timer.Stop()
	select {
	case m.ch <- v:
		return true, nil
	case <-done:
		return false, CbError{msg: "cannot post to closed mailbox"}
	case <-timer.C:
		return false, nil
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

func (m *CbMailbox[T]) checkTimeout(out *T, ms int64) bool {
	if ms <= 0 {
		return m.check(out)
	}
	select {
	case v := <-m.ch:
		*out = v
		return true
	default:
	}
	timer := time.NewTimer(time.Duration(ms) * time.Millisecond)
	defer timer.Stop()
	select {
	case v := <-m.ch:
		*out = v
		return true
	case <-m.done:
		return m.check(out)
	case <-timer.C:
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
			return zero, CbError{msg: "mailbox is closed and empty"}
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

func (m *CbMailbox[T]) isClosed() bool {
	m.mu.Lock()
	c := m.closed
	m.mu.Unlock()
	return c
}

`

	cbJobRuntimeHelper = `type CbJob[T any] struct {
	done chan struct{}
	val  T
	err  error
}

func cbStartJob[T any](fn func() (T, error)) *CbJob[T] {
	j := &CbJob[T]{done: make(chan struct{})}
	go func() {
		defer close(j.done)
		defer func() {
			if r := recover(); r != nil {
				j.err = CbError{msg: fmt.Sprint(r)}
			}
		}()
		j.val, j.err = fn()
	}()
	return j
}

func (j *CbJob[T]) wait() (T, error) {
	<-j.done
	return j.val, j.err
}

func (j *CbJob[T]) waitVoid() error {
	<-j.done
	return j.err
}

func (j *CbJob[T]) waitTimeout(out *T, ms int64) (bool, error) {
	if ms <= 0 {
		select {
		case <-j.done:
			if j.err != nil {
				return false, j.err
			}
			if out != nil {
				*out = j.val
			}
			return true, nil
		default:
			return false, nil
		}
	}
	timer := time.NewTimer(time.Duration(ms) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-j.done:
		if j.err != nil {
			return false, j.err
		}
		if out != nil {
			*out = j.val
		}
		return true, nil
	case <-timer.C:
		return false, nil
	}
}

func (j *CbJob[T]) waitTimeoutVoid(ms int64) (bool, error) {
	return j.waitTimeout(nil, ms)
}

func (j *CbJob[T]) isDone() bool {
	select {
	case <-j.done:
		return true
	default:
		return false
	}
}

`
)

func (e *emitter) emitRuntimeHelpers() string {
	if e.usedMailbox {
		e.useImport("sync")
		e.useImport("time")
		e.usedCbError = true
	}
	if e.usedJob {
		e.useImport("fmt")
		e.useImport("time")
		e.usedCbError = true
	}

	var helpers strings.Builder
	if e.usedCbError {
		helpers.WriteString(cbErrorRuntimeHelper)
	}
	if e.usedListGet {
		helpers.WriteString(cbListGetRuntimeHelper)
	}
	if e.usedStrGet {
		helpers.WriteString(cbStrGetRuntimeHelper)
	}
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
	if e.usedBoxHelpers {
		helpers.WriteString(cbBoxRuntimeHelpers)
	}
	if e.usedMailbox {
		helpers.WriteString(cbMailboxRuntimeHelper)
	}
	if e.usedJob {
		helpers.WriteString(cbJobRuntimeHelper)
	}
	return helpers.String()
}
