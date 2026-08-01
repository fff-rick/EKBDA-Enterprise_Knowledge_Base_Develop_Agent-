package knowledge

import (
	"strings"
	"testing"
)

func TestChunkContentTracksLineRanges(t *testing.T) {
	lines := make([]string, 81)
	for index := range lines {
		lines[index] = "line"
	}
	chunks := ChunkContent(strings.Join(lines, "\n"))
	if len(chunks) != 2 {
		t.Fatalf("expected two chunks, got %d", len(chunks))
	}
	if chunks[0].StartLine != 1 || chunks[0].EndLine != 80 {
		t.Fatalf("unexpected first chunk range: %#v", chunks[0])
	}
	if chunks[1].StartLine != 81 || chunks[1].EndLine != 81 {
		t.Fatalf("unexpected second chunk range: %#v", chunks[1])
	}
}
