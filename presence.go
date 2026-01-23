package llp

// PresenceStatus represents the presence status of a session
type PresenceStatus int

const (
	Unavailable PresenceStatus = iota
	Available
)

// String returns the string representation of the presence status
func (p PresenceStatus) String() string {
	switch p {
	case Available:
		return "available"
	case Unavailable:
		return "unavailable"
	default:
		return "unavailable"
	}
}

// ParsePresenceStatus parses a string into a PresenceStatus
func ParsePresenceStatus(s string) (PresenceStatus, error) {
	switch s {
	case "available":
		return Available, nil
	case "unavailable":
		return Unavailable, nil
	default:
		return Unavailable, ErrInvalidStatus
	}
}
