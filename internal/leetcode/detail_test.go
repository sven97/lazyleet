package leetcode

import (
	"strings"
	"testing"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

func TestPreprocessStatementHTMLSuperscripts(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`1 &lt;= n &lt;= 10<sup>15</sup>`, `1 &lt;= n &lt;= 10¹⁵`},
		{`<code>-2<sup>31</sup> &lt;= x &lt; 2<sup>31</sup></code>`, `<code>-2³¹ &lt;= x &lt; 2³¹</code>`},
		{`H<sub>2</sub>O`, `H₂O`},
		{`a<sup> 5 </sup>`, `a⁵`},  // surrounding whitespace trimmed
		{`10<sup>k</sup>`, `10^k`}, // unmapped char → caret fallback
		{`no tags here`, `no tags here`},
	} {
		if got := preprocessStatementHTML(tc.in); got != tc.want {
			t.Errorf("preprocessStatementHTML(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestStatementExponentSurvivesMarkdownConversion(t *testing.T) {
	html := `<p>Constraints:</p><ul><li><code>1 &lt;= n &lt;= 10<sup>15</sup></code></li></ul>`
	md, err := htmltomarkdown.ConvertString(preprocessStatementHTML(html))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "10¹⁵") {
		t.Fatalf("exponent lost in conversion:\n%s", md)
	}
	if strings.Contains(md, "1015") {
		t.Fatalf("exponent flattened to 1015:\n%s", md)
	}
}

func TestParseExampleOutputs(t *testing.T) {
	// old <pre> markup
	pre := `<pre><strong>Input:</strong> n = 1002
<strong>Output:</strong> 3
<strong>Explanation:</strong> ...</pre>
<pre><strong>Input:</strong> n = 998
<strong>Output:</strong> 0
</pre>`
	if got := parseExampleOutputs(pre); len(got) != 2 || got[0] != "3" || got[1] != "0" {
		t.Fatalf("pre markup: %#v", got)
	}

	// newer example-block markup with entities
	block := `<p><strong class="example-io">Output:</strong> <span class="example-io">[0,1]</span></p>
<p><strong class="example-io">Output:</strong> <span class="example-io">&quot;abc&quot;</span></p>`
	if got := parseExampleOutputs(block); len(got) != 2 || got[0] != "[0,1]" || got[1] != `"abc"` {
		t.Fatalf("example-block markup: %#v", got)
	}
}
