package aliasescsv

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

type Alias struct {
	ID          string
	Email       string
	Active      bool
	Description string
}

// Parse reads an addy.io aliases CSV export and returns alias rows.
func Parse(r io.Reader) ([]Alias, error) {
	rd := csv.NewReader(r)

	header, err := rd.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	index := make(map[string]int, len(header))
	for i, name := range header {
		index[strings.ToLower(strings.TrimSpace(name))] = i
	}

	idIdx, ok := index["id"]
	if !ok {
		return nil, fmt.Errorf("missing required header: id")
	}
	emailIdx, ok := index["email"]
	if !ok {
		return nil, fmt.Errorf("missing required header: email")
	}
	activeIdx, ok := index["active"]
	if !ok {
		return nil, fmt.Errorf("missing required header: active")
	}
	descriptionIdx, hasDescription := index["description"]

	var out []Alias
	for {
		row, err := rd.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read row: %w", err)
		}

		email := get(row, emailIdx)
		if email == "" {
			continue
		}

		active, err := parseCSVBool(get(row, activeIdx))
		if err != nil {
			return nil, fmt.Errorf("parse active for %q: %w", email, err)
		}

		a := Alias{
			ID:     get(row, idIdx),
			Email:  strings.ToLower(email),
			Active: active,
		}
		if hasDescription {
			a.Description = get(row, descriptionIdx)
		}
		out = append(out, a)
	}

	return out, nil
}

func get(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func parseCSVBool(v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "y":
		return true, nil
	case "", "0", "false", "no", "n":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q", v)
	}
}
