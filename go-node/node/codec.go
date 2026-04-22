package node

import "encoding/json"

func DecodeBody[T any](msg *Message) (T, error) {

	var target T
	err := json.Unmarshal(msg.Body, &target)
	return target, err
}
