package protocol

type Exchange struct {
	ID      uint16
	Seq     uint16
	Payload []byte
}

func ClonePayload(payload []byte) []byte {
	if len(payload) == 0 {
		return nil
	}
	cloned := make([]byte, len(payload))
	copy(cloned, payload)
	return cloned
}
