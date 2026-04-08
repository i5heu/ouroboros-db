package byteutil

// CloneBytes returns a copy of src, or nil if src is nil.
func CloneBytes(src []byte) []byte { // A
	if src == nil {
		return nil
	}
	return append([]byte(nil), src...)
}
