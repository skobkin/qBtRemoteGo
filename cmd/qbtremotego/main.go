package main

import (
	"log"

	"github.com/skobkin/qbtremotego/internal/ui"
)

func main() {
	if err := ui.Run(); err != nil {
		log.Fatal(err)
	}
}
