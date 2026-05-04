package codexstate

import (
	"archive/zip"
	"bytes"
	"net/http/httptest"
	"testing"
)

func TestValidateCodexZipAcceptsSafeArchive(t *testing.T) {
	data := makeZip(t, "sessions/current.json", "{}")
	recorder := httptest.NewRecorder()

	if !validateCodexZip(recorder, data) {
		t.Fatalf("expected safe zip to validate, got status %d", recorder.Code)
	}
}

func TestValidateCodexZipRejectsTraversal(t *testing.T) {
	data := makeZip(t, "../secret.json", "{}")
	recorder := httptest.NewRecorder()

	if validateCodexZip(recorder, data) {
		t.Fatal("expected zip with traversal path to fail validation")
	}
}

func makeZip(t *testing.T, name string, contents string) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	file, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte(contents)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
