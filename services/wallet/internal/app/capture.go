package app

import "errors"

var ErrReserveNotFound = errors.New("reserve not found")

type CaptureResult int

const (
	Captured CaptureResult = iota + 1
	CaptureParameterConflict
	AlreadyReleased
)
