package node

import "encoding/json"

type Message struct {
	Src  string          `json:"src"`
	Dest string          `json:"dest"`
	Body json.RawMessage `json:"body"`
}

type BodyHeader struct {
	MsgType string `json:"type"`
}

type ResponseBody interface {
	GetDecodedBody() ([]byte, error)
	SetReplyTo(msgId int)
}
