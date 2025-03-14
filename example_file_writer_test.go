package memfs_test

import (
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/boomhut/memfs"
)

func ExampleFS_Create() {
	// Create a new in-memory filesystem
	rootFS := memfs.New()

	// Create a new file for writing
	fw, err := rootFS.Create("example.txt")
	if err != nil {
		panic(err)
	}

	// Write some data to the file
	data := []byte("Hello, MemFS writer!")
	_, err = fw.Write(data)
	if err != nil {
		panic(err)
	}

	// Close the file when done writing
	if err := fw.Close(); err != nil {
		panic(err)
	}

	// Now read the file back
	file, err := rootFS.Open("example.txt")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		panic(err)
	}

	fmt.Println(string(content))
	// Output: Hello, MemFS writer!
}

func ExampleFS_OpenFile() {
	// Create a new in-memory filesystem
	rootFS := memfs.New()

	// Create a new file using OpenFile
	file, err := rootFS.OpenFile("document.txt", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}

	// Cast to FileWriter to use Write method
	writer, ok := file.(*memfs.FileWriter)
	if !ok {
		panic("Expected *FileWriter")
	}

	_, err = writer.Write([]byte("Initial content"))
	if err != nil {
		panic(err)
	}
	if err := writer.Close(); err != nil {
		panic(err)
	}

	// Open the same file again, but with truncate flag
	file, err = rootFS.OpenFile("document.txt", os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		panic(err)
	}

	writer, ok = file.(*memfs.FileWriter)
	if !ok {
		panic("Expected *FileWriter")
	}

	_, err = writer.Write([]byte("Updated content"))
	if err != nil {
		panic(err)
	}
	if err := writer.Close(); err != nil {
		panic(err)
	}

	// Read the final content
	content, err := fs.ReadFile(rootFS, "document.txt")
	if err != nil {
		panic(err)
	}

	fmt.Println(string(content))
	// Output: Updated content
}
