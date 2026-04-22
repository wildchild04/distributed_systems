package node

import (
	"log"
	"os"
)

var logger = log.New(os.Stderr, "", log.Ldate|log.Ltime|log.Lmicroseconds)

func Log(format string, args ...any) {
	logger.Printf(format, args...)
}
