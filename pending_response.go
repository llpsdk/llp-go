package llp

type PendingResponse struct {
	ch   chan struct{}
	msg  *TextMessage
	err  *PlatformError
	done bool
}

func (r *PendingResponse) Done() <-chan struct{} {
	if r.ch != nil {
		return r.ch
	}
	r.ch = make(chan struct{})
	return r.ch
}

func (r *PendingResponse) Message() (*TextMessage, error) {
	return r.msg, r.err
}

func (r *PendingResponse) setDone(t TextMessage) {
	if !r.done {
		r.done = true
		r.msg = &t
	}
	if r.ch != nil {
		close(r.ch)
	}
}

func (r *PendingResponse) setError(e PlatformError) {
	if !r.done {
		r.done = true
		r.err = &e
	}
	if r.ch != nil {
		close(r.ch)
	}
}
