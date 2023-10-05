
package domain

// UpdateShadow represents a UpdateShadow model.
type UpdateShadow struct {
  Event *Event
  Cluster string
  Kind *Kind
  Name string
  Object string
  AdditionalProperties map[string]interface{}
}