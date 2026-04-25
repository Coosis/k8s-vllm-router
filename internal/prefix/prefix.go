package prefix

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

var fields = regexp.MustCompile(`\S+`)

type Fingerprint struct {
	Length int
	Hash   string
}

func Fingerprints(prompt string, lengths []int) []Fingerprint {
	tokens := fields.FindAllString(Normalize(prompt), -1)
	out := make([]Fingerprint, 0, len(lengths))
	for _, length := range lengths {
		if length <= 0 || len(tokens) < length {
			continue
		}
		sum := sha256.Sum256([]byte(strings.Join(tokens[:length], " ")))
		out = append(out, Fingerprint{
			Length: length,
			Hash:   hex.EncodeToString(sum[:]),
		})
	}
	return out
}

func Normalize(prompt string) string {
	return strings.Join(strings.Fields(prompt), " ")
}
