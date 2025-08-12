package broom

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/djherbis/times"
)

type File struct {
	Path  string
	Name  string
	IsDir bool
	Size  Size
	Date  time.Time
}

func (f File) String() string {
	return fmt.Sprintf("File{Path: %q, Name: %q, IsDir: %t, Size: %v, Date: %s}",
		f.Path, f.Name, f.IsDir, f.Size, f.Date.Format(time.RFC3339))
}

func ListFiles(folderPath string) ([]File, error) {
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return nil, err
	}

	var files []File
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}

		absPath, err := filepath.Abs(filepath.Join(folderPath, entry.Name()))
		if err != nil {
			return nil, err
		}

		t, err := times.Stat(absPath)
		if err != nil {
			return nil, err
		}
		var date time.Time
		if t.HasBirthTime() {
			date = t.BirthTime()
		} else {
			date = t.ChangeTime()
		}

		files = append(files, File{
			Name:  entry.Name(),
			Path:  absPath,
			IsDir: entry.IsDir(),
			Size:  Size(info.Size()),
			Date:  date,
		})
	}
	return files, nil
}

func calculateFolderSize(files []File) Size {
	ret := Size(0)
	for x := range files {
		v := &files[x]
		if !v.IsDir {
			ret += v.Size
		}
	}
	return ret
}

var DEFAULT_REMOVING_STRATEGY RemovingStrategy = func(
	folder *BroomFolder,
	allFiles []File,
	bytesToFree Size,
) []File {
	// Sort by modification date (oldest first)
	sort.Slice(allFiles, func(i, j int) bool {
		return allFiles[i].Date.Before(allFiles[j].Date)
	})

	var totalFreed Size
	var filesToRemove []File

	for _, file := range allFiles {
		if file.IsDir {
			continue
		}

		if totalFreed < bytesToFree {
			totalFreed += file.Size
			filesToRemove = append(filesToRemove, file)
			continue
		}
		// Stop once we've freed enough
		break
	}

	return filesToRemove
}

var NAME_BASED_REMOVING_STRATEGY RemovingStrategy = func(
	folder *BroomFolder,
	allFiles []File,
	bytesToFree Size,
) []File {
	// Sort by name alphabetically (A to Z)
	sort.Slice(allFiles, func(i, j int) bool {
		return allFiles[i].Name < allFiles[j].Name
	})

	var totalFreed Size
	var filesToRemove []File

	for _, file := range allFiles {
		if file.IsDir {
			continue
		}

		if totalFreed < bytesToFree {
			totalFreed += file.Size
			filesToRemove = append(filesToRemove, file)
			continue
		}
		break
	}

	return filesToRemove
}


func DeleteFiles(files []File) error {
	for _, file := range files {
		if file.IsDir {
			continue // skip directories for safety
		}

		err := os.Remove(file.Path)
		if err != nil {
			return fmt.Errorf("failed to delete %s: %w", file.Path, err)
		}
	}
	return nil
}
