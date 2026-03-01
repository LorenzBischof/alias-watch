package report

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/lorenzbischof/alias-watch/internal/db"
)

// Print writes one line per alias showing its known senders:
//
//	alias@example.com  ->  sender1@foo.com, sender2@bar.com
func Print(w io.Writer, aliases []db.Alias, senders map[string][]string) {
	sorted := make([]db.Alias, len(aliases))
	copy(sorted, aliases)
	sort.Slice(sorted, func(i, j int) bool {
		ci := len(senders[sorted[i].Email])
		cj := len(senders[sorted[j].Email])
		if ci != cj {
			return ci > cj
		}
		return sorted[i].Email < sorted[j].Email
	})

	maxLen := 0
	for _, a := range sorted {
		if len(a.Email) > maxLen {
			maxLen = len(a.Email)
		}
	}

	for _, a := range sorted {
		list := senders[a.Email]
		var rhs string
		if len(list) == 0 {
			rhs = "(none)"
		} else {
			rhs = strings.Join(list, ", ")
		}
		fmt.Fprintf(w, "%-*s  ->  %s\n", maxLen, a.Email, rhs)
	}
}
