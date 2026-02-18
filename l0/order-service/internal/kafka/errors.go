package kafka

// PermanentError wraps errors that should not be retried (invalid JSON, validation failures)
// Transient errors (DB down, network) are returned as-is.
type PermanentError struct {
	Err error
}

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }
