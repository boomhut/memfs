package memfs

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"testing"
)

func TestEncryptionBasic(t *testing.T) {
	key := []byte("test-encryption-key-123")
	rootFS := New(WithEncryption(key))

	// Write a file
	testData := []byte("This is secret data that should be encrypted at rest")
	err := rootFS.WriteFile("secret.txt", testData, 0644)
	if err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Verify data is encrypted in memory by directly accessing the File
	child, err := rootFS.get("secret.txt")
	if err != nil {
		t.Fatalf("Failed to get file: %v", err)
	}

	file := child.(*File)
	// The stored content should NOT match the plaintext (it's encrypted)
	if bytes.Equal(file.Content, testData) {
		t.Error("Data is not encrypted at rest - content matches plaintext")
	}

	// Read the file - should get decrypted content
	f, err := rootFS.Open("secret.txt")
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer f.Close()

	readData, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	// The read data should match the original plaintext
	if !bytes.Equal(readData, testData) {
		t.Errorf("Decrypted data doesn't match original.\nExpected: %s\nGot: %s", testData, readData)
	}
}

func TestEncryptionWithFileWriter(t *testing.T) {
	key := []byte("my-secret-key")
	rootFS := New(WithEncryption(key))

	// Create and write using FileWriter
	fw, err := rootFS.Create("data.txt")
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	testData := []byte("Streaming data that needs encryption")
	_, err = fw.Write(testData)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	err = fw.Close()
	if err != nil {
		t.Fatalf("Failed to close: %v", err)
	}

	// Verify encryption at rest
	child, err := rootFS.get("data.txt")
	if err != nil {
		t.Fatalf("Failed to get file: %v", err)
	}

	file := child.(*File)
	if bytes.Equal(file.Content, testData) {
		t.Error("Data is not encrypted at rest")
	}

	// Read back and verify decryption
	f, err := rootFS.Open("data.txt")
	if err != nil {
		t.Fatalf("Failed to open: %v", err)
	}
	defer f.Close()

	readData, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	if !bytes.Equal(readData, testData) {
		t.Errorf("Data mismatch after encryption/decryption")
	}
}

func TestEncryptionWithOpenFile(t *testing.T) {
	key := []byte("openfile-test-key")
	rootFS := New(WithEncryption(key))

	testData := []byte("Test data for OpenFile")

	// Write using OpenFile
	fw, err := rootFS.OpenFile("test.txt", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Failed to open file for writing: %v", err)
	}

	writer := fw.(*FileWriter)
	_, err = writer.Write(testData)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	err = writer.Close()
	if err != nil {
		t.Fatalf("Failed to close: %v", err)
	}

	// Read using OpenFile
	f, err := rootFS.OpenFile("test.txt", os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("Failed to open file for reading: %v", err)
	}

	file := f.(*File)
	defer file.Close()

	readData, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	if !bytes.Equal(readData, testData) {
		t.Errorf("Data mismatch. Expected: %s, Got: %s", testData, readData)
	}
}

func TestEncryptionWithWrongKey(t *testing.T) {
	key1 := []byte("correct-key")
	rootFS := New(WithEncryption(key1))

	testData := []byte("Secret information")
	err := rootFS.WriteFile("secret.txt", testData, 0644)
	if err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Try to read with a different key
	key2 := []byte("wrong-key")
	rootFS2 := New(WithEncryption(key2))

	// Copy the encrypted file content to the new filesystem
	child, _ := rootFS.get("secret.txt")
	file := child.(*File)

	err = rootFS2.MkdirAll(".", 0755)
	if err != nil {
		t.Fatalf("Failed to create dir: %v", err)
	}

	// Manually set the encrypted content (simulating loaded data)
	newFile, err := rootFS2.create("secret.txt")
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	newFile.Content = file.Content

	// Attempt to read should fail with wrong key
	_, err = rootFS2.Open("secret.txt")
	if err == nil {
		t.Error("Expected decryption to fail with wrong key, but it succeeded")
	}
}

func TestEncryptionWithEmptyFile(t *testing.T) {
	key := []byte("test-key")
	rootFS := New(WithEncryption(key))

	// Write empty file
	err := rootFS.WriteFile("empty.txt", []byte{}, 0644)
	if err != nil {
		t.Fatalf("Failed to write empty file: %v", err)
	}

	// Read empty file
	f, err := rootFS.Open("empty.txt")
	if err != nil {
		t.Fatalf("Failed to open empty file: %v", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("Failed to read empty file: %v", err)
	}

	if len(data) != 0 {
		t.Errorf("Expected empty file, got %d bytes", len(data))
	}
}

func TestEncryptionDisabled(t *testing.T) {
	// Create FS without encryption
	rootFS := New()

	testData := []byte("This data should NOT be encrypted")
	err := rootFS.WriteFile("plain.txt", testData, 0644)
	if err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Verify data is NOT encrypted (stored as plaintext)
	child, err := rootFS.get("plain.txt")
	if err != nil {
		t.Fatalf("Failed to get file: %v", err)
	}

	file := child.(*File)
	if !bytes.Equal(file.Content, testData) {
		t.Error("Data should be stored as plaintext when encryption is disabled")
	}

	// Read should also return plaintext
	f, err := rootFS.Open("plain.txt")
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer f.Close()

	readData, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if !bytes.Equal(readData, testData) {
		t.Error("Read data doesn't match original when encryption is disabled")
	}
}

func TestEncryptionWithMultipleFiles(t *testing.T) {
	key := []byte("multi-file-key")
	rootFS := New(WithEncryption(key))

	// Create multiple files
	files := map[string][]byte{
		"file1.txt": []byte("Content of file 1"),
		"file2.txt": []byte("Content of file 2"),
		"file3.txt": []byte("Content of file 3"),
	}

	for path, content := range files {
		err := rootFS.WriteFile(path, content, 0644)
		if err != nil {
			t.Fatalf("Failed to write %s: %v", path, err)
		}
	}

	// Verify all files are encrypted and can be decrypted
	for path, expectedContent := range files {
		// Check encryption at rest
		child, err := rootFS.get(path)
		if err != nil {
			t.Fatalf("Failed to get %s: %v", path, err)
		}

		file := child.(*File)
		if bytes.Equal(file.Content, expectedContent) {
			t.Errorf("%s is not encrypted at rest", path)
		}

		// Check decryption on read
		f, err := rootFS.Open(path)
		if err != nil {
			t.Fatalf("Failed to open %s: %v", path, err)
		}

		readData, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			t.Fatalf("Failed to read %s: %v", path, err)
		}

		if !bytes.Equal(readData, expectedContent) {
			t.Errorf("Content mismatch for %s", path)
		}
	}
}

func TestEncryptionWithSubdirectories(t *testing.T) {
	key := []byte("subdir-key")
	rootFS := New(WithEncryption(key))

	// Create directory structure
	err := rootFS.MkdirAll("dir1/dir2", 0755)
	if err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}

	testData := []byte("Data in subdirectory")
	err = rootFS.WriteFile("dir1/dir2/file.txt", testData, 0644)
	if err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Verify encryption
	child, err := rootFS.get("dir1/dir2/file.txt")
	if err != nil {
		t.Fatalf("Failed to get file: %v", err)
	}

	file := child.(*File)
	if bytes.Equal(file.Content, testData) {
		t.Error("File in subdirectory is not encrypted")
	}

	// Verify decryption
	f, err := rootFS.Open("dir1/dir2/file.txt")
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer f.Close()

	readData, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if !bytes.Equal(readData, testData) {
		t.Error("Decrypted content doesn't match original")
	}
}

func TestEncryptionWithfsReadFile(t *testing.T) {
	key := []byte("fs-readfile-key")
	rootFS := New(WithEncryption(key))

	testData := []byte("Testing with fs.ReadFile")
	err := rootFS.WriteFile("test.txt", testData, 0644)
	if err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Use fs.ReadFile from standard library
	readData, err := fs.ReadFile(rootFS, "test.txt")
	if err != nil {
		t.Fatalf("Failed to read file with fs.ReadFile: %v", err)
	}

	if !bytes.Equal(readData, testData) {
		t.Error("fs.ReadFile returned incorrect data")
	}
}

func TestEncryptionUpdateExistingFile(t *testing.T) {
	key := []byte("update-key")
	rootFS := New(WithEncryption(key))

	// Write initial content
	initialData := []byte("Initial content")
	err := rootFS.WriteFile("update.txt", initialData, 0644)
	if err != nil {
		t.Fatalf("Failed to write initial file: %v", err)
	}

	// Update with new content
	newData := []byte("Updated content with different length")
	err = rootFS.WriteFile("update.txt", newData, 0644)
	if err != nil {
		t.Fatalf("Failed to update file: %v", err)
	}

	// Read and verify
	f, err := rootFS.Open("update.txt")
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer f.Close()

	readData, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if !bytes.Equal(readData, newData) {
		t.Errorf("Updated content doesn't match. Expected: %s, Got: %s", newData, readData)
	}

	// Verify old content is not there
	if bytes.Equal(readData, initialData) {
		t.Error("File still contains old content")
	}
}

func TestEncryptionWithSaveLoad(t *testing.T) {
	key := []byte("save-load-key")
	rootFS := New(WithEncryption(key))

	// Write encrypted data
	testData := []byte("Encrypted data to persist")
	err := rootFS.WriteFile("persistent.txt", testData, 0644)
	if err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Save to temporary file
	tmpfile, err := os.CreateTemp("", "memfs-enc-test-*.gob")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	err = rootFS.SaveToFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to save filesystem: %v", err)
	}

	// Load filesystem without encryption key
	// Note: The data is still encrypted in the saved file
	loadedFS, err := LoadFromFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load filesystem: %v", err)
	}

	// Since we loaded without the encryption key, the data should still be encrypted
	// and reading should fail or return encrypted data
	child, err := loadedFS.get("persistent.txt")
	if err != nil {
		t.Fatalf("Failed to get file: %v", err)
	}

	file := child.(*File)
	// The stored content should be encrypted (not equal to plaintext)
	if bytes.Equal(file.Content, testData) {
		t.Error("Data should remain encrypted after save/load without encryption key")
	}

	// If we try to read with the loaded FS (no encryption), we should get encrypted data
	f, err := loadedFS.Open("persistent.txt")
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer f.Close()

	readData, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	// Since loadedFS has no encryption, it returns the raw (encrypted) content
	if bytes.Equal(readData, testData) {
		t.Error("Should not be able to read plaintext without the encryption key")
	}
}

func TestEncryptionPersistsToDisk(t *testing.T) {
	key := []byte("disk-persistence-key")
	rootFS := New(WithEncryption(key))

	// Write sensitive data
	secretData := []byte("Super secret password: admin123")
	err := rootFS.WriteFile("secrets.txt", secretData, 0644)
	if err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Save to disk
	tmpfile, err := os.CreateTemp("", "memfs-disk-enc-*.gob")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	err = rootFS.SaveToFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to save filesystem: %v", err)
	}

	// Read the raw file content from disk
	diskData, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to read disk file: %v", err)
	}

	// Verify that plaintext does NOT appear in the disk file
	if bytes.Contains(diskData, secretData) {
		t.Error("Plaintext data found in disk file - encryption not persisted!")
	}

	// Verify specific secret parts are not visible
	if bytes.Contains(diskData, []byte("admin123")) {
		t.Error("Secret password found in plaintext on disk!")
	}

	// Now load WITH the encryption key and verify we can read it
	loadedFS2, err := LoadFromFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load from file: %v", err)
	}

	// Set the encryption key to decrypt the content
	err = loadedFS2.SetEncryptionKey(key)
	if err != nil {
		t.Fatalf("Failed to set encryption key: %v", err)
	}

	// Now we should be able to read the decrypted content
	f, err := loadedFS2.Open("secrets.txt")
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer f.Close()

	readData, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if !bytes.Equal(readData, secretData) {
		t.Errorf("Decrypted content doesn't match. Expected: %s, Got: %s", secretData, readData)
	}
}

func TestEncryptionWithCompressedSave(t *testing.T) {
	key := []byte("compressed-save-key")
	rootFS := New(WithEncryption(key))

	// Write data
	testData := []byte("Sensitive data in compressed file")
	err := rootFS.WriteFile("data.txt", testData, 0644)
	if err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Save compressed to disk
	tmpfile, err := os.CreateTemp("", "memfs-compressed-enc-*.gob.gz")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	err = rootFS.CompressAndSaveToFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to save compressed filesystem: %v", err)
	}

	// Read raw disk data
	diskData, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to read disk file: %v", err)
	}

	// Verify plaintext is not in compressed file
	if bytes.Contains(diskData, testData) {
		t.Error("Plaintext found in compressed disk file!")
	}

	// Load and verify with encryption
	loadedFS2, err := DecompressAndLoadFromFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load compressed file: %v", err)
	}

	err = loadedFS2.SetEncryptionKey(key)
	if err != nil {
		t.Fatalf("Failed to set encryption key: %v", err)
	}

	f2, err := loadedFS2.Open("data.txt")
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	content, err := io.ReadAll(f2)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if !bytes.Equal(content, testData) {
		t.Errorf("Content mismatch after compressed save/load")
	}
}
