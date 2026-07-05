package auth

type User struct {
    ID               int64
    Email            string
    PasswordHash     string
    IsEmailVerified    bool
    ActivationToken  string
}

type CreateUserRequest struct {
	Email            string
    PasswordHash     string
    ActivationToken  string
}

type UpdateUserRequest struct {
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
	Email             string `json:"email"`
	IsEmailVerified   bool   `json:"isEmailVerified"`
}

func NewUserDTO(user User) UserDTO {
	return UserDTO{
		ID: user.ID,
		Email: user.Email,
		IsEmailVerified: user.IsEmailVerified,
	}
}

type AuthResponse struct {
    User UserDTO `json:"user"`
    TokenPair
}

type RegistrationRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
