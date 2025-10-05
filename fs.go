package broom

import (
	"container/list"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/djherbis/times"
)

type File struct {
	Path     string
	Name     string
	IsDir    bool
	Size     Size
	CreateAt time.Time
	UpdateAt time.Time
	Metadata map[string]any
}

func (f File) String() string {
	return fmt.Sprintf("File{Path: %q, Name: %q, IsDir: %t, Size: %v, Date: %s}",
		f.Path, f.Name, f.IsDir, f.Size, f.CreateAt.Format(time.RFC3339))
}

// GetName returns the file name (e.g. "video.mp4")
func (f *File) GetName() string {
	return filepath.Base(f.Path)
}

// GetExtension returns the file extension (e.g. ".mp4")
func (f *File) GetExtension() string {
	return filepath.Ext(f.Path)
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
			Name:     entry.Name(),
			Path:     absPath,
			IsDir:    entry.IsDir(),
			Size:     Size(info.Size()),
			CreateAt: date,
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
	allFiles *list.List,
	bytesToFree Size,
) []File {
	// Collect files into a slice for sorting
	var filesSlice []File
	for e := allFiles.Front(); e != nil; e = e.Next() {
		if f, ok := e.Value.(File); ok && !f.IsDir {
			filesSlice = append(filesSlice, f)
		}
	}

	// Sort by creation date (oldest first)
	sort.Slice(filesSlice, func(i, j int) bool {
		return filesSlice[i].CreateAt.Before(filesSlice[j].CreateAt)
	})

	var totalFreed Size
	var filesToRemove []File

	for _, file := range filesSlice {
		if totalFreed >= bytesToFree {
			break
		}
		totalFreed += file.Size
		filesToRemove = append(filesToRemove, file)
	}

	return filesToRemove
}

var NAME_BASED_REMOVING_STRATEGY RemovingStrategy = func(
	folder *BroomFolder,
	allFiles *list.List,
	bytesToFree Size,
) []File {
	// Collect files into a slice for sorting
	var filesSlice []File
	for e := allFiles.Front(); e != nil; e = e.Next() {
		if f, ok := e.Value.(File); ok && !f.IsDir {
			filesSlice = append(filesSlice, f)
		}
	}

	// Sort by name alphabetically (A to Z)
	sort.Slice(filesSlice, func(i, j int) bool {
		return filesSlice[i].Name < filesSlice[j].Name
	})

	var totalFreed Size
	var filesToRemove []File

	for _, file := range filesSlice {
		if totalFreed >= bytesToFree {
			break
		}
		totalFreed += file.Size
		filesToRemove = append(filesToRemove, file)
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

// ────────────────────────────────
// High-level function
// ────────────────────────────────

// BuildFileList scans a folder (recursive if flag=true), filters, sorts, and returns a linked list of files.
func BuildFileList(folder string, extensions []string, recursive bool) (*list.List, error) {
	if err := validateFolder(folder); err != nil {
		return nil, err
	}

	files, err := collectFiles(folder, extensions, recursive)
	if err != nil {
		return nil, err
	}

	sortFilesByCreateTime(files)

	return toLinkedList(files), nil
}

// ────────────────────────────────
// Supporting functions
// ────────────────────────────────

// validateFolder ensures path exists and is a directory.
func validateFolder(folder string) error {
	info, err := os.Stat(folder)
	if err != nil {
		return fmt.Errorf("cannot access folder: %w", err)
	}
	if !info.IsDir() {
		return errors.New("provided path is not a directory")
	}
	return nil
}

// collectFiles walks through a directory and collects files matching given extensions
func collectFiles(folder string, extensions []string, recursive bool) ([]File, error) {
	var files []File

	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, fmt.Errorf("cannot read directory: %w", err)
	}

	for _, entry := range entries {
		fullPath := filepath.Join(folder, entry.Name())

		if entry.IsDir() {
			if recursive {
				// Recursively collect from subdirectory
				subFiles, err := collectFiles(fullPath, extensions, recursive)
				if err != nil {
					continue // skip inaccessible subfolders
				}
				files = append(files, subFiles...)
			}
			continue
		}

		if !hasAllowedExtension(entry.Name(), extensions) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue // skip unreadable files
		}

		f := File{
			Path:     fullPath,
			Name:     entry.Name(),
			IsDir:    false,
			Size:     Size(info.Size()),
			CreateAt: getCreateTime(info),
			UpdateAt: info.ModTime(),
			Metadata: map[string]any{},
		}
		files = append(files, f)
	}

	return files, nil
}

// hasAllowedExtension checks if filename has one of the allowed extensions
func hasAllowedExtension(filename string, extensions []string) bool {
	if extensions == nil {
		return true
	}
	ext := strings.ToLower(filepath.Ext(filename))
	for _, e := range extensions {
		if strings.ToLower(e) == ext || strings.ToLower("."+e) == ext {
			return true
		}
	}
	return false
}

// getCreateTime extracts the file creation timestamp (cross-platform best effort)
func getCreateTime(info os.FileInfo) time.Time {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if ok {
		return time.Unix(int64(stat.Ctim.Sec), int64(stat.Ctim.Nsec))
	}
	return info.ModTime()
}

// sortFilesByCreateTime sorts files by creation time ascending
func sortFilesByCreateTime(files []File) {
	sort.Slice(files, func(i, j int) bool {
		return files[i].CreateAt.Before(files[j].CreateAt)
	})
}

// toLinkedList converts a slice of files into a container/list
func toLinkedList(files []File) *list.List {
	l := list.New()
	for _, f := range files {
		l.PushBack(f)
	}
	return l
}

// PrintFileList prints the linked list of files
func PrintFileList(l *list.List) {
	for e := l.Front(); e != nil; e = e.Next() {
		f := e.Value.(File)
		fmt.Printf("%-25s | %s | %s\n",
			f.Name,
			f.CreateAt.Format(time.RFC3339),
			f.Path,
		)
	}
}
