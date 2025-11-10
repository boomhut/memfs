package main

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/boomhut/memfs"
)

func main() {
	// Example 1: Basic encryption usage
	fmt.Println("=== Example 1: Basic Encryption ===")
	encryptionKey := []byte("my-secret-key-123")
	encryptedFS := memfs.New(memfs.WithEncryption(encryptionKey))

	// Write sensitive data
	err := encryptedFS.WriteFile("passwords.txt", []byte("admin:secret123"), 0644)
	if err != nil {
		panic(err)
	}

	// Read the data back (automatically decrypted)
	content, err := fs.ReadFile(encryptedFS, "passwords.txt")
	if err != nil {
		panic(err)
	}
	fmt.Printf("Decrypted content: %s\n", content)

	// Example 2: Encrypted persistence to disk
	fmt.Println("\n=== Example 2: Encrypted Persistence to Disk ===")
	testData := []byte("This is secret information")
	encryptedFS.WriteFile("secret.txt", testData, 0644)

	// Save to disk - file contents remain encrypted
	err = encryptedFS.SaveToFile("encrypted_fs.gob")
	if err != nil {
		panic(err)
	}
	fmt.Println("✓ Filesystem saved to disk")

	// Read raw file from disk
	diskData, err := os.ReadFile("encrypted_fs.gob")
	if err != nil {
		panic(err)
	}

	// Verify plaintext is NOT in the disk file
	if !bytes.Contains(diskData, testData) {
		fmt.Println("✓ Verified: Plaintext data is NOT readable in disk file")
	} else {
		fmt.Println("✗ Warning: Plaintext found on disk!")
	}

	// Load filesystem without encryption key
	loadedPlain, _ := memfs.LoadFromFile("encrypted_fs.gob")

	// Try to read - will get encrypted data
	encFile, _ := loadedPlain.Open("secret.txt")
	encContent, _ := io.ReadAll(encFile)
	encFile.Close()

	if !bytes.Equal(encContent, testData) {
		fmt.Println("✓ Without key: Cannot read plaintext (got encrypted data)")
	}

	// Load with encryption key - data is accessible
	loadedFS2, _ := memfs.LoadFromFile("encrypted_fs.gob")
	loadedFS2.SetEncryptionKey(encryptionKey) // Reattach encryptor with same key

	decContent, _ := fs.ReadFile(loadedFS2, "secret.txt")
	if bytes.Equal(decContent, testData) {
		fmt.Printf("✓ With key: Successfully decrypted: %s\n", decContent)
	}

	// Cleanup
	os.Remove("encrypted_fs.gob")

	// Example 3: Multiple files with encryption
	fmt.Println("\n=== Example 3: Multiple Files ===")
	files := map[string]string{
		"user1.txt": "User 1 secret data",
		"user2.txt": "User 2 confidential info",
		"user3.txt": "User 3 private notes",
	}

	for filename, content := range files {
		err := encryptedFS.WriteFile(filename, []byte(content), 0644)
		if err != nil {
			panic(err)
		}
	}

	// Read all files back
	for filename := range files {
		content, err := fs.ReadFile(encryptedFS, filename)
		if err != nil {
			panic(err)
		}
		fmt.Printf("%s: %s\n", filename, content)
	}

	// Example 4: Filesystem without encryption (for comparison)
	fmt.Println("\n=== Example 4: No Encryption ===")
	plainFS := memfs.New()
	plainFS.WriteFile("plain.txt", []byte("Not encrypted"), 0644)

	plainContent, _ := fs.ReadFile(plainFS, "plain.txt")
	fmt.Printf("Plain storage: %s\n", plainContent)

	fmt.Println("\n=== Done ===")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
