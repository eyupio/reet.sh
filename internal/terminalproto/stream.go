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
	validIDs := ValidID(h.RequestID) && ValidID(h.SessionID) && ValidID(h.AttachmentID) && ValidID(h.Token)
	validCounters := ValidCounter(h.Generation) && ValidCounter(h.AttachmentGeneration)
	validOperation := h.Operation == "attach" || h.Operation == "history" || h.Operation == UploadOperation
	valid := h.Version == Version && validIDs && validCounters && validOperation
	if !valid {
		return ErrInvalid
	}
	return nil
}

func DecodeStreamHello(b []byte) (StreamHello, error) {
	var h StreamHello
	validData := len(b) <= MaxControlBytes && uniqueKeys(b)
	validBody := strictJSON(b, &h) == nil && h.Validate() == nil
	if !validData || !validBody {
		return h, ErrInvalid
	}
	return h, nil
}
