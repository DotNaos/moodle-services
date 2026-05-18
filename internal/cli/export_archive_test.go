package cli

import (
	"archive/zip"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestResolveExportArchivePathUsesZipOrDirectory(t *testing.T) {
	run := exportRunContext{Semester: "FS26", RunID: "2026-05-17-120000-local"}

	zipPath, err := resolveExportArchivePath(filepath.Join(t.TempDir(), "custom.zip"), run)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(zipPath) != "custom.zip" {
		t.Fatalf("unexpected zip path: %s", zipPath)
	}

	dirPath, err := resolveExportArchivePath(t.TempDir(), run)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dirPath) != "fhgr-moodle-FS26-2026-05-17-120000-local.zip" {
		t.Fatalf("unexpected generated archive name: %s", dirPath)
	}
}

func TestWriteExportArchiveCreatesSanitizedOfflineZip(t *testing.T) {
	root := t.TempDir()
	tempDir := t.TempDir()
	run := exportRunContext{
		Semester:  "FS26",
		RunID:     "2026-05-17-120000-local",
		StartedAt: time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC),
		Workspace: root,
	}
	course := exportCourse{
		ID:    22585,
		Slug:  "deep-learning",
		Title: "Deep Learning",
		Dir:   filepath.Join(root, "FS26", "deep-learning"),
	}
	writeTestFile(t, filepath.Join(course.Dir, "MOODLE.md"), "# Course\n")
	writeTestFile(t, filepath.Join(course.Dir, "moodle-course.yaml"), "id: 22585\n")
	writeTestFile(t, filepath.Join(course.Dir, "materials.index.yaml"), "materials: []\n")
	writeTestFile(t, filepath.Join(course.Dir, "materials-text", "Gears of Neural Networks.md"), "# Text\n")
	writeTestFile(t, filepath.Join(root, "FS26", "calendar", "calendar.index.md"), "# Calendar\n")
	writeTestFile(t, filepath.Join(root, "FS26", "calendar", "calendar.index.yaml"), "events: []\n")
	writeTestFile(t, filepath.Join(root, "FS26", "calendar", "README.md"), "# Calendar\n")
	writeTestFile(t, filepath.Join(tempDir, "raw-files", "deep-learning", "956877-gears-of-neural-networks.pdf"), "%PDF-1.4\n")
	writeTestFile(t, filepath.Join(tempDir, "deep-learning.zip"), "zip-data\n")
	writeTestFile(t, filepath.Join(tempDir, "calendar.ics"), "BEGIN:VCALENDAR\nEND:VCALENDAR\n")

	record := exportMaterialRecord{
		ID:               "FS26/deep-learning/956877",
		Semester:         "FS26",
		CourseSlug:       "deep-learning",
		CourseName:       "Deep Learning",
		CourseID:         "22585",
		ResourceID:       "956877",
		Title:            "Gears of Neural Networks",
		Type:             "pdf",
		MoodleSection:    "Einführung",
		OriginalFilename: "956877-gears-of-neural-networks.pdf",
		RunID:            run.RunID,
	}
	manifest := exportCourseManifest{CourseSlug: course.Slug, RawZipFilename: "deep-learning.zip"}
	calendar := exportCalendarIndex{Semester: "FS26", RunID: run.RunID, EventCount: 1}
	outputPath := filepath.Join(t.TempDir(), "archive.zip")

	if err := writeExportArchive(run, []exportCourse{course}, []exportMaterialRecord{record}, []exportCourseManifest{manifest}, calendar, tempDir, outputPath, "full"); err != nil {
		t.Fatal(err)
	}

	names := zipEntryNames(t, outputPath)
	for _, want := range []string{
		"README.md",
		"manifest.yaml",
		"semester.index.yaml",
		"materials/all-materials.index.md",
		"courses/deep-learning/MOODLE.md",
		"courses/deep-learning/text/gears-of-neural-networks.md",
		"courses/deep-learning/raw/956877-gears-of-neural-networks.pdf",
		"raw-course-zips/deep-learning.zip",
		"calendar/raw/calendar.ics",
		"calendar/calendar.index.yaml",
	} {
		if !containsString(names, want) {
			t.Fatalf("archive missing %s; got %#v", want, names)
		}
	}
}

func TestWriteGoodNotesArchiveCreatesPDFOnlySemesterLayout(t *testing.T) {
	tempDir := t.TempDir()
	run := exportRunContext{Semester: "FS26", RunID: "local"}
	writeTestFile(t, filepath.Join(tempDir, "raw-files", "deep-learning", "956877-gears.pdf"), "%PDF-1.4\n")
	writeTestFile(t, filepath.Join(tempDir, "raw-files", "deep-learning", "956878-notes.txt"), "notes\n")
	writeTestFile(t, filepath.Join(tempDir, "raw-files", "deep-learning", "956879-general.pdf"), "%PDF-1.4\n")
	records := []exportMaterialRecord{
		{
			Semester:         "FS26",
			CourseSlug:       "deep-learning",
			CourseName:       "Deep Learning",
			ResourceID:       "956877",
			Title:            "Gears",
			Type:             "pdf",
			MoodleSection:    "Einführung",
			OriginalFilename: "956877-gears.pdf",
			RunID:            run.RunID,
		},
		{
			Semester:         "FS26",
			CourseSlug:       "deep-learning",
			CourseName:       "Deep Learning",
			ResourceID:       "956878",
			Title:            "Notes",
			Type:             "txt",
			MoodleSection:    "Einführung",
			OriginalFilename: "956878-notes.txt",
			RunID:            run.RunID,
		},
		{
			Semester:         "FS26",
			CourseSlug:       "deep-learning",
			CourseName:       "Deep Learning",
			ResourceID:       "956879",
			Title:            "Module Description",
			Type:             "pdf",
			MoodleSection:    "Allgemeine Informationen",
			OriginalFilename: "956879-general.pdf",
			RunID:            run.RunID,
		},
	}
	outputPath := filepath.Join(t.TempDir(), "goodnotes.zip")

	if err := writeExportArchive(run, nil, records, nil, exportCalendarIndex{}, tempDir, outputPath, "goodnotes"); err != nil {
		t.Fatal(err)
	}

	names := zipEntryNames(t, outputPath)
	if !containsString(names, "FS26/Deep Learning/Einführung/Gears.pdf") {
		t.Fatalf("archive missing GoodNotes PDF path; got %#v", names)
	}
	if containsString(names, "README.md") || containsString(names, "manifest.yaml") {
		t.Fatalf("GoodNotes archive should not include metadata files; got %#v", names)
	}
	if containsString(names, "FS26/Deep Learning/Einführung/Notes.pdf") {
		t.Fatalf("GoodNotes archive should not include non-PDF files; got %#v", names)
	}
	if containsString(names, "FS26/Deep Learning/Allgemeine Informationen/Module Description.pdf") {
		t.Fatalf("GoodNotes archive should skip Allgemeine Informationen; got %#v", names)
	}
}

func TestGoodNotesZipTargetPartsUsesIndexSectionAndActivityTitle(t *testing.T) {
	run := exportRunContext{Semester: "FS26", RunID: "local"}
	course := exportCourse{Slug: "scientific-algos", Title: "Algorithmen des wissenschaftlichen Rechnens"}
	index := map[string]goodNotesZipIndexEntry{
		"Datei_Folien Teil 1 (Update 05.03.26).1315049": {
			Section: "Thema 1: Sparse Grids",
			Title:   "Folien Teil 1 (Update 05.03.26)",
		},
	}

	got, ok := goodNotesZipTargetParts(run, course, "Kurs_Algorithmen des wissenschaftlichen Rechnens.1267996/Datei_Folien Teil 1 (Update 05.03.26).1315049/content/AlgWiss-01.pdf", index)
	if !ok {
		t.Fatal("expected GoodNotes target")
	}
	want := []string{
		"FS26",
		"Algorithmen des wissenschaftlichen Rechnens",
		"Thema 1 - Sparse Grids",
		"Folien Teil 1 (Update 05.03.26).pdf",
	}
	if len(got) != len(want) {
		t.Fatalf("target parts length mismatch: got %#v want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("target parts mismatch: got %#v want %#v", got, want)
		}
	}
}

func TestGoodNotesZipTargetPartsSkipsGeneralSection(t *testing.T) {
	run := exportRunContext{Semester: "FS26", RunID: "local"}
	course := exportCourse{Slug: "scientific-algos", Title: "Algorithmen des wissenschaftlichen Rechnens"}
	index := map[string]goodNotesZipIndexEntry{
		"Datei_Modulbeschreibung.100": {
			Section: "Allgemeine Informationen",
			Title:   "Modulbeschreibung",
		},
	}

	_, ok := goodNotesZipTargetParts(run, course, "Kurs_Algorithmen.1/Datei_Modulbeschreibung.100/content/modulbeschreibung.pdf", index)
	if ok {
		t.Fatal("expected Allgemeine Informationen to be skipped")
	}
}

func TestParseGoodNotesCourseIndexHTMLMapsActivitiesToSections(t *testing.T) {
	index := parseGoodNotesCourseIndexHTML(`
		<h3>Allgemeine Informationen</h3>
		<ul><li><a href="./Forum_Nachrichten_.1268665/index.html">Nachrichten (Forum)</a></li></ul>
		<h3>Thema 1: Sparse Grids</h3>
		<ul>
			<li><a href="./Datei_Folien_Teil_1_Update_05..._.1315049/index.html">Folien Teil 1 (Update 05.03.26) (Datei)</a></li>
			<li><a href="./Datei_Aufgabenblatt_01_--_Lsung_.1317158/index.html">Aufgabenblatt 01 -- Lösung (Datei)</a></li>
		</ul>
	`)
	got := index["Datei_Folien_Teil_1_Update_05..._.1315049"]
	if got.Section != "Thema 1: Sparse Grids" || got.Title != "Folien Teil 1 (Update 05.03.26)" {
		t.Fatalf("unexpected index entry: %#v", got)
	}
	got = index["Datei_Aufgabenblatt_01_--_Lsung_.1317158"]
	if got.Title != "Aufgabenblatt 01 -- Lösung" {
		t.Fatalf("unexpected activity title: %#v", got)
	}
}

func TestSanitizeArchiveFilename(t *testing.T) {
	got := sanitizeArchiveFilename("Folien Teil 1 (Update 05.03.26).PDF")
	if got != "folien-teil-1-update-05-03-26.pdf" {
		t.Fatalf("unexpected sanitized filename: %s", got)
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func zipEntryNames(t *testing.T, path string) []string {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		names = append(names, file.Name)
	}
	sort.Strings(names)
	return names
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
