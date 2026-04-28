package sessionstores

import "errors"

// ErrNotImplemented is returned by optional SessionStore methods that are not
// implemented by a particular adapter. SDK callers should fall back to
// ListSessions + per-session Load when this error is returned.
var ErrNotImplemented = errors.New("not implemented")
