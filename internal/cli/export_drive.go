package cli

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const driveFolderMimeType = "application/vnd.google-apps.folder"

type exportDriveFile struct {
	ID          string `yaml:"id" json:"id"`
	WebViewLink string `yaml:"web_view_link" json:"webViewLink"`
	Name        string `yaml:"name,omitempty" json:"name,omitempty"`
}

type exportDriveUploader interface {
	EnsureFolderPath(ctx context.Context, parts []string) (exportDriveFile, error)
	CreateRunFolder(ctx context.Context, parts []string) (exportDriveFile, error)
	UploadFile(ctx context.Context, path string, folderID string, name string, overwrite bool) (exportDriveFile, error)
	UploadText(ctx context.Context, text string, folderID string, name string, overwrite bool) (exportDriveFile, error)
}

type dryRunExportDriveUploader struct {
	ids          map[string]exportDriveFile
	rootFolderID string
}

func newDryRunExportDriveUploader() *dryRunExportDriveUploader {
	uploader := &dryRunExportDriveUploader{ids: map[string]exportDriveFile{}}
	root := uploader.record(exportDriveRootName)
	uploader.rootFolderID = root.ID
	return uploader
}

func (u *dryRunExportDriveUploader) record(key string) exportDriveFile {
	if file, ok := u.ids[key]; ok {
		return file
	}
	sum := sha1.Sum([]byte(key))
	id := "dryrun-" + hex.EncodeToString(sum[:])[:16]
	file := exportDriveFile{ID: id, WebViewLink: "dry-run://" + id, Name: filepath.Base(key)}
	u.ids[key] = file
	return file
}

func (u *dryRunExportDriveUploader) EnsureFolderPath(ctx context.Context, parts []string) (exportDriveFile, error) {
	return u.record(u.drivePath(parts)), nil
}

func (u *dryRunExportDriveUploader) CreateRunFolder(ctx context.Context, parts []string) (exportDriveFile, error) {
	return u.record(u.drivePath(parts)), nil
}

func (u *dryRunExportDriveUploader) UploadFile(ctx context.Context, path string, folderID string, name string, overwrite bool) (exportDriveFile, error) {
	if name == "" {
		name = filepath.Base(path)
	}
	return u.record(folderID + "/" + name), nil
}

func (u *dryRunExportDriveUploader) UploadText(ctx context.Context, text string, folderID string, name string, overwrite bool) (exportDriveFile, error) {
	return u.record(folderID + "/" + name), nil
}

func (u *dryRunExportDriveUploader) drivePath(parts []string) string {
	if len(parts) == 0 {
		return exportDriveRootName
	}
	return exportDriveRootName + "/" + strings.Join(parts, "/")
}

type googleExportDriveUploader struct {
	httpClient   *http.Client
	accessToken  string
	rootFolderID string
}

type googleServiceAccount struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

type googleAuthorizedUser struct {
	Type         string `json:"type"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
	TokenURI     string `json:"token_uri"`
}

func newGoogleExportDriveUploader(ctx context.Context) (*googleExportDriveUploader, error) {
	token, err := googleDriveAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	uploader := &googleExportDriveUploader{
		httpClient:   &http.Client{Timeout: 5 * time.Minute},
		accessToken:  token,
		rootFolderID: strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_ROOT_FOLDER_ID")),
	}
	if uploader.rootFolderID == "" {
		root, err := uploader.ensureFolder(ctx, "", exportDriveRootName)
		if err != nil {
			return nil, err
		}
		uploader.rootFolderID = root.ID
	}
	return uploader, nil
}

func googleDriveAccessToken(ctx context.Context) (string, error) {
	if raw := strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_OAUTH_CREDENTIALS_JSON")); raw != "" {
		var credentials googleAuthorizedUser
		if err := json.Unmarshal([]byte(raw), &credentials); err != nil {
			return "", fmt.Errorf("GOOGLE_DRIVE_OAUTH_CREDENTIALS_JSON is not valid JSON: %w", err)
		}
		return googleAuthorizedUserAccessToken(ctx, credentials)
	}
	raw := strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_SERVICE_ACCOUNT_JSON"))
	if raw == "" {
		return "", fmt.Errorf("GOOGLE_DRIVE_OAUTH_CREDENTIALS_JSON or GOOGLE_DRIVE_SERVICE_ACCOUNT_JSON is required when --upload is used")
	}
	var account googleServiceAccount
	if err := json.Unmarshal([]byte(raw), &account); err != nil {
		return "", fmt.Errorf("GOOGLE_DRIVE_SERVICE_ACCOUNT_JSON is not valid JSON: %w", err)
	}
	return googleServiceAccountAccessToken(ctx, account)
}

func googleAuthorizedUserAccessToken(ctx context.Context, credentials googleAuthorizedUser) (string, error) {
	if credentials.ClientID == "" || credentials.ClientSecret == "" || credentials.RefreshToken == "" {
		return "", fmt.Errorf("OAuth credentials JSON must include client_id, client_secret, and refresh_token")
	}
	tokenURI := credentials.TokenURI
	if tokenURI == "" {
		tokenURI = "https://oauth2.googleapis.com/token"
	}
	form := url.Values{
		"client_id":     {credentials.ClientID},
		"client_secret": {credentials.ClientSecret},
		"refresh_token": {credentials.RefreshToken},
		"grant_type":    {"refresh_token"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Google OAuth token request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	if payload.AccessToken == "" {
		return "", fmt.Errorf("Google OAuth token response did not include access_token")
	}
	return payload.AccessToken, nil
}

func googleServiceAccountAccessToken(ctx context.Context, account googleServiceAccount) (string, error) {
	if account.ClientEmail == "" || account.PrivateKey == "" {
		return "", fmt.Errorf("service account JSON must include client_email and private_key")
	}
	tokenURI := account.TokenURI
	if tokenURI == "" {
		tokenURI = "https://oauth2.googleapis.com/token"
	}
	key, err := parseServiceAccountPrivateKey(account.PrivateKey)
	if err != nil {
		return "", err
	}
	now := time.Now().Unix()
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	claims := map[string]any{
		"iss":   account.ClientEmail,
		"scope": "https://www.googleapis.com/auth/drive",
		"aud":   tokenURI,
		"iat":   now,
		"exp":   now + 3600,
	}
	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)
	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	assertion := signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Google token request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	if payload.AccessToken == "" {
		return "", fmt.Errorf("Google token response did not include access_token")
	}
	return payload.AccessToken, nil
}

func parseServiceAccountPrivateKey(raw string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, fmt.Errorf("service account private_key is not PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("service account private_key is not RSA")
	}
	return rsaKey, nil
}

func (u *googleExportDriveUploader) EnsureFolderPath(ctx context.Context, parts []string) (exportDriveFile, error) {
	parentID := u.rootFolderID
	var current exportDriveFile
	if len(parts) == 0 {
		return u.getFile(ctx, parentID)
	}
	for _, part := range parts {
		found, err := u.ensureFolder(ctx, parentID, part)
		if err != nil {
			return exportDriveFile{}, err
		}
		current = found
		parentID = found.ID
	}
	if current.ID == "" {
		return exportDriveFile{}, fmt.Errorf("empty Google Drive folder path")
	}
	return current, nil
}

func (u *googleExportDriveUploader) CreateRunFolder(ctx context.Context, parts []string) (exportDriveFile, error) {
	parent, err := u.EnsureFolderPath(ctx, parts[:len(parts)-1])
	if err != nil {
		return exportDriveFile{}, err
	}
	existing, err := u.findChild(ctx, parent.ID, parts[len(parts)-1], driveFolderMimeType)
	if err != nil {
		return exportDriveFile{}, err
	}
	if existing.ID != "" {
		return exportDriveFile{}, fmt.Errorf("Google Drive run folder already exists: %s", strings.Join(parts, "/"))
	}
	return u.createFolder(ctx, parts[len(parts)-1], parent.ID)
}

func (u *googleExportDriveUploader) ensureFolder(ctx context.Context, parentID string, name string) (exportDriveFile, error) {
	found, err := u.findChild(ctx, parentID, name, driveFolderMimeType)
	if err != nil {
		return exportDriveFile{}, err
	}
	if found.ID != "" {
		return found, nil
	}
	return u.createFolder(ctx, name, parentID)
}

func (u *googleExportDriveUploader) UploadFile(ctx context.Context, path string, folderID string, name string, overwrite bool) (exportDriveFile, error) {
	if name == "" {
		name = filepath.Base(path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return exportDriveFile{}, err
	}
	contentType := contentTypeForPath(path)
	if overwrite {
		existing, err := u.findChild(ctx, folderID, name, "")
		if err != nil {
			return exportDriveFile{}, err
		}
		if existing.ID != "" {
			return u.updateMedia(ctx, existing.ID, contentType, data)
		}
	}
	return u.createMultipartFile(ctx, name, folderID, contentType, data)
}

func (u *googleExportDriveUploader) UploadText(ctx context.Context, text string, folderID string, name string, overwrite bool) (exportDriveFile, error) {
	if overwrite {
		existing, err := u.findChild(ctx, folderID, name, "")
		if err != nil {
			return exportDriveFile{}, err
		}
		if existing.ID != "" {
			return u.updateMedia(ctx, existing.ID, "text/plain", []byte(text))
		}
	}
	return u.createMultipartFile(ctx, name, folderID, "text/plain", []byte(text))
}

func (u *googleExportDriveUploader) findChild(ctx context.Context, parentID string, name string, mimeType string) (exportDriveFile, error) {
	clauses := []string{
		fmt.Sprintf("name = '%s'", strings.ReplaceAll(name, "'", "\\'")),
		"trashed = false",
	}
	if parentID != "" {
		clauses = append(clauses, fmt.Sprintf("'%s' in parents", parentID))
	}
	if mimeType != "" {
		clauses = append(clauses, fmt.Sprintf("mimeType = '%s'", mimeType))
	}
	values := url.Values{}
	values.Set("q", strings.Join(clauses, " and "))
	values.Set("fields", "files(id,name,webViewLink)")
	values.Set("spaces", "drive")
	values.Set("pageSize", "10")
	values.Set("includeItemsFromAllDrives", "true")
	values.Set("supportsAllDrives", "true")
	var payload struct {
		Files []exportDriveFile `json:"files"`
	}
	if err := u.driveJSON(ctx, http.MethodGet, "https://www.googleapis.com/drive/v3/files?"+values.Encode(), nil, "application/json", &payload); err != nil {
		return exportDriveFile{}, err
	}
	if len(payload.Files) == 0 {
		return exportDriveFile{}, nil
	}
	return payload.Files[0], nil
}

func (u *googleExportDriveUploader) getFile(ctx context.Context, fileID string) (exportDriveFile, error) {
	if strings.TrimSpace(fileID) == "" {
		return exportDriveFile{}, fmt.Errorf("Google Drive root folder ID is empty")
	}
	values := url.Values{}
	values.Set("fields", "id,name,webViewLink")
	values.Set("supportsAllDrives", "true")
	var file exportDriveFile
	if err := u.driveJSON(ctx, http.MethodGet, "https://www.googleapis.com/drive/v3/files/"+url.PathEscape(fileID)+"?"+values.Encode(), nil, "application/json", &file); err != nil {
		return exportDriveFile{}, err
	}
	return file, nil
}

func (u *googleExportDriveUploader) createFolder(ctx context.Context, name string, parentID string) (exportDriveFile, error) {
	metadata := map[string]any{"name": name, "mimeType": driveFolderMimeType}
	if parentID != "" {
		metadata["parents"] = []string{parentID}
	}
	values := url.Values{}
	values.Set("fields", "id,name,webViewLink")
	values.Set("supportsAllDrives", "true")
	var file exportDriveFile
	err := u.driveJSON(ctx, http.MethodPost, "https://www.googleapis.com/drive/v3/files?"+values.Encode(), metadata, "application/json", &file)
	if err != nil && isGoogleDriveResponseRecoveryError(err) {
		if found, findErr := u.findChildAfterDriveRecovery(ctx, parentID, name, driveFolderMimeType); findErr == nil && found.ID != "" {
			return found, nil
		}
	}
	return file, err
}

func (u *googleExportDriveUploader) createMultipartFile(ctx context.Context, name string, folderID string, contentType string, data []byte) (exportDriveFile, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	metadata := map[string]any{"name": name, "parents": []string{folderID}}
	metaPart, err := writer.CreatePart(map[string][]string{"Content-Type": {"application/json; charset=UTF-8"}})
	if err != nil {
		return exportDriveFile{}, err
	}
	metaJSON, _ := json.Marshal(metadata)
	if _, err := metaPart.Write(metaJSON); err != nil {
		return exportDriveFile{}, err
	}
	filePart, err := writer.CreatePart(map[string][]string{"Content-Type": {contentType}})
	if err != nil {
		return exportDriveFile{}, err
	}
	if _, err := filePart.Write(data); err != nil {
		return exportDriveFile{}, err
	}
	if err := writer.Close(); err != nil {
		return exportDriveFile{}, err
	}
	values := url.Values{}
	values.Set("uploadType", "multipart")
	values.Set("fields", "id,name,webViewLink")
	values.Set("supportsAllDrives", "true")
	var file exportDriveFile
	err = u.driveJSON(ctx, http.MethodPost, "https://www.googleapis.com/upload/drive/v3/files?"+values.Encode(), body.Bytes(), writer.FormDataContentType(), &file)
	if err != nil && isGoogleDriveResponseRecoveryError(err) {
		if found, findErr := u.findChildAfterDriveRecovery(ctx, folderID, name, ""); findErr == nil && found.ID != "" {
			return found, nil
		}
	}
	return file, err
}

func (u *googleExportDriveUploader) updateMedia(ctx context.Context, fileID string, contentType string, data []byte) (exportDriveFile, error) {
	values := url.Values{}
	values.Set("uploadType", "media")
	values.Set("fields", "id,name,webViewLink")
	values.Set("supportsAllDrives", "true")
	var file exportDriveFile
	err := u.driveJSON(ctx, http.MethodPatch, "https://www.googleapis.com/upload/drive/v3/files/"+url.PathEscape(fileID)+"?"+values.Encode(), data, contentType, &file)
	if err != nil && isGoogleDriveResponseRecoveryError(err) {
		if found, findErr := u.getFile(ctx, fileID); findErr == nil && found.ID != "" {
			return found, nil
		}
	}
	return file, err
}

func isGoogleDriveResponseRecoveryError(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "responsePreparationFailure") ||
		strings.Contains(text, "error preparing the response")
}

func (u *googleExportDriveUploader) findChildAfterDriveRecovery(ctx context.Context, parentID string, name string, mimeType string) (exportDriveFile, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		found, err := u.findChild(ctx, parentID, name, mimeType)
		if err == nil && found.ID != "" {
			return found, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return exportDriveFile{}, lastErr
	}
	return exportDriveFile{}, nil
}

func (u *googleExportDriveUploader) driveJSON(ctx context.Context, method string, endpoint string, input any, contentType string, output any) error {
	var bodyBytes []byte
	switch value := input.(type) {
	case nil:
	case []byte:
		bodyBytes = value
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		bodyBytes = data
	}
	for attempt := 0; attempt < 2; attempt++ {
		var body io.Reader
		if bodyBytes != nil {
			body = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+u.accessToken)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		resp, err := u.httpClient.Do(req)
		if err != nil {
			return err
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			token, refreshErr := googleDriveAccessToken(ctx)
			if refreshErr != nil {
				return fmt.Errorf("Google Drive request failed: %s: %s; token refresh failed: %w", resp.Status, strings.TrimSpace(string(data)), refreshErr)
			}
			u.accessToken = token
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("Google Drive request failed: %s: %s", resp.Status, strings.TrimSpace(string(data)))
		}
		if output == nil || len(data) == 0 {
			return nil
		}
		return json.Unmarshal(data, output)
	}
	return fmt.Errorf("Google Drive request failed after token refresh")
}

func driveUploadName(courseSlug string, path string) string {
	name := filepath.Base(path)
	if courseSlug == "" {
		return name
	}
	return courseSlug + "--" + name
}

func contentTypeForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md":
		return "text/markdown"
	case ".yaml", ".yml":
		return "application/x-yaml"
	case ".json":
		return "application/json"
	case ".jsonl":
		return "application/x-ndjson"
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".zip":
		return "application/zip"
	}
	if detected := mime.TypeByExtension(filepath.Ext(path)); detected != "" {
		return detected
	}
	return "application/octet-stream"
}
