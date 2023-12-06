package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"reservation-service/domain"
	"reservation-service/service"
	"reservation-service/utils"

	"github.com/gorilla/mux"
)

type KeyProduct struct{}

type ReservationHandler struct {
	logger *log.Logger

	ReservationService *service.ReservationService
}

func NewReservationsHandler(l *log.Logger, rs *service.ReservationService) *ReservationHandler {
	return &ReservationHandler{l, rs}
}

func (r *ReservationHandler) CreateReservation(rw http.ResponseWriter, h *http.Request) {
	decoder := json.NewDecoder(h.Body)
	decoder.DisallowUnknownFields()
	var res domain.Reservation
	if err := decoder.Decode(&res); err != nil {
		utils.WriteErrorResp("Internal server error", 500, "api/reservations", rw)
	}
	newRes, err := r.ReservationService.CreateReservation(res)
	if err != nil {
		utils.WriteErrorResp(err.Message, err.Status, "api/reservations", rw)
		return
	}
	utils.WriteResp(newRes, 201, rw)
}
func (rh *ReservationHandler) GetReservationsByUser(rw http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["userId"]

	reservations, err := rh.ReservationService.GetReservationsByUser(userID)
	if err != nil {
		utils.WriteErrorResp(err.Message, err.Status, "api/reservations/user/{userId}", rw)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(reservations)
}

func (rh *ReservationHandler) DeleteReservationById(rw http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	deletedReservation, err := rh.ReservationService.DeleteReservationById(id)
	if err != nil {
		utils.WriteErrorResp(err.Message, err.Status, "api/reservations/{id}", rw)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(deletedReservation)
}
