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