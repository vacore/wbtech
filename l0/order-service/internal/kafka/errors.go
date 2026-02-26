package kafka

import "errors"

// ErrPermanent marks errors that should not be retried (invalid JSON, validation failures).
// Transient errors (DB down, network) are returned without this sentinel.
var ErrPermanent = errors.New("permanent")
