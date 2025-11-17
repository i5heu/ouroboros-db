package apiServer

import (
	"fmt"
	"net/http"
)

func defaultAuth(*http.Request) error { // AP
	// TODO : implement authentication

	fmt.Println("defaultAuth called - no authentication implemented")

	return nil
}
