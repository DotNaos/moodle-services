package studypipeline

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	contract "github.com/DotNaos/moodle-services/pkg/apicontracts"
)

const (
	maxCodexWorkspaceEntries = 5000
	maxCodexUploadBytes      = 25 * 1024 * 1024 // 25 MiB per file
	codexUploadsDirName      = "uploads"
)

// Codex CLI internal directories that are noise in the user-facing workspace
// tree (and can be huge, e.g. .tmp's plugin cache exhausting the entry budget).
var codexInternalDirs = map[string]bool{
	"cache":           true,
	"log":             true,
	"logs":            true,
	"shell_snapshots": true,
	"tmp":             true,
	"versions":        true,
}

func skipWorkspaceDir(name string) bool {
	return strings.HasPrefix(name, ".") || codexInternalDirs[name]
}

// OpenCodexWorkspaceFile reads a single file from the user's per-user Codex
// volume for viewing/download. The relative path is sanitized to prevent
// traversal outside the user's directory.
func OpenCodexWorkspaceFile(userID string, root string, relPath string) ([]byte, string, error) {
	rel := strings.TrimPrefix(filepath.Clean("/"+strings.TrimSpace(relPath)), "/")
	if rel == "" || rel == "." {
		return nil, "", fmt.Errorf("invalid path")
	}
	userSegment := safeSegment(firstNonEmpty(userID, "anonymous"))
	stateRoot := filepath.Join(firstNonEmpty(root, ArtifactRootFromEnv()), "codex-users", userSegment)
	full := filepath.Join(stateRoot, rel)
	if check, err := filepath.Rel(stateRoot, full); err != nil || strings.HasPrefix(check, "..") {
		return nil, "", fmt.Errorf("invalid path")
	}
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		return nil, "", fmt.Errorf("file not found")
	}
	if info.Size() > maxCodexUploadBytes {
		return nil, "", fmt.Errorf("file is too large to preview")
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, "", err
	}
	return data, contentTypeForName(full), nil
}

func contentTypeForName(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".md", ".txt", ".log":
		return "text/plain; charset=utf-8"
	case ".json":
		return "application/json"
	default:
		return "application/octet-stream"
	}
}

// SaveCodexUpload writes an uploaded file into the user's per-user Codex volume
// under "uploads/". The file becomes visible inside the Codex container (the
// volume is mounted at /home/codex/.codex) and in the workspace file tree.
func SaveCodexUpload(userID string, root string, name string, data []byte) (contract.CodexWorkspaceFile, error) {
	if len(data) == 0 {
		return contract.CodexWorkspaceFile{}, fmt.Errorf("uploaded file is empty")
	}
	if len(data) > maxCodexUploadBytes {
		return contract.CodexWorkspaceFile{}, fmt.Errorf("uploaded file exceeds the %d MiB limit", maxCodexUploadBytes/(1024*1024))
	}
	safeName := safeUploadFileName(name)
	if safeName == "" {
		return contract.CodexWorkspaceFile{}, fmt.Errorf("invalid file name")
	}

	userSegment := safeSegment(firstNonEmpty(userID, "anonymous"))
	uploadsRoot := filepath.Join(firstNonEmpty(root, ArtifactRootFromEnv()), "codex-users", userSegment, codexUploadsDirName)
	if err := os.MkdirAll(uploadsRoot, 0o700); err != nil {
		return contract.CodexWorkspaceFile{}, err
	}
	dest := filepath.Join(uploadsRoot, safeName)
	if err := os.WriteFile(dest, data, 0o600); err != nil {
		return contract.CodexWorkspaceFile{}, err
	}
	_ = os.Chown(dest, 10001, 10001)

	entry := contract.CodexWorkspaceFile{
		Path:       codexUploadsDirName + "/" + safeName,
		Size:       int64(len(data)),
		Dir:        false,
		ModifiedAt: time.Now().UTC().Format(time.RFC3339),
	}
	return entry, nil
}

// safeUploadFileName reduces a client-supplied name to a safe basename: no path
// traversal, no leading dots, only conservative characters.
func safeUploadFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == ".." || name == string(filepath.Separator) {
		return ""
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_', r == ' ':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	cleaned := strings.TrimLeft(strings.TrimSpace(b.String()), ".")
	if len([]rune(cleaned)) > 120 {
		cleaned = string([]rune(cleaned)[:120])
	}
	return cleaned
}

// CodexWorkspaceFiles lists a user's per-user Codex volume as a flat, sorted
// slice of relative paths (both files and directories). It never creates the
// directory; a missing volume yields an empty list. Only names, sizes and
// modification times are returned — never file contents.
func CodexWorkspaceFiles(userID string, root string) ([]contract.CodexWorkspaceFile, error) {
	userSegment := safeSegment(firstNonEmpty(userID, "anonymous"))
	stateRoot := filepath.Join(firstNonEmpty(root, ArtifactRootFromEnv()), "codex-users", userSegment)
	info, err := os.Stat(stateRoot)
	if err != nil || !info.IsDir() {
		return []contract.CodexWorkspaceFile{}, nil
	}

	files := make([]contract.CodexWorkspaceFile, 0, 64)
	walkErr := filepath.WalkDir(stateRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable entries
		}
		if path == stateRoot {
			return nil
		}
		if d.IsDir() && skipWorkspaceDir(d.Name()) {
			return filepath.SkipDir
		}
		rel, relErr := filepath.Rel(stateRoot, path)
		if relErr != nil {
			return nil
		}
		entry := contract.CodexWorkspaceFile{
			Path: filepath.ToSlash(rel),
			Dir:  d.IsDir(),
		}
		if fi, infoErr := d.Info(); infoErr == nil {
			if !d.IsDir() {
				entry.Size = fi.Size()
			}
			entry.ModifiedAt = fi.ModTime().UTC().Format(time.RFC3339)
		}
		files = append(files, entry)
		if len(files) >= maxCodexWorkspaceEntries {
			return filepath.SkipAll
		}
		return nil
	})
	if walkErr != nil {
		return files, walkErr
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}
