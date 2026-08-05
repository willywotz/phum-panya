package media

import (
	"io"
	"mime/multipart"
	"testing"
)

// assert the local filesystem adapter satisfies the port.
var _ Store = (*LocalStore)(nil)

func TestStoreIsInterface(t *testing.T) {
	var s Store = NewLocalStore(t.TempDir())
	if _, err := s.UsageBytes(); err != nil {
		t.Fatalf("UsageBytes on empty dir: %v", err)
	}
	// compile-time proof the port shape matches usage:
	var _ func(io.Reader) (string, error) = s.SaveReader
	var _ func(*multipart.FileHeader) (string, error) = s.SaveMultipart
}
