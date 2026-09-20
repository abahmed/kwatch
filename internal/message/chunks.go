package message

// Chunks splits text into UTF-8-safe slices whose byte length is at most
// chunkSize where possible. Provider limits are byte limits, not rune limits.
func Chunks(text string, chunkSize int) []string {
	if chunkSize <= 0 || chunkSize >= len(text) {
		return []string{text}
	}

	chunks := make([]string, 0, (len(text)-1)/chunkSize+1)
	start := 0
	for index := range text {
		if index > start && index-start >= chunkSize {
			chunks = append(chunks, text[start:index])
			start = index
		}
	}
	return append(chunks, text[start:])
}
