package main

import (
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/gorilla/mux"

	gorillaHandlers "github.com/gorilla/handlers"
	log "github.com/sirupsen/logrus"
	"github.com/sony/gobreaker"

	"net/http"
	"os"
	"os/signal"
	"time"
	"user-service/client"
	"user-service/config"
	"user-service/handler"
	"user-service/middleware"
	"user-service/repository"
	"user-service/service"
	"user-service/tracing"
	"user-service/utils"
)

func main() {
	const source = "server-main"
	timeoutContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	logger := config.NewLogger("./logs/log.log")

	tracerConfig := tracing.GetConfig()
	tracerProvider, err := tracing.NewTracerProvider("user-service", tracerConfig.JaegerAddress)
	if err != nil {
		log.Fatal("JaegerTraceProvider failed to Initialize", err)
	}
	tracer := tracerProvider.Tracer("user-service")
	otel.SetTextMapPropagator(propagation.TraceContext{})
	if err != nil {
		log.Fatal(err)
	}
	//env

	reservationsServiceHost := os.Getenv("RESERVATIONS_SERVICE_HOST")
	reservationsServicePort := os.Getenv("RESERVATIONS_SERVICE_PORT")
	authServiceHost := os.Getenv("AUTH_SERVICE_HOST")
	authServicePort := os.Getenv("AUTH_SERVICE_PORT")
	accServiceHost := os.Getenv("ACCOMMODATION_SERVICE_HOST")
	accServicePort := os.Getenv("ACCOMMODATION_SERVICE_PORT")

	//clients

	customReservationsServiceClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 10,
			MaxConnsPerHost:     10,
		},
	}

	customAuthServiceClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 10,
			MaxConnsPerHost:     10,
		},
	}

	customAccServiceClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 10,
			MaxConnsPerHost:     10,
		},
	}

	authServiceCircuitBreaker := gobreaker.NewCircuitBreaker(
		gobreaker.Settings{
			Name:        "auth-service",
			MaxRequests: 1,
			Timeout:     10 * time.Second,
			Interval:    0,
			OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
				log.Printf("Circuit Breaker %v: %v -> %v", name, from, to)
			},
		},
	)

	reservationsServiceCircuitBreaker := gobreaker.NewCircuitBreaker(
		gobreaker.Settings{
			Name:        "reservations-service",
			MaxRequests: 1,
			Timeout:     10 * time.Second,
			Interval:    0,
			OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
				log.Printf("Circuit Breaker %v: %v -> %v", name, from, to)
			},
		},
	)
	accommodationServiceCircuitBreaker := gobreaker.NewCircuitBreaker(
		gobreaker.Settings{
			Name:        "accommodation-service",
			MaxRequests: 1,
			Timeout:     30 * time.Second,
			Interval:    0,
			OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
				log.Printf("Circuit Breaker %v: %v -> %v", name, from, to)
			},
		},
	)

	reservationsClient := client.NewReservationClient(reservationsServiceHost, reservationsServicePort, customReservationsServiceClient, reservationsServiceCircuitBreaker, tracer)
	authClient := client.NewAuthClient(authServiceHost, authServicePort, customAuthServiceClient, authServiceCircuitBreaker, tracer)
	accClient := client.NewAccClient(accServiceHost, accServicePort, customAccServiceClient, accommodationServiceCircuitBreaker, tracer)

	// service

	mongoService, err := service.New(timeoutContext, logger)
	if err != nil {
		log.Fatal(err)
	}
	userRepo := repository.NewUserRepository(mongoService.GetCli(), logger, tracer)
	validator := utils.NewValidator()
	userService := service.NewUserService(userRepo, validator, reservationsClient, authClient, accClient, logger, tracer)
	profileHandler := handler.NewUserHandler(userService, logger, tracer)

	// router

	router := mux.NewRouter()

	router.HandleFunc("/{id}", middleware.ValidateJWT(profileHandler.DeleteHandler)).Methods("DELETE")
	router.HandleFunc("/create", profileHandler.CreateHandler).Methods("POST")
	router.HandleFunc("/{id}", middleware.ValidateJWT(profileHandler.UpdateHandler)).Methods("PUT")
	router.HandleFunc("/all", profileHandler.GetAllHandler).Methods("GET")
	router.HandleFunc("/{id}", profileHandler.GetUserById).Methods("GET")
	router.HandleFunc("/creds/{id}", profileHandler.CredsHandler).Methods("POST")
	router.HandleFunc("/rating/{id}", profileHandler.UpdateRating).Methods("POST")

	port := os.Getenv("PORT")
	if len(port) == 0 {
		port = "8080"
	}

	headersOk := gorillaHandlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type", "Authorization"})
	methodsOk := gorillaHandlers.AllowedMethods([]string{"GET", "HEAD", "POST", "PUT", "OPTIONS", "DELETE"})
	originsOk := gorillaHandlers.AllowedOrigins([]string{"http://localhost:4200", "http://localhost:58495"})

	server := http.Server{
		Addr:         ":" + port,
		Handler:      gorillaHandlers.CORS(headersOk, methodsOk, originsOk)(router),
		IdleTimeout:  120 * time.Second,
		ReadTimeout:  1 * time.Second,
		WriteTimeout: 1 * time.Second,
	}

	logger.Println("Server listening on port", port)

	go func() {
		err := server.ListenAndServe()
		if err != nil {
			logger.Fatal("Error while server is listening and serving requests", log.Fields{
				"module": source,
				"error":  err.Error(),
			})
		}
	}()

	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, os.Interrupt)
	signal.Notify(sigCh, os.Kill)

	sig := <-sigCh
	logger.Println("Received terminate, graceful shutdown", sig)

	//Try to shut down gracefully
	if server.Shutdown(timeoutContext) != nil {
		logger.Fatal("Error during graceful shutdown", log.Fields{
			"module": source,
			"error":  err.Error(),
		})
	}
	logger.LogInfo(source, "Server shut down")

}
