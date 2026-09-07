package leetcode

import "testing"

func TestParseFrontendID(t *testing.T) {
	cases := map[string]int{
		"1":         1,
		"  42 ":     42,
		"1768":      1768,
		"LCP 01":    0,
		"面试题 01.01": 0,
		"":          0,
		"12abc":     12,
	}
	for in, want := range cases {
		if got := parseFrontendID(in); got != want {
			t.Errorf("parseFrontendID(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestTitleCaseDifficulty(t *testing.T) {
	for in, want := range map[string]string{
		"EASY": "Easy", "medium": "Medium", "Hard": "Hard", "weird": "weird",
	} {
		if got := titleCaseDifficulty(in); got != want {
			t.Errorf("titleCaseDifficulty(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseMetaFunctionForm(t *testing.T) {
	m, err := parseMeta(`{"name":"twoSum","params":[{"name":"nums","type":"integer[]"},{"name":"target","type":"integer"}],"return":{"type":"integer[]"}}`)
	if err != nil {
		t.Fatalf("parseMeta: %v", err)
	}
	if m.Name != "twoSum" || m.Arity() != 2 || m.Params[1].Type != "integer" || m.Return.Type != "integer[]" {
		t.Fatalf("meta = %+v", m)
	}
}

func TestParseMetaDesignFormHasNoEntry(t *testing.T) {
	m, err := parseMeta(`{"classname":"LRUCache","constructor":{"params":[{"type":"integer","name":"capacity"}]},"methods":[],"systemdesign":true}`)
	if err != nil {
		t.Fatalf("parseMeta: %v", err)
	}
	if m.Name != "" {
		t.Fatalf("design problem should have empty entry name, got %q", m.Name)
	}
}
