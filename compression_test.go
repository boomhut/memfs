package memfs

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"io"
	"testing"
)

// TestCompressDecompressRoundTrip tests the compression and decompression round trip using GzipWriter
func TestCompressDecompressRoundTrip(t *testing.T) {
	// Create a test struct
	type TestStruct struct {
		Name   string
		Values []int
		Data   map[string]string
	}

	testData := TestStruct{
		Name:   "Test Data",
		Values: []int{1, 2, 3, 4, 5},
		Data: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	// Encode and compress the data
	var compressedBuf bytes.Buffer
	gzipWriter := NewGzipWriter(&compressedBuf)
	encoder := gob.NewEncoder(gzipWriter)

	err := encoder.Encode(testData)
	if err != nil {
		t.Fatalf("Failed to encode: %v", err)
	}

	err = gzipWriter.Close()
	if err != nil {
		t.Fatalf("Failed to close GzipWriter: %v", err)
	}

	// Now decompress and decode
	gzipReader, err := gzip.NewReader(bytes.NewReader(compressedBuf.Bytes()))
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}

	var decodedData TestStruct
	decoder := gob.NewDecoder(gzipReader)
	err = decoder.Decode(&decodedData)
	if err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}

	err = gzipReader.Close()
	if err != nil {
		t.Fatalf("Failed to close gzip reader: %v", err)
	}

	// Verify the data matches
	if testData.Name != decodedData.Name {
		t.Errorf("Name mismatch: expected %s, got %s", testData.Name, decodedData.Name)
	}

	if len(testData.Values) != len(decodedData.Values) {
		t.Errorf("Values length mismatch: expected %d, got %d", len(testData.Values), len(decodedData.Values))
	} else {
		for i, v := range testData.Values {
			if v != decodedData.Values[i] {
				t.Errorf("Values[%d] mismatch: expected %d, got %d", i, v, decodedData.Values[i])
			}
		}
	}

	if len(testData.Data) != len(decodedData.Data) {
		t.Errorf("Data map size mismatch: expected %d, got %d", len(testData.Data), len(decodedData.Data))
	} else {
		for k, v := range testData.Data {
			if dv, ok := decodedData.Data[k]; !ok || dv != v {
				t.Errorf("Data[%s] mismatch: expected %s, got %s", k, v, dv)
			}
		}
	}
}

// TestLargeData tests compressing and decompressing large amounts of data
func TestLargeData(t *testing.T) {
	// Create a large byte array (1MB)
	size := 1024 * 1024
	largeData := make([]byte, size)
	for i := 0; i < size; i++ {
		largeData[i] = byte(i % 256)
	}

	// Compress the data
	var compressedBuf bytes.Buffer
	gzipWriter := NewGzipWriter(&compressedBuf)

	written, err := gzipWriter.Write(largeData)
	if err != nil {
		t.Fatalf("Failed to write large data: %v", err)
	}
	if written != size {
		t.Errorf("Expected to write %d bytes, but wrote %d", size, written)
	}

	err = gzipWriter.Close()
	if err != nil {
		t.Fatalf("Failed to close GzipWriter: %v", err)
	}

	// Check that compression actually reduced the size
	compressedSize := compressedBuf.Len()
	t.Logf("Original size: %d bytes, Compressed size: %d bytes, Ratio: %.2f%%",
		size, compressedSize, float64(compressedSize)/float64(size)*100)

	// Decompress the data
	gzipReader, err := gzip.NewReader(bytes.NewReader(compressedBuf.Bytes()))
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}

	decompressed, err := io.ReadAll(gzipReader)
	if err != nil {
		t.Fatalf("Failed to decompress: %v", err)
	}

	err = gzipReader.Close()
	if err != nil {
		t.Fatalf("Failed to close gzip reader: %v", err)
	}

	// Verify the decompressed data matches the original
	if len(decompressed) != size {
		t.Errorf("Decompressed size mismatch: expected %d, got %d", size, len(decompressed))
	} else {
		// Check first 10 and last 10 bytes for simplicity
		for i := 0; i < 10; i++ {
			if largeData[i] != decompressed[i] {
				t.Errorf("Decompressed data mismatch at beginning index %d: expected %d, got %d",
					i, largeData[i], decompressed[i])
				break
			}
		}

		for i := size - 10; i < size; i++ {
			if largeData[i] != decompressed[i] {
				t.Errorf("Decompressed data mismatch at ending index %d: expected %d, got %d",
					i, largeData[i], decompressed[i])
				break
			}
		}
	}
}
