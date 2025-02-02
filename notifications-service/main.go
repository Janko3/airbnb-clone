package main

import (
	"context"
	"log"
	"net/http"
	"notifications-service/client"
	"notifications-service/config"
	"notifications-service/handlers"
	"notifications-service/repository"
	"notifications-service/services"
	"notifications-service/tracing"

	"os"
	"os/signal"
	"time"

	gorillaHandlers "github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/sony/gobreaker"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func main() {
	timeoutContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	logger := config.NewLogger("logs/log.log")

	// env definitions
	port := os.Getenv("PORT")
	mailHost := os.Getenv("MAIL_SERVICE_HOST")
	mailPort := os.Getenv("MAIL_SERVICE_PORT")
	userServiceHost := os.Getenv("USER_SERVICE_HOST")
	userServicePort := os.Getenv("USER_SERVICE_PORT")

	// commms

	customMailClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 10,
			MaxConnsPerHost:     10,
		},
	}
	// mailCircuitBreaker := gobreaker.NewCircuitBreaker(
	// 	gobreaker.Settings{
	// 		Name: "mail-service",
	// 		MaxRequests: 1,
	// 		Timeout: 10 * time.Second,
	// 		Interval: 0,
	// 		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
	// 			log.Printf("Circuit Breaker %v: %v -> %v", name, from, to)
	// 		},
	// 	},
	// )
	mailClient := client.NewMailClient(mailHost, mailPort, customMailClient)

	customUserServiceClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 10,
			MaxConnsPerHost:     10,
		},
	}

	userServiceCircuitBreaker := gobreaker.NewCircuitBreaker(
		gobreaker.Settings{
			Name:        "user-service",
			MaxRequests: 1,
			Timeout:     10 * time.Second,
			Interval:    0,
			OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
				log.Printf("Circuit Breaker %v: %v -> %v", name, from, to)
			},
		},
	)
	userClient := client.NewUserClient(userServiceHost, userServicePort, customUserServiceClient, userServiceCircuitBreaker)

	// services
	tracerConfig := tracing.GetConfig()
	tracerProvider, err := tracing.NewTracerProvider("notifications-service", tracerConfig.JaegerAddress)
	tracer := tracerProvider.Tracer("auth-service")
	otel.SetTextMapPropagator(propagation.TraceContext{})
	mongoService, err := services.New(timeoutContext, logger)
	if err != nil {
		log.Fatal(err)
	}
	notificationRepository := repository.NewNotificationRepository(mongoService.GetCli(), logger, tracer)
	notificationService := services.NewNotificationService(notificationRepository, mailClient, userClient, logger, tracer)
	notificationHandler := handlers.NewNotificationHandler(notificationService, tracer)

	// router definitions

	router := mux.NewRouter()
	router.HandleFunc("/create-new-user-notification/{id}", notificationHandler.CreateNewUserNotification).Methods("POST")
	router.HandleFunc("/{id}", notificationHandler.CreateNewNotificationForUser).Methods("POST")
	router.HandleFunc("/{id}", notificationHandler.ReadAllNotifications).Methods("PUT")
	router.HandleFunc("/{id}", notificationHandler.GetAllNotificationsByID).Methods("GET")

	// server definitions

	if len(port) == 0 {
		port = "8080"
	}
	headersOk := gorillaHandlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type", "Authorization"})
	methodsOk := gorillaHandlers.AllowedMethods([]string{"GET", "HEAD", "POST", "PUT", "OPTIONS"})
	originsOk := gorillaHandlers.AllowedOrigins([]string{"http://localhost:4200"})
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
			logger.Fatalln(err.Error())
		}
	}()

	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, os.Interrupt)
	signal.Notify(sigCh, os.Kill)

	sig := <-sigCh
	logger.Println("Received terminate, graceful shutdown", sig)

	//Try to shutdown gracefully
	if server.Shutdown(timeoutContext) != nil {
		logger.Fatalln(err.Error())
	}
	logger.Println("Server stopped")

}
