package controllers

import (
	"github.com/Jinnrry/pmail/config"
	"net/http"
)

func Interceptor(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Location", "https://"+config.Instance.WebDomain+r.URL.RequestURI())
	w.WriteHeader(http.StatusMovedPermanently)
}
