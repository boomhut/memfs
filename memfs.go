package memfs

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	syspath "path"
	"strings"
	"sync"
	"time"
)

// FS is an in-memory filesystem that implements
// io/fs.FS
type FS struct {
	dir         *Dir
	openHook    func(path string, existingContent []byte, origErr error) ([]byte, error)
	maxStorage  int64      // maximum storage limit in bytes
	usedStorage int64      // current storage usage in bytes
	mu          sync.Mutex // mutex for storage tracking
	encryptor   *encryptor // encryptor for data at rest encryption
}

// New creates a new in-memory FileSystem. It accepts options to customize the filesystem. The options are: openHook, maxStorage, and encryption.
// Set like this: memfs.New(memfs.WithMaxStorage(1000)), memfs.New(memfs.WithOpenHook(myOpenHook)), or memfs.New(memfs.WithEncryption(key))
func New(opts ...Option) *FS {
	var fsOpt fsOption
	for _, opt := range opts {
		opt.setOption(&fsOpt)
	}

	// Initialize encryptor if encryption key is provided
	enc, err := newEncryptor(fsOpt.encryptionKey)
	if err != nil {
		// If encryptor initialization fails, create a disabled encryptor
		enc = &encryptor{enable: false}
	}

	fs := FS{
		dir: &Dir{
			Children: make(map[string]childI),
		},
		maxStorage: -1, // -1 means unlimited
		encryptor:  enc,
	}

	fs.openHook = fsOpt.openHook
	fs.maxStorage = fsOpt.maxStorage

	return &fs
}

// SetEncryptionKey sets or updates the encryption key for the filesystem.
// This is useful when loading an encrypted filesystem from disk - you need to
// provide the same key that was used when the data was encrypted.
func (rootFS *FS) SetEncryptionKey(key []byte) error {
	enc, err := newEncryptor(key)
	if err != nil {
		return err
	}
	rootFS.encryptor = enc
	return nil
}

// MkdirAll creates a directory named path,
// along with any necessary parents, and returns nil,
// or else returns an error.
// The permission bits perm (before umask) are used for all
// directories that MkdirAll creates.
// If path is already a directory, MkdirAll does nothing
// and returns nil.
func (rootFS *FS) MkdirAll(path string, perm os.FileMode) error {
	if !fs.ValidPath(path) {
		return fmt.Errorf("invalid path: %s: %w", path, fs.ErrInvalid)
	}

	if path == "." {
		// root dir always exists
		return nil
	}

	parts := strings.Split(path, "/")

	next := rootFS.dir
	for _, part := range parts {
		cur := next
		cur.mu.Lock()
		child := cur.Children[part]
		if child == nil {
			newDir := &Dir{
				Name:     part,
				Perm:     perm,
				Children: make(map[string]childI),
			}
			cur.Children[part] = newDir
			next = newDir
		} else {
			childDir, ok := child.(*Dir)
			if !ok {
				return fmt.Errorf("not a directory: %s: %w", part, fs.ErrInvalid)
			}
			next = childDir
		}
		cur.mu.Unlock()
	}

	return nil
}

func (rootFS *FS) getDir(path string) (*Dir, error) {
	if path == "" {
		return rootFS.dir, nil
	}
	parts := strings.Split(path, "/")

	cur := rootFS.dir
	for _, part := range parts {
		err := func() error {
			cur.mu.Lock()
			defer cur.mu.Unlock()
			child := cur.Children[part]
			if child == nil {
				return fmt.Errorf("not a directory: %s: %w", part, fs.ErrNotExist)
			} else {
				childDir, ok := child.(*Dir)
				if !ok {
					return fmt.Errorf("no such file or directory: %s: %w", part, fs.ErrNotExist)
				}
				cur = childDir
			}
			return nil
		}()
		if err != nil {
			return nil, err
		}
	}

	return cur, nil
}

func (rootFS *FS) get(path string) (childI, error) {
	if path == "" {
		return rootFS.dir, nil
	}

	parts := strings.Split(path, "/")

	var (
		cur = rootFS.dir

		chld childI
		err  error
	)
	for i, part := range parts {
		chld, err = func() (childI, error) {
			cur.mu.Lock()
			defer cur.mu.Unlock()
			child := cur.Children[part]
			if child == nil {
				return nil, fmt.Errorf("not a directory: %s: %w", part, fs.ErrNotExist)
			} else {
				_, isFile := child.(*File)
				if isFile {
					if i == len(parts)-1 {
						return child, nil
					} else {
						return nil, fmt.Errorf("no such file or directory: %s: %w", part, fs.ErrNotExist)
					}
				}

				childDir, ok := child.(*Dir)
				if !ok {
					return nil, errors.New("not a directory")
				}
				cur = childDir
			}
			return child, nil
		}()
		if err != nil {
			return nil, err
		}
	}

	return chld, nil
}

func (rootFS *FS) create(path string) (*File, error) {
	if !fs.ValidPath(path) {
		return nil, fmt.Errorf("invalid path: %s: %w", path, fs.ErrInvalid)
	}

	if path == "." {
		// root dir
		path = ""
	}

	dirPart, filePart := syspath.Split(path)

	dirPart = strings.TrimSuffix(dirPart, "/")
	dir, err := rootFS.getDir(dirPart)
	if err != nil {
		return nil, err
	}

	dir.mu.Lock()
	defer dir.mu.Unlock()
	existing := dir.Children[filePart]
	if existing != nil {
		_, ok := existing.(*File)
		if !ok {
			return nil, fmt.Errorf("path is a directory: %s: %w", path, fs.ErrExist)
		}
	}

	newFile := &File{
		Name: filePart,
		Perm: 0666,
	}
	dir.Children[filePart] = newFile

	return newFile, nil
}

// WriteFile writes data to a file named by filename.
// If the file does not exist, WriteFile creates it with permissions perm
// (before umask); otherwise WriteFile truncates it before writing, without changing permissions.
func (rootFS *FS) WriteFile(path string, data []byte, perm os.FileMode) error {
	if !fs.ValidPath(path) {
		return fmt.Errorf("invalid path: %s: %w", path, fs.ErrInvalid)
	}

	// Encrypt data before storing if encryption is enabled
	encryptedData := data
	if rootFS.encryptor != nil {
		var err error
		encryptedData, err = rootFS.encryptor.encrypt(data)
		if err != nil {
			return fmt.Errorf("encryption failed: %w", err)
		}
	}

	rootFS.mu.Lock()
	if rootFS.maxStorage > 0 {
		newSize := rootFS.usedStorage + int64(len(encryptedData))
		if newSize > rootFS.maxStorage {
			rootFS.mu.Unlock()
			return fmt.Errorf("storage limit exceeded: %w", fs.ErrInvalid)
		}
	}
	rootFS.mu.Unlock()

	if path == "." {
		// root dir
		path = ""
	}

	f, err := rootFS.create(path)
	if err != nil {
		return err
	}

	rootFS.mu.Lock()
	if rootFS.maxStorage > 0 {
		// Subtract old file size and add new file size (using encrypted size)
		rootFS.usedStorage -= int64(len(f.Content))
		rootFS.usedStorage += int64(len(encryptedData))
	}
	rootFS.mu.Unlock()

	f.Content = encryptedData
	f.Perm = perm
	return nil
}

// Open opens the named file.
func (rootFS *FS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{
			Op:   "open",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}

	child, err := rootFS.open(name)
	if rootFS.openHook != nil {
		var exitingContent []byte
		if child != nil {
			stat, _ := child.Stat()
			if stat.Mode().IsDir() {
				return child, err
			}

			// Note: exitingContent is already decrypted by open() method
			exitingContent, err = io.ReadAll(child)
			if err != nil {
				return nil, err
			}
			newContent, err := rootFS.openHook(name, exitingContent, err)
			if err != nil {
				return nil, err
			}
			f := child.(*File)
			f.Content = newContent
			f.reader = bytes.NewReader(newContent)
			return f, nil
		}
	}
	return child, err
}

func (rootFS *FS) open(name string) (fs.File, error) {
	if name == "." {
		// root dir
		name = ""
	}

	child, err := rootFS.get(name)
	if err != nil {
		return nil, err
	}

	switch cc := child.(type) {
	case *File:
		// Decrypt content if encryption is enabled
		content := cc.Content
		if rootFS.encryptor != nil && rootFS.encryptor.enable {
			decryptedContent, err := rootFS.encryptor.decrypt(cc.Content)
			if err != nil {
				return nil, fmt.Errorf("decryption failed: %w", err)
			}
			content = decryptedContent
		}

		handle := &File{
			Name:    cc.Name,
			Perm:    cc.Perm,
			Content: content,
			reader:  bytes.NewReader(content),
			ModTime: cc.ModTime,
		}
		return handle, nil
	case *Dir:
		handle := &fhDir{
			dir: cc,
		}
		return handle, nil
	}

	return nil, fmt.Errorf("unexpected file type in fs: %s: %w", name, fs.ErrInvalid)
}

// Sub returns an FS corresponding to the subtree rooted at path.
func (rootFS *FS) Sub(path string) (fs.FS, error) {
	dir, err := rootFS.getDir(path)
	if err != nil {
		return nil, err
	}
	return &FS{dir: dir}, nil
}

// SaveToFile saves the entire filesystem structure to a GOB encoded file
func (rootFS *FS) SaveToFile(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	return rootFS.SaveTo(f)
}

// SaveTo saves the filesystem structure to any io.Writer in GOB format
func (rootFS *FS) SaveTo(w io.Writer) error {
	encoder := gob.NewEncoder(w)
	return encoder.Encode(rootFS.dir)
}

// CompressAndSaveToFile saves the entire filesystem structure to a GOB encoded file after compressing the data using gzip
func (rootFS *FS) CompressAndSaveToFile(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	return rootFS.CompressAndSaveTo(f)
}

// CompressAndSaveTo saves the filesystem structure to any io.Writer in GOB format after compressing the data using gzip
func (rootFS *FS) CompressAndSaveTo(w io.Writer) error {
	// Create a gzip writer
	gw := NewGzipWriter(w)
	defer gw.Close()

	// Encode and save the filesystem
	encoder := gob.NewEncoder(gw)
	return encoder.Encode(rootFS.dir)
}

// DecompressAndLoadFromFile loads the entire filesystem structure from a GOB encoded file after decompressing the data using gzip
func DecompressAndLoadFromFile(filename string) (*FS, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return DecompressAndLoadFrom(f)
}

// DecompressAndLoadFrom loads the filesystem structure from any io.Reader in GOB format after decompressing the data using gzip
func DecompressAndLoadFrom(r io.Reader) (*FS, error) {
	// Create a gzip reader
	gr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer gr.Close()

	// Decode and load the filesystem
	var rootDir Dir
	decoder := gob.NewDecoder(gr)
	if err := decoder.Decode(&rootDir); err != nil {
		return nil, err
	}

	// Initialize mutexes after loading
	rootDir.initDir()

	// Initialize a disabled encryptor (encryption key not persisted)
	enc := &encryptor{enable: false}

	// Create new FS with loaded directory structure
	fs := &FS{
		dir:        &rootDir,
		maxStorage: -1, // Default to unlimited
		encryptor:  enc,
	}

	return fs, nil
}

// init registers types for GOB encoding/decoding
func init() {
	gob.Register(&Dir{})
	gob.Register(&File{})
}

// LoadFromFile creates a new FS by loading from a GOB encoded file
func LoadFromFile(filename string) (*FS, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return LoadFrom(f)
}

// LoadFrom creates a new FS by loading from a GOB encoded reader
func LoadFrom(r io.Reader) (*FS, error) {
	var rootDir Dir
	decoder := gob.NewDecoder(r)
	if err := decoder.Decode(&rootDir); err != nil {
		return nil, err
	}

	// Initialize mutexes after loading
	rootDir.initDir()

	// Initialize a disabled encryptor (encryption key not persisted)
	enc := &encryptor{enable: false}

	// Create new FS with loaded directory structure
	fs := &FS{
		dir:        &rootDir,
		maxStorage: -1, // Default to unlimited
		encryptor:  enc,
	}

	return fs, nil
}

// Dir represents a directory in the filesystem
type Dir struct {
	mu       sync.Mutex `json:"-"` // Unexported, won't be serialized
	Name     string
	Perm     os.FileMode
	ModTime  time.Time
	Children map[string]childI
}

// initDir initializes a directory after loading
func (d *Dir) initDir() {
	// Initialize children directories recursively
	for _, child := range d.Children {
		if dir, ok := child.(*Dir); ok {
			dir.initDir()
		}
	}
}

type fhDir struct {
	dir *Dir
	idx int
}

func (d *fhDir) Stat() (fs.FileInfo, error) {
	fi := fileInfo{
		name:    d.dir.Name,
		size:    4096,
		modTime: d.dir.ModTime,
		mode:    d.dir.Perm | fs.ModeDir,
	}
	return &fi, nil
}

func (d *fhDir) Read(b []byte) (int, error) {
	return 0, errors.New("is a directory")
}

func (d *fhDir) Close() error {
	return nil
}

func (d *fhDir) ReadDir(n int) ([]fs.DirEntry, error) {
	d.dir.mu.Lock()
	defer d.dir.mu.Unlock()

	names := make([]string, 0, len(d.dir.Children))
	for name := range d.dir.Children {
		names = append(names, name)
	}

	// directory already exhausted
	if n <= 0 && d.idx >= len(names) {
		return nil, nil
	}

	// read till end
	var err error
	if n > 0 && d.idx+n > len(names) {
		err = io.EOF
		if d.idx > len(names) {
			return nil, err
		}
	}

	if n <= 0 {
		n = len(names)
	}

	out := make([]fs.DirEntry, 0, n)

	for i := d.idx; i < n && i < len(names); i++ {
		name := names[i]
		child := d.dir.Children[name]

		f, isFile := child.(*File)
		if isFile {
			stat, _ := f.Stat()
			out = append(out, &dirEntry{
				info: stat,
			})
		} else {
			d := child.(*Dir)
			fi := fileInfo{
				name:    d.Name,
				size:    4096,
				modTime: d.ModTime,
				mode:    d.Perm | fs.ModeDir,
			}
			out = append(out, &dirEntry{
				info: &fi,
			})
		}

		d.idx = i + 1
	}

	return out, err
}

type File struct {
	Name    string
	Perm    os.FileMode
	Content []byte
	reader  *bytes.Reader `json:"-"` // Unexported, won't be serialized
	ModTime time.Time
	closed  bool `json:"-"` // Unexported, won't be serialized
}

func (f *File) Stat() (fs.FileInfo, error) {
	if f.closed {
		return nil, fs.ErrClosed
	}
	fi := fileInfo{
		name:    f.Name,
		size:    int64(len(f.Content)),
		modTime: f.ModTime,
		mode:    f.Perm,
	}
	return &fi, nil
}

func (f *File) Read(b []byte) (int, error) {
	if f.closed {
		return 0, fs.ErrClosed
	}
	return f.reader.Read(b)
}

func (f *File) Seek(offset int64, whence int) (int64, error) {
	if f.closed {
		return 0, fs.ErrClosed
	}

	return f.reader.Seek(offset, whence)
}

func (f *File) Close() error {
	if f.closed {
		return fs.ErrClosed
	}
	f.closed = true
	return nil
}

// Create creates or truncates the named file. If the file already exists,
// it is truncated. If the file does not exist, it is created with mode 0666.
// The handle returned is open for writing.
func (rootFS *FS) Create(path string) (*FileWriter, error) {
	file, err := rootFS.create(path)
	if err != nil {
		return nil, err
	}

	// Reset content for new/truncated file
	rootFS.mu.Lock()
	if rootFS.maxStorage > 0 {
		rootFS.usedStorage -= int64(len(file.Content))
	}
	rootFS.mu.Unlock()

	file.Content = []byte{}
	file.ModTime = time.Now()

	return &FileWriter{
		file: file,
		fs:   rootFS,
	}, nil
}

// FileWriter is a handle to write to a file in the memory filesystem
type FileWriter struct {
	file   *File
	fs     *FS
	closed bool
}

// Write writes data to the file
func (fw *FileWriter) Write(p []byte) (n int, err error) {
	if fw.closed {
		return 0, fs.ErrClosed
	}

	fw.fs.mu.Lock()
	defer fw.fs.mu.Unlock()

	// Check if the write would exceed the maximum storage limit
	if fw.fs.maxStorage > 0 {
		// Only count the actual new bytes being added
		newSize := fw.fs.usedStorage + int64(len(p))
		if newSize > fw.fs.maxStorage {
			return 0, fmt.Errorf("storage limit exceeded: %w", fs.ErrInvalid)
		}
		fw.fs.usedStorage += int64(len(p))
	}

	// Note: For streaming writes, we append plaintext and will encrypt on Close
	// This is because encryption with AES-GCM needs the complete data
	fw.file.Content = append(fw.file.Content, p...)
	fw.file.ModTime = time.Now()
	return len(p), nil
}

// Close closes the file writer
func (fw *FileWriter) Close() error {
	if fw.closed {
		return fs.ErrClosed
	}
	fw.closed = true

	// Encrypt the content before finalizing if encryption is enabled
	if fw.fs.encryptor != nil && fw.fs.encryptor.enable {
		plaintext := fw.file.Content
		encryptedData, err := fw.fs.encryptor.encrypt(plaintext)
		if err != nil {
			return fmt.Errorf("encryption failed on close: %w", err)
		}

		// Update storage accounting for the difference in size
		fw.fs.mu.Lock()
		if fw.fs.maxStorage > 0 {
			sizeDiff := int64(len(encryptedData)) - int64(len(plaintext))
			fw.fs.usedStorage += sizeDiff
		}
		fw.fs.mu.Unlock()

		fw.file.Content = encryptedData
	}

	// Update the reader in case the file is also open for reading
	fw.file.reader = bytes.NewReader(fw.file.Content)
	return nil
}

// OpenFile opens a file with specified flag and permission
// The flag values are similar to os.OpenFile
func (rootFS *FS) OpenFile(path string, flag int, perm os.FileMode) (interface{}, error) {
	// First, check if path is valid
	if !fs.ValidPath(path) {
		return nil, fmt.Errorf("invalid path: %s: %w", path, fs.ErrInvalid)
	}

	// Handle creating a new file
	if flag&os.O_CREATE != 0 {
		// Try to get the file first
		child, err := rootFS.get(path)

		// File doesn't exist
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				// Create new file
				file, err := rootFS.create(path)
				if err != nil {
					return nil, err
				}

				rootFS.mu.Lock()
				if rootFS.maxStorage > 0 {
					rootFS.usedStorage -= int64(len(file.Content))
				}
				file.Content = []byte{}
				file.ModTime = time.Now()
				rootFS.mu.Unlock()

				if flag&os.O_WRONLY != 0 || flag&os.O_RDWR != 0 {
					return &FileWriter{
						file: file,
						fs:   rootFS,
					}, nil
				} else {
					// Create but only for reading (unusual case)
					file.reader = bytes.NewReader(file.Content)
					return file, nil
				}
			}
			return nil, err
		}

		// File exists
		file, isFile := child.(*File)
		if !isFile {
			return nil, fmt.Errorf("path is a directory: %s: %w", path, fs.ErrInvalid)
		}

		if flag&os.O_TRUNC != 0 && (flag&os.O_WRONLY != 0 || flag&os.O_RDWR != 0) {
			// Truncate the file
			rootFS.mu.Lock()
			if rootFS.maxStorage > 0 {
				rootFS.usedStorage -= int64(len(file.Content))
			}
			file.Content = []byte{}
			file.ModTime = time.Now()
			rootFS.mu.Unlock()
		}

		if flag&os.O_WRONLY != 0 || flag&os.O_RDWR != 0 {
			// For write mode, we need to decrypt first if file has content
			if rootFS.encryptor != nil && rootFS.encryptor.enable && len(file.Content) > 0 {
				decryptedContent, err := rootFS.encryptor.decrypt(file.Content)
				if err != nil {
					return nil, fmt.Errorf("decryption failed: %w", err)
				}
				// Update to decrypted content for write operations
				file.Content = decryptedContent
			}
			return &FileWriter{
				file: file,
				fs:   rootFS,
			}, nil
		} else {
			// Open for reading only - decrypt the content
			content := file.Content
			if rootFS.encryptor != nil && rootFS.encryptor.enable && len(content) > 0 {
				decryptedContent, err := rootFS.encryptor.decrypt(content)
				if err != nil {
					return nil, fmt.Errorf("decryption failed: %w", err)
				}
				content = decryptedContent
			}
			handle := &File{
				Name:    file.Name,
				Perm:    file.Perm,
				Content: content,
				reader:  bytes.NewReader(content),
				ModTime: file.ModTime,
			}
			return handle, nil
		}
	}

	// Handle reading an existing file without creation
	if flag == os.O_RDONLY {
		return rootFS.Open(path)
	}

	// Handle existing file with write permissions
	child, err := rootFS.get(path)
	if err != nil {
		return nil, err
	}

	file, isFile := child.(*File)
	if !isFile {
		return nil, fmt.Errorf("path is a directory: %s: %w", path, fs.ErrInvalid)
	}

	if flag&os.O_TRUNC != 0 && (flag&os.O_WRONLY != 0 || flag&os.O_RDWR != 0) {
		// Truncate the file
		rootFS.mu.Lock()
		if rootFS.maxStorage > 0 {
			rootFS.usedStorage -= int64(len(file.Content))
		}
		file.Content = []byte{}
		file.ModTime = time.Now()
		rootFS.mu.Unlock()
	}

	if flag&os.O_WRONLY != 0 || flag&os.O_RDWR != 0 {
		return &FileWriter{
			file: file,
			fs:   rootFS,
		}, nil
	}

	// Default to opening for reading
	return rootFS.Open(path)
}

// Remove deletes a file or empty directory from the filesystem.
// If the path refers to a non-empty directory, an error is returned.
func (rootFS *FS) Remove(path string) error {
	if !fs.ValidPath(path) {
		return fmt.Errorf("invalid path: %s: %w", path, fs.ErrInvalid)
	}

	if path == "." {
		return fmt.Errorf("cannot remove root directory: %w", fs.ErrInvalid)
	}

	dirPart, filePart := syspath.Split(path)
	dirPart = strings.TrimSuffix(dirPart, "/")

	dir, err := rootFS.getDir(dirPart)
	if err != nil {
		return err
	}

	dir.mu.Lock()
	defer dir.mu.Unlock()

	child, exists := dir.Children[filePart]
	if !exists {
		return fmt.Errorf("no such file or directory: %s: %w", path, fs.ErrNotExist)
	}

	// If it's a directory, check if it's empty
	if childDir, ok := child.(*Dir); ok {
		childDir.mu.Lock()
		isEmpty := len(childDir.Children) == 0
		childDir.mu.Unlock()

		if !isEmpty {
			return fmt.Errorf("directory not empty: %s", path)
		}
	}

	// If it's a file, adjust the storage usage
	if file, ok := child.(*File); ok {
		rootFS.mu.Lock()
		if rootFS.maxStorage > 0 {
			rootFS.usedStorage -= int64(len(file.Content))
		}
		rootFS.mu.Unlock()
	}

	// Remove the entry
	delete(dir.Children, filePart)
	return nil
}

// RemoveAll removes path and any children it contains.
// It removes everything it can but returns the first error it encounters.
// If the path does not exist, RemoveAll returns nil (no error).
func (rootFS *FS) RemoveAll(path string) error {
	if !fs.ValidPath(path) {
		return fmt.Errorf("invalid path: %s: %w", path, fs.ErrInvalid)
	}

	if path == "." {
		// Special case: clear entire filesystem but keep root dir
		rootFS.dir.mu.Lock()

		// Adjust storage counters
		if rootFS.maxStorage > 0 {
			rootFS.mu.Lock()
			rootFS.usedStorage = 0
			rootFS.mu.Unlock()
		}

		// Clear all children
		rootFS.dir.Children = make(map[string]childI)
		rootFS.dir.mu.Unlock()
		return nil
	}

	dirPart, filePart := syspath.Split(path)
	dirPart = strings.TrimSuffix(dirPart, "/")

	dir, err := rootFS.getDir(dirPart)
	if err != nil {
		// If the parent directory doesn't exist, there's nothing to remove
		// which is not an error for RemoveAll (matches os.RemoveAll behavior)
		return nil
	}

	dir.mu.Lock()
	defer dir.mu.Unlock()

	child, exists := dir.Children[filePart]
	if !exists {
		// Path doesn't exist, which is not an error for RemoveAll
		return nil
	}

	// If it's a file, adjust the storage usage and remove it
	if file, ok := child.(*File); ok {
		rootFS.mu.Lock()
		if rootFS.maxStorage > 0 {
			rootFS.usedStorage -= int64(len(file.Content))
		}
		rootFS.mu.Unlock()
		delete(dir.Children, filePart)
		return nil
	}

	// If it's a directory, we need to calculate storage used by all files in it recursively
	if childDir, ok := child.(*Dir); ok {
		// Calculate storage used by the directory and its contents
		if rootFS.maxStorage > 0 {
			rootFS.removeStorageUsed(childDir)
		}

		// Remove the directory entry
		delete(dir.Children, filePart)
	}

	return nil
}

// removeStorageUsed recursively calculates and removes the storage used by a directory
func (rootFS *FS) removeStorageUsed(dir *Dir) {
	// First collect all the files and directories that need to be processed
	var fileSizes []int
	var subdirs []*Dir

	// Lock the directory to safely iterate through its children
	dir.mu.Lock()
	for _, child := range dir.Children {
		if file, ok := child.(*File); ok {
			fileSizes = append(fileSizes, len(file.Content))
		} else if childDir, ok := child.(*Dir); ok {
			subdirs = append(subdirs, childDir)
		}
	}
	dir.mu.Unlock()

	// Process subdirectories recursively
	for _, subdir := range subdirs {
		rootFS.removeStorageUsed(subdir)
	}

	// Update the storage usage for files in this directory
	if len(fileSizes) > 0 {
		rootFS.mu.Lock()
		for _, size := range fileSizes {
			rootFS.usedStorage -= int64(size)
		}
		rootFS.mu.Unlock()
	}
}

// UsedStorage returns the current amount of storage space (in bytes) being used by the filesystem.
// If storage tracking is not enabled (maxStorage <= 0), this will still return the actual space used.
func (rootFS *FS) UsedStorage() int64 {
	rootFS.mu.Lock()
	defer rootFS.mu.Unlock()
	return rootFS.usedStorage
}

type childI any

type fileInfo struct {
	name    string
	size    int64
	modTime time.Time
	mode    fs.FileMode
}

// base name of the file
func (fi *fileInfo) Name() string {
	return fi.name
}

// length in bytes for regular files; system-dependent for others
func (fi *fileInfo) Size() int64 {
	return fi.size
}

// file mode bits
func (fi *fileInfo) Mode() fs.FileMode {
	return fi.mode
}

// modification time
func (fi *fileInfo) ModTime() time.Time {
	return fi.modTime
}

// abbreviation for Mode().IsDir()
func (fi *fileInfo) IsDir() bool {
	return fi.mode&fs.ModeDir > 0
}

// underlying data source (can return nil)
func (fi *fileInfo) Sys() any {
	return nil
}

type dirEntry struct {
	info fs.FileInfo
}

func (de *dirEntry) Name() string {
	return de.info.Name()
}

func (de *dirEntry) IsDir() bool {
	return de.info.IsDir()
}

func (de *dirEntry) Type() fs.FileMode {
	return de.info.Mode() & fs.ModeType
}

func (de *dirEntry) Info() (fs.FileInfo, error) {
	return de.info, nil
}

// NewGzipWriter creates a new gzip writer
func NewGzipWriter(w io.Writer) *GzipWriter {
	return &GzipWriter{
		gw: gzip.NewWriter(w),
		w:  w,
	}
}

// GzipWriter is a wrapper around a gzip.Writer that also implements the io.Writer interface
type GzipWriter struct {
	gw *gzip.Writer
	w  io.Writer
}

// Write writes data to the gzip writer
func (gz *GzipWriter) Write(p []byte) (int, error) {
	return gz.gw.Write(p)
}

// Close closes the gzip writer
func (gz *GzipWriter) Close() error {
	err := gz.gw.Close()
	if err != nil {
		return err
	}
	if closer, ok := gz.w.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
