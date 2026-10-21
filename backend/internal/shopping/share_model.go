package shopping

import "time"

type Share struct {
	ID            int64     `json:"id,string"`
	CreatedAt     time.Time `json:"createdAt"`
	PreserveMarks bool      `json:"preserveMarks"`
}

type CreateShareRequest struct {
	Token         string `json:"token"`
	PreserveMarks *bool  `json:"preserveMarks"`
}

type ImportShareRequest struct {
	Token string `json:"token"`
}
