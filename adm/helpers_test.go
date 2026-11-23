package adm

// PCM layout constant mirrored for tests: header(12) + chunk header(16),
// then the first PTS payload byte. Tampering there invalidates the trailing
// SHA-256 without touching the hash field itself.
func headerSizeInTest() int { return 12 + 16 }
