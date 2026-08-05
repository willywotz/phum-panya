package auth

// assert the concrete adapters satisfy the auth ports.
var (
	_ Sessions = (*SessionStore)(nil)
	_ Limiter  = (*Throttle)(nil)
	_ Users    = gormUsers{}
)
