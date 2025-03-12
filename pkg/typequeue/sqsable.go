package typequeue

type SQSAbleMessage interface {
	SetReceiptID(*string)
	SetTraceID(*string)
	GetTraceID() *string
	GetReceiptID() *string
}

type SQSAble struct {
	TraceID   *string `json:"trace"`
	ReceiptID *string `json:"-"`
}

func (s *SQSAble) SetReceiptID(to *string) {
	s.ReceiptID = to
}

func (s *SQSAble) GetReceiptID() *string {
	return s.ReceiptID
}

func (s *SQSAble) SetTraceID(to *string) {
	s.TraceID = to
}

func (s *SQSAble) GetTraceID() *string {
	return s.TraceID
}
