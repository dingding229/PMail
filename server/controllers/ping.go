package controllers

import (
	"github.com/Jinnrry/pmail/dto/response"
	"net/http"
)

func Ping(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	response.NewSuccessResponse("pong").FPrint(w)
}
