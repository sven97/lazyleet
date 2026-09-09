package leetcode

import (
	"strings"
	"testing"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

func TestPreprocessStatementHTMLSuperscripts(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`1 &lt;= n &lt;= 10<sup>15</sup>`, `1 &lt;= n &lt;= 10^15`},
		{`<code>-2<sup>31</sup> &lt;= x &lt; 2<sup>31</sup></code>`, `<code>-2^31 &lt;= x &lt; 2^31</code>`},
		{`H<sub>2</sub>O`, `H_2O`},
		{`a<sup> 5 </sup>`, `a^5`}, // surrounding whitespace trimmed
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
	if !strings.Contains(md, "10^15") {
		t.Fatalf("exponent lost in conversion:\n%s", md)
	}
	if strings.Contains(md, "1015") {
		t.Fatalf("exponent flattened to 1015:\n%s", md)
	}
}
