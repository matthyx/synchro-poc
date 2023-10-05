
package domain

// Checksum represents a Checksum model.
type Checksum struct {
  Event *Event
  Cluster string
  Kind *Kind
  Name string
  Object string
  AdditionalProperties map[string]interface{}
}