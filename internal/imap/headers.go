package imap

import (
	"net/mail"
	"strings"
)

// ExtractSenderEmail extracts the first From address from the X-AnonAddy-Original-Sender
// header, falling back to the From header.
func ExtractSenderEmail(headers map[string][]string) string {
	// Try X-AnonAddy-Original-Sender first
	if vals, ok := headers["X-Anonaddy-Original-Sender"]; ok && len(vals) > 0 {
		addr := parseFirstAddress(vals[0])
		if addr != "" {
			return strings.ToLower(addr)
		}
	}
	// Fallback to From
	if vals, ok := headers["From"]; ok && len(vals) > 0 {
		addr := parseFirstAddress(vals[0])
		if addr != "" {
			return strings.ToLower(addr)
		}
	}
	return ""
}

// ExtractAliasEmail extracts the alias email from To or Delivered-To headers.
func ExtractAliasEmail(headers map[string][]string, aliasDomain string) string {
	for _, key := range []string{"To", "Delivered-To"} {
		if vals, ok := headers[key]; ok {
			for _, val := range vals {
				addrs, err := mail.ParseAddressList(val)
				if err != nil {
					// Try single address
					addr, err2 := mail.ParseAddress(val)
					if err2 == nil && strings.Contains(addr.Address, aliasDomain) {
						return strings.ToLower(addr.Address)
					}
					continue
				}
				for _, addr := range addrs {
					if strings.Contains(addr.Address, aliasDomain) {
						return strings.ToLower(addr.Address)
					}
				}
			}
		}
	}
	return ""
}

// DomainFromEmail extracts the domain part from an email address.
func DomainFromEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) == 2 {
		return strings.ToLower(parts[1])
	}
	return ""
}

// parseHeadersInto parses raw RFC2822 headers into the map.
func parseHeadersInto(raw []byte, out map[string][]string) {
	lines := splitHeaderLines(string(raw))
	for _, line := range lines {
		idx := indexOf(line, ':')
		if idx < 0 {
			continue
		}
		key := canonicalHeader(line[:idx])
		val := trimWhitespace(line[idx+1:])
		out[key] = append(out[key], val)
	}
}

func parseFirstAddress(s string) string {
	// Try list first
	addrs, err := mail.ParseAddressList(s)
	if err == nil && len(addrs) > 0 {
		return addrs[0].Address
	}
	// Try single
	addr, err := mail.ParseAddress(s)
	if err == nil {
		return addr.Address
	}
	// Fallback: strip display name manually if present
	s = strings.TrimSpace(s)
	if idx := strings.LastIndex(s, "<"); idx >= 0 {
		end := strings.Index(s[idx:], ">")
		if end >= 0 {
			return s[idx+1 : idx+end]
		}
	}
	return s
}

// splitHeaderLines splits raw header text into logical lines (handling folding).
func splitHeaderLines(raw string) []string {
	var lines []string
	var current string
	for _, line := range splitByNewline(raw) {
		if len(line) == 0 {
			break // end of headers
		}
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			// Folded line
			current += " " + trimWhitespace(line)
		} else {
			if current != "" {
				lines = append(lines, current)
			}
			current = line
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func splitByNewline(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			end := i
			if end > start && s[end-1] == '\r' {
				end--
			}
			lines = append(lines, s[start:end])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func trimWhitespace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r' || s[start] == '\n') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r' || s[end-1] == '\n') {
		end--
	}
	return s[start:end]
}

func canonicalHeader(s string) string {
	result := make([]byte, len(s))
	capNext := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '-' {
			result[i] = c
			capNext = true
		} else if capNext && c >= 'a' && c <= 'z' {
			result[i] = c - 32
			capNext = false
		} else if !capNext && c >= 'A' && c <= 'Z' {
			result[i] = c + 32
		} else {
			result[i] = c
			capNext = false
		}
	}
	return string(result)
}
