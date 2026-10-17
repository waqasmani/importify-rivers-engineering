package htmlx

import (
	"strings"
	"testing"
)

func TestSanitizeSnapshot(t *testing.T) {
	in := `<html><head><script>steal()</script></head><body onload="x()">
<form action="javascript:evil()"><input type="password" name="pw" value="hunter2"><input name="csrf_token" value="abc">
<textarea>secret notes</textarea></form><a href="javascript:alert(1)">x</a><img src="/a.png" onerror="y()">
<iframe src="https://ads.example"></iframe><!-- comment --><p>Visible</p></body></html>`
	out, err := SanitizeSnapshot(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"steal()", "hunter2", "onload", "onerror", "javascript:", "<iframe", "secret notes", "comment", `value="abc"`} {
		if strings.Contains(out, bad) {
			t.Errorf("sanitized snapshot still contains %q", bad)
		}
	}
	if !strings.Contains(out, "Visible") || !strings.Contains(out, `src="/a.png"`) {
		t.Errorf("sanitizer removed safe content: %s", out)
	}
}

func TestText(t *testing.T) {
	got := Text(`<html><head><title>T</title><style>p{}</style></head><body><h1>Title</h1><p>One  two</p><script>x</script><ul><li>A</li><li>B</li></ul></body></html>`, 1000)
	want := "Title\nOne two\nA\nB"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
