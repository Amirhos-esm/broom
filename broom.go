// Package broom provides a background file cleanup utility that manages
// folders and removes files based on configurable strategies to free disk space.
package broom

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Predefined errors returned by broom operations.
var (
	ERROR_FOLDER_EXIST     = errors.New("folder exist")
	ERROR_FOLDER_NOT_EXIST = errors.New("folder not exist")
	ERROR_NOT_STARTED      = errors.New("not started")
	ERROR_TIMEOUT          = errors.New("timeout")
)

// RemovingStrategy defines the signature of functions that select files
// to be removed from a folder in order to free up a given amount of space.
type RemovingStrategy func(folder *BroomFolder, files []File, needReduce Size) []File

// Broom manages a set of folders and periodically sweeps them to remove files
// based on the configured RemovingStrategy. It handles operations via a queue
// and runs in a background goroutine.
type Broom struct {
	mutex            sync.Mutex
	operationQueue   chan broomOperation
	folders          map[string]BroomFolder
	isStarted        bool
	cancel           context.CancelFunc
	endSig           chan struct{}
	RemovingStrategy RemovingStrategy
	sweepTime        time.Duration
	// state
}

// NewBroom creates and returns a new Broom instance that sweeps folders
// at the given sweepTime interval. The default RemovingStrategy is used initially.
func NewBroom(sweepTime time.Duration) *Broom {
	return &Broom{
		operationQueue:   make(chan broomOperation, 10),
		folders:          make(map[string]BroomFolder),
		RemovingStrategy: DEFAULT_REMOVING_STRATEGY,
		sweepTime:        sweepTime,
	}
}

// Run starts the broom background process that handles folder cleanup.
// If it is already running, Run does nothing.
func (br *Broom) Run() {
	if br.isStarted {
		return
	}
	go br.loop()

	// wait until goroutine has started by sending a ping operation
	op := broomOperation{
		op:  OperationPing,
		sig: make(chan broomOperationResponse),
	}
	br.operationQueue <- op
	select {
	case <-op.sig:
		return
	}
}

// Stop stops the broom background process gracefully, waiting
// for it to finish cleanup and exit.
func (br *Broom) Stop() {
	if br.isStarted {
		br.cancel()
		select {
		case <-br.endSig:
		}
	}
}

// AddFolder adds a folder to the broom management system with the specified
// location and maximum size. Returns an error if the broom is not started or
// if the folder already exists.
func (br *Broom) AddFolder(location string, maxSize Size) error {
	if !br.isStarted {
		return ERROR_NOT_STARTED
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
	select {
	case x := <-op.sig:
		return x.err
	}
}

// scan and recheck folder without waiting for interval
func (br *Broom) RecheckFolder(location string) error {
	if !br.isStarted {
		return ERROR_NOT_STARTED
	}
	op := broomOperation{
		op: OperationRecheck,
		folder: BroomFolder{
			Location: location,
		},
		sig: make(chan broomOperationResponse),
	}
	br.operationQueue <- op
	select {
	case x := <-op.sig:
		return x.err
	}
}

// RemoveFolder removes a folder from the broom management system by location.
// Returns an error if the broom is not started or if the folder does not exist.
func (br *Broom) RemoveFolder(location string) error {
	if !br.isStarted {
		return ERROR_NOT_STARTED
	}
	op := broomOperation{
		op: OperationRemove,
		folder: BroomFolder{
			Location: location,
		},
		sig: make(chan broomOperationResponse),
	}
	br.operationQueue <- op
	select {
	case x := <-op.sig:
		return x.err
	}
}

// GetFolder retrieves information about a managed folder by location.
// Returns the folder details or an error if the broom is not started or
// the folder is not found.
func (br *Broom) GetFolder(location string) (BroomFolder, error) {
	if !br.isStarted {
		return BroomFolder{}, ERROR_NOT_STARTED
	}
	op := broomOperation{
		op: OperationGet,
		folder: BroomFolder{
			Location: location,
		},
		sig: make(chan broomOperationResponse),
	}
	br.operationQueue <- op
	select {
	case x := <-op.sig:
		return x.data, x.err
	}
}
