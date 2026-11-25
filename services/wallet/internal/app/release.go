package app

type ReleaseResult int

const (
	Released ReleaseResult = iota + 1
	ReleaseParameterConflict
	AlreadyCaptured
)
