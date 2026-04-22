package node

import (
	"fmt"
)

type HandlerFunc func(n *Node, msg Message) error

type Router struct {
	handlers map[string]HandlerFunc
}

func NewRouter() *Router {
	return &Router{
		handlers: make(map[string]HandlerFunc),
	}
}

func (r *Router) Register(msgType string, fn HandlerFunc) {
	r.handlers[msgType] = fn
}

func (r *Router) Dispatch(n *Node, msgType string, msg Message) error {
	h, ok := r.handlers[msgType]

	if !ok {
		return fmt.Errorf("no handler for %s", msgType)
	}

	return h(n, msg)
}
