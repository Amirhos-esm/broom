package broom

import (
	"container/list"
	"context"
	"fmt"
	"time"
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
}

func (bf BroomFolder) String() string {
	return fmt.Sprintf("BroomFolder{ Location: %q, MaxSize: %v, CurrentSize: %v }",
		bf.Location, bf.MaxSize, bf.CurrentSize)
}
func (bf *BroomFolder) scan() error {
	files, err := collectFiles(bf.Location, bf.parent.exts, false)
	if err != nil {
		return err
	}
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
func (bf *BroomFolder) delete() error {
	if bf.list == nil {
		return nil
	}
	br := bf.parent
	if bf.MaxSize != 0 && bf.MaxSize < bf.CurrentSize {
		if br.RemovingStrategy != nil {
			rms := br.RemovingStrategy(bf, bf.list, bf.CurrentSize-bf.MaxSize)
			err := DeleteFiles(rms)
			if br.onRemoveCb != nil {
				br.onRemoveCb(bf,rms)
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

			ret = &broomOperationResponse{
				err:  nil,
				data: *got,
			}
		}
	case OperationPing:
		ret = &broomOperationResponse{
			err: nil,
		}
	case OperationRemove:
		if _, exist := br.folders[op.folder.Location]; !exist {
			ret = &broomOperationResponse{
				err: ErrFolderNotExist,
			}
		} else {
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
			err := br.checkFolder(&folder)
			if err == nil {
				br.folders[op.folder.Location] = folder // write back modified value
			}
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
func (br *Broom) checkFolder(folder *BroomFolder) error {
	files, err := ListFiles(folder.Location)
	if err != nil {
		return err
	}
	folder.CurrentSize = calculateFolderSize(files)
	if folder.MaxSize != 0 && folder.MaxSize < folder.CurrentSize {
		if br.RemovingStrategy != nil {
			rms := br.RemovingStrategy(folder, files, folder.CurrentSize-folder.MaxSize)
			err := DeleteFiles(rms)
			if err == nil {
				folder.CurrentSize -= calculateFolderSize(rms)
			}
			return err
		}
	}
	return nil
}
func (br *Broom) loop(ctx context.Context) {

	for {
		if br.handle(br.sweepTime, ctx) {
			goto exit
		}
	}
exit:
}
