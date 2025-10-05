package broom

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// helper: create a file of given size (in bytes)
func createFile(path string, size Size) error {
	data := make([]byte, int(size))
	return ioutil.WriteFile(path, data, 0644)
}

// get total size of all files in a folder
func folderSize(path string) (Size, error) {
	var total Size = 0
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += Size(info.Size())
		}
		return nil
	})
	return total, err
}

func TestRealFileDeletionAfterSizeLimit(t *testing.T) {
	// Create temp dir
	tmpDir, err := ioutil.TempDir("", "broom_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir) // cleanup after test

	// Create files:
	// file1: 600 KB, file2: 300 KB, file3: 400 KB (total 1.3 MB)
	err = createFile(filepath.Join(tmpDir, "file1"), 600*KByte)
	if err != nil {
		t.Fatal(err)
	}
	err = createFile(filepath.Join(tmpDir, "file2"), 300*KByte)
	if err != nil {
		t.Fatal(err)
	}
	err = createFile(filepath.Join(tmpDir, "file3"), 400*KByte)
	if err != nil {
		t.Fatal(err)
	}

	br := NewBroom(1 * time.Second)
	// br.RemovingStrategy = DEFAULT_REMOVING_STRATEGY // old files first

	br.Start()
	defer br.Stop()

	// Add folder with max size 1 MB (1,000,000 bytes)
	err = br.AddFolder(tmpDir, 1*MByte)
	if err != nil {
		t.Fatalf("failed to add folder: %v", err)
	}

	// Wait some time for broom to sweep and clean
	time.Sleep(2 * time.Second)

	// Check folder size after cleanup
	size, err := folderSize(tmpDir)
	if err != nil {
		t.Fatalf("failed to get folder size: %v", err)
	}

	if size > 1*MByte {
		t.Errorf("folder size after cleanup is %d bytes, expected <= %d bytes", size, 1*MByte)
	}

	f , err := br.GetFolder(tmpDir)
		if err != nil {
		t.Fatalf("failed to get folder : %v", err)
	}
	if f.CurrentSize != size {
		t.Fatalf("CurrentSize != size %v != %v" , f.CurrentSize , size)
	
	}

	// Optionally check which files remain or were deleted
}
