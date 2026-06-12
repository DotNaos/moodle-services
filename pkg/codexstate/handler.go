package codexstate

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"strings"

	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
	svc "github.com/DotNaos/moodle-services/pkg/moodleservices"
)

const maxCodexStateZipBytes = 8 * 1024 * 1024

var allowedCodexStateKinds = map[string]bool{
	"codex-auth":      true,
	"codex-session":   true,
	"codex-memory":    true,
	"codex-artifacts": true,
}

func Handle(w http.ResponseWriter, r *http.Request, clerkUserID string) {
	switch r.Method {
	case http.MethodGet:
		codexStateLatest(w, r, clerkUserID)
	case http.MethodPost:
		codexStateCreate(w, r, clerkUserID)
	default:
		svc.AllowMethods(w, r, http.MethodGet, http.MethodPost)
	}
}

func codexStateCreate(w http.ResponseWriter, r *http.Request, clerkUserID string) {
	var input contract.CreateCodexStateSnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	kind, ok := normalizeCodexStateKind(w, input.Kind)
	if !ok {
		return
	}
	zipData, ok := decodeCodexZip(w, input.ZipBase64)
	if !ok {
		return
	}
	if !validateCodexZip(w, zipData) {
		return
	}
	cfg := svc.LoadServerEnv()
	store, err := svc.OpenStoreFromEnv(cfg)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	defer store.Close()
	user, err := store.UserForClerkID(r.Context(), clerkUserID)
	if errors.Is(err, svc.ErrNotFound) {
		writeCodexUserNotReady(w)
		return
	}
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	quotaBytes, err := store.EffectiveCodexStateQuotaBytes(r.Context(), user.ID, cfg.CodexStateUserQuotaBytes, cfg.CodexStateAdminQuotaBytes)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	if quotaBytes > 0 && int64(len(zipData)) > quotaBytes {
		svc.WriteJSON(w, http.StatusRequestEntityTooLarge, map[string]any{
			"error":      "Codex state snapshot exceeds this user's storage quota.",
			"quotaBytes": quotaBytes,
		})
		return
	}
	box, err := svc.EncryptionBox(cfg)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	encryptedZip, err := box.EncryptString(base64.RawStdEncoding.EncodeToString(zipData))
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	hash := sha256.Sum256(zipData)
	snapshot, err := store.CreateCodexStateSnapshot(r.Context(), svc.CreateCodexStateSnapshotInput{
		UserID:         user.ID,
		Kind:           kind,
		StorageBackend: "postgres",
		EncryptedZip:   encryptedZip,
		ZipSHA256:      hex.EncodeToString(hash[:]),
		ZipSizeBytes:   len(zipData),
		Metadata:       input.Metadata,
		UserQuotaBytes: quotaBytes,
	})
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	svc.WriteJSON(w, http.StatusOK, contract.CodexStateSnapshotResponse{Snapshot: snapshot})
}

func codexStateLatest(w http.ResponseWriter, r *http.Request, clerkUserID string) {
	kind, ok := normalizeCodexStateKind(w, r.URL.Query().Get("kind"))
	if !ok {
		return
	}
	cfg := svc.LoadServerEnv()
	store, err := svc.OpenStoreFromEnv(cfg)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	defer store.Close()
	user, err := store.UserForClerkID(r.Context(), clerkUserID)
	if errors.Is(err, svc.ErrNotFound) {
		writeCodexUserNotReady(w)
		return
	}
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	snapshot, err := store.LatestCodexStateSnapshot(r.Context(), user.ID, kind)
	if errors.Is(err, svc.ErrNotFound) {
		svc.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "No Codex state snapshot found."})
		return
	}
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	if snapshot.Snapshot.StorageBackend != "postgres" {
		svc.WriteJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "Codex state snapshot object storage is not configured on this deployment.",
		})
		return
	}
	box, err := svc.EncryptionBox(cfg)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	zipBase64, err := box.DecryptString(snapshot.EncryptedZip)
	if err != nil {
		svc.WriteError(w, err)
		return
	}
	svc.WriteJSON(w, http.StatusOK, contract.CodexStateSnapshotResponse{
		Snapshot:  snapshot.Snapshot,
		ZipBase64: zipBase64,
	})
}

func writeCodexUserNotReady(w http.ResponseWriter) {
	svc.WriteJSON(w, http.StatusConflict, map[string]string{
		"code":  "moodle_not_connected",
		"error": "Connect Moodle before persisting Codex state.",
	})
}

func normalizeCodexStateKind(w http.ResponseWriter, raw string) (string, bool) {
	kind := strings.TrimSpace(raw)
	if !allowedCodexStateKinds[kind] {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{
			"error": "kind must be codex-auth, codex-session, codex-memory, or codex-artifacts",
		})
		return "", false
	}
	return kind, true
}

func decodeCodexZip(w http.ResponseWriter, raw string) ([]byte, bool) {
	encoded := strings.TrimSpace(raw)
	if encoded == "" {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "zipBase64 is required"})
		return nil, false
	}
	data, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		data, err = base64.StdEncoding.DecodeString(encoded)
	}
	if err != nil {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "zipBase64 must be valid base64"})
		return nil, false
	}
	if len(data) == 0 || len(data) > maxCodexStateZipBytes {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "zipBase64 exceeds the allowed size"})
		return nil, false
	}
	return data, true
}

func validateCodexZip(w http.ResponseWriter, data []byte) bool {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "zipBase64 must contain a valid zip archive"})
		return false
	}
	if len(reader.File) == 0 {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "zip archive must contain at least one file"})
		return false
	}
	hasFile := false
	for _, file := range reader.File {
		name := strings.TrimSpace(file.Name)
		cleanName := path.Clean(name)
		if name == "" || strings.HasPrefix(name, "/") || cleanName == "." || cleanName == ".." || strings.HasPrefix(cleanName, "../") {
			svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "zip archive contains an unsafe path"})
			return false
		}
		if !file.FileInfo().IsDir() {
			hasFile = true
		}
	}
	if !hasFile {
		svc.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "zip archive must contain at least one file"})
		return false
	}
	return true
}
