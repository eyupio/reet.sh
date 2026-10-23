package terminalproto

// StreamHello authenticates a reverse stream to one outstanding private
// control operation. It contains no terminal contents and is never persisted.
type StreamHello struct {
	Version              int    `json:"version"`
	RequestID            string `json:"requestId"`
	SessionID            string `json:"sessionId"`
	AttachmentID         string `json:"attachmentId"`
	Generation           int64  `json:"generation"`
	AttachmentGeneration int64  `json:"attachmentGeneration"`
	Token                string `json:"token"`
	Operation            string `json:"operation"`
}

func (h StreamHello) Validate() error {
	if h.Version != Version || !ValidID(h.RequestID) || !ValidID(h.SessionID) || !ValidID(h.AttachmentID) || !ValidCounter(h.Generation) || !ValidCounter(h.AttachmentGeneration) || !ValidID(h.Token) || h.Operation != "attach" && h.Operation != "history" {
		return ErrInvalid
	}
	return nil
}

func DecodeStreamHello(b []byte) (StreamHello, error) {
	var h StreamHello
	if len(b) > MaxControlBytes || !uniqueKeys(b) || strictJSON(b, &h) != nil || h.Validate() != nil {
		return h, ErrInvalid
	}
	return h, nil
}
