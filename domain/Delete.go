
package domain

// Delete represents a Delete model.
type Delete struct {
  Event *Event
  Cluster string
  Kind *Kind
  Name string
  AdditionalProperties map[string]interface{}
}