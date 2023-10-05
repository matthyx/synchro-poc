
package domain

// Event represents an enum of Event.
type Event uint

const (
  EventAdd Event = iota
  EventChecksum
  EventDelete
  EventPatch
  EventRetrieve
  EventUpdateShadow
)

// Value returns the value of the enum.
func (op Event) Value() any {
	if op >= Event(len(EventValues)) {
		return nil
	}
	return EventValues[op]
}

var EventValues = []any{"add","checksum","delete","patch","retrieve","updateShadow"}
var ValuesToEvent = map[any]Event{
  EventValues[EventAdd]: EventAdd,
  EventValues[EventChecksum]: EventChecksum,
  EventValues[EventDelete]: EventDelete,
  EventValues[EventPatch]: EventPatch,
  EventValues[EventRetrieve]: EventRetrieve,
  EventValues[EventUpdateShadow]: EventUpdateShadow,
}
