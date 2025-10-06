package broom

import (
	"container/list"
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Size uint64

func (s Size) String() string {
	const unit = 1024
	if s < unit {
		return fmt.Sprintf("%d B", s)
	}

	div, exp := uint64(unit), 0
	for n := s / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.2f %cB",
		float64(s)/float64(div), "KMGTPE"[exp])
}

const (
	Byte  Size = 1
	KByte      = 1000 * Byte
	MByte      = 1000 * KByte
	GByte      = 1000 * MByte
	TByte      = 1000 * GByte
)

type BroomState uint

const (
	BroomStateInit BroomState = iota
	BroomStateScan
	BroomStateRest
)

type BroomFolder struct {
	Location    string
	MaxSize     Size
	CurrentSize Size
	list        *list.List
	parent      *Broom
	watcher     *fsnotify.Watcher
}

func (bf BroomFolder) String() string {
	return fmt.Sprintf("BroomFolder{ Location: %q, MaxSize: %v, CurrentSize: %v }",
		bf.Location, bf.MaxSize, bf.CurrentSize)
}
func (bf *BroomFolder) isInitialized() bool {
	return bf.list != nil
}

func (bf *BroomFolder) onFolderEvent(event fsnotify.Event) {
	switch {
	case event.Has(fsnotify.Create):
		log.Printf("[+] New file: %s\n", event.Name)
		// bf.Rescan()
	case event.Has(fsnotify.Write):
		log.Printf("[~] Modified: %s\n", event.Name)
	case event.Has(fsnotify.Remove):
		log.Printf("[-] Removed: %s\n", event.Name)
		// bf.Rescan()
	case event.Has(fsnotify.Rename):
		log.Printf("[>] Renamed: %s\n", event.Name)
		// bf.Rescan()
	}
}
func (bf *BroomFolder) deInit() {
	if !bf.isInitialized() {
		return
	}
	if bf.watcher != nil {
		bf.watcher.Close()
	}
	if bf.list != nil {
		bf.list = nil
	}
}
func (bf *BroomFolder) fetchMetadata() {
	if bf.isInitialized() {
		return
	}
	if bf.parent.metaDataReader == nil {
		return
	}
	const MaxSimultaneouslyGoroutine int = 10
	var wg sync.WaitGroup
	// Foreach style loop
	i := 0
	for e := bf.list.Front(); e != nil; e = e.Next() {
		file := e.Value.(File)
		file.Metadata = nil
		file.Metadata = map[string]any{}
		wg.Add(1)
		go func(folder *BroomFolder, file File) {
			bf.parent.metaDataReader(folder, &file)
			wg.Done()
		}(bf, file)
		i++
		if (i % MaxSimultaneouslyGoroutine) == 0 {
			wg.Wait()
		}

	}
	wg.Wait()
}

// func (bf *BroomFolder) fetchMetadata() {
// 	if bf.isInitialized() {
// 		return
// 	}
// 	if bf.parent.metaDataReader == nil {
// 		return
// 	}

// 	const MaxSimultaneousGoroutines = 10
// 	var wg sync.WaitGroup
// 	sem := make(chan struct{}, MaxSimultaneousGoroutines) // limit concurrency

// 	for e := bf.list.Front(); e != nil; e = e.Next() {
// 		file := e.Value.(File)

// 		wg.Add(1)
// 		sem <- struct{}{} // acquire a slot

// 		go func(folder *BroomFolder, f File) {
// 			defer wg.Done()
// 			defer func() { <-sem }() // release slot when done

// 			bf.parent.metaDataReader(folder, &f)
// 		}(bf, file)
// 	}

//		wg.Wait()
//	}
func (bf *BroomFolder) watch() {
	// Start listening for events.
	go func(bf *BroomFolder) {
		for {
			select {
			case event, ok := <-bf.watcher.Events:
				if !ok {
					return
				}
				bf.onFolderEvent(event)
			case err, ok := <-bf.watcher.Errors:
				if !ok {
					return
				}
				log.Println("error:", err)
			}
		}
	}(bf)

}
func (bf *BroomFolder) initialize() error {
	if bf.isInitialized() {
		return nil
	}

	if err := bf.scan(); err != nil {
		return err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}
	bf.watcher = watcher
	bf.watch()

	if err := bf.watcher.Add(bf.Location); err != nil {
		bf.watcher.Close()
		return fmt.Errorf("failed to watch folder %q: %w", bf.Location, err)
	}

	bf.fetchMetadata()
	if err := bf.check(); err != nil {
		bf.deInit()
		return err
	}

	return nil
}
func (bf *BroomFolder) scan() error {
	files, err := collectFiles(bf.Location, bf.parent.exts, false)
	if err != nil {
		return err
	}
	bf.list = nil
	// calculate total size of the directory
	totalSize := Size(0)
	for x, _ := range files {
		if !files[x].IsDir {
			totalSize += files[x].Size
		}
	}
	bf.CurrentSize = totalSize

	sortFilesByCreateTime(files)
	bf.list = toLinkedList(files)
	return nil
}

// check folder if execed max size it will delete some files
func (bf *BroomFolder) check() error {
	if !bf.isInitialized() {
		return nil
	}
	br := bf.parent
	if bf.MaxSize != 0 && bf.MaxSize < bf.CurrentSize {
		if br.RemovingStrategy != nil {
			rms := br.RemovingStrategy(bf, bf.list, bf.CurrentSize-bf.MaxSize)
			err := DeleteFiles(rms)
			if br.onRemoveCb != nil {
				br.onRemoveCb(bf, rms)
			}
			if err == nil {
				bf.CurrentSize -= calculateFolderSize(rms)
			}
			return err
		}
	}
	return nil
}

func (br *Broom) handleQueue(op broomOperation) {
	var ret *broomOperationResponse
	switch op.op {
	case OperationAdd:
		if _, exist := br.folders[op.folder.Location]; exist {
			ret = &broomOperationResponse{
				err: ErrFolderExist,
			}
		} else {
			br.folders[op.folder.Location] = &op.folder
			ret = &broomOperationResponse{
				err: nil,
			}
		}
	case OperationGet:
		if got, exist := br.folders[op.folder.Location]; !exist {
			ret = &broomOperationResponse{
				err: ErrFolderNotExist,
			}

		} else {
			var err error
			if !got.isInitialized() {
				err = got.initialize()
			}
			ret = &broomOperationResponse{
				err:  err,
				data: *got,
			}
		}
	case OperationPing:
		ret = &broomOperationResponse{
			err: nil,
		}
	case OperationRemove:
		if folder, exist := br.folders[op.folder.Location]; !exist {
			ret = &broomOperationResponse{
				err: ErrFolderNotExist,
			}
		} else {
			folder.deInit()
			delete(br.folders, op.folder.Location)
			ret = &broomOperationResponse{
				err: nil,
			}
		}
	case OperationRecheck:
		if folder, exist := br.folders[op.folder.Location]; !exist {
			ret = &broomOperationResponse{
				err: ErrFolderNotExist,
			}
		} else {
			folder.deInit()
			err := folder.initialize()
			ret = &broomOperationResponse{
				err: err,
			}
		}

	default:
		panic("not handled operation")
	}
	op.sig <- *ret
	close(op.sig)
}
func (br *Broom) handleOperationQueue() {
	for {

		select {
		case op := <-br.operationQueue:
			br.handleQueue(op)
		default:
			return
		}
	}
}
func (br *Broom) handle(wait time.Duration, ctx context.Context) bool {
	sig := time.After(wait)
	for {

		select {
		case op := <-br.operationQueue:
			br.handleQueue(op)
		case <-ctx.Done():
			return true
		case <-sig:
			return false
		}
	}
}

//	func (br *Broom) checkFolder(folder *BroomFolder) error {
//		files, err := ListFiles(folder.Location)
//		if err != nil {
//			return err
//		}
//		folder.CurrentSize = calculateFolderSize(files)
//		if folder.MaxSize != 0 && folder.MaxSize < folder.CurrentSize {
//			if br.RemovingStrategy != nil {
//				rms := br.RemovingStrategy(folder, files, folder.CurrentSize-folder.MaxSize)
//				err := DeleteFiles(rms)
//				if err == nil {
//					folder.CurrentSize -= calculateFolderSize(rms)
//				}
//				return err
//			}
//		}
//		return nil
//	}
func (br *Broom) loop(ctx context.Context) {

	for {
		if br.handle(br.sweepTime, ctx) {
			goto exit
		}
	}
exit:
}
