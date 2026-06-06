package moodleservice

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	fhgrIDP        = "https://aai-login.fhgr.ch/idp/shibboleth"
	webexLTIOrigin = "https://lti.webex.com"
	webexAppURL    = webexLTIOrigin + "/application"
	webexSite      = "fhgr.webex.com"
	webexSiteID    = "14682867"
	webexUserAgent = "Mozilla/5.0 StudyReplay"
)

type browserPage struct {
	status  int
	url     string
	headers http.Header
	text    string
}

type webBrowser struct {
	client *http.Client
	jar    *simpleCookieJar
}

type formField struct {
	name  string
	value string
	kind  string
}

type pageForm struct {
	action string
	method string
	id     string
	name   string
	fields []formField
}

func newWebBrowser() *webBrowser {
	return &webBrowser{
		client: &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}},
		jar: newSimpleCookieJar(),
	}
}

func (b *webBrowser) loginFHGR(ctx context.Context, siteURL string, targetPath string, credentials WebexCredentials) (browserPage, error) {
	siteURL = strings.TrimRight(siteURL, "/")
	targetURL := absoluteURL(siteURL, targetPath)
	callback, _ := url.Parse(siteURL + "/auth/shibboleth/index.php")
	values := callback.Query()
	values.Set("wantsurl", targetURL)
	callback.RawQuery = values.Encode()
	loginURL, _ := url.Parse(siteURL + "/Shibboleth.sso/Login")
	query := loginURL.Query()
	query.Set("entityID", fhgrIDP)
	query.Set("target", callback.String())
	loginURL.RawQuery = query.Encode()

	page, err := b.request(ctx, http.MethodGet, loginURL.String(), "", nil, nil)
	if err != nil {
		return browserPage{}, err
	}
	page, err = b.followLogin(ctx, page, credentials, 60)
	if err != nil {
		return browserPage{}, err
	}
	if !strings.Contains(b.jar.header(siteURL), "MoodleSession=") {
		return browserPage{}, fmt.Errorf("Moodle browser login did not create a Moodle session")
	}
	if !strings.Contains(page.url, targetPath) {
		var err error
		page, err = b.request(ctx, http.MethodGet, targetURL, page.url, nil, nil)
		if err != nil {
			return browserPage{}, err
		}
	}
	return page, nil
}

func (b *webBrowser) followLogin(ctx context.Context, page browserPage, credentials WebexCredentials, maxSteps int) (browserPage, error) {
	current := page
	for step := 0; step < maxSteps; step++ {
		if next := redirectURL(current); next != "" {
			var err error
			current, err = b.request(ctx, http.MethodGet, next, current.url, nil, nil)
			if err != nil {
				return browserPage{}, err
			}
			continue
		}
		if next := htmlRedirect(current.text, current.url); next != "" {
			var err error
			current, err = b.request(ctx, http.MethodGet, next, current.url, nil, nil)
			if err != nil {
				return browserPage{}, err
			}
			continue
		}
		form := selectLoginForm(parseForms(current.text, current.url), current.text)
		if form == nil {
			return current, nil
		}
		next, err := b.submitForm(ctx, *form, current.url, buildLoginFormBody(*form, credentials))
		if err != nil {
			return browserPage{}, err
		}
		current = next
	}
	return current, nil
}

func (b *webBrowser) followWebexFlow(ctx context.Context, page browserPage, maxSteps int) (browserPage, error) {
	current := page
	for step := 0; step < maxSteps; step++ {
		if next := redirectURL(current); next != "" {
			var err error
			current, err = b.request(ctx, http.MethodGet, next, current.url, nil, nil)
			if err != nil {
				return browserPage{}, err
			}
			continue
		}
		if next := htmlRedirect(current.text, current.url); next != "" {
			var err error
			current, err = b.request(ctx, http.MethodGet, next, current.url, nil, nil)
			if err != nil {
				return browserPage{}, err
			}
			continue
		}
		if form := firstWebexFlowForm(current.text, current.url); form != nil {
			if isWebexLTILogin(form.action) {
				nextURL, err := formGETURL(*form, true)
				if err != nil {
					return browserPage{}, err
				}
				current, err = b.request(ctx, http.MethodGet, nextURL, current.url, nil, nil)
				if err != nil {
					return browserPage{}, err
				}
				continue
			}
			next, err := b.submitForm(ctx, *form, current.url, formBody(*form))
			if err != nil {
				return browserPage{}, err
			}
			current = next
			continue
		}
		if isWebexLaunch(current.url) && !isWebexApplication(current.url, current.text) {
			var err error
			current, err = b.request(ctx, http.MethodGet, webexAppURL, current.url, nil, nil)
			if err != nil {
				return browserPage{}, err
			}
			continue
		}
		return current, nil
	}
	return current, nil
}

func (b *webBrowser) request(ctx context.Context, method string, rawURL string, referer string, headers map[string]string, body io.Reader) (browserPage, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return browserPage{}, err
	}
	req.Header.Set("User-Agent", webexUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,de;q=0.8")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if cookie := b.jar.header(rawURL); cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return browserPage{}, err
	}
	defer resp.Body.Close()
	b.jar.storeFromResponse(rawURL, resp)
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		return browserPage{}, err
	}
	return browserPage{status: resp.StatusCode, url: rawURL, headers: resp.Header.Clone(), text: string(data)}, nil
}

func (b *webBrowser) submitForm(ctx context.Context, form pageForm, referer string, body url.Values) (browserPage, error) {
	if form.method == http.MethodGet {
		nextURL, err := formGETURL(form, false)
		if err != nil {
			return browserPage{}, err
		}
		return b.request(ctx, http.MethodGet, nextURL, referer, nil, nil)
	}
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	if parsed, err := url.Parse(referer); err == nil {
		headers["Origin"] = parsed.Scheme + "://" + parsed.Host
	}
	return b.request(ctx, http.MethodPost, form.action, referer, headers, strings.NewReader(body.Encode()))
}

func redirectURL(page browserPage) string {
	if page.status != 301 && page.status != 302 && page.status != 303 && page.status != 307 && page.status != 308 {
		return ""
	}
	location := page.headers.Get("Location")
	if location == "" {
		return ""
	}
	return absoluteURL(page.url, location)
}

func absoluteURL(base string, target string) string {
	parsed, err := url.Parse(target)
	if err == nil && parsed.IsAbs() {
		return parsed.String()
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return target
	}
	return baseURL.ResolveReference(parsed).String()
}

func selectLoginForm(forms []pageForm, htmlText string) *pageForm {
	for _, predicate := range []func(pageForm) bool{
		func(form pageForm) bool { return hasFieldType(form, "password") },
		func(form pageForm) bool { return hasFieldName(form, "SAMLResponse") },
		func(form pageForm) bool { return hasFieldName(form, "user_idp") },
		func(form pageForm) bool { return hasFieldName(form, "_eventId_proceed") },
	} {
		for index := range forms {
			if predicate(forms[index]) {
				return &forms[index]
			}
		}
	}
	if regexp.MustCompile(`(?i)submit\(\)`).MatchString(htmlText) {
		for index := range forms {
			if forms[index].id == "autopostme" || forms[index].name == "autopostme" {
				return &forms[index]
			}
		}
	}
	return nil
}

func firstWebexFlowForm(htmlText string, baseURL string) *pageForm {
	forms := parseForms(htmlText, baseURL)
	for index := range forms {
		if isWebexLTILogin(forms[index].action) || isWebexLaunch(forms[index].action) {
			return &forms[index]
		}
	}
	if regexp.MustCompile(`(?i)submit\(\)`).MatchString(htmlText) {
		for index := range forms {
			if forms[index].id == "autopostme" || forms[index].name == "autopostme" {
				return &forms[index]
			}
		}
	}
	return nil
}

func buildLoginFormBody(form pageForm, credentials WebexCredentials) url.Values {
	body := formBody(form)
	if hasFieldType(form, "password") {
		for _, field := range form.fields {
			if regexp.MustCompile(`(?i)j_username|username|user`).MatchString(field.name) {
				body.Set(field.name, credentials.Username)
			}
			if field.kind == "password" {
				body.Set(field.name, credentials.Password)
			}
		}
		body.Set("_eventId_proceed", "Login")
	}
	if hasFieldName(form, "user_idp") {
		body.Set("user_idp", fhgrIDP)
	}
	if hasFieldName(form, "_eventId_proceed") {
		body.Set("_eventId_proceed", "Accept")
	}
	return body
}

func formBody(form pageForm) url.Values {
	body := url.Values{}
	for _, field := range form.fields {
		if field.name != "" && field.kind != "submit" {
			body.Add(field.name, field.value)
		}
	}
	return body
}

func formGETURL(form pageForm, forceWebexNewWindow bool) (string, error) {
	next, err := url.Parse(form.action)
	if err != nil {
		return "", err
	}
	query := next.Query()
	for _, field := range form.fields {
		query.Set(field.name, field.value)
	}
	if forceWebexNewWindow {
		query.Set("lti1p3_new_window", "1")
	}
	next.RawQuery = query.Encode()
	return next.String(), nil
}

func hasFieldType(form pageForm, kind string) bool {
	for _, field := range form.fields {
		if field.kind == kind {
			return true
		}
	}
	return false
}

func hasFieldName(form pageForm, name string) bool {
	for _, field := range form.fields {
		if field.name == name {
			return true
		}
	}
	return false
}

var (
	formRe        = regexp.MustCompile(`(?is)<form\b[\s\S]*?(?:</form>|$)`)
	inputRe       = regexp.MustCompile(`(?is)<input\b[^>]*>`)
	refreshRe     = regexp.MustCompile(`(?is)<meta[^>]+http-equiv=["']?refresh["']?[^>]+content=["'][^"']*url=([^"']+)["']`)
	locationRegex = regexp.MustCompile(`(?is)(?:window\.)?location(?:\.href)?\s*=\s*["']([^"']+)["']`)
	replaceRegex  = regexp.MustCompile(`(?is)(?:window\.)?location\.(?:replace|assign)\(\s*["']([^"']+)["']\s*\)`)
)

func parseForms(htmlText string, baseURL string) []pageForm {
	matches := formRe.FindAllString(htmlText, -1)
	forms := make([]pageForm, 0, len(matches))
	for _, formHTML := range matches {
		action := attr(formHTML, "action")
		if action == "" {
			action = baseURL
		}
		fields := []formField{}
		for _, input := range inputRe.FindAllString(formHTML, -1) {
			name := html.UnescapeString(attr(input, "name"))
			if name == "" {
				continue
			}
			kind := strings.ToLower(html.UnescapeString(attr(input, "type")))
			if kind == "" {
				kind = "text"
			}
			fields = append(fields, formField{name: name, value: html.UnescapeString(attr(input, "value")), kind: kind})
		}
		forms = append(forms, pageForm{
			action: absoluteURL(baseURL, html.UnescapeString(action)),
			method: strings.ToUpper(nonEmpty(attr(formHTML, "method"), http.MethodGet)),
			id:     html.UnescapeString(attr(formHTML, "id")),
			name:   html.UnescapeString(attr(formHTML, "name")),
			fields: fields,
		})
	}
	return forms
}

func htmlRedirect(htmlText string, baseURL string) string {
	for _, pattern := range []*regexp.Regexp{refreshRe, locationRegex, replaceRegex} {
		if match := pattern.FindStringSubmatch(htmlText); len(match) > 1 {
			return absoluteURL(baseURL, html.UnescapeString(strings.TrimSpace(match[1])))
		}
	}
	return ""
}

func attr(source string, name string) string {
	re := regexp.MustCompile(`(?is)` + regexp.QuoteMeta(name) + `\s*=\s*("([^"]*)"|'([^']*)'|([^\s>]+))`)
	match := re.FindStringSubmatch(source)
	if len(match) < 5 {
		return ""
	}
	for _, value := range match[2:] {
		if value != "" {
			return value
		}
	}
	return ""
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func isWebexLTILogin(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	return err == nil && parsed.Scheme+"://"+parsed.Host == webexLTIOrigin && parsed.Path == "/lti/login"
}

func isWebexLaunch(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	return err == nil && parsed.Scheme+"://"+parsed.Host == webexLTIOrigin && strings.Contains(parsed.Path, "/lti/launch")
}

func isWebexApplication(rawURL string, htmlText string) bool {
	parsed, err := url.Parse(rawURL)
	return err == nil && parsed.Scheme+"://"+parsed.Host == webexLTIOrigin && (strings.Contains(parsed.Path, "/application") || strings.Contains(htmlText, "/api/webex/"))
}
