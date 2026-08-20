package api

import "net/http"

func NewRouter(h *Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /accounts", h.createAccount)
	mux.HandleFunc("GET /accounts/{id}", h.getAccount)
	mux.HandleFunc("POST /accounts/{id}/deposits", h.deposit)
	mux.HandleFunc("POST /transfers", h.transfer)
	mux.HandleFunc("GET /healthz", h.health)
	return withMiddleware(mux)
}
