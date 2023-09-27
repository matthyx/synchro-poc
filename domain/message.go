package domain

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// EventType defines the possible types of events.
type EventType string

const (
	Added    EventType = "ADDED"
	Modified EventType = "MODIFIED"
	Deleted  EventType = "DELETED"
	Checksum EventType = "CHECKSUM"
)

type Message struct {
	Type    EventType
	Cluster string
	Kind    schema.GroupVersionResource
	Key     string
	Object  []byte
	Patch   []byte
}
