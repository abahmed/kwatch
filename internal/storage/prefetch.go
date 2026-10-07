package storage

import (
	"io"
	"os"
)

// prefetchChunk is the read size of prefetch.
const prefetchChunk = 4 << 20

// prefetch reads the whole file once, front to back, so that the page
// check after it finds every page in the operating system's cache. The
// check visits pages in tree order, which on a network volume means many
// small random reads; one sequential pass is many times faster there.
// It only warms the cache: an error is ignored, and the check reads what
// is missing itself.
func prefetch(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = io.CopyBuffer(io.Discard, file, make([]byte, prefetchChunk))
}
