package keepass

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tobischo/gokeepasslib/v3"
)

// Load reads a KDBX file and returns a map of alias_email → []account_name.
// Only entries whose username contains aliasDomain are included.
func Load(dbPath, password, aliasDomain string) (map[string][]string, error) {
	f, err := os.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open kdbx: %w", err)
	}
	defer f.Close()

	db := gokeepasslib.NewDatabase()
	db.Credentials = gokeepasslib.NewPasswordCredentials(password)

	if err := gokeepasslib.NewDecoder(f).Decode(db); err != nil {
		return nil, fmt.Errorf("decode kdbx: %w", err)
	}
	if err := db.UnlockProtectedEntries(); err != nil {
		return nil, fmt.Errorf("unlock entries: %w", err)
	}

	result := make(map[string][]string)
	recycleBinUUID := db.Content.Meta.RecycleBinUUID
	walkGroups(db.Content.Root.Groups, recycleBinUUID, aliasDomain, result)

	var dups []string
	for alias, accounts := range result {
		seen := make(map[string]bool)
		for _, acc := range accounts {
			if seen[acc] {
				dups = append(dups, fmt.Sprintf("%s → %q", alias, acc))
			}
			seen[acc] = true
		}
	}
	if len(dups) > 0 {
		sort.Strings(dups)
		return nil, fmt.Errorf("duplicate KeePass entries (check Recycle Bin): %s", strings.Join(dups, "; "))
	}

	return result, nil
}

func walkGroups(groups []gokeepasslib.Group, recycleBinUUID gokeepasslib.UUID, aliasDomain string, result map[string][]string) {
	for _, g := range groups {
		if g.UUID == recycleBinUUID {
			continue
		}
		for _, e := range g.Entries {
			username := entryValue(&e, "UserName")
			if !strings.Contains(username, aliasDomain) {
				continue
			}
			alias := strings.ToLower(username)
			account := accountName(&e)
			result[alias] = append(result[alias], account)
		}
		walkGroups(g.Groups, recycleBinUUID, aliasDomain, result)
	}
}

// accountName returns Title → URL (stripped) → "<unnamed>".
func accountName(e *gokeepasslib.Entry) string {
	title := entryValue(e, "Title")
	if title != "" {
		return title
	}
	u := entryValue(e, "URL")
	if u != "" {
		u = strings.TrimPrefix(u, "https://")
		u = strings.TrimPrefix(u, "http://")
		u = strings.TrimRight(u, "/")
		return u
	}
	return "<unnamed>"
}

func entryValue(e *gokeepasslib.Entry, key string) string {
	for _, v := range e.Values {
		if v.Key == key {
			return v.Value.Content
		}
	}
	return ""
}
