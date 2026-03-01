package aliasescsv

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	input := strings.NewReader(`"id","email","active","description"
"id-1","alias1@example.com","TRUE","Alias 1"
"id-2","alias2@example.com","",""
"id-3","","TRUE","skip row"
`)

	aliases, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if len(aliases) != 2 {
		t.Fatalf("expected 2 aliases, got %d", len(aliases))
	}

	if aliases[0].ID != "id-1" || aliases[0].Email != "alias1@example.com" || !aliases[0].Active || aliases[0].Description != "Alias 1" {
		t.Fatalf("unexpected first alias: %+v", aliases[0])
	}
	if aliases[1].ID != "id-2" || aliases[1].Email != "alias2@example.com" || aliases[1].Active || aliases[1].Description != "" {
		t.Fatalf("unexpected second alias: %+v", aliases[1])
	}
}

func TestParse_MissingHeaders(t *testing.T) {
	_, err := Parse(strings.NewReader(`"id","active"
"id-1","TRUE"
`))
	if err == nil || !strings.Contains(err.Error(), "missing required header: email") {
		t.Fatalf("expected missing email header error, got: %v", err)
	}
}

func TestParse_InvalidActive(t *testing.T) {
	_, err := Parse(strings.NewReader(`"id","email","active"
"id-1","alias@example.com","MAYBE"
`))
	if err == nil || !strings.Contains(err.Error(), "invalid boolean") {
		t.Fatalf("expected invalid boolean error, got: %v", err)
	}
}
