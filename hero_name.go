package main

// startupHeroNameKey returns the comparison key used for startup hero names.
// It handles the OCR ambiguities I/l and c/e without approximate name search.
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
		switch c {
		case 'l':
			c = 'i'
		case 'c':
			c = 'e'
		}
		key = append(key, c)
	}
	return string(key)
}
