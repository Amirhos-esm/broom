package broom

type Operation uint

const (
	OperationAdd Operation = iota
	OperationRemove
	OperationGet
	OperationPing
)

type broomOperationResponse struct {
	err  error
	data BroomFolder
}

type broomOperation struct {
	op     Operation
	folder BroomFolder
	sig    chan broomOperationResponse
}
