package export

import "testing"

func TestGormSourceSatisfiesSource(t *testing.T) {
	var _ Source = gormSource{}
}
