package domain

type Strategy string

const (
	CopyStrategy  Strategy = "copy"
	PatchStrategy Strategy = "patch"
)
