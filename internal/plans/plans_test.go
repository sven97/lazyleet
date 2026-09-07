package plans

import "testing"

func TestBundledStarterLoads(t *testing.T) {
	b, ok := Get("lazyleet-starter")
	if !ok {
		t.Fatal("lazyleet-starter plan not embedded")
	}
	if b.Name == "" || len(b.Problems) != 10 {
		t.Fatalf("plan = %+v", b)
	}
	if b.Problems[0] != "two-sum" {
		t.Errorf("first problem = %q, want two-sum", b.Problems[0])
	}
}

func TestSlugsAndAllConsistent(t *testing.T) {
	slugs := Slugs()
	if len(slugs) != len(All()) {
		t.Fatalf("Slugs()=%d All()=%d", len(slugs), len(All()))
	}
	for _, s := range slugs {
		if _, ok := Get(s); !ok {
			t.Errorf("Slugs() lists %q but Get() misses it", s)
		}
	}
}
