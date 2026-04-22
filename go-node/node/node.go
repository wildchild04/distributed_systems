package node

import (
	"encoding/json"
	"sync/atomic"
)

type InitBody struct {
	MsgType string   `json:"type"`
	NodeId  string   `json:"node_id"`
	NodeIds []string `json:"node_ids"`
	MsgId   int      `json:"msg_id,omitempty"`
}

// body: {msg_id: 123
//
//	in_reply_to: 1
//	type: "init_ok"}}
type InitResponse struct {
	MsgType   string `json:"type"`
	InReplyTo int    `json:"in_reply_to"`
	MsgId     int    `json:"msg_id"`
}

type Node struct {
	Id        string
	nodeIDs   []string
	transport Transport
	router    *Router
	sequence  atomic.Int64
}

func NewNode(transport Transport, router *Router) *Node {
	return &Node{
		nodeIDs:   make([]string, 0, 3),
		transport: transport,
		router:    router,
		sequence:  atomic.Int64{},
	}
}

func (n *Node) init() error {

	data, err := n.transport.Recv()

	if err != nil {
		Log("init(): err recv, ERR %s", err)
		return err
	}

	var msg Message
	err = json.Unmarshal(data, &msg)

	if err != nil {
		Log("init(): err decode msg, ERR %s", err)
		return err
	}

	body, err := DecodeBody[InitBody](&msg)

	if err != nil {
		Log("init(): err decode body, ERR %s", err)
		return err
	}

	if body.MsgType != "init" {
		Log("wrong message type, expected 'init' got %s", body.MsgType)
	}

	n.Id = body.NodeId
	n.nodeIDs = append(n.nodeIDs, body.NodeIds...)

	initResponse := InitResponse{
		MsgType:   "init_ok",
		MsgId:     int(n.sequence.Add(1)),
		InReplyTo: body.MsgId,
	}

	initResponseBytes, err := json.Marshal(initResponse)

	if err != nil {
		Log("could not encode init response, %s", err)
		return err
	}

	err = n.Send(msg.Src, initResponseBytes)

	if err != nil {
		Log("could not send init response, %s", err)
		return err
	}

	Log("Node init with id '%s' and node ids'%s'", n.Id, n.nodeIDs)
	return nil
}

func (n *Node) Start() {

	err := n.init()
	if err != nil {
		Log("init failed %s", err)
		return
	}

	var loopErr error
	var data []byte

	for loopErr == nil {
		data, loopErr = n.transport.Recv()
		if loopErr != nil {
			Log("Error recieving init msg %s", loopErr)
			return
		}
		var msg Message
		loopErr = json.Unmarshal(data, &msg)
		if loopErr != nil {
			Log("Error decoding msg %s", loopErr)
			return
		}

		Log("Got message %s", msg)
		var msgMap map[string]any
		var bodyHeader BodyHeader
		loopErr = json.Unmarshal(msg.Body, &bodyHeader)
		if loopErr != nil {
			Log("could not get sg type. msg %s err %s", msg, loopErr)
			return
		}

		msgType := bodyHeader.MsgType
		if msgType == "" {
			Log("messega type is not a string or malformed, %v", msgMap["type"])
			continue
		}

		dispatchErr := n.router.Dispatch(n, msgType, msg)
		if dispatchErr != nil {
			Log("Could not dispatch for type %s, err %s", msgType, dispatchErr)
			continue
		}

	}

}

func (n *Node) Send(dst string, body []byte) error {

	msg := Message{
		Src:  n.Id,
		Dest: dst,
		Body: body,
	}

	encodedMsg, err := json.Marshal(msg)

	if err != nil {
		return err
	}

	err = n.transport.Send(encodedMsg)

	return err
}

func (n *Node) Reply(msgId int, dst string, body ResponseBody) error {

	body.SetReplyTo(msgId)
	bodyBytes, err := body.GetDecodedBody()
	if err != nil {
		return err
	}
	return n.Send(dst, bodyBytes)

}
