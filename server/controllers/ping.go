package controllers

import (
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/services/maintenance"
	"net"
	"net/http"
)

func Ping(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	// Docker's loopback liveness probe must keep working while browser
	// readiness waits for the replacement database and the new process.
	if maintenance.Default.Active() {
		host, _, _ := net.SplitHostPort(req.RemoteAddr)
		if !net.ParseIP(host).IsLoopback() || req.Header.Get("Sec-Fetch-Mode") != "" || req.Header.Get("Origin") != "" {
			InboundError(i18n.ForRequest(req), w, http.StatusServiceUnavailable, response.ServerError, "maintenance")
			return
		}
	}
	response.NewSuccessResponse("pong").FPrint(w)
}
