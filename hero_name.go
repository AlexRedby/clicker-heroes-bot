package main

// startupHeroNameKey returns the comparison key used for startup hero names.
// It deliberately handles only the common OCR ambiguity between I and l.
func startupHeroNameKey(raw string) string {
	key := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case c >= 'A' && c <= 'Z':
			c += 'a' - 'A'
		case c >= 'a' && c <= 'z':
		default:
			continue
		}
		if c == 'l' {
			c = 'i'
		}
		key = append(key, c)
	}
	return string(key)
}
