
package domain

// Retrieve represents a Retrieve model.
type Retrieve struct {
  Event *Event
  Cluster string
  Kind *Kind
  Name string
  AdditionalProperties map[string]interface{}
}