package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
	"github.com/DotNaos/moodle-services/pkg/codexstate"
	svc "github.com/DotNaos/moodle-services/pkg/moodleservices"
)

const internalWebSecretEnv = "MOODLE_WEB_INTERNAL_SECRET"

func AuthQrExchange(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("bridge") != "" {
		mobileBridge(w, r)
		return
	}
	if r.URL.Query().Get("codex") == "state" {
		clerkUserID, ok := authorizeInternalRequest(w, r, true)
		if ok {
			codexstate.Handle(w, r, clerkUserID)
		}
		return
	}
	if r.URL.Query().Get("codex") == "admin" {
		clerkUserID, ok := authorizeInternalRequest(w, r, true)
		if ok {
			codexstate.HandleAdmin(w, r, clerkUserID)
		}
		return
	}
	if !svc.AllowMethods(w, r, http.MethodPost) {
		return
	}
	switch r.URL.Query().Get("clerk") {
	case "1":
		authClerkQRExchange(w, r)
		return
	case "login":
		authClerkCredentialLogin(w, r)
		return
	case "session":
		authClerkSession(w, r)
		return
	}
	var input contract.QRExchangeRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	exchangeAndPersistQR(w, r, input, "")
}

func authClerkQRExchange(w http.ResponseWriter, r *http.Request) {
	expectedSecret := strings.TrimSpace(os.Getenv(internalWebSecretEnv))
	if expectedSecret == "" {
		svc.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": internalWebSecretEnv + " is not configured"})
		return
	}
	providedSecret := strings.TrimSpace(r.Header.Get("X-Moodle-Internal-Secret"))
	if !svc.ConstantTimeEqual(providedSecret, expectedSecret) {
		svc.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}
	clerkUserID := strings.TrimSpace(r.Header.Get("X-Clerk-User-Id"))
	if clerkUserID == "" {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing Clerk user id"})
		return
	}
	var input contract.QRExchangeRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	exchangeAndPersistQR(w, r, input, clerkUserID)
}

func authClerkCredentialLogin(w http.ResponseWriter, r *http.Request) {
	clerkUserID, ok := authorizeInternalRequest(w, r, true)
	if !ok {
		return
	}
	var input contract.CredentialLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	username := strings.TrimSpace(input.Username)
	if username == "" || strings.TrimSpace(input.Password) == "" {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
		return
	}

	session, siteInfo, err := svc.MobileSessionFromCredentials(r.Context(), svc.WebexCredentials{
		Username: username,
		Password: input.Password,
	})
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	response, err := persistMobileSession(r, session, siteInfo.UserName, clerkUserID, "Web credential login key")
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	svc.WriteJSON(w, http.StatusOK, response)
}

func authClerkSession(w http.ResponseWriter, r *http.Request) {
	clerkUserID, ok := authorizeInternalRequest(w, r, true)
	if !ok {
		return
	}
	cfg := svc.LoadServerEnv()
	st, err := svc.OpenStoreFromEnv(cfg)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	defer st.Close()
	user, err := st.UserForClerkID(r.Context(), clerkUserID)
	if errors.Is(err, svc.ErrNotFound) {
		writeMoodleNotConnected(w)
		return
	}
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	if _, err := st.MoodleCredentialsForUserID(r.Context(), user.ID); err != nil {
		writeMoodleNotConnected(w)
		return
	}
	apiKey, err := svc.GenerateAPIKey()
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	record, err := st.CreateAPIKey(r.Context(), user.ID, "Web restored session key", apiKey, cfg.HashSecret, []string{"moodle:read", "pdf:read", "calendar:read"})
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	svc.WriteJSON(w, http.StatusOK, contract.QRExchangeResponse{User: user, APIKey: apiKey, APIKeyRecord: record})
}

func writeMoodleNotConnected(w http.ResponseWriter) {
	svc.WriteJSON(w, http.StatusConflict, map[string]string{
		"code":  "moodle_not_connected",
		"error": "Connect Moodle before creating a web session.",
	})
}

func exchangeAndPersistQR(w http.ResponseWriter, r *http.Request, input contract.QRExchangeRequest, clerkUserID string) {
	link, err := svc.ParseMobileQRLink(input.QR)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	token, err := svc.ExchangeMobileQRToken(link)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	session := svc.MobileSessionFromToken(token)
	session.SchoolID = svc.ActiveSchoolID
	response, err := persistMobileSession(r, session, input.Name, clerkUserID, "Initial API key")
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	svc.WriteJSON(w, http.StatusOK, response)
}

func persistMobileSession(r *http.Request, session svc.MobileSession, displayNameFallback string, clerkUserID string, keyName string) (contract.QRExchangeResponse, error) {
	client, err := svc.NewMobileClient(session, session.ResolvedSchoolID())
	if err != nil {
		return contract.QRExchangeResponse{}, err
	}
	siteInfo, err := client.FetchMobileSiteInfo()
	if err != nil {
		return contract.QRExchangeResponse{}, err
	}
	sessionData, err := json.Marshal(session)
	if err != nil {
		return contract.QRExchangeResponse{}, err
	}
	cfg := svc.LoadServerEnv()
	box, err := svc.EncryptionBox(cfg)
	if err != nil {
		return contract.QRExchangeResponse{}, err
	}
	encryptedSession, err := box.EncryptString(string(sessionData))
	if err != nil {
		return contract.QRExchangeResponse{}, err
	}
	st, err := svc.OpenStoreFromEnv(cfg)
	if err != nil {
		return contract.QRExchangeResponse{}, err
	}
	defer st.Close()
	displayName := strings.TrimSpace(siteInfo.UserName)
	if displayName == "" {
		displayName = strings.TrimSpace(displayNameFallback)
	}
	user, err := st.UpsertMoodleAccount(r.Context(), svc.UpsertMoodleAccountInput{
		SiteURL:                    session.SiteURL,
		MoodleUserID:               session.UserID,
		DisplayName:                displayName,
		ClerkUserID:                clerkUserID,
		SchoolID:                   session.SchoolID,
		EncryptedMobileSessionJSON: encryptedSession,
	})
	if err != nil {
		return contract.QRExchangeResponse{}, err
	}
	apiKey, err := svc.GenerateAPIKey()
	if err != nil {
		return contract.QRExchangeResponse{}, err
	}
	record, err := st.CreateAPIKey(r.Context(), user.ID, keyName, apiKey, cfg.HashSecret, []string{"moodle:read", "pdf:read", "calendar:read"})
	if err != nil {
		return contract.QRExchangeResponse{}, err
	}
	return contract.QRExchangeResponse{User: user, APIKey: apiKey, APIKeyRecord: record}, nil
}

const mobileBridgeTTL = 10 * time.Minute

type mobileBridgeStartInput struct {
	Origin   string `json:"origin"`
	Endpoint string `json:"endpoint"`
	AppName  string `json:"appName"`
}

type mobileBridgeCompleteInput struct {
	Challenge         string `json:"challenge"`
	PairID            string `json:"pairId"`
	State             string `json:"state"`
	Origin            string `json:"origin"`
	MoodleSiteURL     string `json:"moodleSiteUrl"`
	MoodleUserID      int    `json:"moodleUserId"`
	MoodleMobileToken string `json:"moodleMobileToken"`
	Source            string `json:"source"`
}

func mobileBridge(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Query().Get("bridge") {
	case "start":
		mobileBridgeStart(w, r)
	case "status":
		mobileBridgeStatus(w, r)
	case "complete":
		mobileBridgeComplete(w, r)
	default:
		svc.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "unknown mobile bridge route"})
	}
}

func mobileBridgeStart(w http.ResponseWriter, r *http.Request) {
	if !svc.AllowMethods(w, r, http.MethodPost) {
		return
	}
	clerkUserID, ok := authorizeInternalRequest(w, r, true)
	if !ok {
		return
	}
	var input mobileBridgeStartInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	origin, endpoint, ok := validateBridgeTarget(w, input.Origin, input.Endpoint)
	if !ok {
		return
	}
	challenge, err := randomToken("moodle_bridge_")
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	state, err := randomToken("moodle_bridge_state_")
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	appName := strings.TrimSpace(input.AppName)
	if appName == "" {
		appName = "Moodle Web"
	}
	cfg := svc.LoadServerEnv()
	store, err := svc.OpenStoreFromEnv(cfg)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	defer store.Close()
	expiresAt := time.Now().Add(mobileBridgeTTL)
	if _, err := store.CreateMobileBridgeRequest(r.Context(), svc.CreateMobileBridgeRequestInput{
		Challenge:   challenge,
		ClerkUserID: clerkUserID,
		Origin:      origin,
		Endpoint:    endpoint,
		AppName:     appName,
		State:       state,
		ExpiresAt:   expiresAt,
		HashSecret:  cfg.HashSecret,
	}); err != nil {
		svc.WriteError(w, err)
		return
	}
	bridgeURL := buildBridgeURL(origin, endpoint, challenge, state, appName)
	svc.WriteJSON(w, http.StatusOK, map[string]any{
		"bridgeUrl": bridgeURL,
		"challenge": challenge,
		"state":     state,
		"expiresAt": expiresAt.Format(time.RFC3339),
	})
}

func mobileBridgeStatus(w http.ResponseWriter, r *http.Request) {
	if !svc.AllowMethods(w, r, http.MethodGet) {
		return
	}
	clerkUserID, ok := authorizeInternalRequest(w, r, true)
	if !ok {
		return
	}
	challenge := strings.TrimSpace(r.URL.Query().Get("challenge"))
	if challenge == "" {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "challenge is required"})
		return
	}
	cfg := svc.LoadServerEnv()
	store, err := svc.OpenStoreFromEnv(cfg)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	defer store.Close()
	request, err := store.MobileBridgeRequest(r.Context(), challenge, cfg.HashSecret)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	if request.ClerkUserID != clerkUserID {
		svc.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}
	if time.Now().After(request.ExpiresAt) && request.CompletedAt == nil {
		svc.WriteJSON(w, http.StatusGone, map[string]string{"status": "expired"})
		return
	}
	if request.CompletedAt == nil {
		svc.WriteJSON(w, http.StatusOK, map[string]any{"status": "pending"})
		return
	}
	box, err := svc.EncryptionBox(cfg)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	apiKey, err := box.DecryptString(request.EncryptedAPIKey)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	svc.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "connected",
		"userId": request.UserID,
		"apiKey": apiKey,
	})
}

func mobileBridgeComplete(w http.ResponseWriter, r *http.Request) {
	if !svc.AllowMethods(w, r, http.MethodPost) {
		return
	}
	if _, ok := authorizeInternalRequest(w, r, false); !ok {
		return
	}
	var input mobileBridgeCompleteInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	challenge := strings.TrimSpace(input.Challenge)
	if challenge == "" {
		challenge = strings.TrimSpace(input.PairID)
	}
	if challenge == "" || strings.TrimSpace(input.MoodleSiteURL) == "" || input.MoodleUserID == 0 || strings.TrimSpace(input.MoodleMobileToken) == "" {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "bridge completion is incomplete"})
		return
	}
	cfg := svc.LoadServerEnv()
	store, err := svc.OpenStoreFromEnv(cfg)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	defer store.Close()
	request, err := store.MobileBridgeRequest(r.Context(), challenge, cfg.HashSecret)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	if request.CompletedAt != nil {
		svc.WriteJSON(w, http.StatusConflict, map[string]string{"error": "bridge request is already completed"})
		return
	}
	if time.Now().After(request.ExpiresAt) {
		svc.WriteJSON(w, http.StatusGone, map[string]string{"error": "bridge request expired"})
		return
	}
	if strings.TrimSpace(input.Origin) != request.Origin {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "bridge origin mismatch"})
		return
	}
	session := svc.MobileSession{
		SchoolID:  svc.ActiveSchoolID,
		SiteURL:   strings.TrimSpace(input.MoodleSiteURL),
		UserID:    input.MoodleUserID,
		Token:     strings.TrimSpace(input.MoodleMobileToken),
		CreatedAt: time.Now(),
	}
	client, err := svc.NewMobileClient(session, session.ResolvedSchoolID())
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	siteInfo, err := client.FetchMobileSiteInfo()
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	sessionData, err := json.Marshal(session)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	box, err := svc.EncryptionBox(cfg)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	encryptedSession, err := box.EncryptString(string(sessionData))
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	displayName := strings.TrimSpace(siteInfo.UserName)
	user, err := store.UpsertMoodleAccount(r.Context(), svc.UpsertMoodleAccountInput{
		SiteURL:                    session.SiteURL,
		MoodleUserID:               session.UserID,
		DisplayName:                displayName,
		ClerkUserID:                request.ClerkUserID,
		SchoolID:                   session.SchoolID,
		EncryptedMobileSessionJSON: encryptedSession,
	})
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	apiKey, err := svc.GenerateAPIKey()
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	if _, err := store.CreateAPIKey(r.Context(), user.ID, "Web bridge key", apiKey, cfg.HashSecret, []string{"moodle:read", "pdf:read", "calendar:read"}); err != nil {
		svc.WriteError(w, err)
		return
	}
	encryptedAPIKey, err := box.EncryptString(apiKey)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	if err := store.CompleteMobileBridgeRequest(r.Context(), challenge, cfg.HashSecret, user.ID, encryptedAPIKey); err != nil {
		svc.WriteError(w, err)
		return
	}
	svc.WriteJSON(w, http.StatusOK, map[string]any{"status": "connected", "user": user})
}

func authorizeInternalRequest(w http.ResponseWriter, r *http.Request, requireClerkUser bool) (string, bool) {
	expectedSecret := strings.TrimSpace(os.Getenv(internalWebSecretEnv))
	if expectedSecret == "" {
		svc.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": internalWebSecretEnv + " is not configured"})
		return "", false
	}
	providedSecret := strings.TrimSpace(r.Header.Get("X-Moodle-Internal-Secret"))
	if !svc.ConstantTimeEqual(providedSecret, expectedSecret) {
		svc.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return "", false
	}
	clerkUserID := strings.TrimSpace(r.Header.Get("X-Clerk-User-Id"))
	if requireClerkUser && clerkUserID == "" {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing Clerk user id"})
		return "", false
	}
	return clerkUserID, true
}

func validateBridgeTarget(w http.ResponseWriter, rawOrigin string, rawEndpoint string) (string, string, bool) {
	originURL, err := url.Parse(strings.TrimSpace(rawOrigin))
	if err != nil || !isSafeBridgeURL(originURL) {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "origin must be HTTPS or localhost"})
		return "", "", false
	}
	endpointURL, err := url.Parse(strings.TrimSpace(rawEndpoint))
	if err != nil || !isSafeBridgeURL(endpointURL) {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "endpoint must be HTTPS or localhost"})
		return "", "", false
	}
	if endpointURL.Scheme != originURL.Scheme || endpointURL.Host != originURL.Host {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "endpoint must match origin"})
		return "", "", false
	}
	return originURL.Scheme + "://" + originURL.Host, endpointURL.String(), true
}

func isSafeBridgeURL(candidate *url.URL) bool {
	if candidate == nil || candidate.Host == "" {
		return false
	}
	if candidate.Scheme == "https" {
		return true
	}
	if candidate.Scheme != "http" {
		return false
	}
	host := candidate.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func buildBridgeURL(origin string, endpoint string, challenge string, state string, appName string) string {
	values := url.Values{}
	values.Set("origin", origin)
	values.Set("endpoint", endpoint)
	values.Set("challenge", challenge)
	values.Set("state", state)
	values.Set("app", appName)
	return "moodleauth://bridge?" + values.Encode()
}
