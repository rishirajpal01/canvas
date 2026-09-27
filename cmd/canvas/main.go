package main

import (
	"log"

	"canvas/internal/localserver"
)

func main() {
	log.Fatal(localserver.Run())
}
