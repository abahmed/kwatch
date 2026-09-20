package message

import "testing"

func TestChunksPreserveUTF8ByteBoundaries(t *testing.T) {
	chunks := Chunks("aé日b", 3)
	if len(chunks) != 3 || chunks[0] != "aé" ||
		chunks[1] != "日" || chunks[2] != "b" {
		t.Fatalf("unexpected chunks: %#v", chunks)
	}
}

func TestChunksKeepsSmallAndNonPositiveInputs(t *testing.T) {
	for _, test := range []struct {
		name string
		size int
	}{
		{name: "small", size: 10},
		{name: "zero", size: 0},
		{name: "negative", size: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := Chunks("hello", test.size)
			if len(got) != 1 || got[0] != "hello" {
				t.Fatalf("Chunks() = %#v", got)
			}
		})
	}
}
