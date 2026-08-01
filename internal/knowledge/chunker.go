package knowledge

import "strings"

const (
	defaultChunkMaxLines = 80
	defaultChunkMaxChars = 4000
)

func ChunkContent(content string) []Chunk {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	chunks := make([]Chunk, 0, len(lines)/defaultChunkMaxLines+1)
	start := 0
	length := 0

	flush := func(end int) {
		if end <= start {
			return
		}
		value := strings.TrimSpace(strings.Join(lines[start:end], "\n"))
		if value != "" {
			chunks = append(chunks, Chunk{
				Index:     len(chunks),
				Content:   value,
				StartLine: start + 1,
				EndLine:   end,
			})
		}
		start = end
		length = 0
	}

	for index, line := range lines {
		lineLength := len([]rune(line)) + 1
		if index > start && (index-start >= defaultChunkMaxLines || length+lineLength > defaultChunkMaxChars) {
			flush(index)
		}
		length += lineLength
	}
	flush(len(lines))
	return chunks
}
