package auth

type User struct {
    ID               int64
    Name             string
    Email            string
    PasswordHash     string
    IsEmailVerified    bool
    ActivationToken  string
}

type CreateUserRequest struct {
    Name             string
	Email            string
    PasswordHash     string
    ActivationToken  string
}

type UpdateUserRequest struct {
    Name             string
	Email            string
    PasswordHash     string
    IsEmailVerified    bool
    ActivationToken  string
}

type RefreshToken struct {
    ID        int64
    UserID    int64
    TokenHash string
}

type TokenPair struct {
    AccesToken string `json:"accessToken"`
    RefreshToken string `json:"refreshToken"`
}

type UserDTO struct {
	ID                int64  `json:"id"`
    Name              string `json:"name"`
	Email             string `json:"email"`
	IsEmailVerified   bool   `json:"isEmailVerified"`
}

func NewUserDTO(user User) UserDTO {
	return UserDTO{
		ID: user.ID,
        Name: user.Name,
		Email: user.Email,
		IsEmailVerified: user.IsEmailVerified,
	}
}

type AuthResponse struct {
    User UserDTO `json:"user"`
    TokenPair
}

type RegistrationRequest struct {
    Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6,max=72"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}
