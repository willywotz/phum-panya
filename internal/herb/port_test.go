package herb

// assert the GORM adapter satisfies the handler's port.
var _ repository = (*Repo)(nil)
