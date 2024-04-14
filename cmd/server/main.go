package main

import (
	"OuroborosDB/internal/config"
	"OuroborosDB/pkg/ouroborosMQ"
	"fmt"
)

func main() {
	conf := config.GetConfig()
	fmt.Println("Starting Services...")

	ouroborosMQ.StartServer(conf)
}
