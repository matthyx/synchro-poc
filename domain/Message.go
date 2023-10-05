package domain

type Message interface {
}

var _ Message = (*Add)(nil)
var _ Message = (*Checksum)(nil)
var _ Message = (*Delete)(nil)
var _ Message = (*Patch)(nil)
var _ Message = (*Retrieve)(nil)
var _ Message = (*UpdateShadow)(nil)
