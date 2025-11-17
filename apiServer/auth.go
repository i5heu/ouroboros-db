package apiServer

import (
	"fmt"
	"net/http"

	ouroboros "github.com/i5heu/ouroboros-db"
)

func defaultAuth(req *http.Request, db *ouroboros.OuroborosDB) error { // AP
	// TODO : implement authentication

	fmt.Println("defaultAuth called - no authentication implemented")

	return nil
}
