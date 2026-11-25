package app

type ReserveResult int

const (
	Reserved ReserveResult = iota + 1
	InsufficientFunds
	ParameterConflict
)
