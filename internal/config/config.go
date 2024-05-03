package config

import (
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Server      string `yaml:"server"`
	Port        int    `yaml:"port"`
	ServingPort int    `yaml:"servingPort"`
}

func GetConfig() Config {
	// Read YAML file
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	// Unmarshal YAML data into the Config struct
	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	if config.Server == "" {
		config.Server = "localhost"
	}

	if config.Port == 0 {
		config.Port = 4242
	}

	if config.ServingPort == 0 {
		config.ServingPort = 4243
	}

	// overwrite with cli arguments if provided
	if len(os.Args) > 1 {
		config.Server = os.Args[1]
	}

	if len(os.Args) > 2 {
		fmt.Sscanf(os.Args[2], "%d", &config.Port)
	}

	if len(os.Args) > 3 {
		fmt.Sscanf(os.Args[3], "%d", &config.ServingPort)
	}

	fmt.Printf("Server: %s\n", config.Server)
	fmt.Printf("Port: %d\n", config.Port)
	fmt.Printf("Serving Port: %d\n", config.ServingPort)

	return config
}
