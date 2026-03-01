package imap

import (
	"bufio"
	"bytes"
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
func ExtractAliasEmail(headers map[string][]string) string {
	for _, key := range []string{"To", "Delivered-To", "X-Original-To", "Envelope-To", "X-Envelope-To"} {
		if vals, ok := headers[key]; ok {
			for _, val := range vals {
				addrs, err := mail.ParseAddressList(val)
				if err != nil {
					// Try single address
					addr, err2 := mail.ParseAddress(val)
					if err2 == nil {
						return normalizeAliasEmail(addr.Address)
					}
					continue
				}
				for _, addr := range addrs {
					return normalizeAliasEmail(addr.Address)
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

func normalizeAliasEmail(addr string) string {
	addr = strings.ToLower(strings.TrimSpace(addr))
	at := strings.LastIndex(addr, "@")
	if at < 1 {
		return addr
	}
	local := addr[:at]
	domain := addr[at+1:]
	if idx := strings.Index(local, "+sender="); idx >= 0 {
		local = local[:idx]
		return local + "@" + domain
	}
	return addr
}

// parseHeadersInto parses raw RFC2822 headers into the map.
func parseHeadersInto(raw []byte, out map[string][]string) {
	msg, err := mail.ReadMessage(bufio.NewReader(bytes.NewReader(raw)))
	if err != nil {
		return
	}
	for key, vals := range msg.Header {
		canonical := canonicalHeader(key)
		out[canonical] = append(out[canonical], vals...)
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
