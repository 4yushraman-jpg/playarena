package public

import "errors"

// ErrNotFound is the ONLY not-found signal the public path emits. A tournament
// that is private, draft, owned by an inactive org, or simply non-existent all
// map to this single error → HTTP 404. The public path never returns 403 and
// never distinguishes "exists but hidden" from "does not exist", so private and
// draft tournaments cannot be probed for existence.
var ErrNotFound = errors.New("not found")
