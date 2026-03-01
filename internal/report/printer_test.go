package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/lorenzbischof/alias-watch/internal/db"
)

func TestReport_SortOrder(t *testing.T) {
	aliases := []db.Alias{
		{Email: "xyz@user.anonaddy.com", Active: true},
		{Email: "abc@user.anonaddy.com", Active: true},
		{Email: "many@user.anonaddy.com", Active: true},
	}
	senders := map[string][]string{
		"abc@user.anonaddy.com":  {"noreply@github.com"},
		"xyz@user.anonaddy.com":  {},
		"many@user.anonaddy.com": {"a@foo.com", "b@foo.com"},
	}

	var buf bytes.Buffer
	Print(&buf, aliases, senders)
	out := buf.String()

	// many (2 senders) first, abc (1 sender) second, xyz (0 senders) last
	manyIdx := strings.Index(out, "many@")
	abcIdx := strings.Index(out, "abc@")
	xyzIdx := strings.Index(out, "xyz@")
	if manyIdx == -1 || abcIdx == -1 || xyzIdx == -1 {
		t.Fatalf("missing alias in output:\n%s", out)
	}
	if manyIdx > abcIdx {
		t.Errorf("expected many before abc in output:\n%s", out)
	}
	if abcIdx > xyzIdx {
		t.Errorf("expected abc before xyz in output:\n%s", out)
	}
}

func TestReport_MultipleSenders(t *testing.T) {
	aliases := []db.Alias{
		{Email: "abc@user.anonaddy.com", Active: true},
	}
	senders := map[string][]string{
		"abc@user.anonaddy.com": {"sender1@foo.com", "sender2@bar.com"},
	}

	var buf bytes.Buffer
	Print(&buf, aliases, senders)
	out := buf.String()

	if !strings.Contains(out, "sender1@foo.com") || !strings.Contains(out, "sender2@bar.com") {
		t.Errorf("missing senders in output:\n%s", out)
	}
}

func TestReport_Aligned(t *testing.T) {
	aliases := []db.Alias{
		{Email: "short@user.anonaddy.com", Active: true},
		{Email: "a-much-longer-alias@user.anonaddy.com", Active: true},
	}
	senders := map[string][]string{
		"short@user.anonaddy.com":               {"s1@foo.com"},
		"a-much-longer-alias@user.anonaddy.com": {"s2@bar.com"},
	}

	var buf bytes.Buffer
	Print(&buf, aliases, senders)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d:\n%s", len(lines), buf.String())
	}

	col0 := strings.Index(lines[0], "->")
	col1 := strings.Index(lines[1], "->")
	if col0 != col1 {
		t.Errorf("-> not aligned: line0 col %d, line1 col %d\n%s\n%s", col0, col1, lines[0], lines[1])
	}
}

func TestReport_NoSenders(t *testing.T) {
	aliases := []db.Alias{
		{Email: "abc@user.anonaddy.com", Active: true},
	}
	senders := map[string][]string{}

	var buf bytes.Buffer
	Print(&buf, aliases, senders)
	out := buf.String()

	if !strings.Contains(out, "(none)") {
		t.Errorf("expected (none) for alias with no senders:\n%s", out)
	}
}
