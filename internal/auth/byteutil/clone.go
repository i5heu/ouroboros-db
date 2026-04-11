package byteutil

import "bytes"

// CloneBytes returns a copy of src, or nil if src is nil.
func CloneBytes(src []byte) []byte { // A
	return bytes.Clone(src)
}
