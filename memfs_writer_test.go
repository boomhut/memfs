package memfs

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
	"sync"
	"testing"
)

// TestFileWriter tests the FileWriter implementation
func TestFileWriter(t *testing.T) {
	rootFS := New()

	// Test creating and writing to a new file
	fw, err := rootFS.Create("newfile.txt")
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("hello world")
	n, err := fw.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(data) {
		t.Fatalf("Expected to write %d bytes, wrote %d", len(data), n)
	}

	// Test closing the file
	if err := fw.Close(); err != nil {
		t.Fatal(err)
	}

	// Test writing to a closed file
	_, err = fw.Write([]byte("more data"))
	if !errors.Is(err, fs.ErrClosed) {
		t.Fatalf("Expected ErrClosed when writing to closed file, got: %v", err)
	}

	// Verify content was written correctly
	content, err := fs.ReadFile(rootFS, "newfile.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, data) {
		t.Fatalf("Expected content %q, got %q", data, content)
	}

	// Test truncating an existing file
	fw, err = rootFS.Create("newfile.txt")
	if err != nil {
		t.Fatal(err)
	}
	newData := []byte("new content")
	if _, err := fw.Write(newData); err != nil {
		t.Fatal(err)
	}
	if err := fw.Close(); err != nil {
		t.Fatal(err)
	}

	// Verify content was overwritten
	content, err = fs.ReadFile(rootFS, "newfile.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, newData) {
		t.Fatalf("Expected truncated content %q, got %q", newData, content)
	}

	// Test incremental writes
	fw, err = rootFS.Create("incremental.txt")
	if err != nil {
		t.Fatal(err)
	}

	chunks := []string{"Hello, ", "world", "!"}
	var expectedContent strings.Builder
	for _, chunk := range chunks {
		if _, err := fw.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
		expectedContent.WriteString(chunk)
	}
	if err := fw.Close(); err != nil {
		t.Fatal(err)
	}

	content, err = fs.ReadFile(rootFS, "incremental.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != expectedContent.String() {
		t.Fatalf("Expected incremental content %q, got %q", expectedContent.String(), string(content))
	}
}

// TestOpenFile tests the OpenFile implementation with various flags
func TestOpenFile(t *testing.T) {
	rootFS := New()

	// Test O_CREATE flag for a new file
	file, err := rootFS.OpenFile("test1.txt", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	fw, ok := file.(*FileWriter)
	if !ok {
		t.Fatal("Expected *FileWriter from OpenFile with O_WRONLY")
	}
	if _, err := fw.Write([]byte("test content")); err != nil {
		t.Fatal(err)
	}
	if err := fw.Close(); err != nil {
		t.Fatal(err)
	}

	// Test O_RDONLY flag for reading
	file, err = rootFS.OpenFile("test1.txt", os.O_RDONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	reader, ok := file.(fs.File)
	if !ok {
		t.Fatal("Expected fs.File from OpenFile with O_RDONLY")
	}
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "test content" {
		t.Fatalf("Expected content %q, got %q", "test content", string(content))
	}

	// Test O_TRUNC flag
	file, err = rootFS.OpenFile("test1.txt", os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatal(err)
	}
	fw, ok = file.(*FileWriter)
	if !ok {
		t.Fatal("Expected *FileWriter from OpenFile with O_WRONLY|O_TRUNC")
	}
	if _, err := fw.Write([]byte("truncated")); err != nil {
		t.Fatal(err)
	}
	if err := fw.Close(); err != nil {
		t.Fatal(err)
	}

	content, err = fs.ReadFile(rootFS, "test1.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "truncated" {
		t.Fatalf("Expected truncated content %q, got %q", "truncated", string(content))
	}

	// Test opening a non-existent file without O_CREATE
	_, err = rootFS.OpenFile("nonexistent.txt", os.O_RDONLY, 0644)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Expected ErrNotExist for non-existent file, got: %v", err)
	}

	// Test opening a directory for writing
	err = rootFS.MkdirAll("testdir", 0755)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rootFS.OpenFile("testdir", os.O_WRONLY, 0644)
	if err == nil {
		t.Fatal("Expected error when opening directory for writing")
	}

	// Test unsupported flag combination
	_, err = rootFS.OpenFile("test_unsupported.txt", os.O_APPEND, 0644)
	if err == nil {
		t.Fatal("Expected error for unsupported flag")
	}
}

// TestConcurrentAccess tests concurrent access to the filesystem
func TestConcurrentAccess(t *testing.T) {
	rootFS := New()

	// Create initial file
	err := rootFS.WriteFile("concurrent.txt", []byte("initial"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines * 2) // Read and write goroutines

	// Test concurrent reads
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_, err := fs.ReadFile(rootFS, "concurrent.txt")
				if err != nil {
					t.Errorf("Concurrent read error: %v", err)
					return
				}
			}
		}()
	}

	// Test concurrent writes to different files
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			filename := "concurrent_" + string(rune('A'+id)) + ".txt"
			for j := 0; j < 10; j++ {
				fw, err := rootFS.Create(filename)
				if err != nil {
					t.Errorf("Create error: %v", err)
					return
				}
				_, err = fw.Write([]byte("data"))
				if err != nil {
					t.Errorf("Write error: %v", err)
					fw.Close()
					return
				}
				err = fw.Close()
				if err != nil {
					t.Errorf("Close error: %v", err)
					return
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestMaxStorageLimits tests the behavior when approaching and exceeding max storage limits
func TestMaxStorageLimits(t *testing.T) {
	// Test with very tight storage limit
	rootFS := New(WithMaxStorage(20))

	// Write a file that fits within the limit
	err := rootFS.WriteFile("small.txt", []byte("1234567890"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Test incremental writing up to the limit
	fw, err := rootFS.Create("incremental.txt")
	if err != nil {
		t.Fatal(err)
	}

	// Write first chunk (should succeed)
	_, err = fw.Write([]byte("12345"))
	if err != nil {
		t.Fatalf("First write should succeed: %v", err)
	}

	// Write another chunk (should succeed)
	_, err = fw.Write([]byte("6789"))
	if err != nil {
		t.Fatalf("Second write should succeed: %v", err)
	}

	// This write should exceed the limit (10 + 5 + 4 + 2 = 21 > 20)
	_, err = fw.Write([]byte("AB"))
	if err == nil {
		t.Fatal("Expected error when exceeding storage limit")
	}

	// Close the file
	if err := fw.Close(); err != nil {
		t.Fatal(err)
	}

	// Test that we can still read files
	content, err := fs.ReadFile(rootFS, "small.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "1234567890" {
		t.Fatalf("Expected content %q, got %q", "1234567890", string(content))
	}

	// Delete a file to free up space
	rootFS.dir.mu.Lock()
	delete(rootFS.dir.Children, "small.txt")
	rootFS.mu.Lock()
	rootFS.usedStorage -= 10 // Adjust used storage
	rootFS.mu.Unlock()
	rootFS.dir.mu.Unlock()

	// Now we should be able to write a new file
	err = rootFS.WriteFile("new.txt", []byte("new content"), 0644)
	if err != nil {
		t.Fatal(err)
	}
}

// TestGzipWriter tests the GzipWriter implementation
func TestGzipWriter(t *testing.T) {
	// Set up a buffer to capture the output
	var buf bytes.Buffer

	// Create a GzipWriter that writes to the buffer
	gw := NewGzipWriter(&buf)

	// Write some data
	testData := "Hello, compressed world!"
	_, err := gw.Write([]byte(testData))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}

	// Close the writer to flush the data
	err = gw.Close()
	if err != nil {
		t.Fatalf("Close error: %v", err)
	}

	// The data should now be compressed in the buffer
	// We can decompress it to verify
	gr, err := gzip.NewReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("NewReader error: %v", err)
	}

	// Read the decompressed data
	decompressed, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}

	if string(decompressed) != testData {
		t.Fatalf("Expected decompressed data %q, got %q", testData, string(decompressed))
	}

	err = gr.Close()
	if err != nil {
		t.Fatalf("GzipReader Close error: %v", err)
	}

	// Test multiple writes
	buf.Reset()
	gw = NewGzipWriter(&buf)

	firstChunk := "First chunk of data. "
	secondChunk := "Second chunk of data."
	expectedData := firstChunk + secondChunk

	_, err = gw.Write([]byte(firstChunk))
	if err != nil {
		t.Fatalf("First write error: %v", err)
	}

	_, err = gw.Write([]byte(secondChunk))
	if err != nil {
		t.Fatalf("Second write error: %v", err)
	}

	err = gw.Close()
	if err != nil {
		t.Fatalf("Close after multiple writes error: %v", err)
	}

	// Verify the multiple writes were compressed correctly
	gr, err = gzip.NewReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("NewReader after multiple writes error: %v", err)
	}

	decompressed, err = io.ReadAll(gr)
	if err != nil {
		t.Fatalf("ReadAll after multiple writes error: %v", err)
	}

	if string(decompressed) != expectedData {
		t.Fatalf("Expected decompressed data after multiple writes %q, got %q", expectedData, string(decompressed))
	}

	gr.Close()

	// Test with a writer that also implements io.Closer
	closeCalled := false
	mockCloser := &MockWriteCloser{
		writeFunc: func(p []byte) (int, error) {
			return len(p), nil
		},
		closeFunc: func() error {
			closeCalled = true
			return nil
		},
	}

	gw = NewGzipWriter(mockCloser)
	_, err = gw.Write([]byte("test data"))
	if err != nil {
		t.Fatalf("Write to MockWriteCloser error: %v", err)
	}

	err = gw.Close()
	if err != nil {
		t.Fatalf("Close with MockWriteCloser error: %v", err)
	}

	if !closeCalled {
		t.Fatal("Expected underlying writer's Close method to be called")
	}

	// Test Close error handling from underlying writer
	errorMsg := "mock close error"
	mockFailCloser := &MockWriteCloser{
		writeFunc: func(p []byte) (int, error) {
			return len(p), nil
		},
		closeFunc: func() error {
			return errors.New(errorMsg)
		},
	}

	gw = NewGzipWriter(mockFailCloser)
	err = gw.Close()
	if err == nil || err.Error() != errorMsg {
		t.Fatalf("Expected error %q from Close, got: %v", errorMsg, err)
	}
}

// MockWriteCloser is a helper for testing that implements both io.Writer and io.Closer
type MockWriteCloser struct {
	writeFunc func([]byte) (int, error)
	closeFunc func() error
}

func (m *MockWriteCloser) Write(p []byte) (int, error) {
	return m.writeFunc(p)
}

func (m *MockWriteCloser) Close() error {
	return m.closeFunc()
}

// TestInvalidPaths tests behavior with invalid paths
func TestInvalidPaths(t *testing.T) {
	rootFS := New()

	// Test invalid paths for various operations
	invalidPaths := []string{
		"../escape",
		"/absolute",
		"./path/../../../escape",
		"path//with//double//slash",
		"",
	}

	for _, path := range invalidPaths {
		// Skip empty path as it's handled specially in some cases
		if path == "" {
			continue
		}

		// Test MkdirAll
		err := rootFS.MkdirAll(path, 0755)
		if err == nil {
			t.Fatalf("Expected error for MkdirAll(%q), got nil", path)
		}

		// Test WriteFile
		err = rootFS.WriteFile(path, []byte("test"), 0644)
		if err == nil {
			t.Fatalf("Expected error for WriteFile(%q), got nil", path)
		}

		// Test Open
		_, err = rootFS.Open(path)
		if err == nil {
			t.Fatalf("Expected error for Open(%q), got nil", path)
		}

		// Test Create
		_, err = rootFS.Create(path)
		if err == nil {
			t.Fatalf("Expected error for Create(%q), got nil", path)
		}
	}
}

// Open file for reading using func (rootFS *FS) OpenFile(path string, flag int, perm os.FileMode)
// First write some data to the file using func (fw *FileWriter) Write(p []byte) (n int, err error)
// Close the file using func (fw *FileWriter) Close() error
// Open the file again for reading using func (rootFS *FS) OpenFile(path string, flag int, perm os.FileMode)
// Read the content of the file using io.ReadAll func
func TestFS_OpenFile(t *testing.T) {
	rootFS := New()

	file, err := rootFS.OpenFile("document.txt", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}

	writer, ok := file.(*FileWriter)
	if !ok {
		t.Fatal("Expected *FileWriter")
	}

	_, err = writer.Write([]byte("Initial content"))
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	file, err = rootFS.OpenFile("document.txt", os.O_RDONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}

	reader, ok := file.(fs.File)
	if !ok {
		t.Fatal("Expected fs.File")
	}

	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	if string(content) != "Initial content" {
		t.Fatalf("Expected content %q, got %q", "Initial content", string(content))
	}
}
