package encoding

import "testing"

// The encoding package is deprecated, MIME types live in metadata. Keep a no-op
// regression test to make sure the package still builds while migration is
// ongoing.
func TestEncodingPackageDeprecated(t *testing.T) {}
