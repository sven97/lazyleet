package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sven97/lazyleet/internal/leetcode"
)

func TestScaffoldCreatesFilesAndIsIdempotent(t *testing.T) {
	q, ok := leetcode.Fixture("two-sum")
	if !ok {
		t.Fatal("two-sum fixture missing")
	}
	root := t.TempDir()

	ws, err := Scaffold(root, q, "python3")
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	wantDir := filepath.Join(root, "1-two-sum")
	if ws.Dir != wantDir {
		t.Errorf("Dir = %q, want %q", ws.Dir, wantDir)
	}
	for _, p := range []string{ws.SolutionPath, ws.TestsPath, ws.MetaPath, ws.NotesPath} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected file %s: %v", p, err)
		}
	}

	cases, err := ws.ReadCases()
	if err != nil {
		t.Fatalf("ReadCases: %v", err)
	}
	if len(cases) != 3 || cases[0].Out != "[0,1]" {
		t.Fatalf("cases = %+v, want the 3 seeded example cases", cases)
	}

	// User edits the solution; re-scaffolding must not clobber it.
	const edited = "class Solution:\n    def twoSum(self, nums, target):\n        return [0, 1]\n"
	if err := os.WriteFile(ws.SolutionPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Scaffold(root, q, "python3"); err != nil {
		t.Fatalf("re-Scaffold: %v", err)
	}
	got, _ := os.ReadFile(ws.SolutionPath)
	if string(got) != edited {
		t.Fatalf("Scaffold clobbered an existing solution file:\n%s", got)
	}
}

func TestScaffoldRejectsUnknownLanguage(t *testing.T) {
	q, _ := leetcode.Fixture("two-sum")
	if _, err := Scaffold(t.TempDir(), q, "cobol"); err == nil {
		t.Fatal("expected error for a language with no snippet")
	}
}
