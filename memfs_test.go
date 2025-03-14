package memfs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
)

func TestFS(t *testing.T) {
	rootFS := New()

	err := rootFS.MkdirAll("foo/bar", 0o777)
	if err != nil {
		t.Fatal(err)
	}
	err = rootFS.WriteFile("foo/bar/buz.txt", []byte("buz"), 0o777)
	if err != nil {
		t.Fatal(err)
	}
	err = fstest.TestFS(rootFS, "foo/bar/buz.txt")
	if err != nil {
		t.Fatal(err)
	}
}

func TestMemFS(t *testing.T) {
	rootFS := New()

	err := rootFS.MkdirAll("foo/bar", 0o777)
	if err != nil {
		t.Fatal(err)
	}

	var gotPaths []string

	err = fs.WalkDir(rootFS, ".", func(path string, d fs.DirEntry, err error) error {
		gotPaths = append(gotPaths, path)
		if !d.IsDir() {
			return fmt.Errorf("%s is not a directory", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	expectPaths := []string{
		".",
		"foo",
		"foo/bar",
	}

	if diff := cmp.Diff(expectPaths, gotPaths); diff != "" {
		t.Fatalf("WalkDir mismatch %s", diff)
	}

	err = rootFS.WriteFile("foo/baz/buz.txt", []byte("buz"), 0o777)
	if err == nil && errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Expected missing directory error but got none")
	}

	_, err = fs.ReadFile(rootFS, "foo/baz/buz.txt")
	if err == nil && errors.Is(err, fs.ErrNotExist) {
		t.Fatal("Expected no such file but got no error")
	}

	body := []byte("baz")
	err = rootFS.WriteFile("foo/bar/baz.txt", body, 0o777)
	if err != nil {
		t.Fatal(err)
	}

	gotBody, err := fs.ReadFile(rootFS, "foo/bar/baz.txt")
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(body, gotBody); diff != "" {
		t.Fatalf("write/read baz.txt mismatch %s", diff)
	}

	subFS, err := rootFS.Sub("foo/bar")
	if err != nil {
		t.Fatal(err)
	}

	gotSubBody, err := fs.ReadFile(subFS, "baz.txt")
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(body, gotSubBody); diff != "" {
		t.Fatalf("write/read baz.txt mismatch %s", diff)
	}

	body = []byte("top_level_file")
	err = rootFS.WriteFile("top_level_file.txt", body, 0o777)
	if err != nil {
		t.Fatalf("Write top_level_file error: %s", err)
	}

	gotBody, err = fs.ReadFile(rootFS, "top_level_file.txt")
	if err != nil {
		t.Fatalf("Read top_level_file error: %s", err)
	}

	if diff := cmp.Diff(body, gotBody); diff != "" {
		t.Fatalf("write/read top_level_file.txt mismatch %s", diff)
	}
}

func TestOpenHook(t *testing.T) {
	openHook := func(path string, content []byte, origError error) ([]byte, error) {
		if path == "foo/bar/override" {
			return []byte("overriden content"), nil
		}

		return content, origError
	}

	rootFS := New(WithOpenHook(openHook))

	err := rootFS.MkdirAll("foo/bar", 0o777)
	if err != nil {
		t.Fatal(err)
	}

	rootFS.WriteFile("foo/bar/f1", []byte("f1"), 0o777)
	rootFS.WriteFile("foo/bar/override", []byte("orig content"), 0o777)

	content, err := fs.ReadFile(rootFS, "foo/bar/f1")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(string(content), "f1"); diff != "" {
		t.Fatalf("write/read roo/bar/f1 mismatch %s", diff)
	}

	content, err = fs.ReadFile(rootFS, "foo/bar/override")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(string(content), "overriden content"); diff != "" {
		t.Fatalf("hook read mismatch %s", diff)
	}

	_, err = fs.ReadFile(rootFS, "foo/bar/non_existing_file")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Expected ErrNotExist for non-existing file, got: %v", err)
	}
}

func TestSeek(t *testing.T) {
	rootFS := New()

	err := rootFS.WriteFile("foo", []byte("0123456789"), 0o777)
	if err != nil {
		t.Fatal(err)
	}

	f, err := rootFS.Open("foo")
	if err != nil {
		t.Fatal(err)
	}

	seeker, ok := f.(io.Seeker)
	if !ok {
		t.Fatalf("File does not implement io.Seeker")
	}

	// Read first bytes.
	bs := make([]byte, 3)
	n, err := f.Read(bs)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("Expected 3 bytes read, got %d", n)
	}
	if diff := cmp.Diff(bs, []byte("012")); diff != "" {
		t.Fatalf("read mismatch %s", diff)
	}

	// Read more bytes, make sure reader tracks.
	n, err = f.Read(bs)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("Expected 3 bytes read, got %d", n)
	}
	if diff := cmp.Diff(bs, []byte("345")); diff != "" {
		t.Fatalf("read mismatch %s", diff)
	}

	// Seek to beginning.
	ofs, err := seeker.Seek(0, io.SeekStart)
	if err != nil {
		t.Fatal(err)
	}
	if ofs != 0 {
		t.Fatalf("Expected offset 0, got %d", ofs)
	}

	// Read first bytes again.
	n, err = f.Read(bs)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("Expected 3 bytes read, got %d", n)
	}
	if diff := cmp.Diff(bs, []byte("012")); diff != "" {
		t.Fatalf("read mismatch %s", diff)
	}
}

func TestMaxStorage(t *testing.T) {
	rootFS := New(WithMaxStorage(10)) // Set max storage to 10 bytes

	err := rootFS.WriteFile("file1.txt", []byte("12345"), 0o777)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	err = rootFS.WriteFile("file2.txt", []byte("67890"), 0o777)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	err = rootFS.WriteFile("file3.txt", []byte("exceed"), 0o777)
	if err == nil {
		t.Fatal("expected error due to exceeding max storage limit, but got none")
	}
	if !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("expected fs.ErrInvalid, but got: %v", err)
	}
}

func TestSaveLoad(t *testing.T) {
	// Create a test filesystem with some content
	rootFS := New()

	// Create directories and files
	if err := rootFS.MkdirAll("foo/bar", 0o755); err != nil {
		t.Fatal(err)
	}

	testFiles := map[string][]byte{
		"foo/file1.txt":     []byte("content1"),
		"foo/bar/file2.txt": []byte("content2"),
		"root.txt":          []byte("root content"),
	}

	for path, content := range testFiles {
		if err := rootFS.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Save the filesystem to a temporary file
	tmpfile, err := os.CreateTemp("", "memfs_test_*.gob")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if err := rootFS.SaveToFile(tmpfile.Name()); err != nil {
		t.Fatal(err)
	}

	// Load the filesystem back
	loadedFS, err := LoadFromFile(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Verify the loaded filesystem has the same content
	err = fs.WalkDir(loadedFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip root directory
		if path == "." {
			return nil
		}

		// For files, verify content matches
		if !d.IsDir() {
			expectedContent, exists := testFiles[path]
			if !exists {
				return fmt.Errorf("unexpected file in loaded fs: %s", path)
			}

			gotContent, err := fs.ReadFile(loadedFS, path)
			if err != nil {
				return fmt.Errorf("reading file %s: %w", path, err)
			}

			if diff := cmp.Diff(expectedContent, gotContent); diff != "" {
				return fmt.Errorf("content mismatch for %s: %s", path, diff)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify we can still write to the loaded filesystem
	newContent := []byte("new file")
	if err := loadedFS.WriteFile("foo/bar/new.txt", newContent, 0o644); err != nil {
		t.Fatal(err)
	}

	// Verify the new file exists and has correct content
	gotContent, err := fs.ReadFile(loadedFS, "foo/bar/new.txt")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(newContent, gotContent); diff != "" {
		t.Fatalf("new file content mismatch: %s", diff)
	}
}

// TestSeekWithClosedFile tests that seeking on a closed file returns an error.
func TestSeekWithClosedFile(t *testing.T) {
	rootFS := New()

	err := rootFS.WriteFile("foo", []byte("0123456789"), 0o777)
	if err != nil {
		t.Fatal(err)
	}

	f, err := rootFS.Open("foo")
	if err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	seeker, ok := f.(io.Seeker)
	if !ok {
		t.Fatalf("File does not implement io.Seeker")
	}

	_, err = seeker.Seek(0, io.SeekStart)
	if err == nil {
		t.Fatalf("Expected error when seeking on a closed file, got none")
	}
}

// TestReadWithClosedFile tests that reading from a closed file returns an error.
func TestReadWithClosedFile(t *testing.T) {
	rootFS := New()

	err := rootFS.WriteFile("foo", []byte("0123456789"), 0o777)
	if err != nil {
		t.Fatal(err)
	}

	f, err := rootFS.Open("foo")
	if err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = f.Read(make([]byte, 1))
	if err == nil {
		t.Fatalf("Expected error when reading from a closed file, got none")
	}
}

// TestCompressedSaveLoad tests saving and loading a compressed filesystem.
func TestCompressedSaveLoad(t *testing.T) {
	// Create a test filesystem with some content
	rootFS := New()

	// Create directories and files
	if err := rootFS.MkdirAll("foo/bar", 0o755); err != nil {
		t.Fatal(err)
	}

	testFiles := map[string][]byte{
		"foo/file1.txt":     []byte("content1"),
		"foo/bar/file2.txt": []byte("content2"),
		"root.txt":          []byte("root content"),
	}

	for path, content := range testFiles {
		if err := rootFS.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Save the filesystem to a temporary file
	tmpfile, err := os.CreateTemp("", "memfs_test_*.gob")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if err := rootFS.CompressAndSaveToFile(tmpfile.Name()); err != nil {
		t.Fatal(err)
	}

	// Load the filesystem back
	loadedFS, err := DecompressAndLoadFromFile(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Verify the loaded filesystem has the same content
	err = fs.WalkDir(loadedFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip root directory
		if path == "." {
			return nil
		}

		// For files, verify content matches
		if !d.IsDir() {
			expectedContent, exists := testFiles[path]
			if !exists {
				return fmt.Errorf("unexpected file in loaded fs: %s", path)
			}

			gotContent, err := fs.ReadFile(loadedFS, path)
			if err != nil {
				return fmt.Errorf("reading file %s: %w", path, err)
			}

			if diff := cmp.Diff(expectedContent, gotContent); diff != "" {
				return fmt.Errorf("content mismatch for %s: %s", path, diff)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify we can still write to the loaded filesystem
	newContent := []byte("new file")
	if err := loadedFS.WriteFile("foo/bar/new.txt", newContent, 0o644); err != nil {
		t.Fatal(err)
	}

	// Verify the new file exists and has correct content
	gotContent, err := fs.ReadFile(loadedFS, "foo/bar/new.txt")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(newContent, gotContent); diff != "" {
		t.Fatalf("new file content mismatch: %s", diff)
	}
}

// OpenFile tests opening a file with different flags. It also tests the error cases. This test is a copy of the
// OpenFile test in the standard library's fstest package, but adapted to work with MemFS.
func TestOpenFile2(t *testing.T) {
	rootFS := New()

	// Test opening a non-existent file without O_CREATE
	_, err := rootFS.OpenFile("nonexistent.txt", os.O_RDONLY, 0o644)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Expected ErrNotExist for non-existent file, got: %v", err)
	}

	// Test opening a directory for writing
	err = rootFS.MkdirAll("testdir", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rootFS.OpenFile("testdir", os.O_WRONLY, 0o644)
	if err == nil {
		t.Fatal("Expected error when opening directory for writing")
	}

	// Test unsupported flag combination
	_, err = rootFS.OpenFile("test_unsupported.txt", os.O_APPEND, 0o644)
	if err == nil {
		t.Fatal("Expected error for unsupported flag")
	}

	// Save a file to the filesystem
	err = rootFS.WriteFile("testfile.txt", []byte("test content"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	// Test opening a file with O_TRUNC flag
	file, err := rootFS.OpenFile("testfile.txt", os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	fw, ok := file.(*FileWriter)
	if !ok {
		t.Fatal("Expected *FileWriter from OpenFile with O_WRONLY|O_TRUNC")
	}

	_, err = fw.Write([]byte("truncated"))
	if err != nil {
		t.Fatal(err)
	}
	if err := fw.Close(); err != nil {
		t.Fatal(err)
	}

	// Add some more tests to cover 100% of the code
	_, err = rootFS.OpenFile("testfile.txt", os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = rootFS.OpenFile("testfile.txt", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = rootFS.OpenFile("testfile.txt", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = rootFS.OpenFile("testfile.txt", os.O_RDONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}

}
