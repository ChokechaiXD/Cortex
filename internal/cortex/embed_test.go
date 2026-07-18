package cortex

import (
	"testing"
)

func TestEmbedOfflineDeterministic(t *testing.T) {
	a := embedOffline("curator automatic mode")
	b := embedOffline("curator automatic mode")
	if len(a) != EmbedDim {
		t.Fatalf("offline dim = %d, want %d", len(a), EmbedDim)
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("offline embedder not deterministic at %d", i)
		}
	}
	// unrelated text should differ
	c := embedOffline("banana smoothie recipe")
	diff := 0
	for i := range a {
		if a[i] != c[i] {
			diff++
		}
	}
	if diff < 10 {
		t.Fatalf("offline embedder collisions too high (diff=%d)", diff)
	}
}

func TestEmbedRemoteFallbackOnBadURL(t *testing.T) {
	// Point at an impossible endpoint; embedRemote must return nil (no panic,
	// no error) so the caller falls back to the offline embedder.
	SetEmbedEndpoint("http://127.0.0.1:1/v1/embeddings", "x")
	vec := embedRemote("anything")
	if vec != nil {
		t.Fatalf("embedRemote should return nil on unreachable endpoint, got %d-dim vec", len(vec))
	}
	// embedText should then return the offline vector, not nil.
	out := embedText("fallback test")
	if len(out) != EmbedDim {
		t.Fatalf("embedText fallback dim = %d, want %d", len(out), EmbedDim)
	}
}

func TestEncodeDecodeVectorRoundTrip(t *testing.T) {
	vec := embedOffline("round trip")
	blob := encodeVector(vec)
	got := decodeVector(blob)
	if len(got) != len(vec) {
		t.Fatalf("decode len %d != %d", len(got), len(vec))
	}
	for i := range vec {
		if vec[i] != got[i] {
			t.Fatalf("round trip mismatch at %d", i)
		}
	}
}
