package handlers

import (
	"encoding/json"
	"fmt"
	"maelstrom-node/node"
)

type EchoBodyRequest struct {
	Type  string `json:"type"`
	Echo  string `json:"echo"`
	MsgId int    `json:"msg_id,omitempty"`
}

// {:type (eq "echo_ok"),
//
//	:echo Any,
//	#schema.core.OptionalKey{:k :msg_id} Int,
//	:in_reply_to Int}
type EchoBodyResponse struct {
	Type      string `json:"type"`
	Echo      string `json:"echo"`
	InReplyTo int    `json:"in_reply_to"`
}

// GetDecodedBody implements [node.ResponseBody].
func (e *EchoBodyResponse) GetDecodedBody() ([]byte, error) {
	return json.Marshal(e)
}

// SetReplyTo implements [node.ResponseBody].
func (e *EchoBodyResponse) SetReplyTo(msgId int) {
	e.InReplyTo = msgId
}

func HandleEcho(n *node.Node, msg node.Message) error {
	node.Log("processing msg %s", msg)

	echoBody, err := node.DecodeBody[EchoBodyRequest](&msg)

	if err != nil {
		node.Log("could not decode echo body %s", err)
		return fmt.Errorf("could not decode echo body %s", err)

	}

	echoResponse := EchoBodyResponse{
		Type: "echo_ok",
		Echo: echoBody.Echo,
	}

	return n.Reply(echoBody.MsgId, msg.Src, &echoResponse)
}
