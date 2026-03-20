package handler

import (
	"encoding/json"
	"net/http"

	"go-api-server/internal/model"
)

// UserHandler는 RESTful한 유저 API를 처리합니다.
func UserHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		user := model.User{ID: "1", Name: "Cursor User", Email: "user@example.com"}
		json.NewEncoder(w).Encode(user)

	case http.MethodPost:              
		var u model.User
		if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(u)

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}
