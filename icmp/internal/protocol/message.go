package protocol

type Exchange struct {
	ID      uint16
	Seq     uint16
	Payload []byte
}
