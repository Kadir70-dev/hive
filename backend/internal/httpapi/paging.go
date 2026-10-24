package httpapi

import (
	"net/http"
	"strconv"
	"strings"
)

// page holds pagination parameters: ?page=1-based &page_size= (capped) &q=search.
type page struct {
	Number, Size int
	Query        string
}

func (p page) offset() int { return (p.Number - 1) * p.Size }

func pageParams(r *http.Request, defaultSize, maxSize int) page {
	p := page{Number: 1, Size: defaultSize}
	if n, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && n > 0 && n <= 10000 {
		p.Number = n
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil && n > 0 {
		p.Size = min(n, maxSize)
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > 100 {
		q = q[:100]
	}
	p.Query = q
	return p
}

// likePattern escapes LIKE wildcards in user search input.
func likePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(q) + "%"
}

func paged(items []any, p page, total int) map[string]any {
	return map[string]any{"items": items, "page": p.Number, "page_size": p.Size, "total": total,
		"has_more": p.offset()+len(items) < total}
}

// validText rejects control characters other than newlines/tabs in user text.
func validText(s string, max int, multiline bool) bool {
	if len(s) > max {
		return false
	}
	for _, c := range s {
		if c == '\n' || c == '\t' || c == '\r' {
			if !multiline && c != '\t' {
				return false
			}
			continue
		}
		if c < 0x20 || c == 0x7f {
			return false
		}
	}
	return true
}
