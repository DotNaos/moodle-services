package moodleservice

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type storedCookie struct {
	domain string
	path   string
	name   string
	value  string
}

type simpleCookieJar struct {
	cookies map[string]storedCookie
}

type webexCookieSnapshot struct {
	Cookies []webexSnapshotCookie `json:"cookies"`
}

type webexSnapshotCookie struct {
	Domain string `json:"domain"`
	Path   string `json:"path"`
	Name   string `json:"name"`
	Value  string `json:"value"`
}

func newSimpleCookieJar() *simpleCookieJar {
	return &simpleCookieJar{cookies: map[string]storedCookie{}}
}

func (j *simpleCookieJar) storeFromResponse(rawURL string, response *http.Response) {
	for _, value := range response.Header.Values("Set-Cookie") {
		j.store(rawURL, value)
	}
}

func (j *simpleCookieJar) store(rawURL string, setCookie string) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	parts := strings.Split(setCookie, ";")
	if len(parts) == 0 {
		return
	}
	nameValue := strings.TrimSpace(parts[0])
	separator := strings.Index(nameValue, "=")
	if separator <= 0 {
		return
	}
	name := strings.TrimSpace(nameValue[:separator])
	value := strings.TrimSpace(nameValue[separator+1:])
	domain := strings.ToLower(parsed.Hostname())
	path := "/"
	expires := ""
	for _, rawPart := range parts[1:] {
		key, attrValue, ok := strings.Cut(strings.TrimSpace(rawPart), "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "domain":
			domain = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(attrValue)), ".")
		case "path":
			path = strings.TrimSpace(attrValue)
		case "expires":
			expires = strings.TrimSpace(attrValue)
		}
	}
	key := domain + "|" + path + "|" + name
	if value == "" || isExpiredCookie(expires) {
		delete(j.cookies, key)
		return
	}
	j.cookies[key] = storedCookie{domain: domain, path: path, name: name, value: value}
}

func (j *simpleCookieJar) header(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	pairs := []string{}
	for _, cookie := range j.cookies {
		if (host == cookie.domain || strings.HasSuffix(host, "."+cookie.domain)) && strings.HasPrefix(path, cookie.path) {
			pairs = append(pairs, cookie.name+"="+cookie.value)
		}
	}
	return strings.Join(pairs, "; ")
}

func (j *simpleCookieJar) exportJSON() (string, error) {
	keys := make([]string, 0, len(j.cookies))
	for key := range j.cookies {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	snapshot := webexCookieSnapshot{Cookies: make([]webexSnapshotCookie, 0, len(keys))}
	for _, key := range keys {
		cookie := j.cookies[key]
		snapshot.Cookies = append(snapshot.Cookies, webexSnapshotCookie{
			Domain: cookie.domain,
			Path:   cookie.path,
			Name:   cookie.name,
			Value:  cookie.value,
		})
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (j *simpleCookieJar) importJSON(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var snapshot webexCookieSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return err
	}
	j.cookies = map[string]storedCookie{}
	for _, cookie := range snapshot.Cookies {
		if strings.TrimSpace(cookie.Domain) == "" || strings.TrimSpace(cookie.Name) == "" {
			continue
		}
		path := strings.TrimSpace(cookie.Path)
		if path == "" {
			path = "/"
		}
		stored := storedCookie{
			domain: strings.ToLower(strings.TrimSpace(cookie.Domain)),
			path:   path,
			name:   strings.TrimSpace(cookie.Name),
			value:  cookie.Value,
		}
		j.cookies[stored.domain+"|"+stored.path+"|"+stored.name] = stored
	}
	return nil
}

func isExpiredCookie(value string) bool {
	if value == "" {
		return false
	}
	expiresAt, err := time.Parse(time.RFC1123, value)
	return err == nil && expiresAt.Before(time.Now())
}
