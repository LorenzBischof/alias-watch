package keepass

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tobischo/gokeepasslib/v3"
)

const testDomain = "anonaddy.com"
const testPassword = "test-password"

// createTestDB writes a KDBX database to a temp file and returns its path.
func createTestDB(t *testing.T, entries []testEntry) string {
	t.Helper()

	db := gokeepasslib.NewDatabase()
	db.Credentials = gokeepasslib.NewPasswordCredentials(testPassword)

	root := &db.Content.Root.Groups[0]
	for _, e := range entries {
		root.Entries = append(root.Entries, makeEntry(e))
	}

	path := filepath.Join(t.TempDir(), "test.kdbx")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create kdbx file: %v", err)
	}
	defer f.Close()

	enc := gokeepasslib.NewEncoder(f)
	if err := enc.Encode(db); err != nil {
		t.Fatalf("encode kdbx: %v", err)
	}
	return path
}

type testEntry struct {
	title    string
	url      string
	username string
}

func makeEntry(e testEntry) gokeepasslib.Entry {
	entry := gokeepasslib.NewEntry()
	entry.Values = []gokeepasslib.ValueData{
		{Key: "Title", Value: gokeepasslib.V{Content: e.title}},
		{Key: "URL", Value: gokeepasslib.V{Content: e.url}},
		{Key: "UserName", Value: gokeepasslib.V{Content: e.username}},
	}
	return entry
}

func TestLoad_BasicMapping(t *testing.T) {
	path := createTestDB(t, []testEntry{
		{title: "GitHub", username: "abc@user.anonaddy.com"},
	})

	result, err := Load(path, testPassword, testDomain)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	accounts, ok := result["abc@user.anonaddy.com"]
	if !ok {
		t.Fatal("alias not found in result")
	}
	if len(accounts) != 1 || accounts[0] != "GitHub" {
		t.Errorf("want [GitHub], got %v", accounts)
	}
}

func TestLoad_TitlePriority(t *testing.T) {
	path := createTestDB(t, []testEntry{
		{title: "Amazon", url: "https://amazon.com", username: "xyz@user.anonaddy.com"},
	})

	result, err := Load(path, testPassword, testDomain)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	accounts := result["xyz@user.anonaddy.com"]
	if len(accounts) != 1 || accounts[0] != "Amazon" {
		t.Errorf("want [Amazon] (title priority), got %v", accounts)
	}
}

func TestLoad_URLFallback(t *testing.T) {
	path := createTestDB(t, []testEntry{
		{title: "", url: "https://example.com/", username: "xyz@user.anonaddy.com"},
	})

	result, err := Load(path, testPassword, testDomain)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	accounts := result["xyz@user.anonaddy.com"]
	if len(accounts) != 1 || accounts[0] != "example.com" {
		t.Errorf("want [example.com], got %v", accounts)
	}
}

func TestLoad_MultipleAccounts(t *testing.T) {
	path := createTestDB(t, []testEntry{
		{title: "GitHub", username: "abc@user.anonaddy.com"},
		{title: "GitHub Enterprise", username: "abc@user.anonaddy.com"},
	})

	result, err := Load(path, testPassword, testDomain)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	accounts := result["abc@user.anonaddy.com"]
	if len(accounts) != 2 {
		t.Fatalf("want 2 accounts, got %d: %v", len(accounts), accounts)
	}
}

func TestLoad_WrongPassword(t *testing.T) {
	path := createTestDB(t, []testEntry{
		{title: "GitHub", username: "abc@user.anonaddy.com"},
	})

	_, err := Load(path, "wrong-password", testDomain)
	if err == nil {
		t.Error("expected error for wrong password")
	}
}

func TestLoad_IgnoresNonAliasEntries(t *testing.T) {
	path := createTestDB(t, []testEntry{
		{title: "GitHub", username: "abc@user.anonaddy.com"},
		{title: "LocalApp", username: "localuser"},
		{title: "Other", username: "user@gmail.com"},
	})

	result, err := Load(path, testPassword, testDomain)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("want 1 entry, got %d: %v", len(result), result)
	}
	if _, ok := result["abc@user.anonaddy.com"]; !ok {
		t.Error("expected abc@user.anonaddy.com in result")
	}
}
