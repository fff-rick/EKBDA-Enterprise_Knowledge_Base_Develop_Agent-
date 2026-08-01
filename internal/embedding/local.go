package embedding

import (
	"context"
	"math"
	"unicode"
)

const defaultLocalDimension = 384

type LocalHash struct {
	dimension int
}

func NewLocalHash(dimensions ...int) *LocalHash {
	dimension := defaultLocalDimension
	if len(dimensions) > 0 && dimensions[0] > 0 {
		dimension = dimensions[0]
	}
	return &LocalHash{dimension: dimension}
}

func (p *LocalHash) Name() string {
	return "local-hash-v1"
}

func (p *LocalHash) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for index, text := range texts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result[index] = p.embedText(text)
	}
	return result, nil
}

func (p *LocalHash) embedText(text string) []float32 {
	vector := make([]float32, p.dimension)
	runes := make([]rune, 0, len(text))
	for _, value := range []rune(text) {
		if unicode.IsSpace(value) || unicode.IsPunct(value) {
			continue
		}
		runes = append(runes, unicode.ToLower(value))
	}
	for index, value := range runes {
		addFeature(vector, []rune{value})
		if index > 0 {
			addFeature(vector, runes[index-1:index+1])
		}
	}
	normalize(vector)
	return vector
}

func addFeature(vector []float32, values []rune) {
	hash := uint64(14695981039346656037)
	for _, value := range values {
		hash ^= uint64(value)
		hash *= 1099511628211
	}
	index := int(hash % uint64(len(vector)))
	if hash&(1<<63) == 0 {
		vector[index]++
	} else {
		vector[index]--
	}
}

func normalize(vector []float32) {
	var magnitude float64
	for _, value := range vector {
		magnitude += float64(value * value)
	}
	if magnitude == 0 {
		return
	}
	scale := float32(1 / math.Sqrt(magnitude))
	for index := range vector {
		vector[index] *= scale
	}
}
