package claim

import "time"

// timeNow is the indirection used by ReapExpired so tests can override
// the clock without changing every call site.
var timeNow = func() time.Time { return time.Now().UTC() }
