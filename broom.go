// Package broom provides a background file cleanup utility that manages
// folders and removes files based on configurable strategies to free disk space.
package broom

import (
	"container/list"
	"context"
	"errors"
	"time"

	"github.com/Amirhos-esm/startable"
)

// Predefined errors returned by broom operations.
var (
	ErrFolderExist    = errors.New("folder exists")
	ErrFolderNotExist = errors.New("folder not exist")
	ErrNotStarted     = errors.New("not started")
	ErrTimeout        = errors.New("timeout")
)

// RemovingStrategy defines the signature of functions that select files
// to be removed from a folder in order to free up a given amount of space.
type RemovingStrategy func(folder *BroomFolder, files *list.List, needReduce Size) []File

// call when a file has beem removed
type OnRemoveCallback func(folder *BroomFolder, fileToRemove []File)

// MetadateReader use can add metadata to each file so it will be saved and user can read it
type MetadateReader func(folder *BroomFolder, file *File) any

// Broom manages a set of folders and periodically sweeps them to remove files
// based on the configured RemovingStrategy. It handles operations via a queue
// and runs in a background goroutine.
type Broom struct {
	startable.Startable
	operationQueue chan broomOperation
	folders        map[string]*BroomFolder
	exts           []string

	RemovingStrategy RemovingStrategy
	onRemoveCb       OnRemoveCallback
	sweepTime        time.Duration
	// state
}

// NewBroom creates and returns a new Broom instance that sweeps folders
// at the given sweepTime interval. The default RemovingStrategy is used initially.
func NewBroom(sweepTime time.Duration) *Broom {
	broom := &Broom{
		operationQueue:   make(chan broomOperation, 10),
		folders:          make(map[string]*BroomFolder),
		RemovingStrategy: DEFAULT_REMOVING_STRATEGY,
		sweepTime:        sweepTime,
	}
	broom.SetStartFunction(func(ctx context.Context) any {
		broom.loop(ctx)
		return nil
	})
	return broom
}

// Run starts the broom background process that handles folder cleanup.
// If it is already running, Run does nothing.
func (br *Broom) Start() error {
	err := br.Startable.Start()
	if err != nil {
		return err
	}
	// wait until goroutine has started by sending a ping operation
	op := broomOperation{
		op:  OperationPing,
		sig: make(chan broomOperationResponse),
	}
	br.operationQueue <- op
	<-op.sig
	return nil

}

// Stop stops the broom background process gracefully, waiting
// for it to finish cleanup and exit.
func (br *Broom) Stop() error {
	return br.Startable.Stop()
}

// AddFolder adds a folder to the broom management system with the specified
// location and maximum size. Returns an error if the broom is not started or
// if the folder already exists.
func (br *Broom) AddFolder(location string, maxSize Size) error {
	if !br.IsStarted() {
		return ErrNotStarted
	}
	op := broomOperation{
		op: OperationAdd,
		folder: BroomFolder{
			Location: location,
			MaxSize:  maxSize,
		},
		sig: make(chan broomOperationResponse),
	}
	br.operationQueue <- op
	x := <-op.sig
	return x.err

}

// scan and recheck folder without waiting for interval
func (br *Broom) RecheckFolder(location string) error {
	if !br.IsStarted() {
		return ErrNotStarted
	}
	op := broomOperation{
		op: OperationRecheck,
		folder: BroomFolder{
			Location: location,
		},
		sig: make(chan broomOperationResponse),
	}
	br.operationQueue <- op

	x := <-op.sig
	return x.err

}

// RemoveFolder removes a folder from the broom management system by location.
// Returns an error if the broom is not started or if the folder does not exist.
func (br *Broom) RemoveFolder(location string) error {
	if !br.IsStarted() {
		return ErrNotStarted
	}
	op := broomOperation{
		op: OperationRemove,
		folder: BroomFolder{
			Location: location,
		},
		sig: make(chan broomOperationResponse),
	}
	br.operationQueue <- op

	x := <-op.sig
	return x.err

}

// GetFolder retrieves information about a managed folder by location.
// Returns the folder details or an error if the broom is not started or
// the folder is not found.
func (br *Broom) GetFolder(location string) (BroomFolder, error) {
	if !br.IsStarted() {
		return BroomFolder{}, ErrNotStarted
	}
	op := broomOperation{
		op: OperationGet,
		folder: BroomFolder{
			Location: location,
		},
		sig: make(chan broomOperationResponse),
	}
	br.operationQueue <- op

	x := <-op.sig
	return x.data, x.err

}
