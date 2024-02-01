package services

import (
	"accommodations-service/client"
	"accommodations-service/domain"
	"accommodations-service/errors"
	"accommodations-service/repository"
	"accommodations-service/utils"
	"context"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"log"
	"mime/multipart"
	"time"
)

type AccommodationService struct {
	accommodationRepository *repository.AccommodationRepo
	validator               *utils.Validator
	reservationsClient      *client.ReservationsClient
	userClient              *client.UserClient
	fileStorage             *repository.FileStorage
	cache                   *repository.ImageCache
}

func NewAccommodationService(accommodationRepo *repository.AccommodationRepo, validator *utils.Validator, reservationsClient *client.ReservationsClient, userClient *client.UserClient, fileStorage *repository.FileStorage, cache *repository.ImageCache) *AccommodationService {
	return &AccommodationService{
		accommodationRepository: accommodationRepo,
		validator:               validator,
		reservationsClient:      reservationsClient,
		userClient:              userClient,
		fileStorage:             fileStorage,
		cache:                   cache,
	}
}

func (as *AccommodationService) CreateAccommodation(accommodation domain.CreateAccommodation, image multipart.File, ctx context.Context) (*domain.AccommodationDTO, *errors.ErrorStruct) {
	var imageIds []string
	accomm := domain.Accommodation{
		Name:             accommodation.Name,
		Address:          accommodation.Address,
		City:             accommodation.City,
		Country:          accommodation.Country,
		UserName:         accommodation.UserName,
		UserId:           accommodation.UserId,
		Email:            accommodation.Email,
		Conveniences:     accommodation.Conveniences,
		MinNumOfVisitors: accommodation.MinNumOfVisitors,
		MaxNumOfVisitors: accommodation.MaxNumOfVisitors,
	}
	as.validator.ValidateAccommodation(&accomm)
	//as.validator.ValidateAvailabilities(&accommodation)
	validatorErrors := as.validator.GetErrors()
	if len(validatorErrors) > 0 {
		var constructedError string
		for _, message := range validatorErrors {
			constructedError += message + "\n"
		}
		as.validator.ClearErrors()
		return nil, errors.NewError(constructedError, 400)
	}

	log.Println(accomm)
	uuidStr := uuid.New().String()
	imageIds = append(imageIds, uuidStr)
	as.fileStorage.WriteFile(image, uuidStr)
	as.cache.Post(image, uuidStr)
	accomm.ImageIds = imageIds
	accomm.Status = domain.Pending

	newAccommodation, foundErr := as.accommodationRepository.SaveAccommodation(accomm)
	if foundErr != nil {
		return nil, foundErr
	}
	id := newAccommodation.Id.Hex()

	err := as.reservationsClient.SendCreatedReservationsAvailabilities(ctx, id, accommodation)
	if err != nil {
		as.DeleteAccommodation(id)
		return nil, errors.NewError("Service is not responding correcrtly", 500)
	} else {
		accomId := newAccommodation.Id.Hex()
		accomm.Status = domain.Created

		log.Println("Uslo je u update", accomm.Status)
		as.accommodationRepository.UpdateAccommodationStatus(accomm, accomId)
	}

	return &domain.AccommodationDTO{
		Id:               id,
		Name:             accommodation.Name,
		UserName:         accommodation.UserName,
		UserId:           accommodation.UserId,
		Email:            accommodation.Email,
		Address:          accommodation.Address,
		City:             accommodation.City,
		Country:          accommodation.Country,
		Conveniences:     accommodation.Conveniences,
		MinNumOfVisitors: accommodation.MinNumOfVisitors,
		MaxNumOfVisitors: accommodation.MaxNumOfVisitors,
		ImageIds:         imageIds,
		Status:           accommodation.Status,
	}, nil
}

func (as *AccommodationService) GetImage(id string) ([]byte, *errors.ErrorStruct) {
	file, err := as.fileStorage.ReadFile(id)
	if err != nil {
		return nil, errors.NewError("image read error", 500)
	}
	as.cache.Create(file, id)
	return file, nil
}

func (as *AccommodationService) GetCache(key string) ([]byte, error) {
	data, err := as.cache.Get(key)
	return data, err
}

func (as *AccommodationService) GetAllAccommodations() ([]*domain.AccommodationDTO, *errors.ErrorStruct) {
	accommodations, err := as.accommodationRepository.GetAllAccommodations()
	if err != nil {
		return nil, err
	}

	var domainAccommodations []*domain.AccommodationDTO
	for _, accommodation := range accommodations {
		id := accommodation.Id.Hex()
		imageIds := accommodation.ImageIds

		domainAccommodations = append(domainAccommodations, &domain.AccommodationDTO{
			Id:               id,
			Name:             accommodation.Name,
			UserName:         accommodation.UserName,
			UserId:           accommodation.UserId,
			Email:            accommodation.Email,
			Address:          accommodation.Address,
			City:             accommodation.City,
			Country:          accommodation.Country,
			Conveniences:     accommodation.Conveniences,
			MinNumOfVisitors: accommodation.MinNumOfVisitors,
			MaxNumOfVisitors: accommodation.MaxNumOfVisitors,
			ImageIds:         imageIds,
			Rating:           accommodation.Rating,
			Status:           accommodation.Status,
		})
	}

	return domainAccommodations, nil
}
func (as *AccommodationService) GetAccommodationById(accommodationId string) (*domain.Accommodation, *errors.ErrorStruct) {
	accomm, err := as.accommodationRepository.GetAccommodationById(accommodationId)
	if err != nil {
		return nil, err
	}
	id, _ := accomm.Id.MarshalJSON()
	return &domain.Accommodation{
		Id:               primitive.ObjectID(id),
		Name:             accomm.Name,
		UserName:         accomm.UserName,
		UserId:           accomm.UserId,
		Email:            accomm.Email,
		Address:          accomm.Address,
		City:             accomm.City,
		Country:          accomm.Country,
		Conveniences:     accomm.Conveniences,
		MinNumOfVisitors: accomm.MinNumOfVisitors,
		MaxNumOfVisitors: accomm.MaxNumOfVisitors,
		ImageIds:         accomm.ImageIds,
		Status:           accomm.Status,
	}, nil

}

func (as *AccommodationService) FindAccommodationByIds(ids []string) ([]*domain.AccommodationDTO, *errors.ErrorStruct) {
	accomm, err := as.accommodationRepository.FindAccommodationByIds(ids)
	if err != nil {
		return nil, err
	}
	var domainAccommodations []*domain.AccommodationDTO
	for _, accommodation := range accomm {

		imageIds := accommodation.ImageIds
		id := accommodation.Id.Hex()
		domainAccommodations = append(domainAccommodations, &domain.AccommodationDTO{
			Id:               id,
			Name:             accommodation.Name,
			UserName:         accommodation.UserName,
			UserId:           accommodation.UserId,
			Email:            accommodation.Email,
			Address:          accommodation.Address,
			City:             accommodation.City,
			Country:          accommodation.Country,
			Conveniences:     accommodation.Conveniences,
			MinNumOfVisitors: accommodation.MinNumOfVisitors,
			MaxNumOfVisitors: accommodation.MaxNumOfVisitors,
			ImageIds:         imageIds,
			Rating:           accommodation.Rating,
			Status:           accommodation.Status,
		})
	}
	return domainAccommodations, nil

}

func (as *AccommodationService) UpdateAccommodation(updatedAccommodation domain.Accommodation) (*domain.Accommodation, *errors.ErrorStruct) {
	as.validator.ValidateAccommodation(&updatedAccommodation)
	validatorErrors := as.validator.GetErrors()
	if len(validatorErrors) > 0 {
		var constructedError string
		for _, message := range validatorErrors {
			constructedError += message + "\n"
		}
		return nil, errors.NewError(constructedError, 400)
	}

	log.Println("Prije update")
	_, updateErr := as.accommodationRepository.UpdateAccommodationById(updatedAccommodation)
	if updateErr != nil {
		return nil, errors.NewError("Unable to update", 500)
	}
	log.Println("Poslije update")

	return &domain.Accommodation{
		Id:               updatedAccommodation.Id,
		Name:             updatedAccommodation.Name,
		UserName:         updatedAccommodation.UserName,
		UserId:           updatedAccommodation.UserId,
		Email:            updatedAccommodation.Email,
		Address:          updatedAccommodation.Address,
		City:             updatedAccommodation.City,
		Country:          updatedAccommodation.Country,
		Conveniences:     updatedAccommodation.Conveniences,
		MinNumOfVisitors: updatedAccommodation.MinNumOfVisitors,
		MaxNumOfVisitors: updatedAccommodation.MaxNumOfVisitors,
		Status:           updatedAccommodation.Status,
	}, nil
}

func (as *AccommodationService) DeleteAccommodation(accommodationID string) (*domain.Accommodation, *errors.ErrorStruct) {
	// Assuming validation checks are not necessary for deletion

	existingAccommodation, foundErr := as.accommodationRepository.GetAccommodationById(accommodationID)
	if foundErr != nil {
		return nil, foundErr
	}

	deleteErr := as.accommodationRepository.DeleteAccommodationById(accommodationID)
	if deleteErr != nil {
		return nil, deleteErr
	}

	return existingAccommodation, nil
}

func (as *AccommodationService) DeleteAccommodationsByUserId(userID string) *errors.ErrorStruct {

	deleteErr := as.accommodationRepository.DeleteAccommodationsByUserId(userID)
	if deleteErr != nil {
		return deleteErr
	}

	return nil
}
func (as *AccommodationService) PutAccommodationRating(accommodationID string, accommodation domain.Accommodation) *errors.ErrorStruct {

	err := as.accommodationRepository.PutAccommodationRating(accommodationID, accommodation.Rating)
	if err != nil {
		return errors.NewError("Error calling repository service", 500)
	}
	return nil
}

func (as *AccommodationService) SearchAccommodations(city, country string, numOfVisitors int, startDate string, endDate string, maxPrice int, conveniences []string, isDistinguishedString string, ctx context.Context) ([]domain.Accommodation, *errors.ErrorStruct) {
	log.Println("USLO U SERVIS")
	isDistinguished := false
	if isDistinguishedString == "true" {
		isDistinguished = true
	}

	log.Println("Start date", startDate)
	log.Println("EndDate", endDate)
	log.Println("Max Price", maxPrice)
	log.Println("isDistinguished", isDistinguished)

	accommodations, err := as.accommodationRepository.SearchAccommodations(city, country, numOfVisitors, maxPrice, conveniences)
	if err != nil {
		// Handle the error returned by the repository
		return nil, errors.NewError("Failed to find accommodations", 500) // Modify according to your error handling approach
	}
	var accommodationIDs []string

	for _, acc := range accommodations {
		accommodationIDs = append(accommodationIDs, acc.Id.Hex())

	}
	log.Println(accommodationIDs)
	if startDate == "" && endDate == "" && isDistinguished == false && maxPrice == 0 {
		return accommodations, nil
	}

	if startDate != "" || endDate != "" && isDistinguished == false && maxPrice == 0 {

		dateRange, err := generateDateRange(startDate, endDate)
		if err != nil {
			// Handle the error returned by the repository
			return nil, errors.NewError("Failed to generate dateRange", 500) // Modify according to your error handling approach
		}
		log.Println("dateRange je", dateRange)

		reservedIDs, err := as.reservationsClient.CheckAvailabilityForAccommodations(ctx, accommodationIDs, dateRange)
		if err != nil {
			return nil, errors.NewError("Failed to get reserved ids ", 500)
		}
		log.Println("Reservisani idevi", reservedIDs)
		log.Println("Sve nadjene akomodacije", accommodations)
		filteredAccommodations := removeAccommodations(accommodations, reservedIDs)
		return filteredAccommodations, nil
	}

	if startDate != "" && endDate != "" && isDistinguished == true && maxPrice == 0 {

		dateRange, err := generateDateRange(startDate, endDate)
		if err != nil {
			// Handle the error returned by the repository
			return nil, errors.NewError("Failed to generate dateRange", 500) // Modify according to your error handling approach
		}

		reservedIDs, err := as.reservationsClient.CheckAvailabilityForAccommodations(ctx, accommodationIDs, dateRange)
		if err != nil {
			return nil, errors.NewError("Failed to get reserved ids ", 500)
		}
		log.Println("Reservisani idevi", reservedIDs)
		log.Println("Sve nadjene akomodacije", accommodations)
		filteredAccommodations := removeAccommodations(accommodations, reservedIDs)

		var distFiltered []domain.Accommodation

		for _, acc := range filteredAccommodations {
			user, _ := as.userClient.GetUserById(ctx, acc.UserId)
			log.Println("User je", user)

			if user.Distinguished == true {

				distFiltered = append(distFiltered, acc)
			}
		}
		log.Println("Akomodacije sa distinguished likovima su", distFiltered)
		return distFiltered, nil

	}

	if startDate == "" && endDate == "" && isDistinguished == true && maxPrice == 0 {
		log.Println("USLO JE GDJE TREBA")

		var distFiltered []domain.Accommodation
		log.Println("Sve akomodacije", accommodations)

		for _, acc := range accommodations {
			log.Println("UserId:", acc.UserId)
			user, err := as.userClient.GetUserById(ctx, acc.UserId)
			log.Println("User je", user)
			if err != nil {
				log.Println("Error getting user:", err)
				// Handle the error if needed
				continue
			}

			if user != nil && user.Distinguished == true {
				distFiltered = append(distFiltered, acc)
			}
		}

		log.Println("Akomodacije sa distinguished likovima su", distFiltered)
		return distFiltered, nil
	}

	return nil, errors.NewError("Failed to return anything", 500)

}

func removeAccommodations(accommodations []domain.Accommodation, accommodationIDs []string) []domain.Accommodation {
	var filteredAccommodations []domain.Accommodation

	// Create a map for faster lookup of accommodationIDs
	idMap := make(map[string]bool)
	for _, id := range accommodationIDs {
		idMap[id] = true
	}

	// Check accommodations against accommodationIDs and remove if necessary
	for _, acc := range accommodations {
		if idMap[acc.Id.Hex()] {
			// If the ID exists in accommodationIDs, exclude it from filteredAccommodations
			continue
		}
		filteredAccommodations = append(filteredAccommodations, acc)
	}

	return filteredAccommodations
}

func generateDateRange(startDateStr, endDateStr string) ([]string, *errors.ErrorStruct) {
	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		if err != nil {
			// Handle the error returned by the repository
			return nil, errors.NewError("Failed to parse date", 500) // Modify according to your error handling approach
		}
	}

	endDate, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		if err != nil {
			// Handle the error returned by the repository
			return nil, errors.NewError("Failed to parse date", 500) // Modify according to your error handling approach
		}
	}

	var dates []string
	for currentDate := startDate; !currentDate.After(endDate); currentDate = currentDate.AddDate(0, 0, 1) {
		dates = append(dates, currentDate.Format("2006-01-02"))
	}

	return dates, nil
}
