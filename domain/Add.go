
package domain

// Add represents a Add model.
type Add struct {
  Event *Event
  Cluster string
  Kind *Kind
  Name string
  Object string
  AdditionalProperties map[string]interface{}
}