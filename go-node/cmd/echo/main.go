package main

import (
	"maelstrom-node/handlers"
	"maelstrom-node/node"
)

func main() {
	node.Log("Starting Echo node")
	router := node.NewRouter()

	router.Register("echo", handlers.HandleEcho)
	n := node.NewNode(node.NewStdioTransport(), router)
	n.Start()

}
