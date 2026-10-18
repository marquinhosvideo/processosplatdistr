package httpx

import (
	"encoding/json"
	"net/url"
	"testing"
)

func TestParsePage(t *testing.T) {
	tests := []struct {
		query      string
		want       Page
		badFields  []string
		wantOffset int
	}{
		{"", Page{1, 25}, nil, 0},
		{"page=3&pageSize=10", Page{3, 10}, nil, 20},
		{"page=1&pageSize=100", Page{1, 100}, nil, 0},
		{"page=0", Page{}, []string{"page"}, 0},
		{"page=-1", Page{}, []string{"page"}, 0},
		{"page=dois", Page{}, []string{"page"}, 0},
		{"pageSize=101", Page{}, []string{"pageSize"}, 0},
		{"pageSize=0", Page{}, []string{"pageSize"}, 0},
		{"page=x&pageSize=y", Page{}, []string{"page", "pageSize"}, 0},
	}
	for _, tt := range tests {
		q, _ := url.ParseQuery(tt.query)
		p, err := ParsePage(q)
		if len(tt.badFields) > 0 {
			e := apiErr(t, err)
			for _, f := range tt.badFields {
				if e.Errors[f] == "" {
					t.Errorf("%q: sem erro para %s: %v", tt.query, f, e.Errors)
				}
			}
			continue
		}
		if err != nil || p != tt.want {
			t.Errorf("%q: %+v, %v; want %+v", tt.query, p, err, tt.want)
		}
		if p.Offset() != tt.wantOffset || p.Limit() != tt.want.Size {
			t.Errorf("%q: offset=%d limit=%d", tt.query, p.Offset(), p.Limit())
		}
	}
}

func TestPageResultJSON(t *testing.T) {
	b, err := json.Marshal(NewPageResult[string](nil, Page{2, 25}, 30))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"items":[],"page":2,"pageSize":25,"total":30}` {
		t.Errorf("JSON = %s", b)
	}
}

func TestParseSort(t *testing.T) {
	allowed := []string{"-updatedAt", "name", "-createdAt"}
	for query, want := range map[string]string{"": "-updatedAt", "sort=name": "name", "sort=-createdAt": "-createdAt"} {
		q, _ := url.ParseQuery(query)
		if got, err := ParseSort(q, allowed...); err != nil || got != want {
			t.Errorf("%q: %q, %v; want %q", query, got, err, want)
		}
	}
	for _, bad := range []string{"updatedAt", "-name", "full_name; DROP TABLE x"} {
		if _, err := ParseSort(url.Values{"sort": {bad}}, allowed...); apiErr(t, err).Errors["sort"] == "" {
			t.Errorf("%q aceito", bad)
		}
	}
	defer func() {
		if recover() == nil {
			t.Error("allowlist vazia não deu panic")
		}
	}()
	_, _ = ParseSort(url.Values{})
}
