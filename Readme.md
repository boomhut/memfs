# memfs: A simple in-memory io/fs.FS filesystem

memfs is an in-memory implementation of Go's io/fs.FS interface.
The goal is to make it easy and quick to build an fs.FS filesystem
when you don't have any complex requirements.

Documentation: https://pkg.go.dev/github.com/boomhut/memfs

`io/fs` docs: https://tip.golang.org/pkg/io/fs/

## Features

- ✅ In-memory filesystem implementing `io/fs.FS`
- ✅ **Encryption at rest** using AES-256-GCM
- ✅ Compression support with gzip
- ✅ Storage limits
- ✅ Save/load to disk
- ✅ Thread-safe operations
- ✅ Open hooks for custom file handling

## Usage

```
package main

import (
	"fmt"
	"io/fs"

	"github.com/boomhut/memfs"
)

func main() {
	rootFS := memfs.New()

	err := rootFS.MkdirAll("dir1/dir2", 0777)
	if err != nil {
		panic(err)
	}

	err = rootFS.WriteFile("dir1/dir2/f1.txt", []byte("incinerating-unsubstantial"), 0755)
	if err != nil {
		panic(err)
	}

	err = fs.WalkDir(rootFS, ".", func(path string, d fs.DirEntry, err error) error {
		fmt.Println(path)
		return nil
	})
	if err != nil {
		panic(err)
	}

	content, err := fs.ReadFile(rootFS, "dir1/dir2/f1.txt")
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s\n", content)

	// Example saving FS to file
	err = rootFS.SaveToFile(tmpfile.Name())
	if err != nil {
		panic(err)
	}

	// Load the filesystem back
	loadedFS, err := LoadFromFile(tmpfile.Name())
	if err != nil {
		panic(err)
	}

}
```

## Encryption at Rest

memfs supports transparent encryption at rest using AES-256-GCM. All file contents are automatically encrypted when written and decrypted when read.

```go
package main

import (
"fmt"
"io/fs"

"github.com/boomhut/memfs"
)

func main() {
// Create filesystem with encryption
encryptionKey := []byte("your-secret-encryption-key")
rootFS := memfs.New(memfs.WithEncryption(encryptionKey))

// Write sensitive data - it will be encrypted at rest
err := rootFS.WriteFile("secrets.txt", []byte("sensitive data"), 0644)
if err != nil {
panic(err)
}

// Read the file - content is automatically decrypted
content, err := fs.ReadFile(rootFS, "secrets.txt")
if err != nil {
panic(err)
}
fmt.Printf("%s\n", content) // Outputs: sensitive data
}
```

**Important Notes:**
- The encryption key can be of any length (it will be hashed to 32 bytes for AES-256)
- All file contents are encrypted at rest in memory **and on disk when saved**
- Encryption/decryption is transparent to the application
- The encryption key is NOT persisted when saving the filesystem to disk
- You must provide the same encryption key when loading an encrypted filesystem
- Directory names and file metadata (names, permissions) are not encrypted, only file contents
- Uses AES-256-GCM which provides both encryption and authentication

### Encryption with Save/Load

When you save an encrypted filesystem to disk, the encrypted file contents are persisted:

```go
key := []byte("my-encryption-key")
fs := memfs.New(memfs.WithEncryption(key))

// Write encrypted data
fs.WriteFile("secrets.txt", []byte("sensitive information"), 0644)

// Save to disk - file contents remain encrypted on disk
fs.SaveToFile("filesystem.gob")

// Load back - you need the same key to decrypt
loadedFS, _ := memfs.LoadFromFile("filesystem.gob")
loadedFS.SetEncryptionKey(key) // Reattach the same encryption key

// Now you can read the decrypted content
content, _ := fs.ReadFile(loadedFS, "secrets.txt")
// content = "sensitive information"
```

**Security Note**: Without the encryption key, the file contents in the saved file remain encrypted and unreadable.
