package auth

type User struct {
	ID              int64
	Name            string
	Email           string
	PasswordHash    string `json:"-"`
	IsEmailVerified bool
	ActivationToken string `json:"-"`
}

type CreateUserRequest struct {
	Name            string
	Email           string
	PasswordHash    string `json:"-"`
	ActivationToken string `json:"-"`
}

type UpdateUserRequest struct {
	Name            string
	Email           string
	PasswordHash    string `json:"-"`
	IsEmailVerified bool
	ActivationToken string `json:"-"`
}

type RefreshToken struct {
	ID        int64
	UserID    int64
	TokenHash string `json:"-"`
}

type TokenPair struct {
	AccessToken  string `json:"-"`
	RefreshToken string `json:"-"`
}

type UserDTO struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Email           string `json:"email"`
	IsEmailVerified bool   `json:"isEmailVerified"`
}

func NewUserDTO(user User) UserDTO {
	return UserDTO{
		ID:              user.ID,
		Name:            user.Name,
		Email:           user.Email,
		IsEmailVerified: user.IsEmailVerified,
	}
}

type AuthResponse struct {
	User        UserDTO `json:"user"`
	AccessToken string  `json:"accessToken"`
}

type AuthResult struct {
	User   UserDTO
	Tokens TokenPair `json:"-"`
}

type RegistrationResponse struct {
	ActivationRequired bool `json:"activationRequired"`
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
