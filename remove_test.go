package memfs

import (
	"errors"
	"io/fs"
	"testing"
)

// TestRemove tests the Remove function for files and directories
func TestRemove(t *testing.T) {
	// Create a new filesystem
	rootFS := New()

	// Create a directory with files and subdirectories
	if err := rootFS.MkdirAll("dir1/subdir", 0755); err != nil {
		t.Fatal(err)
	}

	// Create some files
	if err := rootFS.WriteFile("file1.txt", []byte("file1 content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := rootFS.WriteFile("dir1/file2.txt", []byte("file2 content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := rootFS.WriteFile("dir1/subdir/file3.txt", []byte("file3 content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Test removing a file
	if err := rootFS.Remove("file1.txt"); err != nil {
		t.Fatalf("Failed to remove file: %v", err)
	}

	// Verify file is gone
	_, err := rootFS.Open("file1.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Expected file1.txt to be gone, but got: %v", err)
	}

	// Test removing an empty directory
	if err := rootFS.MkdirAll("emptydir", 0755); err != nil {
		t.Fatal(err)
	}
	if err := rootFS.Remove("emptydir"); err != nil {
		t.Fatalf("Failed to remove empty directory: %v", err)
	}

	// Verify directory is gone
	_, err = rootFS.Open("emptydir")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Expected emptydir to be gone, but got: %v", err)
	}

	// Test removing a non-empty directory (should fail)
	err = rootFS.Remove("dir1")
	if err == nil {
		t.Fatal("Expected error when removing non-empty directory, but got nil")
	}

	// Test removing a non-existent file
	err = rootFS.Remove("nonexistent.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Expected ErrNotExist when removing non-existent file, got: %v", err)
	}

	// Test removing the root directory (should fail)
	err = rootFS.Remove(".")
	if err == nil {
		t.Fatal("Expected error when removing root directory, but got nil")
	}

	// Test removing a file with an invalid path
	err = rootFS.Remove("../invalid/path")
	if err == nil {
		t.Fatal("Expected error when removing file with invalid path, but got nil")
	}
}

// TestRemoveAll tests the RemoveAll function for directories with contents
func TestRemoveAll(t *testing.T) {
	// Create a new filesystem
	rootFS := New(WithMaxStorage(1000)) // Enable storage tracking for testing

	// Create a nested directory structure with files
	if err := rootFS.MkdirAll("dir1/subdir1/subsubdir", 0755); err != nil {
		t.Fatal(err)
	}
	if err := rootFS.MkdirAll("dir1/subdir2", 0755); err != nil {
		t.Fatal(err)
	}

	// Create some files with known content sizes
	if err := rootFS.WriteFile("dir1/file1.txt", []byte("content1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := rootFS.WriteFile("dir1/subdir1/file2.txt", []byte("content2content2"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := rootFS.WriteFile("dir1/subdir1/subsubdir/file3.txt", []byte("content3content3content3"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := rootFS.WriteFile("dir1/subdir2/file4.txt", []byte("content4"), 0644); err != nil {
		t.Fatal(err)
	}

	// Store initial storage usage
	initialStorage := rootFS.usedStorage

	// Test removing a directory hierarchy
	if err := rootFS.RemoveAll("dir1/subdir1"); err != nil {
		t.Fatalf("Failed to remove directory tree: %v", err)
	}

	// Verify the removed directory and its files are gone
	_, err := rootFS.Open("dir1/subdir1")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Expected dir1/subdir1 to be gone, but got: %v", err)
	}
	_, err = rootFS.Open("dir1/subdir1/file2.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Expected dir1/subdir1/file2.txt to be gone, but got: %v", err)
	}
	_, err = rootFS.Open("dir1/subdir1/subsubdir/file3.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Expected dir1/subdir1/subsubdir/file3.txt to be gone, but got: %v", err)
	}

	// Verify that subdir2 still exists
	_, err = rootFS.Open("dir1/subdir2/file4.txt")
	if err != nil {
		t.Fatalf("dir1/subdir2/file4.txt should still exist: %v", err)
	}

	// Test removing a file with RemoveAll
	if err := rootFS.RemoveAll("dir1/subdir2/file4.txt"); err != nil {
		t.Fatalf("Failed to remove file with RemoveAll: %v", err)
	}
	_, err = rootFS.Open("dir1/subdir2/file4.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Expected dir1/subdir2/file4.txt to be gone, but got: %v", err)
	}

	// Test removing a non-existent path (should succeed with no error)
	if err := rootFS.RemoveAll("nonexistent/path"); err != nil {
		t.Fatalf("RemoveAll on non-existent path should succeed, got: %v", err)
	}

	// Verify storage tracking is working
	if rootFS.usedStorage >= initialStorage {
		t.Fatalf("Storage usage should have decreased. Initial: %d, Current: %d", initialStorage, rootFS.usedStorage)
	}

	// Create some more files to test clearing the entire filesystem
	if err := rootFS.WriteFile("root_file1.txt", []byte("root content 1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := rootFS.WriteFile("root_file2.txt", []byte("root content 2"), 0644); err != nil {
		t.Fatal(err)
	}

	// Test clearing the entire filesystem
	if err := rootFS.RemoveAll("."); err != nil {
		t.Fatalf("Failed to clear entire filesystem: %v", err)
	}

	// Verify all files are gone
	entries, err := fs.ReadDir(rootFS, ".")
	if err != nil {
		t.Fatalf("Failed to read root directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("Expected empty filesystem after RemoveAll(\".\"), but found %d entries", len(entries))
	}

	// Verify storage usage is reset to 0
	if rootFS.usedStorage != 0 {
		t.Fatalf("Storage usage should be 0 after clearing all files, got: %d", rootFS.usedStorage)
	}

	// Test with an invalid path
	err = rootFS.RemoveAll("../invalid/path")
	if err == nil {
		t.Fatal("Expected error when removing path with invalid syntax, but got nil")
	}
}

// TestRemoveEdgeCases tests edge cases for Remove and RemoveAll
func TestRemoveEdgeCases(t *testing.T) {
	rootFS := New()

	// Test removing from a path where a parent directory doesn't exist
	err := rootFS.Remove("nonexistent/file.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Expected ErrNotExist when parent dir doesn't exist, got: %v", err)
	}

	err = rootFS.RemoveAll("nonexistent/dir")
	if err != nil {
		t.Fatalf("RemoveAll should succeed even when path doesn't exist, got: %v", err)
	}

	// Create a file and try to remove it as if it were a directory
	if err := rootFS.WriteFile("file.txt", []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a directory with the same name as a file but in different path
	if err := rootFS.MkdirAll("dir/file.txt", 0755); err != nil {
		t.Fatal(err)
	}

	// Should be able to remove both without conflicts
	if err := rootFS.Remove("file.txt"); err != nil {
		t.Fatalf("Failed to remove file: %v", err)
	}

	if err := rootFS.RemoveAll("dir"); err != nil {
		t.Fatalf("Failed to remove directory: %v", err)
	}
}
