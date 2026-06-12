package provider

import (
	"strings"
	"testing"
)

type pageItem struct {
	Name string `json:"name"`
	N    int    `json:"n"`
}

func TestDecodePages_SingleArray(t *testing.T) {
	got, err := DecodePages[pageItem]([]byte(`[{"name":"a","n":1},{"name":"b","n":2}]`))
	if err != nil {
		t.Fatalf("DecodePages: %v", err)
	}
	if len(got) != 2 || got[0].Name != "a" || got[1].N != 2 {
		t.Errorf("got %+v, want 2 items a/1 b/2", got)
	}
}

// TestDecodePages_ConcatenatedPages pins the actual --paginate output
// shape: `gh api --help` — "Each page is a separate JSON array or
// object" — so multi-page responses are back-to-back arrays, not one.
func TestDecodePages_ConcatenatedPages(t *testing.T) {
	raw := []byte(`[{"name":"a","n":1},{"name":"b","n":2}]` + "\n" +
		`[{"name":"c","n":3}]` + "\n" + `[{"name":"d","n":4}]`)
	got, err := DecodePages[pageItem](raw)
	if err != nil {
		t.Fatalf("DecodePages on concatenated pages: %v", err)
	}
	if len(got) != 4 || got[2].Name != "c" || got[3].Name != "d" {
		t.Errorf("got %+v, want 4 items spanning all pages", got)
	}
}

func TestDecodePages_EmptyAndWhitespaceInput(t *testing.T) {
	for _, raw := range []string{"", "  \n"} {
		got, err := DecodePages[pageItem]([]byte(raw))
		if err != nil || len(got) != 0 {
			t.Errorf("input %q: got %v items, err %v; want 0, nil", raw, len(got), err)
		}
	}
}

func TestDecodePages_MalformedErrors(t *testing.T) {
	_, err := DecodePages[pageItem]([]byte(`[{"name":"a"}] not-json`))
	if err == nil || !strings.Contains(err.Error(), "page") {
		t.Errorf("malformed trailing page must error mentioning page; got %v", err)
	}
}
