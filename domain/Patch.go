
package domain

// Patch represents a Patch model.
type Patch struct {
  Event *Event
  Cluster string
  Kind *Kind
  Name string
  Patch string
  AdditionalProperties map[string]interface{}
}