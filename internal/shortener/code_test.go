package shortener

import (
	"strings"
	"testing"
)

func TestRandomCodeFormat(t *testing.T) {
	const alphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ123456789"

	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		c, err := RandomCode(6)
		if err != nil {
			t.Fatal(err)
		}
		if len(c) != 6 {
			t.Fatalf("len = %d, want 6", len(c))
		}
		for _, r := range c {
			if !strings.ContainsRune(alphabet, r) {
				t.Fatalf("unexpected char %q in %q", r, c)
			}
		}
		seen[c] = true
	}
	if len(seen) < 150 {
		t.Fatalf("only %d unique codes in 200 attempts — bad entropy", len(seen))
	}
}
