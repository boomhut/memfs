package memfs_test

import (
	"fmt"
	"io/fs"

	"github.com/boomhut/memfs"
)

func ExampleNew() {
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
}

// Test for the reader and writer implementations
func ExampleMemFS() {
	rootFS := memfs.New()

	err := rootFS.WriteFile("f1.txt", []byte("incinerating-unsubstantial"), 0755)
	if err != nil {
		panic(err)
	}

	content, err := fs.ReadFile(rootFS, "f1.txt")
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s\n", content)

	err = rootFS.WriteFile("f2.txt", []byte("unsubstantial-incinerating"), 0755)
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
}
