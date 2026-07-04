package auth

import (
	"log/slog"
)

type UserService struct {
	log *slog.Logger
	userRepository *UserRepository
	tokenService *TokenService
}

func NewUserService(
	log *slog.Logger, 
	userRepository *UserRepository,
	tokenService *TokenService,
) *UserService {
	return &UserService{
		userRepository: userRepository,
		log: log.With("component", "service", "entity", "user"),
		tokenService: tokenService,
	}
}

// func (s *UserService) Registrarion(ctx *gin.Context, email string, password string) (User, error) {
	
// 	userData := CreateUserRequest{
// 		Email: email,

// 	}



// 	user, err := s.repository.Create(ctx, ) 

// 	if errors.Is(err, ErrUserAlreadyExist) {
// 		s.log.Error("")
// 	}
// 	Проверка на существование
// 	var userData CreateUserRequest
// 	Заполнение сущности
// 	user, err := s.repository.Create(userData)
// 	Проверка ошибок
// 	Возврат
// }

// func (s *UserService) Login(ctx *gin.Context, email string, passwordHash string) (User, error) {
// 	Получение пользователя из БД с проверкой ошибок
// 	Сравнить хеши

// }

// func (s *UserService) Logout(ctx *gin.Context, email string, passwordHash string) (User, error) {
	
// }